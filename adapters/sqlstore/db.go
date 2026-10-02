// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/jmoiron/sqlx"
)

// Config selects and sizes the database.
type Config struct {
	Driver     string       // "sqlite" | "postgres"
	DataDir    string       // SQLite file lives at DataDir/liturgist.db
	URL        string       // PostgreSQL
	MaxConns   int          // PostgreSQL pool; 0 = 10
	MaxReaders int          // SQLite readers; 0 = 4
	Logger     *slog.Logger // warnings about unknown stored scopes; nil = discard
}

// DB implements app.Tx.
type DB struct {
	d      Dialect
	writer *sqlx.DB
	reader *sqlx.DB // == writer on PostgreSQL
	path   string   // SQLite file, "" on PostgreSQL
	log    *slog.Logger
}

var _ app.Tx = (*DB)(nil)

const (
	txDeadline  = 10 * time.Second
	maxAttempts = 3
)

// Waits between attempts 1→2 and 2→3, each ±50 % (02 §2).
var backoff = [maxAttempts - 1]time.Duration{10 * time.Millisecond, 50 * time.Millisecond}

// Open connects to the configured database.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	switch cfg.Driver {
	case "sqlite":
		if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
			return nil, fmt.Errorf("data folder: %w", err)
		}
		path := filepath.Join(cfg.DataDir, "liturgist.db")
		w, r, err := openSQLite(ctx, path, orDefault(cfg.MaxReaders, 4))
		if err != nil {
			return nil, err
		}
		return &DB{d: sqliteDialect{}, writer: w, reader: r, path: path, log: logger(cfg.Logger)}, nil
	case "postgres":
		db, err := openPostgres(ctx, cfg.URL, orDefault(cfg.MaxConns, 10))
		if err != nil {
			return nil, err
		}
		return &DB{d: postgresDialect{}, writer: db, reader: db, log: logger(cfg.Logger)}, nil
	}
	return nil, fmt.Errorf("unknown database driver %q", cfg.Driver)
}

// Close closes all connections.
func (db *DB) Close() error {
	err := db.writer.Close()
	if db.reader != db.writer {
		err = errors.Join(err, db.reader.Close())
	}
	return err
}

// Dialect returns the database dialect.
func (db *DB) Dialect() Dialect { return db.d }

// Path returns the SQLite file path ("" on PostgreSQL).
func (db *DB) Path() string { return db.path }

// Ping checks the database answers.
func (db *DB) Ping(ctx context.Context) error { return db.reader.PingContext(ctx) }

// Read runs fn in a read-only transaction.
func (db *DB) Read(ctx context.Context, fn func(app.Store) error) error {
	var opts *sql.TxOptions
	if db.d.Name() == "postgres" { // SQLite readers are query_only already
		opts = &sql.TxOptions{ReadOnly: true}
	}
	return db.run(ctx, db.reader, opts, fn)
}

// Write runs fn in a read-write transaction.
func (db *DB) Write(ctx context.Context, fn func(app.Store) error) error {
	return db.run(ctx, db.writer, nil, fn)
}

// run executes fn in a transaction with a 10 s deadline, retrying busy,
// serialisation and deadlock errors up to 3 attempts with jittered backoff (02 §2).
func (db *DB) run(ctx context.Context, pool *sqlx.DB, opts *sql.TxOptions, fn func(app.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, txDeadline)
	defer cancel()
	var err error
	for attempt := range maxAttempts {
		if attempt > 0 {
			timer := time.NewTimer(jitter(backoff[attempt-1]))
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("%w: %w", app.ErrUnavailable, ctx.Err())
			case <-timer.C:
			}
		}
		err = db.attempt(ctx, pool, opts, fn)
		if err == nil || !db.d.Retryable(err) {
			break
		}
	}
	if err != nil && (db.d.Retryable(err) || errors.Is(err, context.DeadlineExceeded)) {
		return fmt.Errorf("%w: %w", app.ErrUnavailable, err)
	}
	return err
}

func (db *DB) attempt(ctx context.Context, pool *sqlx.DB, opts *sql.TxOptions, fn func(app.Store) error) error {
	tx, err := pool.BeginTxx(ctx, opts)
	if err != nil {
		return db.d.MapError(err)
	}
	if err := fn(&store{tx: tx, d: db.d, log: db.log}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return db.d.MapError(tx.Commit())
}

// jitter returns d ± 50 %.
func jitter(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int64N(int64(d))) //nolint:gosec // jitter needs no crypto randomness
}

func logger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.New(slog.DiscardHandler)
	}
	return l
}

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
