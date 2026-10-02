// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// cenv is a set-up church with every use case wired on one database.
type cenv struct {
	t        *testing.T
	db       *sqlstore.DB
	clock    *clock
	ids      app.IDGenerator
	setup    *app.Setup
	churches *app.Churches
	account  *app.Account
	auth     *app.Auth
	church   domain.ChurchID
	ctx      context.Context // carries the tenant
	admin    *domain.Session
}

func wireChurch(t *testing.T, db *sqlstore.DB) cenv {
	t.Helper()
	c := &clock{now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	ids := ulidgen.New()
	h := argon2pw.New(fast)
	auth := &app.Auth{Tx: db, Hasher: h, Clock: c, SessionTTL: 90 * 24 * time.Hour, SessionMaxAge: 365 * 24 * time.Hour}
	return cenv{t: t, db: db, clock: c, ids: ids, auth: auth,
		setup:    &app.Setup{Tx: db, Hasher: h, Clock: c, IDs: ids, Auth: auth},
		churches: &app.Churches{Tx: db, Clock: c},
		account:  &app.Account{Tx: db, Hasher: h, Clock: c, Auth: auth},
		ctx:      ctx,
	}
}

// newChurch sets up "GKY Citragarden" with admin@example.org as Church admin.
func newChurch(t *testing.T, db *sqlstore.DB) cenv {
	t.Helper()
	e := wireChurch(t, db)
	res, err := e.setup.Run(ctx, app.SetupInput{ViaCLI: true, Church: churchInput(), AdminName: "Admin",
		AdminIdentifier: "admin@example.org", AdminPassword: password})
	if err != nil {
		t.Fatal(err)
	}
	e.church = res.Church.ID
	e.ctx = app.WithTenant(ctx, e.church)
	e.admin = &domain.Session{UserID: res.User.ID}
	return e
}

func churchInput() app.ChurchInput {
	return app.ChurchInput{Name: "GKY Citragarden", DefaultUILanguage: "en", DefaultLanguage: "id",
		DefaultTranslationCode: "TB", TimeZone: "Asia/Jakarta", KeyDisplay: "do"}
}

func (e cenv) write(fn func(s app.Store) error) {
	e.t.Helper()
	if err := e.db.Write(ctx, fn); err != nil {
		e.t.Fatal(err)
	}
}

// member creates a user with a membership holding roles; returns its session and membership.
func (e cenv) member(email string, roles ...domain.RoleID) (*domain.Session, domain.MembershipID) {
	e.t.Helper()
	uid := domain.UserID(e.ids.NewID())
	mid := domain.MembershipID(e.ids.NewID())
	now := e.clock.Now()
	e.write(func(s app.Store) error {
		if err := s.Users().Create(ctx, domain.User{ID: uid, Name: strings.Split(email, "@")[0], Email: email,
			PasswordHash: "x", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		cs, err := s.ForChurch(ctx, e.church)
		if err != nil {
			return err
		}
		return cs.Memberships().Create(ctx, domain.Membership{ID: mid, UserID: uid, RoleIDs: roles, CreatedAt: now})
	})
	return &domain.Session{UserID: uid}, mid
}

func isNotFound(err error, reason string) bool {
	var nf *app.NotFoundError
	return errors.As(err, &nf) && nf.Reason == reason
}

func tokenReason(err error) domain.TokenReason {
	var e *app.InvalidTokenError
	if errors.As(err, &e) {
		return e.Reason
	}
	return "-"
}

// IT-A-001 (use case): tokens, ready-made roles, refusal once set up.
func TestSetup(t *testing.T) {
	db := sqlstoretest.NewSQLite(t)
	e := wireChurch(t, db)

	if done, err := e.setup.Status(ctx); err != nil || done {
		t.Fatalf("status before setup: %v %v", done, err)
	}
	first, _, err := e.setup.IssueToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	token, expires, err := e.setup.IssueToken(ctx) // the newest link is the only valid one
	if err != nil || !expires.Equal(e.clock.Now().Add(24*time.Hour)) {
		t.Fatalf("second token: %v %v", expires, err)
	}
	in := app.SetupInput{Token: first, Church: churchInput(), AdminName: "Admin", AdminIdentifier: "0812-3456-7890", AdminPassword: password}
	if _, err := e.setup.Run(ctx, in); tokenReason(err) != domain.TokenUnknown {
		t.Errorf("replaced token: %v", err)
	}
	bad := in
	bad.Church.DefaultTranslationCode = "XYZ"
	bad.Token = token
	var invalid *domain.InvalidInputError
	if _, err := e.setup.Run(ctx, bad); !errors.As(err, &invalid) || invalid.Field != "church.default_translation_code" {
		t.Errorf("unknown translation: %v", err)
	}
	bad = in
	bad.AdminPassword = "gkycitragarden"
	var weak *domain.WeakPasswordError
	if _, err := e.setup.Run(ctx, bad); !errors.As(err, &weak) || weak.Reason != domain.PasswordMatchesIdentity {
		t.Errorf("church name as password: %v", err)
	}

	e.clock.add(25 * time.Hour)
	in.Token = token
	if _, err := e.setup.Run(ctx, in); tokenReason(err) != domain.TokenUnknown {
		t.Errorf("25-hour-old token: %v", err)
	}
	token, _, _ = e.setup.IssueToken(ctx)
	in.Token, in.UserAgent = token, "test"
	res, err := e.setup.Run(ctx, in)
	if err != nil || res.Token == "" || res.Session.UserID != res.User.ID || res.User.Phone != "+6281234567890" {
		t.Fatalf("setup: %+v %v", res, err)
	}
	if done, _ := e.setup.Status(ctx); !done {
		t.Error("status after setup")
	}
	e.church, e.ctx, e.admin = res.Church.ID, app.WithTenant(ctx, res.Church.ID), &domain.Session{UserID: res.User.ID}
	var roles []domain.Role
	e.write(func(s app.Store) error {
		cs, err := s.ForChurch(ctx, e.church)
		if err != nil {
			return err
		}
		roles, err = cs.Roles().List(ctx)
		return err
	})
	if len(roles) != 3 {
		t.Fatalf("roles: %+v", roles)
	}
	for _, r := range roles {
		rm, _ := domain.ReadyMade(r.Origin)
		if r.Name != rm.Name("en") || len(r.Scopes) != len(rm.Scopes) {
			t.Errorf("ready-made role %+v", r)
		}
	}
	me, err := e.account.MeView(e.ctx, e.admin)
	if err != nil || me.Church == nil || me.Membership == nil || !me.Membership.Scopes.CanAdminister() ||
		me.Church.TranslationCode != "TB" {
		t.Errorf("me after setup: %+v %v", me, err)
	}
	if _, err := e.setup.Run(ctx, in); !errors.Is(err, app.ErrAlreadySetUp) {
		t.Errorf("second setup: %v", err)
	}
	if _, _, err := e.setup.IssueToken(ctx); !errors.Is(err, app.ErrAlreadySetUp) {
		t.Errorf("token after setup: %v", err)
	}
	cli := in
	cli.ViaCLI, cli.Token = true, ""
	if _, err := e.setup.Run(ctx, cli); !errors.Is(err, app.ErrAlreadySetUp) {
		t.Errorf("CLI setup after web setup: %v", err)
	}
}

// IT-T-001, IT-T-002 (use case): not a member → 404 not_member; missing scope → 403.
func TestMembershipAndScopeChecks(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t))
	outsider := &domain.Session{UserID: domain.UserID(e.ids.NewID())}
	team, _ := e.member("team@example.org")

	if _, err := e.churches.Get(e.ctx, outsider); !isNotFound(err, app.ReasonNotMember) {
		t.Errorf("outsider: %v", err)
	}
	if _, err := e.churches.Get(e.ctx, nil); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no session: %v", err)
	}
	if _, err := e.churches.Get(ctx, team); !errors.Is(err, app.ErrNotSetUp) {
		t.Errorf("no tenant: %v", err)
	}
	c, err := e.churches.Get(e.ctx, team)
	if err != nil || c.Actions.Edit {
		t.Errorf("baseline church view: %+v %v", c, err)
	}
	name := "X"
	if _, err := e.churches.Update(e.ctx, team, app.ChurchChange{Name: &name}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("update church by team member: %v", err)
	}

	// Church settings: PATCH semantics.
	url, empty, contact := "https://forms.example.org/f", "", "office@example.org"
	upd, err := e.churches.Update(e.ctx, e.admin, app.ChurchChange{FeedbackURL: &url, PrivacyContact: &contact})
	if err != nil || upd.Church.Settings.FeedbackURL != url || upd.Church.Name != "GKY Citragarden" || !upd.Actions.Edit {
		t.Errorf("update: %+v %v", upd, err)
	}
	upd, err = e.churches.Update(e.ctx, e.admin, app.ChurchChange{FeedbackURL: &empty})
	if err != nil || upd.Church.Settings.FeedbackURL != "" || upd.Church.Settings.PrivacyContact != contact {
		t.Errorf("clear feedback URL: %+v %v", upd, err)
	}
	httpURL, tz := "http://insecure.example.org", "Mars/Olympus"
	var invalid *domain.InvalidInputError
	if _, err := e.churches.Update(e.ctx, e.admin, app.ChurchChange{FeedbackURL: &httpURL}); !errors.As(err, &invalid) {
		t.Errorf("http feedback URL: %v", err)
	}
	if _, err := e.churches.Update(e.ctx, e.admin, app.ChurchChange{TimeZone: &tz}); !errors.As(err, &invalid) {
		t.Errorf("bad time zone: %v", err)
	}
	if _, err := e.churches.Update(e.ctx, e.admin, app.ChurchChange{Name: &empty}); !errors.As(err, &invalid) {
		t.Errorf("empty name: %v", err)
	}
}
