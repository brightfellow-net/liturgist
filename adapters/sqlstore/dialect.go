// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package sqlstore is the shared SQL persistence adapter for SQLite and PostgreSQL (02).
package sqlstore

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// Dialect holds every database difference (decisions log: one adapter, small dialect layer).
type Dialect interface {
	Name() string // "sqlite" | "postgres"
	Rebind(query string) string
	// MapError turns driver errors into app errors (02 §8); nil and unknown errors pass through.
	MapError(err error) error
	// Retryable reports busy, serialisation and deadlock errors (02 §2).
	Retryable(err error) bool
	LockInstall(ctx context.Context, tx *sqlx.Tx) error
	LockUser(ctx context.Context, tx *sqlx.Tx, id string) error
	LockChurch(ctx context.Context, tx *sqlx.Tx, id string) error
	SetTenant(ctx context.Context, tx *sqlx.Tx, churchID string) error // RLS hook (02 §7)
	TimeArg(t time.Time) any                                           // value to bind for a timestamp
}
