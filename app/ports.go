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
