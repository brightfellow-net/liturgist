// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/entitlements/unlimited"
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
	members  *app.Members
	roles    *app.Roles
	account  *app.Account
	auth     *app.Auth
	church   domain.ChurchID
	ctx      context.Context // carries the tenant
	admin    *domain.Session
}

func wireChurch(t *testing.T, db *sqlstore.DB, ent app.Entitlements) cenv {
	t.Helper()
	c := &clock{now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	ids := ulidgen.New()
	h := argon2pw.New(fast)
	auth := &app.Auth{Tx: db, Hasher: h, Clock: c, SessionTTL: 90 * 24 * time.Hour, SessionMaxAge: 365 * 24 * time.Hour}
	if ent == nil {
		ent = unlimited.Entitlements{}
	}
	return cenv{t: t, db: db, clock: c, ids: ids, auth: auth,
		setup:    &app.Setup{Tx: db, Hasher: h, Clock: c, IDs: ids, Auth: auth},
		churches: &app.Churches{Tx: db, Clock: c},
		members:  &app.Members{Tx: db, Clock: c, Entitlements: ent},
		roles:    &app.Roles{Tx: db, Clock: c, IDs: ids},
		account:  &app.Account{Tx: db, Hasher: h, Clock: c, Auth: auth},
		ctx:      ctx,
	}
}

// newChurch sets up "GKY Citragarden" with admin@example.org as Church admin.
func newChurch(t *testing.T, db *sqlstore.DB, ent app.Entitlements) cenv {
	t.Helper()
	e := wireChurch(t, db, ent)
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

// role returns the role with the given origin.
func (e cenv) role(o domain.RoleOrigin) domain.Role {
	e.t.Helper()
	var r domain.Role
	e.write(func(s app.Store) error {
		cs, err := s.ForChurch(ctx, e.church)
		if err != nil {
			return err
		}
		r, err = cs.Roles().ByOrigin(ctx, o)
		return err
	})
	return r
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

func (e cenv) membershipOf(sess *domain.Session) domain.MembershipID {
	e.t.Helper()
	var id domain.MembershipID
	e.write(func(s app.Store) error {
		cs, err := s.ForChurch(ctx, e.church)
		if err != nil {
			return err
		}
		m, err := cs.Memberships().ByUser(ctx, sess.UserID)
		id = m.ID
		return err
	})
	return id
}

func isNotFound(err error, reason string) bool {
	var nf *app.NotFoundError
	return errors.As(err, &nf) && nf.Reason == reason
}

func scopeNotHeld(err error, want ...domain.Scope) bool {
	var e *app.ScopeNotHeldError
	return errors.As(err, &e) && slices.Equal(e.Scopes, want)
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
	e := wireChurch(t, db, nil)

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
	roles, err := e.roles.List(e.ctx, e.admin)
	if err != nil || len(roles) != 3 {
		t.Fatalf("roles: %+v %v", roles, err)
	}
	for _, r := range roles {
		rm, _ := domain.ReadyMade(r.Role.Origin)
		if r.Role.Name != rm.Name("en") || len(r.Role.Scopes) != len(rm.Scopes) {
			t.Errorf("ready-made role %+v", r.Role)
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
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
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
	for op, err := range map[string]error{
		"update church": func() error { _, err := e.churches.Update(e.ctx, team, app.ChurchChange{Name: &name}); return err }(),
		"list members":  func() error { _, err := e.members.List(e.ctx, team); return err }(),
		"create role":   func() error { _, err := e.roles.Create(e.ctx, team, app.RoleInput{Name: "R"}); return err }(),
		"list roles":    func() error { _, err := e.roles.List(e.ctx, team); return err }(),
		"list scopes":   func() error { _, err := e.roles.Scopes(e.ctx, team); return err }(),
	} {
		if !errors.Is(err, app.ErrForbidden) {
			t.Errorf("%s by team member: %v", op, err)
		}
	}

	viewer, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "Viewer", Scopes: []domain.Scope{domain.ScopeMembersView}})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := e.member("viewer@example.org", viewer.Role.ID)
	list, err := e.members.List(e.ctx, v)
	if err != nil || len(list.Members) != 3 || list.Used != 3 || list.Max != nil {
		t.Fatalf("viewer lists members: %+v %v", list, err)
	}
	for _, m := range list.Members {
		if m.Actions != (app.MemberActions{}) {
			t.Errorf("members.view only must have no actions: %+v", m)
		}
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

// IT-T-007, TC-A-007, TC-A-009: role editor safeguards.
func TestRoleSafeguards(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	admin := e.role(domain.OriginChurchAdmin)
	lit := e.role(domain.OriginLiturgist)
	l, _ := e.member("lit@example.org", lit.ID)

	noRolesManage := []domain.Scope{domain.ScopeChurchSettings, domain.ScopeMembersView, domain.ScopeMembersManage}
	if _, err := e.roles.Update(e.ctx, e.admin, admin.ID, app.RoleChange{Scopes: &noRolesManage}); !errors.Is(err, app.ErrLockout) {
		t.Errorf("removing roles.manage from the only holder: %v", err)
	}
	if err := e.roles.Delete(e.ctx, e.admin, admin.ID); !errors.Is(err, app.ErrLockout) {
		t.Errorf("deleting the only admin role: %v", err)
	}
	name := "Mine"
	if _, err := e.roles.Update(e.ctx, l, lit.ID, app.RoleChange{Name: &name}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("liturgist edits roles: %v", err)
	}
	if _, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "Approver", Scopes: []domain.Scope{domain.ScopeLiturgyApprove}}); !scopeNotHeld(err, domain.ScopeLiturgyApprove) {
		t.Errorf("granting a scope not held: %v", err)
	}
	if _, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: " church ADMIN "}); !errors.Is(err, app.ErrRoleNameTaken) {
		t.Errorf("duplicate name: %v", err)
	}
	var invalid *domain.InvalidInputError
	if _, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "X", Scopes: []domain.Scope{"liturgy.fly"}}); !errors.As(err, &invalid) {
		t.Errorf("unknown scope: %v", err)
	}

	// Another member holding both scopes through a custom role lifts the lock-out.
	keeper, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "Keeper", Description: "Backup",
		Scopes: []domain.Scope{domain.ScopeRolesManage, domain.ScopeMembersManage}})
	if err != nil || keeper.MemberCount != 0 || !keeper.Actions.Delete {
		t.Fatalf("keeper: %+v %v", keeper, err)
	}
	e.member("keeper@example.org", keeper.Role.ID)
	updated, err := e.roles.Update(e.ctx, e.admin, admin.ID, app.RoleChange{Scopes: &noRolesManage})
	if err != nil || updated.Role.Scopes.Has(domain.ScopeRolesManage) || updated.Role.Origin != domain.OriginChurchAdmin || updated.MemberCount != 1 {
		t.Errorf("allowed with a second holder: %+v %v", updated, err)
	}
	// The admin lost roles.manage, so can no longer edit roles.
	if _, err := e.roles.List(e.ctx, e.admin); err != nil {
		t.Errorf("members.manage may still list roles: %v", err)
	}
	if _, err := e.roles.Update(e.ctx, e.admin, admin.ID, app.RoleChange{Name: &name}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("after losing roles.manage: %v", err)
	}
}

// TC-T-002, TC-A-009 and member safeguards.
func TestMemberSafeguardsAndActions(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	admin := e.role(domain.OriginChurchAdmin)
	lit := e.role(domain.OriginLiturgist)
	editor := e.role(domain.OriginEditor)
	_, teamID := e.member("team@example.org")
	_, litID := e.member("lit@example.org", lit.ID)
	adminID := e.membershipOf(e.admin)

	list, err := e.members.List(e.ctx, e.admin)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[domain.MembershipID]app.MemberActions{}
	for _, m := range list.Members {
		actions[m.Member.ID] = m.Actions
	}
	if a := actions[adminID]; a.Remove || !a.EditRoles {
		t.Errorf("own row as the only admin: %+v", a)
	}
	if a := actions[teamID]; !a.Remove || !a.EditRoles {
		t.Errorf("team member row: %+v", a)
	}
	if a := actions[litID]; a.Remove || a.EditRoles {
		t.Errorf("liturgist holds scopes the admin lacks: %+v", a)
	}

	if _, err := e.members.SetRoles(e.ctx, e.admin, teamID, []domain.RoleID{lit.ID}); !scopeNotHeld(err, domain.ScopeLiturgyApprove, domain.ScopeLiturgyComment, domain.ScopeLiturgyEdit) {
		t.Errorf("assigning Liturgist: %v", err)
	}
	if err := e.members.Remove(e.ctx, e.admin, litID); !scopeNotHeld(err, domain.ScopeLiturgyApprove, domain.ScopeLiturgyComment, domain.ScopeLiturgyEdit) {
		t.Errorf("removing someone more powerful: %v", err)
	}
	if err := e.members.Remove(e.ctx, e.admin, adminID); !errors.Is(err, app.ErrLockout) {
		t.Errorf("only admin leaves: %v", err)
	}
	if _, err := e.members.SetRoles(e.ctx, e.admin, adminID, nil); !errors.Is(err, app.ErrLockout) {
		t.Errorf("only admin drops own role: %v", err)
	}
	if _, err := e.members.SetRoles(e.ctx, e.admin, teamID, []domain.RoleID{"01JNOSUCHROLE0000000000000"}); err == nil {
		t.Error("unknown role accepted")
	}
	if _, err := e.members.SetRoles(e.ctx, e.admin, "01JNOSUCHMEMBER00000000000", nil); !isNotFound(err, app.ReasonMissing) {
		t.Errorf("unknown member: %v", err)
	}

	// Escalation is checked per changed role: give the team member Church admin.
	v, err := e.members.SetRoles(e.ctx, e.admin, teamID, []domain.RoleID{admin.ID, admin.ID})
	if err != nil || len(v.Roles) != 1 || !v.Scopes.CanAdminister() {
		t.Fatalf("make team member admin: %+v %v", v, err)
	}
	if err := e.members.Remove(e.ctx, e.admin, adminID); err != nil {
		t.Errorf("admin leaves when another admin exists: %v", err)
	}
	if _, err := e.churches.Get(e.ctx, e.admin); !isNotFound(err, app.ReasonNotMember) {
		t.Errorf("removed member: %v", err)
	}
	if me, err := e.account.MeView(e.ctx, e.admin); err != nil || me.Membership != nil || me.Church != nil {
		t.Errorf("removed member's /me: %+v %v", me, err)
	}

	// TC-A-010: effective scopes are the union.
	custom, _ := e.roles.Create(e.ctx, &domain.Session{UserID: v.Member.UserID}, app.RoleInput{Name: "Settings", Scopes: []domain.Scope{domain.ScopeChurchSettings}})
	s, _ := e.member("both@example.org", editor.ID, custom.Role.ID)
	me, err := e.account.MeView(e.ctx, s)
	want := domain.NewScopeSet(append(slices.Clone(domain.ReadyMadeRoles[2].Scopes), domain.ScopeChurchSettings)...)
	if err != nil || len(me.Membership.Scopes) != len(want) || len(me.Membership.Scopes.Missing(want)) != 0 {
		t.Errorf("union of scopes: %v %v", me.Membership.Scopes, err)
	}
}

// Deleting a role held by members needs its scopes and removes it from them.
func TestDeleteRole(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	editor := e.role(domain.OriginEditor)
	_, mid := e.member("ed@example.org", editor.ID)
	if err := e.roles.Delete(e.ctx, e.admin, editor.ID); !scopeNotHeld(err, domain.ScopeLibraryEdit, domain.ScopeLiturgyComment, domain.ScopeLiturgyEdit) {
		t.Errorf("delete a held role with scopes not held: %v", err)
	}
	e.write(func(s app.Store) error { // nobody holds it any more
		cs, _ := s.ForChurch(ctx, e.church)
		return cs.Memberships().SetRoles(ctx, mid, nil)
	})
	if err := e.roles.Delete(e.ctx, e.admin, editor.ID); err != nil {
		t.Errorf("delete an unheld role: %v", err)
	}
	if err := e.roles.Delete(e.ctx, e.admin, editor.ID); !isNotFound(err, app.ReasonMissing) {
		t.Errorf("delete again: %v", err)
	}
	scopes, err := e.roles.Scopes(e.ctx, e.admin)
	if err != nil || len(scopes) != 10 || scopes[0].Description != "Change church settings" {
		t.Errorf("scopes: %+v %v", scopes, err)
	}
}
