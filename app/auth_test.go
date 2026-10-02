// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func TestMain(m *testing.M) { os.Exit(sqlstoretest.Main(m)) }

var ctx = context.Background()

// clock is a settable app.Clock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

var fast = argon2pw.Params{Memory: 64, Time: 1, Threads: 1}

type env struct {
	auth  *app.Auth
	db    *sqlstore.DB
	clock *clock
	user  domain.User
}

const password = "kopi susu pagi hari" //nolint:gosec // test password

func newEnv(t *testing.T, hasher *argon2pw.Hasher) env {
	t.Helper()
	db := sqlstoretest.NewSQLite(t)
	c := &clock{now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	if hasher == nil {
		hasher = argon2pw.New(fast)
	}
	h, err := hasher.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	u := domain.User{ID: "01JUSER0000000000000000000", Name: "Budi", Email: "budi@example.org", Phone: "+6281234567890",
		PasswordHash: h, CreatedAt: c.now, UpdatedAt: c.now}
	if err := db.Write(ctx, func(s app.Store) error { return s.Users().Create(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	return env{
		auth: &app.Auth{Tx: db, Hasher: argon2pw.New(fast), Clock: c, SessionTTL: 90 * 24 * time.Hour, SessionMaxAge: 365 * 24 * time.Hour},
		db:   db, clock: c, user: u,
	}
}

func (e env) login(identifier, pw, addr string) (app.LoginResult, error) {
	return e.auth.Login(ctx, app.LoginInput{Identifier: identifier, Password: pw, ClientAddr: addr, UserAgent: "test"})
}

// IT-A-002
func TestLogin(t *testing.T) {
	e := newEnv(t, nil)

	res, err := e.login("0812-3456-7890", password, "203.0.113.5")
	if err != nil || len(res.Token) != 43 || res.Session.UserID != e.user.ID {
		t.Fatalf("login by phone: %+v %v", res, err)
	}
	if _, err := e.login(" BUDI@example.org ", password, "203.0.113.5"); err != nil {
		t.Errorf("login by email: %v", err)
	}

	for _, wrong := range []string{"wrong password!", ""} {
		if _, err := e.login("budi@example.org", wrong, "203.0.113.5"); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Errorf("wrong password: %v", err)
		}
	}
	if _, err := e.login("nobody@example.org", password, "203.0.113.5"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("unknown identifier must look the same: %v", err)
	}

	// 5 failures for one identifier from one IP lock it there, not elsewhere.
	e2 := newEnv(t, nil)
	for range 5 {
		_, _ = e2.login("budi@example.org", "wrong password!", "203.0.113.5")
	}
	var many *app.TooManyAttemptsError
	if _, err := e2.login("budi@example.org", password, "203.0.113.5"); !errors.As(err, &many) || many.RetryAfter != 15*time.Minute {
		t.Errorf("6th attempt must be throttled: %v", err)
	}
	if _, err := e2.login("budi@example.org", password, "198.51.100.7"); err != nil {
		t.Errorf("other IP must still work: %v", err)
	}
	e2.clock.add(15*time.Minute + time.Second)
	if _, err := e2.login("budi@example.org", password, "203.0.113.5"); err != nil {
		t.Errorf("lock must end after 15 min: %v", err)
	}
}

func TestLoginMalformedIdentifierIsCounted(t *testing.T) {
	e := newEnv(t, nil)
	for range 5 {
		if _, err := e.login("08xx-not-a-number", "whatever pw", "203.0.113.9"); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Fatalf("malformed: %v", err)
		}
	}
	var many *app.TooManyAttemptsError
	if _, err := e.login("08xx-not-a-number", "whatever pw", "203.0.113.9"); !errors.As(err, &many) {
		t.Errorf("malformed identifiers must be throttled too: %v", err)
	}
}

func TestLoginSuccessClearsIdentifierIPCounter(t *testing.T) {
	e := newEnv(t, nil)
	for range 4 {
		_, _ = e.login("budi@example.org", "wrong password!", "203.0.113.5")
	}
	if _, err := e.login("budi@example.org", password, "203.0.113.5"); err != nil {
		t.Fatal(err)
	}
	for range 4 { // would be the 5th..8th failure without the reset
		_, _ = e.login("budi@example.org", "wrong password!", "203.0.113.5")
	}
	if _, err := e.login("budi@example.org", password, "203.0.113.5"); err != nil {
		t.Errorf("counter was not cleared by the success: %v", err)
	}
}

func TestLoginReplacesPreviousSessionAndRehashes(t *testing.T) {
	old := argon2pw.New(argon2pw.Params{Memory: 32, Time: 1, Threads: 1})
	e := newEnv(t, old) // stored hash uses parameters the Auth hasher doesn't
	first, err := e.login("budi@example.org", password, "203.0.113.5")
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.auth.Login(ctx, app.LoginInput{Identifier: "budi@example.org", Password: password,
		ClientAddr: "203.0.113.5", PreviousTokenHash: first.Session.TokenHash})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.auth.Authenticate(ctx, first.Token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("previous session must be deleted: %v", err)
	}
	if _, _, err := e.auth.Authenticate(ctx, second.Token); err != nil {
		t.Errorf("new session: %v", err)
	}
	var stored domain.User
	_ = e.db.Read(ctx, func(s app.Store) (err error) { stored, err = s.Users().ByID(ctx, e.user.ID); return })
	if stored.PasswordHash == e.user.PasswordHash || stored.LastSeenAt.IsZero() {
		t.Errorf("hash must be upgraded and last_seen_at set: %+v", stored)
	}
}

// IT-A-003
func TestAuthenticate(t *testing.T) {
	e := newEnv(t, nil)
	res, _ := e.login("budi@example.org", password, "203.0.113.5")

	if s, refreshed, err := e.auth.Authenticate(ctx, res.Token); err != nil || refreshed || s.UserID != e.user.ID {
		t.Errorf("fresh session: %v %v", refreshed, err)
	}
	e.clock.add(2 * time.Hour)
	s, refreshed, err := e.auth.Authenticate(ctx, res.Token)
	if err != nil || !refreshed || !s.ExpiresAt.Equal(e.clock.Now().Add(90*24*time.Hour)) {
		t.Errorf("stale session must extend: %+v %v %v", s, refreshed, err)
	}
	if _, _, err := e.auth.Authenticate(ctx, "not-a-token"); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("unknown token: %v", err)
	}
	if _, _, err := e.auth.Authenticate(ctx, ""); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("empty token: %v", err)
	}
	e.clock.add(91 * 24 * time.Hour)
	if _, _, err := e.auth.Authenticate(ctx, res.Token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("expired session: %v", err)
	}
}

func TestAuthenticateCappedAtMaxAge(t *testing.T) {
	e := newEnv(t, nil)
	res, _ := e.login("budi@example.org", password, "203.0.113.5")
	created := e.clock.Now()
	var s domain.Session
	for range 6 { // used every ~60 days for a year
		e.clock.add(60 * 24 * time.Hour)
		var err error
		if s, _, err = e.auth.Authenticate(ctx, res.Token); err != nil {
			break
		}
	}
	if !s.ExpiresAt.Equal(created.Add(365 * 24 * time.Hour)) {
		t.Errorf("expiry %v must be capped at created + 1 year", s.ExpiresAt)
	}
	e.clock.add(60 * 24 * time.Hour)
	if _, _, err := e.auth.Authenticate(ctx, res.Token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("after one year: %v", err)
	}
}

// IT-A-011 (part)
func TestLogoutAndEndOthers(t *testing.T) {
	e := newEnv(t, nil)
	var tokens []app.LoginResult
	for range 3 {
		r, err := e.login("budi@example.org", password, "203.0.113.5")
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, r)
	}
	if err := e.auth.EndOtherSessions(ctx, e.user.ID, tokens[0].Session.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.auth.Authenticate(ctx, tokens[0].Token); err != nil {
		t.Errorf("current session must stay: %v", err)
	}
	for _, r := range tokens[1:] {
		if _, _, err := e.auth.Authenticate(ctx, r.Token); !errors.Is(err, app.ErrUnauthenticated) {
			t.Errorf("other session must end: %v", err)
		}
	}
	if err := e.auth.Logout(ctx, tokens[0].Session.TokenHash); err != nil {
		t.Fatal(err)
	}
	if err := e.auth.Logout(ctx, tokens[0].Session.TokenHash); err != nil {
		t.Errorf("logout must be idempotent: %v", err)
	}
	if _, _, err := e.auth.Authenticate(ctx, tokens[0].Token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("logged out: %v", err)
	}
}
