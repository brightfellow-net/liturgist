// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/internal/backup"
	"github.com/brightfellow-net/liturgist/migrations"
)

// Errors of backup and restore (14 §3, §12).
var (
	ErrNotSQLite      = errors.New("backups of PostgreSQL are made with pg_dump; see the backup guide")
	ErrNoDatabase     = errors.New("there is no database in the data folder")
	ErrBackupExists   = errors.New("that file already exists")
	ErrNotEnoughSpace = errors.New("not enough free disk space")
	ErrStorageFull    = errors.New("server storage is full")
	ErrServerRunning  = errors.New("stop the server first: another Liturgist process is using this data folder")
	ErrDeclined       = errors.New("nothing was changed")
	ErrBackupDamaged  = backup.ErrDamaged
)

// BackupNewerError means the backup was made with a newer schema than this
// program knows (exit code 3).
type BackupNewerError struct{ Backup, Program int64 }

func (e *BackupNewerError) Error() string {
	return fmt.Sprintf("this backup was made by a newer Liturgist (schema %d, this program knows %d); upgrade the program first", e.Backup, e.Program)
}

// IsBackupNewer reports whether err is a *BackupNewerError.
func IsBackupNewer(err error) bool {
	var e *BackupNewerError
	return errors.As(err, &e)
}

// storageErr turns "the disk is full" into ErrStorageFull.
func storageErr(err error) error {
	if errors.Is(err, app.ErrUnavailable) || errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %w", ErrStorageFull, err)
	}
	return err
}

// BackupResult reports a written backup.
type BackupResult struct {
	Path   string
	Size   int64
	Schema int64
}

func checkSQLite(cfg Config) (dataDir, dbPath string, err error) {
	if cfg.DBDriver != "sqlite" {
		return "", "", ErrNotSQLite
	}
	return cfg.DataDir, filepath.Join(cfg.DataDir, "liturgist.db"), nil
}

// Backup writes one archive of the database and files/ (14 §3.1). dest is the
// file to write; "" means backups/manual-<UTC time>.zip in the data folder.
// It runs while the server runs.
func Backup(ctx context.Context, cfg Config, dest string) (BackupResult, error) {
	cfg = withDefaults(cfg)
	dataDir, dbPath, err := checkSQLite(cfg)
	if err != nil {
		return BackupResult{}, err
	}
	if _, err := os.Stat(dbPath); err != nil {
		return BackupResult{}, ErrNoDatabase
	}
	if dest == "" {
		dest = filepath.Join(dataDir, "backups", "manual-"+time.Now().UTC().Format("20060102T150405Z")+".zip")
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return BackupResult{}, storageErr(err)
		}
	}
	dest, err = filepath.Abs(dest)
	if err != nil {
		return BackupResult{}, err
	}
	if _, err := os.Lstat(dest); err == nil {
		return BackupResult{}, ErrBackupExists
	}
	return writeBackup(ctx, cfg, dest)
}

// writeBackup is Backup after the checks: space, snapshot, integrity, archive.
func writeBackup(ctx context.Context, cfg Config, dest string) (BackupResult, error) {
	dbPath := filepath.Join(cfg.DataDir, "liturgist.db")
	filesDir := filepath.Join(cfg.DataDir, "files")
	destDir := filepath.Dir(dest)
	need := sqlstore.SnapshotSpace(dbPath) + backup.TreeSize(filesDir)
	free, err := freeBytes(destDir)
	if err != nil {
		return BackupResult{}, fmt.Errorf("cannot read free disk space: %w", err)
	}
	if free < need {
		return BackupResult{}, fmt.Errorf("%w: %d MB needed, %d MB free", ErrNotEnoughSpace, need>>20, free>>20)
	}
	tmp, err := os.MkdirTemp(destDir, ".liturgist-backup-")
	if err != nil {
		return BackupResult{}, storageErr(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	snap := filepath.Join(tmp, "liturgist.db")
	if err := sqlstore.SnapshotFile(ctx, dbPath, snap); err != nil {
		return BackupResult{}, storageErr(err)
	}
	schema, err := sqlstore.CheckSQLiteFile(ctx, snap)
	if err != nil {
		return BackupResult{}, fmt.Errorf("the database copy failed its check, no backup was made: %w", err)
	}
	m := backup.Manifest{Program: Version, Schema: schema, CreatedAt: time.Now().UTC().Truncate(time.Second), Driver: "sqlite"}
	warn := func(msg string) { cfg.Logger.Warn("backup: " + msg) }
	if err := backup.CreateFile(dest, m, snap, filesDir, warn); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return BackupResult{}, ErrBackupExists
		}
		return BackupResult{}, storageErr(err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return BackupResult{}, err
	}
	cfg.Logger.Info("backup_written", "file", dest, "bytes", fi.Size(), "schema", schema)
	return BackupResult{Path: dest, Size: fi.Size(), Schema: schema}, nil
}

// RestoreInfo describes the backup about to be restored, for the confirmation.
type RestoreInfo struct {
	CreatedAt time.Time
	Program   string
	Schema    int64
}

// RestoreResult names what a restore kept.
type RestoreResult struct {
	PreRestoreDB    string // "" when there was no database
	PreRestoreFiles string // "" when there was no files/ folder
	Schema          int64
}

// freeBytes reads free disk space; tests replace it.
var freeBytes = sqlstore.FreeBytes

// afterKeep is a test hook that runs after the current data has been set
// aside and before the backup replaces it.
var afterKeep func() error

// Restore replaces the data folder's database and files/ with the archive at
// file (14 §3.2). The server must be stopped. confirm may refuse after the
// backup has been checked; nil means yes. The current data is kept as
// backups/pre-restore-*.
func Restore(ctx context.Context, cfg Config, file string, confirm func(RestoreInfo) bool) (RestoreResult, error) {
	cfg = withDefaults(cfg)
	dataDir, dbPath, err := checkSQLite(cfg)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return RestoreResult{}, storageErr(err)
	}
	releaseRun, err := sqlstore.LockRun(dataDir)
	if err != nil {
		if errors.Is(err, sqlstore.ErrAlreadyRunning) {
			return RestoreResult{}, ErrServerRunning
		}
		return RestoreResult{}, err
	}
	defer releaseRun()
	releaseData, err := sqlstore.LockDataDir(ctx, dataDir)
	if err != nil {
		return RestoreResult{}, err
	}
	defer releaseData()

	arc, err := backup.Open(file)
	if err != nil {
		return RestoreResult{}, err
	}
	defer func() { _ = arc.Close() }()
	m := arc.Manifest
	if m.Format != backup.Format || m.Driver != "sqlite" {
		return RestoreResult{}, fmt.Errorf("%w: unknown format %d or driver %q", ErrBackupDamaged, m.Format, m.Driver)
	}
	program, err := sqlstore.ProgramVersion(migrations.For("sqlite"))
	if err != nil {
		return RestoreResult{}, err
	}
	if m.Schema > program {
		return RestoreResult{}, &BackupNewerError{Backup: m.Schema, Program: program}
	}
	need := arc.Size + sqlstore.SnapshotSpace(dbPath)
	free, err := freeBytes(dataDir)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("cannot read free disk space: %w", err)
	}
	if free < need {
		return RestoreResult{}, fmt.Errorf("%w: %d MB needed, %d MB free", ErrNotEnoughSpace, need>>20, free>>20)
	}

	tmp := filepath.Join(dataDir, "restore-tmp")
	if err := os.RemoveAll(tmp); err != nil {
		return RestoreResult{}, err
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return RestoreResult{}, storageErr(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // after a successful restore it is empty or gone
	if err := arc.Extract(tmp); err != nil {
		return RestoreResult{}, storageErr(err)
	}
	schema, err := sqlstore.CheckSQLiteFile(ctx, filepath.Join(tmp, "liturgist.db"))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("%w: %w", ErrBackupDamaged, err)
	}
	if schema > program {
		return RestoreResult{}, &BackupNewerError{Backup: schema, Program: program}
	}
	if confirm != nil && !confirm(RestoreInfo{CreatedAt: m.CreatedAt, Program: m.Program, Schema: schema}) {
		return RestoreResult{}, ErrDeclined
	}

	res := RestoreResult{Schema: schema}
	if res.PreRestoreDB, res.PreRestoreFiles, err = keepCurrent(ctx, dataDir, dbPath); err != nil {
		return RestoreResult{}, storageErr(err)
	}
	fail := func(err error) (RestoreResult, error) {
		return res, fmt.Errorf("restore stopped (%w); the data it set aside is in %s", storageErr(err), filepath.Join(dataDir, "backups"))
	}
	if afterKeep != nil {
		if err := afterKeep(); err != nil {
			return fail(err)
		}
	}
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fail(err)
		}
	}
	if err := os.Rename(filepath.Join(tmp, "liturgist.db"), dbPath); err != nil {
		return fail(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "files")); err == nil {
		if err := os.Rename(filepath.Join(tmp, "files"), filepath.Join(dataDir, "files")); err != nil {
			return fail(err)
		}
	}
	cfg.Logger.Info("restore_done", "file", file, "schema", schema, "kept_db", res.PreRestoreDB, "kept_files", res.PreRestoreFiles)
	return res, nil
}

// keepCurrent sets the current database and files/ aside under backups/. It
// copies the database (renaming it when it cannot be opened) and renames
// files/. The live database is removed by the caller, after this succeeded.
func keepCurrent(ctx context.Context, dataDir, dbPath string) (db, files string, err error) {
	var rnd [3]byte
	_, _ = rand.Read(rnd[:])
	stamp := time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(rnd[:])
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	if _, statErr := os.Stat(dbPath); statErr == nil {
		db = filepath.Join(dir, "pre-restore-"+stamp+".db")
		if err := sqlstore.SnapshotFile(ctx, dbPath, db); err != nil {
			// A database that cannot be copied is damaged; keep its bytes as they are.
			if err := renameWithSidecars(dbPath, db); err != nil {
				return "", "", err
			}
		}
	}
	filesDir := filepath.Join(dataDir, "files")
	if _, statErr := os.Stat(filesDir); statErr == nil {
		files = filepath.Join(dir, "pre-restore-files-"+stamp)
		if err := os.Rename(filesDir, files); err != nil {
			return "", "", err
		}
	}
	return db, files, nil
}

func renameWithSidecars(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	for _, sfx := range []string{"-wal", "-shm"} {
		if err := os.Rename(from+sfx, to+sfx); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
