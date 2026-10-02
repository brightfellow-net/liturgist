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
	Users() UserRepo
	Sessions() SessionRepo
	AuthThrottle() ThrottleRepo
	Churches() ChurchRepo
	Translations() TranslationRepo
	SetupTokens() SetupTokenRepo

	LockInstall(ctx context.Context) error                // setup, setup-link issuing (02 §2.1)
	LockUser(ctx context.Context, id domain.UserID) error // reset-link creation
	ForChurch(ctx context.Context, id domain.ChurchID) (ChurchStore, error)
}

// ChurchStore gives access to church-scoped repositories. Every query they
// run filters by the church; IT-P-007 checks each method.
type ChurchStore interface {
	ChurchID() domain.ChurchID
	LockChurch(ctx context.Context) error // first statement of every check-then-write rule
	Church() ChurchSettingsRepo
	Memberships() MembershipRepo
	Roles() RoleRepo
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

// UserRepo stores platform-wide users.
type UserRepo interface {
	ByIdentifier(ctx context.Context, id domain.Identifier) (domain.User, error) // ErrNotFound
	ByID(ctx context.Context, id domain.UserID) (domain.User, error)             // ErrNotFound
	Create(ctx context.Context, u domain.User) error                             // UniqueError users_email_key / users_phone_key
	SetPasswordHash(ctx context.Context, id domain.UserID, hash string, now time.Time) error
	UpdateProfile(ctx context.Context, id domain.UserID, name string, prefs domain.Preferences, now time.Time) error
	Touch(ctx context.Context, id domain.UserID, now time.Time) error // last_seen_at
	MembershipChurchIDs(ctx context.Context, id domain.UserID) ([]domain.ChurchID, error)
}

// SessionRepo stores login sessions.
type SessionRepo interface {
	Create(ctx context.Context, s domain.Session) error
	ByTokenHash(ctx context.Context, hash string) (domain.Session, error) // ErrNotFound
	// Extend is the conditional update of 03 §4: false when the session was
	// deleted or had expired meanwhile.
	Extend(ctx context.Context, hash string, now, expires time.Time) (bool, error)
	Delete(ctx context.Context, hash string) error
	DeleteOthers(ctx context.Context, user domain.UserID, keepHash string) error
	DeleteAllForUser(ctx context.Context, user domain.UserID) error
}

// ThrottleRepo stores login-throttle counters (03 §5).
type ThrottleRepo interface {
	// LockedUntil returns the latest lock among keys still active at now (zero if none).
	LockedUntil(ctx context.Context, keys []string, now time.Time) (time.Time, error)
	// RecordFailure atomically adds one failure (02 §2.1).
	RecordFailure(ctx context.Context, key string, rule domain.ThrottleRule, now time.Time) error
	Delete(ctx context.Context, keys ...string) error
}

// ChurchRepo is the platform view of churches (setup, resolver).
type ChurchRepo interface {
	Create(ctx context.Context, c domain.Church) error
	Count(ctx context.Context) (int, error)
	IDs(ctx context.Context, limit int) ([]domain.ChurchID, error) // ordered by ID
	ByID(ctx context.Context, id domain.ChurchID) (domain.Church, error)
}

// TranslationRepo reads the seeded Bible translations.
type TranslationRepo interface {
	List(ctx context.Context) ([]domain.Translation, error) // ordered by code
	ByCode(ctx context.Context, code string) (domain.Translation, error)
	ByID(ctx context.Context, id domain.TranslationID) (domain.Translation, error)
}

// SetupTokenRepo stores the singleton setup token (03 §10).
type SetupTokenRepo interface {
	Put(ctx context.Context, hash string, created, expires time.Time) error // upsert of row 1
	// Claim deletes the token if hash matches and it hasn't expired (atomic claim).
	Claim(ctx context.Context, hash string, now time.Time) (bool, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}

// ChurchSettingsRepo reads and writes the scoped church row.
type ChurchSettingsRepo interface {
	Get(ctx context.Context) (domain.Church, error)
	Update(ctx context.Context, c domain.Church) error // name, languages, translation, zone, settings, updated_at
}

// MembershipRepo stores the church's memberships and their roles.
type MembershipRepo interface {
	List(ctx context.Context) ([]domain.Member, error) // with users and role IDs, ordered by name
	ByID(ctx context.Context, id domain.MembershipID) (domain.Membership, error)
	ByUser(ctx context.Context, user domain.UserID) (domain.Membership, error)
	Create(ctx context.Context, m domain.Membership) error // with its roles
	SetRoles(ctx context.Context, id domain.MembershipID, roles []domain.RoleID) error
	Delete(ctx context.Context, id domain.MembershipID) error
	Count(ctx context.Context) (int, error)
}

// RoleRepo stores the church's roles. Unknown stored scopes are dropped on load.
type RoleRepo interface {
	List(ctx context.Context) ([]domain.Role, error) // ordered by name
	ByID(ctx context.Context, id domain.RoleID) (domain.Role, error)
	ByOrigin(ctx context.Context, o domain.RoleOrigin) (domain.Role, error)
	Create(ctx context.Context, r domain.Role) error // UniqueError roles_church_name_key
	Update(ctx context.Context, r domain.Role) error // name, description, scopes, updated_at
	Delete(ctx context.Context, id domain.RoleID) error
	MemberCounts(ctx context.Context) (map[domain.RoleID]int, error)
}
