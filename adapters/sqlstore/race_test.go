// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// barrier holds two transactions after their first read until both have
// read. When the second can't reach its read until the first ends (SQLite's
// single writer, or a lock taken first), the first goes on after a timeout:
// that waiting is itself the serialisation under test.
type barrier struct {
	arrived atomic.Int32
	all     chan struct{}
	once    sync.Once
}

const barrierTimeout = 300 * time.Millisecond

func (b *barrier) wait() {
	if b.arrived.Add(1) == 2 {
		b.once.Do(func() { close(b.all) })
	}
	select {
	case <-b.all:
	case <-time.After(barrierTimeout):
	}
}

// race runs fn in two write transactions at once and returns what each reported.
func race(t *testing.T, db *sqlstore.DB, fn func(i int, s app.Store, wait func()) (bool, error)) [2]bool {
	t.Helper()
	b := &barrier{all: make(chan struct{})}
	var (
		oks  [2]bool
		errs [2]error
		wg   sync.WaitGroup
	)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = db.Write(ctx, func(s app.Store) (err error) {
				oks[i], err = fn(i, s, b.wait)
				return err
			})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	return oks
}

func exactlyOne(t *testing.T, what string, oks [2]bool) {
	t.Helper()
	if oks[0] == oks[1] {
		t.Errorf("%s: want exactly one success, got %v", what, oks)
	}
}

func count(t *testing.T, db *sqlstore.DB, q string, args ...any) int {
	t.Helper()
	var n int
	mustRead(t, db, func(s app.Store) error {
		return sqlstore.RawTx(s).QueryRowContext(ctx, db.Dialect().Rebind(q), args...).Scan(&n)
	})
	return n
}

// IT-P-008: every pattern of 02 §2.1 under two concurrent transactions.
func TestRacePatterns(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		now := f.now

		t.Run("claim invite", func(t *testing.T) {
			exactlyOne(t, "invite", race(t, db, func(_ int, s app.Store, wait func()) (bool, error) {
				if _, err := s.InviteTokens().ByTokenHash(ctx, hash("1")); err != nil {
					return false, err
				}
				wait()
				_, ok, err := s.InviteTokens().Claim(ctx, hash("1"), "", now)
				return ok, err
			}))
		})
		t.Run("claim reset link", func(t *testing.T) {
			exactlyOne(t, "reset", race(t, db, func(_ int, s app.Store, wait func()) (bool, error) {
				if _, err := s.PasswordResets().ByTokenHash(ctx, hash("2")); err != nil {
					return false, err
				}
				wait()
				_, ok, err := s.PasswordResets().Claim(ctx, hash("2"), now)
				return ok, err
			}))
		})
		t.Run("claim setup token", func(t *testing.T) {
			exactlyOne(t, "setup token", race(t, db, func(_ int, s app.Store, wait func()) (bool, error) {
				if _, err := s.Churches().Count(ctx); err != nil {
					return false, err
				}
				wait()
				return s.SetupTokens().Claim(ctx, hash("4"), now)
			}))
		})

		t.Run("counter", func(t *testing.T) {
			const key = "ip:203.0.113.5" // one failure in the fixture
			race(t, db, func(_ int, s app.Store, wait func()) (bool, error) {
				if _, err := s.AuthThrottle().LockedUntil(ctx, []string{key}, now); err != nil {
					return false, err
				}
				wait()
				return true, s.AuthThrottle().RecordFailure(ctx, key, domain.ThrottleRules[domain.ThrottleIP], now)
			})
			if n := count(t, db, "SELECT failures FROM auth_throttle WHERE key = ?", key); n != 3 {
				t.Errorf("failures %d, want 3 (no lost update)", n)
			}
		})

		t.Run("conditional update", func(t *testing.T) {
			// Extension racing with logout: the session stays deleted.
			race(t, db, func(i int, s app.Store, wait func()) (bool, error) {
				if _, err := s.Sessions().ByTokenHash(ctx, hash("3")); err != nil {
					return false, err
				}
				wait()
				if i == 0 {
					return true, s.Sessions().Delete(ctx, hash("3"))
				}
				return s.Sessions().Extend(ctx, hash("3"), now, now.Add(2*time.Hour))
			})
			if n := count(t, db, "SELECT count(*) FROM sessions WHERE token_hash = ?", hash("3")); n != 0 {
				t.Error("an extension racing with logout brought the session back")
			}
		})

		t.Run("lock church", func(t *testing.T) {
			// Check-then-insert under LockChurch: only one membership is added.
			for _, u := range []string{"U2", "U3"} {
				if err := f.user(id(u), u+"@example.org", ""); err != nil {
					t.Fatal(err)
				}
			}
			exactlyOne(t, "membership", race(t, db, func(i int, s app.Store, wait func()) (bool, error) {
				cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
				if err != nil {
					return false, err
				}
				if err := cs.LockChurch(ctx); err != nil {
					return false, err
				}
				n, err := cs.Memberships().Count(ctx)
				if err != nil {
					return false, err
				}
				wait()
				if n != 1 {
					return false, nil // the other transaction added one
				}
				u := []string{"U2", "U3"}[i]
				return true, cs.Memberships().Create(ctx, domain.Membership{ID: domain.MembershipID(id("M" + u)),
					UserID: domain.UserID(id(u)), CreatedAt: now})
			}))
		})

		t.Run("lock user", func(t *testing.T) {
			// Two reset-link creations: one open link remains.
			race(t, db, func(i int, s app.Store, wait func()) (bool, error) {
				uid := domain.UserID(id("U1"))
				if err := s.LockUser(ctx, uid); err != nil {
					return false, err
				}
				if _, err := s.PasswordResets().Latest(ctx, []domain.UserID{uid}); err != nil {
					return false, err
				}
				wait()
				if err := s.PasswordResets().CloseOpen(ctx, uid, now); err != nil {
					return false, err
				}
				return true, s.PasswordResets().Create(ctx, domain.PasswordReset{ID: id([]string{"PX", "PY"}[i]), UserID: uid,
					TokenHash: hash([]string{"8", "9"}[i]), CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
			})
			if n := count(t, db, "SELECT count(*) FROM password_resets WHERE user_id = ? AND used_at IS NULL", id("U1")); n != 1 {
				t.Errorf("open reset links: %d", n)
			}
		})

		t.Run("lock install", func(t *testing.T) {
			// Check-then-create under LockInstall: only one church is created.
			exactlyOne(t, "church", race(t, db, func(i int, s app.Store, wait func()) (bool, error) {
				if err := s.LockInstall(ctx); err != nil {
					return false, err
				}
				n, err := s.Churches().Count(ctx)
				if err != nil {
					return false, err
				}
				wait()
				if n != 2 {
					return false, nil
				}
				return true, s.Churches().Create(ctx, domain.Church{ID: domain.ChurchID(id([]string{"CHX", "CHY"}[i])),
					Name: "New", DefaultUILanguage: "en", DefaultLanguage: "id", DefaultTranslationID: tb,
					TimeZone: "Asia/Jakarta", Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: now, UpdatedAt: now})
			}))
		})
	})
}
