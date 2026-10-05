// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/internal/backup"
)

// seeded returns the config of a migrated install with one church and admin.
func seeded(t *testing.T) Config {
	t.Helper()
	base, _ := url.Parse("https://liturgi.example.org")
	cfg := Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0", BaseURL: base, DBDriver: "sqlite", AutoMigrate: true}
	ctx := context.Background()
	if _, err := Migrate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	op, err := OpenOperator(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = op.Close() }()
	if err := op.Setup(ctx, SetupParams{ChurchName: "GKY Citragarden", AdminName: "Admin", AdminIdentifier: "admin@example.org",
		Password: "kopi susu pagi hari", UILanguage: "en", Language: "id", Translation: "TB", TimeZone: "Asia/Jakarta", KeyDisplay: "do"}); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func dbExec(t *testing.T, cfg Config, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(cfg.DataDir, "liturgist.db"))+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func churchName(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n string
	if err := db.QueryRow("SELECT name FROM churches").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// IT-601: the restore drill.
func TestBackupRestoreDrill(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	live := filepath.Join(cfg.DataDir, "liturgist.db")
	write(t, filepath.Join(cfg.DataDir, "files", "logo.txt"), "original file")

	res, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "church.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Schema < 1 || res.Size == 0 {
		t.Errorf("result %+v", res)
	}

	dbExec(t, cfg, "UPDATE churches SET name = 'Changed'")
	write(t, filepath.Join(cfg.DataDir, "files", "logo.txt"), "changed file")
	write(t, filepath.Join(cfg.DataDir, "files", "new.txt"), "added later")

	out, err := Restore(ctx, cfg, res.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := churchName(t, live); got != "GKY Citragarden" {
		t.Errorf("restored name %q", got)
	}
	if got := read(t, filepath.Join(cfg.DataDir, "files", "logo.txt")); got != "original file" {
		t.Errorf("restored file %q", got)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "files", "new.txt")); err == nil {
		t.Error("a file added after the backup survived the restore")
	}
	// What was replaced is kept.
	if got := churchName(t, out.PreRestoreDB); got != "Changed" {
		t.Errorf("pre-restore database holds %q", got)
	}
	if got := read(t, filepath.Join(out.PreRestoreFiles, "new.txt")); got != "added later" {
		t.Errorf("pre-restore files: %q", got)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "restore-tmp")); err == nil {
		t.Error("restore-tmp left behind")
	}
	// The restored install starts.
	srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("serve after restore: %v", err)
	}
	_ = srv.Close()
}

// TC-603: a backup taken while another connection writes is consistent.
func TestBackupWhileWriting(t *testing.T) {
	cfg := seeded(t)
	dbExec(t, cfg, "CREATE TABLE scratch (n INTEGER)")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				dbExec(t, cfg, "INSERT INTO scratch VALUES (?)", i)
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	res, err := Backup(context.Background(), cfg, filepath.Join(t.TempDir(), "w.zip"))
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	a, err := backup.Open(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	dir := t.TempDir()
	if err := a.Extract(dir); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "liturgist.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n, max int
	if err := db.QueryRow("SELECT COUNT(*), COALESCE(MAX(n), -1) FROM scratch").Scan(&n, &max); err != nil {
		t.Fatal(err)
	}
	if n == 0 || max != n-1 { // rows are 0..n-1 with no gap: a snapshot at one moment
		t.Errorf("snapshot has %d rows, max %d", n, max)
	}
}

func TestBackupRefusals(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "b.zip")
	if _, err := Backup(ctx, cfg, dest); err != nil { // TC-605
		t.Fatal(err)
	}
	before := read(t, dest)
	if _, err := Backup(ctx, cfg, dest); !errors.Is(err, ErrBackupExists) {
		t.Errorf("existing file: %v", err)
	}
	if read(t, dest) != before {
		t.Error("the existing file changed")
	}
	pg := cfg
	pg.DBDriver = "postgres"
	if _, err := Backup(ctx, pg, ""); !errors.Is(err, ErrNotSQLite) {
		t.Errorf("postgres backup: %v", err)
	}
	if _, err := Restore(ctx, pg, dest, nil); !errors.Is(err, ErrNotSQLite) {
		t.Errorf("postgres restore: %v", err)
	}
	empty := cfg
	empty.DataDir = t.TempDir()
	if _, err := Backup(ctx, empty, ""); !errors.Is(err, ErrNoDatabase) {
		t.Errorf("no database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(empty.DataDir, "backups")); err == nil {
		t.Error("a refused backup created a folder")
	}
}

// TC-604
func TestBackupNotEnoughSpace(t *testing.T) {
	cfg := seeded(t)
	old := freeBytes
	freeBytes = func(string) (int64, error) { return 1024, nil }
	defer func() { freeBytes = old }()
	dir := t.TempDir()
	dest := filepath.Join(dir, "b.zip")
	if _, err := Backup(context.Background(), cfg, dest); !errors.Is(err, ErrNotEnoughSpace) {
		t.Fatalf("error %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files left behind: %v", entries)
	}
}

// TC-608 and the "stop the server first" rule.
func TestRunLock(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	zipPath, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "b.zip"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := LockRun(cfg) // what serve holds
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockRun(cfg); err == nil {
		t.Error("a second server could take the lock")
	}
	dbExec(t, cfg, "UPDATE churches SET name = 'Changed'")
	if _, err := Restore(ctx, cfg, zipPath.Path, nil); !errors.Is(err, ErrServerRunning) {
		t.Errorf("restore while serving: %v", err)
	}
	if got := churchName(t, filepath.Join(cfg.DataDir, "liturgist.db")); got != "Changed" {
		t.Errorf("a refused restore changed the data: %q", got)
	}
	// Backups are allowed while serving.
	if _, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "again.zip")); err != nil {
		t.Errorf("backup while serving: %v", err)
	}
	release()
	if _, err := Restore(ctx, cfg, zipPath.Path, nil); err != nil {
		t.Errorf("restore after stop: %v", err)
	}
}

// TC-602: a backup of a newer schema, a damaged one, and a declined one.
func TestRestoreRefusals(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	live := filepath.Join(cfg.DataDir, "liturgist.db")
	good, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "good.zip"))
	if err != nil {
		t.Fatal(err)
	}
	dbExec(t, cfg, "UPDATE churches SET name = 'Current'")
	unchanged := func(t *testing.T) {
		t.Helper()
		if got := churchName(t, live); got != "Current" {
			t.Errorf("the data changed: %q", got)
		}
		if _, err := os.Stat(filepath.Join(cfg.DataDir, "restore-tmp")); err == nil {
			t.Error("restore-tmp left behind")
		}
	}

	t.Run("newer schema", func(t *testing.T) {
		a, err := backup.Open(good.Path)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := a.Extract(dir); err != nil {
			t.Fatal(err)
		}
		m := a.Manifest
		_ = a.Close()
		m.Schema = 9999
		newer := filepath.Join(t.TempDir(), "newer.zip")
		if err := backup.CreateFile(newer, m, filepath.Join(dir, "liturgist.db"), "", nil); err != nil {
			t.Fatal(err)
		}
		_, err = Restore(ctx, cfg, newer, nil)
		if !IsBackupNewer(err) {
			t.Fatalf("error %v", err)
		}
		unchanged(t)
	})
	t.Run("manifest understates the schema", func(t *testing.T) {
		a, err := backup.Open(good.Path)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := a.Extract(dir); err != nil {
			t.Fatal(err)
		}
		m := a.Manifest
		_ = a.Close()
		inner := Config{DataDir: dir}
		dbExec(t, inner, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, 1)")
		liar := filepath.Join(t.TempDir(), "liar.zip")
		if err := backup.CreateFile(liar, m, filepath.Join(dir, "liturgist.db"), "", nil); err != nil { // manifest still says the old schema
			t.Fatal(err)
		}
		if _, err := Restore(ctx, cfg, liar, nil); !IsBackupNewer(err) {
			t.Fatalf("error %v", err)
		}
		unchanged(t)
	})
	t.Run("damaged", func(t *testing.T) {
		b := []byte(read(t, good.Path))
		for i := len(b) / 3; i < len(b)/3+40; i++ { // inside the compressed database
			b[i] ^= 0xFF
		}
		p := filepath.Join(t.TempDir(), "damaged.zip")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Restore(ctx, cfg, p, nil); !errors.Is(err, ErrBackupDamaged) {
			t.Fatalf("error %v", err)
		}
		unchanged(t)
	})
	t.Run("not a backup", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "x.zip")
		write(t, p, "hello")
		if _, err := Restore(ctx, cfg, p, nil); !errors.Is(err, ErrBackupDamaged) {
			t.Fatalf("error %v", err)
		}
		unchanged(t)
	})
	t.Run("declined", func(t *testing.T) {
		asked := false
		_, err := Restore(ctx, cfg, good.Path, func(RestoreInfo) bool { asked = true; return false })
		if !asked || !errors.Is(err, ErrDeclined) {
			t.Fatalf("asked %v, error %v", asked, err)
		}
		unchanged(t)
	})
	t.Run("not enough space", func(t *testing.T) {
		old := freeBytes
		freeBytes = func(string) (int64, error) { return 10, nil }
		defer func() { freeBytes = old }()
		if _, err := Restore(ctx, cfg, good.Path, nil); !errors.Is(err, ErrNotEnoughSpace) {
			t.Fatalf("error %v", err)
		}
		unchanged(t)
	})
}

// IT-603: an interruption after the current data was set aside.
func TestRestoreInterrupted(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	live := filepath.Join(cfg.DataDir, "liturgist.db")
	good, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "good.zip"))
	if err != nil {
		t.Fatal(err)
	}
	dbExec(t, cfg, "UPDATE churches SET name = 'Current'")
	write(t, filepath.Join(cfg.DataDir, "files", "f.txt"), "current")

	afterKeep = func() error { return errors.New("simulated crash") }
	res, err := Restore(ctx, cfg, good.Path, nil)
	afterKeep = nil
	if err == nil {
		t.Fatal("no error")
	}
	if churchName(t, res.PreRestoreDB) != "Current" {
		t.Error("the set-aside database does not hold the current data")
	}
	if read(t, filepath.Join(res.PreRestoreFiles, "f.txt")) != "current" {
		t.Error("the set-aside files do not hold the current files")
	}
	// A second restore of the same file succeeds.
	if _, err := Restore(ctx, cfg, good.Path, nil); err != nil {
		t.Fatal(err)
	}
	if got := churchName(t, live); got != "GKY Citragarden" {
		t.Errorf("name %q", got)
	}
}

// A damaged live database is kept as it is.
func TestRestoreOverDamagedDatabase(t *testing.T) {
	cfg := seeded(t)
	ctx := context.Background()
	good, err := Backup(ctx, cfg, filepath.Join(t.TempDir(), "good.zip"))
	if err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(cfg.DataDir, "liturgist.db")
	write(t, live, "this is not a database at all, just bytes")
	res, err := Restore(ctx, cfg, good.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, res.PreRestoreDB); got != "this is not a database at all, just bytes" {
		t.Errorf("kept bytes %q", got)
	}
	if got := churchName(t, live); got != "GKY Citragarden" {
		t.Errorf("name %q", got)
	}
}

// TC-609 (mapping part)
func TestStorageErr(t *testing.T) {
	for _, in := range []error{syscall.ENOSPC, app.ErrUnavailable} {
		if err := storageErr(in); !errors.Is(err, ErrStorageFull) {
			t.Errorf("%v: %v", in, err)
		}
	}
	other := errors.New("other")
	if err := storageErr(other); err != other { //nolint:errorlint // identity is the point
		t.Errorf("other error changed: %v", err)
	}
}
