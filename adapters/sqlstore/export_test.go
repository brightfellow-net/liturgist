// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/jmoiron/sqlx"
)

// Test-only access to internals for the external sqlstore_test package.

var (
	SQLiteConstraintName = sqliteConstraintName
	SQLiteUniqueNames    = sqliteUniqueNames
	Jitter               = jitter
)

// ExecForTest runs a statement outside any Tx (DDL in tests).
func (db *DB) ExecForTest(ctx context.Context, q string, args ...any) error {
	_, err := db.writer.ExecContext(ctx, db.d.Rebind(q), args...)
	return db.d.MapError(err)
}

// RawTx exposes the transaction behind a Store.
func RawTx(s app.Store) *sqlx.Tx {
	if cs, ok := s.(*churchStore); ok {
		return cs.tx
	}
	return s.(*store).tx
}

// WithDialect returns a copy of db using d (to fake retryable errors).
func (db *DB) WithDialect(d Dialect) *DB {
	c := *db
	c.d = d
	return &c
}
