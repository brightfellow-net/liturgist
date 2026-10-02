// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/gofrs/flock"
	"github.com/jmoiron/sqlx"
)

func TestMain(m *testing.M) { os.Exit(sqlstoretest.Main(m)) }

var ctx = context.Background()

// TC-P-001: timestamps round-trip as UTC at microsecond precision on both dialects.
func TestTimeRoundTrip(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		colType := "TEXT"
		if db.Dialect().Name() == "postgres" {
			colType = "timestamptz"
		}
		if err := db.ExecForTest(ctx, "CREATE TABLE t_time (ts "+colType+" NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		wib := time.FixedZone("WIB", 7*3600)
		in := time.Date(2026, 10, 4, 7, 0, 0, 123456789, wib)
		if err := db.ExecForTest(ctx, "INSERT INTO t_time (ts) VALUES (?)", db.Dialect().TimeArg(in)); err != nil {
			t.Fatal(err)
		}
		var got sqlstore.Time
		err := db.Read(ctx, func(s app.Store) error {
			return sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT ts FROM t_time").Scan(&got)
		})
		if err != nil {
			t.Fatal(err)
		}
		want := in.UTC().Truncate(time.Microsecond)
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("got %v (%v), want %v UTC", got.Time, got.Location(), want)
		}
	})
}

// TC-P-002
func TestRebind(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		got := db.Dialect().Rebind("SELECT ? , ?")
		want := map[string]string{"sqlite": "SELECT ? , ?", "postgres": "SELECT $1 , $2"}[db.Dialect().Name()]
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// TC-P-003: driver errors map to the same app errors on both dialects.
func TestErrorMapping(t *testing.T) {
	sqlstore.SQLiteUniqueNames["t_child.name"] = "t_child_name_key"
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		for _, ddl := range []string{
			"CREATE TABLE t_parent (id TEXT PRIMARY KEY)",
			"CREATE TABLE t_child (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES t_parent(id), " +
				"name TEXT CONSTRAINT t_child_name_key UNIQUE, n INTEGER CHECK (n > 0))",
			"INSERT INTO t_parent (id) VALUES ('p')",
			"INSERT INTO t_child (id, parent_id, name, n) VALUES ('c1', 'p', 'a', 1)",
		} {
			if err := db.ExecForTest(ctx, ddl); err != nil {
				t.Fatalf("%s: %v", ddl, err)
			}
		}
		insert := func(id, parent, name string, n int) error {
			return db.Write(ctx, func(s app.Store) error {
				tx := sqlstore.RawTx(s)
				_, err := tx.ExecContext(ctx, db.Dialect().Rebind(
					"INSERT INTO t_child (id, parent_id, name, n) VALUES (?, ?, ?, ?)"), id, parent, name, n)
				return db.Dialect().MapError(err)
			})
		}

		var u *app.UniqueError
		if err := insert("c2", "p", "a", 1); !errors.As(err, &u) || u.Constraint != "t_child_name_key" {
			t.Errorf("duplicate name: got %v", err)
		}
		if err := insert("c3", "missing", "b", 1); !errors.Is(err, app.ErrReferenced) {
			t.Errorf("foreign key: got %v", err)
		}
		if err := insert("c4", "p", "c", 0); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("check: got %v", err)
		}
		err := db.Read(ctx, func(s app.Store) error {
			var id string
			return db.Dialect().MapError(sqlstore.RawTx(s).QueryRowContext(ctx,
				db.Dialect().Rebind("SELECT id FROM t_child WHERE id = ?"), "nope").Scan(&id))
		})
		if !errors.Is(err, app.ErrNotFound) {
			t.Errorf("no rows: got %v", err)
		}
	})
}

func TestSQLiteConstraintName(t *testing.T) {
	cases := map[string]string{
		"UNIQUE constraint failed: index 'invites_church_email_open_key' (2067)": "invites_church_email_open_key",
		"UNIQUE constraint failed: t_other.col (2067)":                           "t_other.col",
		"something else": "",
	}
	for msg, want := range cases {
		if got := sqlstore.SQLiteConstraintName(msg); got != want {
			t.Errorf("%q → %q, want %q", msg, got, want)
		}
	}
}

var errFakeBusy = errors.New("fake busy")

type busyDialect struct{ sqlstore.Dialect }

func (busyDialect) Retryable(err error) bool { return errors.Is(err, errFakeBusy) }

// TC-P-004
func TestWriteRetries(t *testing.T) {
	base := sqlstoretest.NewSQLite(t)
	db := base.WithDialect(busyDialect{base.Dialect()})

	calls := 0
	err := db.Write(ctx, func(app.Store) error {
		calls++
		if calls < 3 {
			return errFakeBusy
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("succeeds on 3rd attempt: err=%v calls=%d", err, calls)
	}

	calls = 0
	err = db.Write(ctx, func(app.Store) error { calls++; return errFakeBusy })
	if !errors.Is(err, app.ErrUnavailable) || calls != 3 {
		t.Errorf("gives up after 3 attempts: err=%v calls=%d", err, calls)
	}

	cctx, cancel := context.WithCancel(ctx)
	calls = 0
	start := time.Now()
	err = db.Write(cctx, func(app.Store) error { calls++; cancel(); return errFakeBusy })
	if !errors.Is(err, app.ErrUnavailable) || calls != 1 || time.Since(start) > time.Second {
		t.Errorf("cancel stops waiting: err=%v calls=%d", err, calls)
	}

	other := errors.New("not retryable")
	calls = 0
	if err := db.Write(ctx, func(app.Store) error { calls++; return other }); !errors.Is(err, other) || calls != 1 {
		t.Errorf("other errors are not retried: err=%v calls=%d", err, calls)
	}
}

func TestJitter(t *testing.T) {
	for range 1000 {
		if d := sqlstore.Jitter(10 * time.Millisecond); d < 5*time.Millisecond || d >= 15*time.Millisecond {
			t.Fatalf("jitter out of range: %v", d)
		}
	}
}

// IT-P-004
func TestReadIsReadOnly(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		if err := db.ExecForTest(ctx, "CREATE TABLE t_ro (id TEXT)"); err != nil {
			t.Fatal(err)
		}
		err := db.Read(ctx, func(s app.Store) error {
			_, err := sqlstore.RawTx(s).ExecContext(ctx, "INSERT INTO t_ro (id) VALUES ('x')")
			return err
		})
		if err == nil {
			t.Fatal("insert inside Read must fail")
		}
		var n int
		_ = db.Read(ctx, func(s app.Store) error {
			return sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT count(*) FROM t_ro").Scan(&n)
		})
		if n != 0 {
			t.Errorf("row written by a read transaction")
		}
	})
}

// IT-P-005
func TestTenantSetting(t *testing.T) {
	db := sqlstoretest.NewPostgres(t)
	current := func(tx *sqlx.Tx) string {
		var v string
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(current_setting('liturgist.church_id', true), '')").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	err := db.Write(ctx, func(s app.Store) error {
		if _, err := s.ForChurch(ctx, "01JCHURCHAAAAAAAAAAAAAAAAA"); err != nil {
			return err
		}
		if got := current(sqlstore.RawTx(s)); got != "01JCHURCHAAAAAAAAAAAAAAAAA" {
			t.Errorf("inside transaction: %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Read(ctx, func(s app.Store) error {
		if got := current(sqlstore.RawTx(s)); got != "" {
			t.Errorf("leaked into the next transaction: %q", got)
		}
		return nil
	})
}

// --- migrations (TC-P-005, TC-P-006, TC-P-008) ---

// testMigrations returns n migrations, versions 1..n, each creating one table.
func testMigrations(n int) fstest.MapFS {
	fs := fstest.MapFS{}
	for i := 1; i <= n; i++ {
		fs[fmt.Sprintf("%05d_m.sql", i)] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf("-- +goose Up\nCREATE TABLE m%d (id TEXT);\n", i)),
		}
	}
	return fs
}

func freshSQLite(t *testing.T) (*sqlstore.DB, string) {
	dir := t.TempDir()
	db, err := sqlstore.Open(ctx, sqlstore.Config{Driver: "sqlite", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func opts(fs fstest.MapFS) sqlstore.MigrateOptions {
	return sqlstore.MigrateOptions{FS: fs, Logger: sqlstoretest.Logger, Now: time.Now}
}

func TestMigrateVersions(t *testing.T) {
	run := func(t *testing.T, db *sqlstore.DB) {
		res, err := db.Migrate(ctx, opts(testMigrations(2)))
		if err != nil || res.From != 0 || res.To != 2 {
			t.Fatalf("fresh: %+v %v", res, err)
		}
		if res, err := db.Migrate(ctx, opts(testMigrations(2))); err != nil || res.From != 2 || res.To != 2 {
			t.Errorf("second run must be a no-op: %+v %v", res, err)
		}
		// The program now knows only version 1: the database is newer.
		var newer *sqlstore.SchemaNewerError
		if _, _, err := db.CheckVersion(ctx, opts(testMigrations(1))); !errors.As(err, &newer) || newer.DB != 2 || newer.Program != 1 {
			t.Errorf("newer database: %v", err)
		}
		if _, err := db.Migrate(ctx, opts(testMigrations(1))); !errors.As(err, &newer) {
			t.Errorf("migrate never runs on a newer database: %v", err)
		}
		o := opts(testMigrations(1))
		o.AllowNewerSchema = true
		if cur, tgt, err := db.CheckVersion(ctx, o); err != nil || cur != 2 || tgt != 1 {
			t.Errorf("--allow-newer-schema: %d %d %v", cur, tgt, err)
		}
	}
	t.Run("sqlite", func(t *testing.T) { db, _ := freshSQLite(t); run(t, db) })
	t.Run("postgres", func(t *testing.T) {
		db, err := sqlstore.Open(ctx, sqlstore.Config{Driver: "postgres", URL: sqlstoretest.FreshPostgresDSN(t), MaxConns: 2})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		run(t, db)
	})
}

func copies(_ *testing.T, dir string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, "backups", "pre-upgrade-v*.db"))
	return m
}

// TC-P-008
func TestPreUpgradeCopy(t *testing.T) {
	db, dir := freshSQLite(t)
	if _, err := db.Migrate(ctx, opts(testMigrations(1))); err != nil {
		t.Fatal(err)
	}
	res, err := db.Migrate(ctx, opts(testMigrations(2)))
	if err != nil || res.CopyPath == "" {
		t.Fatalf("copy expected: %+v %v", res, err)
	}
	if !strings.Contains(filepath.Base(res.CopyPath), "pre-upgrade-v00001-") {
		t.Errorf("name: %s", res.CopyPath)
	}
	// The copy opens and is at the old version.
	cp, err := sqlx.Open("sqlite", "file:"+filepath.ToSlash(res.CopyPath))
	if err != nil {
		t.Fatal(err)
	}
	var v int64
	if err := cp.QueryRow("SELECT max(version_id) FROM goose_db_version").Scan(&v); err != nil || v != 1 {
		t.Errorf("copy version %d %v", v, err)
	}
	_ = cp.Close()

	for n := 3; n <= 5; n++ {
		if _, err := db.Migrate(ctx, opts(testMigrations(n))); err != nil {
			t.Fatal(err)
		}
	}
	if got := copies(t, dir); len(got) != 3 {
		t.Errorf("keep newest 3, got %d: %v", len(got), got)
	}
	if res, _ := db.Migrate(ctx, opts(testMigrations(5))); res.CopyPath != "" {
		t.Errorf("no copy without pending migrations")
	}
}

func TestPreUpgradeCopyStrict(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a non-root Unix user to make a folder read-only")
	}
	db, dir := freshSQLite(t)
	if _, err := db.Migrate(ctx, opts(testMigrations(1))); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backups, 0o500); err != nil { //nolint:gosec // directory: read + execute, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backups, 0o700) }) //nolint:gosec // directory needs execute

	o := opts(testMigrations(2))
	o.RequireCopy = true
	if _, err := db.Migrate(ctx, o); err == nil || !strings.Contains(err.Error(), "migration not started") {
		t.Fatalf("strict mode must refuse: %v", err)
	}
	if cur, _, _ := db.CheckVersion(ctx, opts(testMigrations(2))); cur != 1 {
		t.Errorf("version changed to %d despite refusal", cur)
	}
	o.RequireCopy = false
	if res, err := db.Migrate(ctx, o); err != nil || res.CopyPath != "" || res.To != 2 {
		t.Errorf("default mode skips the copy and migrates: %+v %v", res, err)
	}
}

func TestMigrationLockFile(t *testing.T) {
	db, dir := freshSQLite(t)
	other := flock.New(filepath.Join(dir, "liturgist.lock"))
	if ok, err := other.TryLock(); !ok || err != nil {
		t.Fatal("cannot take lock", err)
	}
	defer func() { _ = other.Unlock() }()
	short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if _, err := db.Migrate(short, opts(testMigrations(1))); err == nil || !strings.Contains(err.Error(), "another Liturgist process") {
		t.Errorf("expected lock error, got %v", err)
	}
}
