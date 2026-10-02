// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package app holds use cases and the ports they depend on (02 §2).
package app

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Tx runs fn inside one database transaction. fn may run up to 3 times
// (retries), so it must have no side effects outside the transaction, and only
// values from the committed attempt may be used afterwards.
type Tx interface {
	Read(ctx context.Context, fn func(s Store) error) error
	Write(ctx context.Context, fn func(s Store) error) error
}

// Store gives access to repositories inside a transaction. Repositories are
// added by the slices that need them.
type Store interface {
	LockInstall(ctx context.Context) error                // setup, setup-link issuing (02 §2.1)
	LockUser(ctx context.Context, id domain.UserID) error // reset-link creation
	ForChurch(ctx context.Context, id domain.ChurchID) (ChurchStore, error)
}

// ChurchStore gives access to church-scoped repositories.
type ChurchStore interface {
	ChurchID() domain.ChurchID
	LockChurch(ctx context.Context) error // first statement of every check-then-write rule
}

// Clock returns the current time in UTC, truncated to microseconds (02 §4).
type Clock interface{ Now() time.Time }

// IDGenerator returns new ULIDs.
type IDGenerator interface{ NewID() string }

// PasswordHasher hashes and verifies passwords. Implementations normalise with
// domain.NormalizePassword, bound concurrency, and must never be called inside
// a database transaction (03 §3).
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Verify reports whether password matches encoded, and whether encoded
	// uses outdated parameters and should be replaced after a successful login.
	Verify(ctx context.Context, encoded, password string) (ok, needsRehash bool, err error)
	// VerifyDummy costs the same as Verify; used when no user exists, so
	// timing doesn't reveal whether an account exists.
	VerifyDummy(ctx context.Context, password string) error
}
