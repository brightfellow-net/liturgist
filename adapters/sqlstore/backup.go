// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
)

// ErrAlreadyRunning means another Liturgist process holds the data folder's
// run lock (14 §3).
var ErrAlreadyRunning = errors.New("another Liturgist process is using this data folder")

// FreeBytes returns the free disk space available to the program in dir.
func FreeBytes(dir string) (int64, error) { return freeBytes(dir) }

// SnapshotSpace is the free space a copy of the SQLite file at path needs:
// the database and its WAL, plus 20 %.
func SnapshotSpace(path string) int64 {
	return int64(float64(fileSize(path)+fileSize(path+"-wal")) * 1.2)
}

// LockRun takes the run lock of the data folder, without waiting. serve holds
// it for its whole life; restore needs it (14 §3, H-2). The returned function
// releases it.
func LockRun(dir string) (func(), error) {
	fl := flock.New(filepath.Join(dir, "liturgist.run"))
	ok, err := fl.TryLock()
	if err != nil {
		return nil, fmt.Errorf("run lock: %w", err)
	}
	if !ok {
		return nil, ErrAlreadyRunning
	}
	return func() { _ = fl.Unlock() }, nil
}

// LockDataDir takes the migration lock of the data folder (waits up to 30 s).
func LockDataDir(ctx context.Context, dir string) (func(), error) { return lockDataDir(ctx, dir) }

// SnapshotFile writes a consistent copy of the SQLite database src to dest
// (which must not exist) with VACUUM INTO. It uses a connection of its own,
// so it works while a server has the file open and never queues behind its
// writer. When the disk is full the error wraps app.ErrStorageFull.
func SnapshotFile(ctx context.Context, src, dest string) error {
	db, err := sql.Open("sqlite", plainDSN(src, false)) // VACUUM INTO is refused under query_only
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		_ = os.Remove(dest)
		return mapDiskFull(err)
	}
	return nil
}

func plainDSN(path string, queryOnly bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	if queryOnly {
		q.Add("_pragma", "query_only(1)")
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

func mapDiskFull(err error) error {
	return sqliteDialect{}.MapError(err) // SQLITE_FULL becomes app.ErrStorageFull
}

// CheckSQLiteFile opens the database file at path read-only, runs SQLite's
// integrity and foreign-key checks, and returns its schema version. A file
// that is not a Liturgist database is an error.
func CheckSQLiteFile(ctx context.Context, path string) (version int64, err error) {
	db, err := sql.Open("sqlite", plainDSN(path, true))
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return 0, fmt.Errorf("integrity check: %w", err)
	}
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if line != "ok" {
			problems = append(problems, line)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, fmt.Errorf("integrity check: %w", err)
	}
	if len(problems) > 0 {
		return 0, fmt.Errorf("integrity check failed: %s", strings.Join(problems[:min(len(problems), 3)], "; "))
	}
	fk, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return 0, fmt.Errorf("foreign key check: %w", err)
	}
	broken := fk.Next()
	if err := errors.Join(fk.Err(), fk.Close()); err != nil {
		return 0, fmt.Errorf("foreign key check: %w", err)
	}
	if broken {
		return 0, errors.New("foreign key check failed")
	}
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&version); err != nil {
		return 0, errors.New("not a Liturgist database")
	}
	return version, nil
}

// ProgramVersion is the newest schema version of the migrations in fsys,
// from the numeric prefix of the file names (00012_x.sql is 12).
func ProgramVersion(fsys fs.FS) (int64, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	var newest int64
	for _, e := range entries {
		digits, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			continue
		}
		if n, err := strconv.ParseInt(digits, 10, 64); err == nil {
			newest = max(newest, n)
		}
	}
	return newest, nil
}
