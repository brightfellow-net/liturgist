// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

var t0 = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

func newUser(uid, email, phone string) domain.User {
	return domain.User{ID: domain.UserID(id(uid)), Name: "Budi", Email: email, Phone: phone,
		PasswordHash: "h", Preferences: domain.Preferences{TextSize: "large"}, CreatedAt: t0, UpdatedAt: t0}
}

func mustWrite(t *testing.T, db *sqlstore.DB, fn func(s app.Store) error) {
	t.Helper()
	if err := db.Write(ctx, fn); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, db *sqlstore.DB, fn func(s app.Store) error) {
	t.Helper()
	if err := db.Read(ctx, fn); err != nil {
		t.Fatal(err)
	}
}

// IT-P-001 (users, sessions)
func TestUserAndSessionRepos(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		write := func(fn func(s app.Store) error) error { return db.Write(ctx, fn) }

		if err := write(func(s app.Store) error {
			if err := s.Users().Create(ctx, newUser("U1", "budi@example.org", "+6281234567890")); err != nil {
				return err
			}
			return s.Users().Create(ctx, newUser("U2", "", "+6281111111111"))
		}); err != nil {
			t.Fatal(err)
		}
		var u *app.UniqueError
		if err := write(func(s app.Store) error { return s.Users().Create(ctx, newUser("U3", "budi@example.org", "")) }); !errors.As(err, &u) || u.Constraint != "users_email_key" {
			t.Errorf("duplicate email: %v", err)
		}
		if err := write(func(s app.Store) error { return s.Users().Create(ctx, newUser("U4", "", "+6281111111111")) }); !errors.As(err, &u) || u.Constraint != "users_phone_key" {
			t.Errorf("duplicate phone: %v", err)
		}

		mustRead(t, db, func(s app.Store) error {
			byPhone, err := s.Users().ByIdentifier(ctx, domain.Identifier{Kind: domain.Phone, Value: "+6281234567890"})
			if err != nil || byPhone.ID != domain.UserID(id("U1")) || byPhone.Email != "budi@example.org" ||
				byPhone.Preferences.TextSize != "large" || !byPhone.CreatedAt.Equal(t0) || !byPhone.LastSeenAt.IsZero() {
				t.Errorf("by phone: %+v %v", byPhone, err)
			}
			u2, err := s.Users().ByIdentifier(ctx, domain.Identifier{Kind: domain.Phone, Value: "+6281111111111"})
			if err != nil || u2.Email != "" {
				t.Errorf("user without email: %+v %v", u2, err)
			}
			if _, err := s.Users().ByIdentifier(ctx, domain.Identifier{Kind: domain.Email, Value: "nobody@example.org"}); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("unknown: %v", err)
			}
			return nil
		})

		later := t0.Add(time.Hour)
		if err := write(func(s app.Store) error {
			if err := s.Users().Touch(ctx, domain.UserID(id("U1")), later); err != nil {
				return err
			}
			return s.Users().SetPasswordHash(ctx, domain.UserID(id("U1")), "h2", later)
		}); err != nil {
			t.Fatal(err)
		}
		if err := write(func(s app.Store) error { return s.Users().Touch(ctx, "missing00000000000000000000", later) }); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("touch unknown user: %v", err)
		}

		sess := func(h string, user string) domain.Session {
			return domain.Session{TokenHash: hash(h), UserID: domain.UserID(id(user)), CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(24 * time.Hour)}
		}
		if err := write(func(s app.Store) error {
			for _, x := range []domain.Session{sess("1", "U1"), sess("2", "U1"), sess("3", "U1"), sess("4", "U2")} {
				if err := s.Sessions().Create(ctx, x); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		// Conditional extension.
		var ok bool
		mustWrite(t, db, func(s app.Store) (err error) {
			ok, err = s.Sessions().Extend(ctx, hash("1"), t0.Add(2*time.Hour), t0.Add(48*time.Hour))
			return
		})
		if !ok {
			t.Error("live session must extend")
		}
		mustWrite(t, db, func(s app.Store) (err error) {
			ok, err = s.Sessions().Extend(ctx, hash("2"), t0.Add(25*time.Hour), t0.Add(48*time.Hour))
			return
		})
		if ok {
			t.Error("expired session must not extend")
		}
		mustWrite(t, db, func(s app.Store) error { return s.Sessions().Delete(ctx, hash("3")) })
		mustWrite(t, db, func(s app.Store) (err error) {
			ok, err = s.Sessions().Extend(ctx, hash("3"), t0.Add(time.Hour), t0.Add(48*time.Hour))
			return
		})
		if ok {
			t.Error("deleted session must not extend")
		}

		mustRead(t, db, func(s app.Store) error {
			got, err := s.Sessions().ByTokenHash(ctx, hash("1"))
			if err != nil || !got.ExpiresAt.Equal(t0.Add(48*time.Hour)) || !got.LastSeenAt.Equal(t0.Add(2*time.Hour)) {
				t.Errorf("after extend: %+v %v", got, err)
			}
			return nil
		})

		mustWrite(t, db, func(s app.Store) error { return s.Sessions().DeleteOthers(ctx, domain.UserID(id("U1")), hash("1")) })
		mustRead(t, db, func(s app.Store) error {
			if _, err := s.Sessions().ByTokenHash(ctx, hash("2")); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("other session must be gone: %v", err)
			}
			if _, err := s.Sessions().ByTokenHash(ctx, hash("4")); err != nil {
				t.Errorf("another user's session must stay: %v", err)
			}
			return nil
		})
		mustWrite(t, db, func(s app.Store) error { return s.Sessions().DeleteAllForUser(ctx, domain.UserID(id("U1"))) })
		mustRead(t, db, func(s app.Store) error {
			if _, err := s.Sessions().ByTokenHash(ctx, hash("1")); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("all sessions must be gone: %v", err)
			}
			return nil
		})
	})
}

// IT-A-014: concurrent failures are never lost; windows and locks behave.
func TestThrottleCounters(t *testing.T) {
	rule := domain.ThrottleRules[domain.ThrottleIdentifierIP] // 5 per 15 min, 15 min lock
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := db.Write(ctx, func(s app.Store) error { return s.AuthThrottle().RecordFailure(ctx, "k", rule, t0) }); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		var failures int
		var locked sqlstore.NullTime
		if err := db.Read(ctx, func(s app.Store) error {
			return sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT failures, locked_until FROM auth_throttle WHERE key = 'k'").Scan(&failures, &locked)
		}); err != nil {
			t.Fatal(err)
		}
		if failures != 10 || !locked.Valid || !locked.Time.Equal(t0.Add(rule.Lock)) {
			t.Errorf("failures=%d locked=%+v", failures, locked)
		}

		lockedUntil := func(at time.Time) time.Time {
			var u time.Time
			mustRead(t, db, func(s app.Store) (err error) {
				u, err = s.AuthThrottle().LockedUntil(ctx, []string{"k", "other"}, at)
				return
			})
			return u
		}
		if got := lockedUntil(t0.Add(time.Minute)); !got.Equal(t0.Add(rule.Lock)) {
			t.Errorf("active lock: %v", got)
		}
		if got := lockedUntil(t0.Add(rule.Lock + time.Second)); !got.IsZero() {
			t.Errorf("ended lock must be ignored: %v", got)
		}

		// After the window (and the lock) ended, counting restarts at 1 without a lock.
		if err := db.Write(ctx, func(s app.Store) error {
			return s.AuthThrottle().RecordFailure(ctx, "k", rule, t0.Add(16*time.Minute))
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.Read(ctx, func(s app.Store) error {
			return sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT failures, locked_until FROM auth_throttle WHERE key = 'k'").Scan(&failures, &locked)
		}); err != nil {
			t.Fatal(err)
		}
		if failures != 1 || locked.Valid {
			t.Errorf("new window must restart at 1 without a lock, got %d %+v", failures, locked)
		}
		if err := db.Write(ctx, func(s app.Store) error { return s.AuthThrottle().Delete(ctx, "k") }); err != nil {
			t.Fatal(err)
		}
		if got := lockedUntil(t0); !got.IsZero() {
			t.Errorf("deleted counter still locks: %v", got)
		}
	})
}
