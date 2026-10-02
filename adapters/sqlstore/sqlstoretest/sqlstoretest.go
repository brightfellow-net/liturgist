// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package sqlstoretest provides migrated databases for tests (02 §6).
//
// Packages using it should call it from TestMain so templates and the
// PostgreSQL container are cleaned up:
//
//	func TestMain(m *testing.M) { os.Exit(sqlstoretest.Main(m)) }
package sqlstoretest

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const pgTemplate = "liturgist_template"

var (
	sqliteOnce     sync.Once
	sqliteTemplate string // migrated template file
	sqliteErr      error

	pgOnce      sync.Once
	pgContainer *postgres.PostgresContainer
	pgBaseDSN   string // connection string for the "postgres" admin database
	pgErr       error
	pgCreateMu  sync.Mutex // CREATE DATABASE … TEMPLATE needs exclusive use of the template

	ids = ulidgen.New()
)

// Logger discards output; tests that check logs build their own.
var Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Main runs the tests and removes templates and the PostgreSQL container afterwards.
func Main(m *testing.M) int {
	code := m.Run()
	if sqliteTemplate != "" {
		_ = os.RemoveAll(filepath.Dir(sqliteTemplate))
	}
	if pgContainer != nil {
		_ = pgContainer.Terminate(context.Background())
	}
	return code
}

func migrateOptions(dialect string) sqlstore.MigrateOptions {
	return sqlstore.MigrateOptions{FS: migrations.For(dialect), Logger: Logger, Now: time.Now}
}

// NewSQLite returns a store on a private copy of a migrated template file.
func NewSQLite(t testing.TB) *sqlstore.DB {
	t.Helper()
	sqliteOnce.Do(func() {
		dir, err := os.MkdirTemp("", "liturgist-template-*")
		if err != nil {
			sqliteErr = err
			return
		}
		ctx := context.Background()
		db, err := sqlstore.Open(ctx, sqlstore.Config{Driver: "sqlite", DataDir: dir})
		if err != nil {
			sqliteErr = err
			return
		}
		_, sqliteErr = db.Migrate(ctx, migrateOptions("sqlite"))
		if err := db.Close(); err != nil && sqliteErr == nil {
			sqliteErr = err
		}
		sqliteTemplate = filepath.Join(dir, "liturgist.db")
	})
	if sqliteErr != nil {
		t.Fatalf("sqlite template: %v", sqliteErr)
	}
	dir := t.TempDir()
	if err := copyFile(sqliteTemplate, filepath.Join(dir, "liturgist.db")); err != nil {
		t.Fatal(err)
	}
	db, err := sqlstore.Open(context.Background(), sqlstore.Config{Driver: "sqlite", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// PostgresEnabled reports whether PostgreSQL tests should run.
func PostgresEnabled() bool { return os.Getenv("LITURGIST_TEST_POSTGRES") == "1" }

// NewPostgres returns a store on a fresh database created from a migrated
// template in a shared postgres:17-alpine container. It skips the test unless
// LITURGIST_TEST_POSTGRES=1.
func NewPostgres(t testing.TB) *sqlstore.DB {
	t.Helper()
	if !PostgresEnabled() {
		t.Skip("PostgreSQL tests skipped; set LITURGIST_TEST_POSTGRES=1 (needs Docker)")
	}
	pgOnce.Do(startPostgres)
	if pgErr != nil {
		t.Fatalf("postgres: %v", pgErr)
	}
	name := "t_" + strings.ToLower(ids.NewID())
	if err := pgAdmin(func(admin *sql.DB) error {
		pgCreateMu.Lock()
		defer pgCreateMu.Unlock()
		_, err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE " + pgTemplate)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	db, err := sqlstore.Open(context.Background(), sqlstore.Config{Driver: "postgres", URL: dsnFor(name), MaxConns: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = pgAdmin(func(admin *sql.DB) error {
			_, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
			return err
		})
	})
	return db
}

// ForEachDialect runs fn as the subtests "sqlite" and "postgres".
func ForEachDialect(t *testing.T, fn func(t *testing.T, db *sqlstore.DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, NewSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, NewPostgres(t)) })
}

// FreshPostgresDSN returns the DSN of a new, empty (unmigrated) database, for migration tests.
func FreshPostgresDSN(t testing.TB) string {
	t.Helper()
	if !PostgresEnabled() {
		t.Skip("PostgreSQL tests skipped; set LITURGIST_TEST_POSTGRES=1 (needs Docker)")
	}
	pgOnce.Do(startPostgres)
	if pgErr != nil {
		t.Fatalf("postgres: %v", pgErr)
	}
	name := "f_" + strings.ToLower(ids.NewID())
	if err := pgAdmin(func(admin *sql.DB) error {
		_, err := admin.Exec("CREATE DATABASE " + name)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pgAdmin(func(admin *sql.DB) error {
			_, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
			return err
		})
	})
	return dsnFor(name)
}

func startPostgres() {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("liturgist"),
		postgres.WithPassword("liturgist"),
		postgres.BasicWaitStrategies())
	if err != nil {
		pgErr = err
		return
	}
	pgContainer = ctr
	if pgBaseDSN, err = ctr.ConnectionString(ctx, "sslmode=disable"); err != nil {
		pgErr = err
		return
	}
	if pgErr = pgAdmin(func(admin *sql.DB) error {
		_, err := admin.Exec("CREATE DATABASE " + pgTemplate)
		return err
	}); pgErr != nil {
		return
	}
	db, err := sqlstore.Open(ctx, sqlstore.Config{Driver: "postgres", URL: dsnFor(pgTemplate), MaxConns: 2})
	if err != nil {
		pgErr = err
		return
	}
	_, pgErr = db.Migrate(ctx, migrateOptions("postgres"))
	_ = db.Close()
}

func dsnFor(database string) string {
	cfg, err := pgx.ParseConfig(pgBaseDSN)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", cfg.User, cfg.Password, cfg.Host, cfg.Port, database)
}

func pgAdmin(fn func(*sql.DB) error) error {
	cfg, err := pgx.ParseConfig(pgBaseDSN)
	if err != nil {
		return err
	}
	admin := stdlib.OpenDB(*cfg)
	defer func() { _ = admin.Close() }()
	return fn(admin)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // test template path, not user input
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // path inside t.TempDir()
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
