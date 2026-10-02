// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// SchemaNewerError means the database was migrated by a newer program (exit code 3).
type SchemaNewerError struct{ DB, Program int64 }

func (e *SchemaNewerError) Error() string {
	return fmt.Sprintf("database version %d is newer than this program (max %d); install a newer Liturgist or restore a backup", e.DB, e.Program)
}

// MigrateOptions configures CheckVersion and Migrate.
type MigrateOptions struct {
	FS               fs.FS // migrations for this dialect
	AllowNewerSchema bool  // only skips the refusal to start (02 §5.1)
	RequireCopy      bool  // LITURGIST_REQUIRE_PREUPGRADE_COPY
	Logger           *slog.Logger
	Now              func() time.Time
}

// MigrateResult reports what Migrate did.
type MigrateResult struct {
	From, To int64
	CopyPath string // "" when no copy was made
}

func (db *DB) provider(fsys fs.FS) (*goose.Provider, error) {
	var opts []goose.ProviderOption
	dialect := goose.DialectSQLite3
	if db.d.Name() == "postgres" {
		dialect = goose.DialectPostgres
		locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(lockKeyMigration))
		if err != nil {
			return nil, err
		}
		opts = append(opts, goose.WithSessionLocker(locker))
	}
	return goose.NewProvider(dialect, db.writer.DB, fsys, opts...)
}

// CheckVersion returns the database and program schema versions. It fails with
// *SchemaNewerError when the database is ahead, unless AllowNewerSchema.
func (db *DB) CheckVersion(ctx context.Context, o MigrateOptions) (current, target int64, err error) {
	p, err := db.provider(o.FS)
	if errors.Is(err, goose.ErrNoMigrations) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	current, target, err = p.GetVersions(ctx)
	if err != nil {
		return 0, 0, err
	}
	if current > target {
		if !o.AllowNewerSchema {
			return current, target, &SchemaNewerError{DB: current, Program: target}
		}
		o.Logger.Warn("starting on a newer database schema (--allow-newer-schema)",
			"db_version", current, "program_version", target)
	}
	return current, target, nil
}

// Migrate applies pending migrations (02 §5). On SQLite it holds the lock file
// for the whole run and makes the pre-upgrade copy first. It never runs on a
// newer schema.
func (db *DB) Migrate(ctx context.Context, o MigrateOptions) (MigrateResult, error) {
	if db.path != "" {
		release, err := lockDataDir(ctx, filepath.Dir(db.path))
		if err != nil {
			return MigrateResult{}, err
		}
		defer release()
	}
	o.AllowNewerSchema = false
	current, target, err := db.CheckVersion(ctx, o)
	if err != nil {
		return MigrateResult{}, err
	}
	res := MigrateResult{From: current, To: current}
	if current == target {
		return res, nil
	}
	if db.path != "" && current > 0 { // nothing worth copying in a brand-new database
		if res.CopyPath, err = db.preUpgradeCopy(ctx, current, o); err != nil {
			return res, err
		}
	}
	p, err := db.provider(o.FS)
	if err != nil {
		return res, err
	}
	if _, err := p.Up(ctx); err != nil {
		if res.CopyPath != "" {
			return res, fmt.Errorf("migration failed (restore %s if needed): %w", res.CopyPath, err)
		}
		return res, fmt.Errorf("migration failed: %w", err)
	}
	res.To = target
	return res, nil
}

func lockDataDir(ctx context.Context, dir string) (func(), error) {
	fl := flock.New(filepath.Join(dir, "liturgist.lock"))
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ok, err := fl.TryLockContext(ctx, 200*time.Millisecond)
	if err != nil || !ok {
		return nil, errors.New("another Liturgist process is migrating this database")
	}
	return func() { _ = fl.Unlock() }, nil
}

// preUpgradeCopy writes backups/pre-upgrade-vNNNNN-<UTC>-<rand>.db with VACUUM
// INTO and keeps the newest 3. When the copy can't be made it is skipped with a
// warning, or, in strict mode, migration is refused.
func (db *DB) preUpgradeCopy(ctx context.Context, version int64, o MigrateOptions) (string, error) {
	skip := func(reason string, attrs ...any) (string, error) {
		if o.RequireCopy {
			return "", fmt.Errorf("pre-upgrade copy could not be made (%s); migration not started", reason)
		}
		o.Logger.Warn("pre-upgrade copy skipped: "+reason, attrs...)
		return "", nil
	}
	dir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return skip("cannot create backups folder", "error", err)
	}
	need := int64(float64(fileSize(db.path)+fileSize(db.path+"-wal")) * 1.2)
	free, err := freeBytes(dir)
	if err != nil {
		return skip("cannot read free disk space", "error", err)
	}
	if free < need {
		return skip("not enough disk space", "needed_bytes", need, "free_bytes", free)
	}
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	name := fmt.Sprintf("pre-upgrade-v%05d-%s-%s.db", version, o.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(rnd[:]))
	path := filepath.Join(dir, name)
	if _, err := db.writer.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return skip("copy failed", "error", err)
	}
	pruneCopies(dir, 3, o.Logger)
	o.Logger.Info("pre-upgrade copy written", "file", path)
	return path, nil
}

func pruneCopies(dir string, keep int, log *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pre-upgrade-v") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names) // zero-padded version, then timestamp: name order is age order
	for _, n := range names[:max(0, len(names)-keep)] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			log.Warn("cannot delete old pre-upgrade copy", "file", n, "error", err)
		}
	}
}

func fileSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}
