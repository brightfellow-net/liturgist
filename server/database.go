// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sysclock"
	"github.com/brightfellow-net/liturgist/migrations"
)

// IsSchemaNewer reports whether err means the database is newer than this
// program (the CLI exits with code 3).
func IsSchemaNewer(err error) bool {
	var e *sqlstore.SchemaNewerError
	return errors.As(err, &e)
}

func openDB(ctx context.Context, cfg Config) (*sqlstore.DB, sqlstore.MigrateOptions, error) {
	db, err := sqlstore.Open(ctx, sqlstore.Config{
		Driver: cfg.DBDriver, DataDir: cfg.DataDir, URL: cfg.DBURL,
		MaxConns: cfg.DBMaxConns, MaxReaders: cfg.DBMaxReaders,
	})
	if err != nil {
		return nil, sqlstore.MigrateOptions{}, err
	}
	return db, sqlstore.MigrateOptions{
		FS:               migrations.For(db.Dialect().Name()),
		AllowNewerSchema: cfg.AllowNewerSchema,
		RequireCopy:      cfg.RequirePreUpgradeCopy,
		Logger:           cfg.Logger,
		Now:              sysclock.Clock{}.Now,
	}, nil
}

// prepareSchema checks the schema version and, when AutoMigrate is set,
// applies pending migrations (02 §5).
func prepareSchema(ctx context.Context, cfg Config, db *sqlstore.DB, mo sqlstore.MigrateOptions) error {
	current, target, err := db.CheckVersion(ctx, mo)
	if err != nil {
		return err
	}
	if current >= target {
		return nil
	}
	if !cfg.AutoMigrate {
		return fmt.Errorf("database version %d needs migrating to %d; run \"liturgist migrate\"", current, target)
	}
	res, err := db.Migrate(ctx, mo)
	if err != nil {
		return err
	}
	cfg.Logger.Info("database migrated", "from", res.From, "to", res.To)
	return nil
}

func readyCheck(db *sqlstore.DB, mo sqlstore.MigrateOptions) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return errors.New("database unreachable")
		}
		current, target, err := db.CheckVersion(ctx, mo)
		if err != nil {
			return errors.New("cannot read schema version")
		}
		if current < target {
			return errors.New("migrations pending")
		}
		return nil
	}
}

// MigrateResult reports a migration run for the CLI.
type MigrateResult = sqlstore.MigrateResult

// Migrate applies pending migrations (liturgist migrate).
func Migrate(ctx context.Context, cfg Config) (MigrateResult, error) {
	db, mo, err := openDB(ctx, cfg)
	if err != nil {
		return MigrateResult{}, err
	}
	defer func() { _ = db.Close() }()
	return db.Migrate(ctx, mo)
}

// MigrateStatus returns the database and program schema versions (liturgist migrate status).
func MigrateStatus(ctx context.Context, cfg Config) (current, target int64, err error) {
	db, mo, err := openDB(ctx, cfg)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = db.Close() }()
	return db.CheckVersion(ctx, mo)
}
