// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/entitlements/unlimited"
	"github.com/brightfellow-net/liturgist/adapters/importers/chordpro"
	"github.com/brightfellow-net/liturgist/adapters/importers/openlyrics"
	"github.com/brightfellow-net/liturgist/adapters/importers/paste"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// limitStub is an Entitlements with a team-member limit (0 = unlimited).
type limitStub struct{ max int }

func (limitStub) Has(context.Context, domain.ChurchID, app.Feature) (bool, error) { return true, nil }
func (l limitStub) Limit(context.Context, domain.ChurchID, app.LimitName) (app.Limit, error) {
	if l.max == 0 {
		return app.Limit{Unlimited: true}, nil
	}
	return app.Limit{Max: l.max}, nil
}

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
	invites  *app.Invites
	resets   *app.Resets
	songs    *app.Songs
	usage    *usageStub
	readings *app.Readings
	readUse  *readingUsageStub
	vocab    *app.Vocabulary
	tpls     *app.Templates
	svcs     *app.Services
	seed     *app.Seed
	planUse  *planUsageStub
	imports  *app.Imports
	operator *app.Operator
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
	urls := tenancy.NoPrefix{BaseURL: mustURL("https://liturgi.example.org")}
	if ent == nil {
		ent = unlimited.Entitlements{}
	}
	usage := &usageStub{}
	readUse := &readingUsageStub{}
	planUse := &planUsageStub{}
	songs := &app.Songs{Tx: db, Clock: c, IDs: ids, Usage: usage}
	return cenv{t: t, db: db, clock: c, ids: ids, auth: auth, usage: usage, readUse: readUse,
		songs: songs,
		imports: &app.Imports{Tx: db, Clock: c, IDs: ids, Songs: songs, Importers: map[domain.ImportFormat]app.Importer{
			domain.FormatPaste: paste.Importer{}, domain.FormatChordPro: chordpro.Importer{}, domain.FormatOpenLyrics: openlyrics.Importer{}}},
		readings: &app.Readings{Tx: db, Clock: c, IDs: ids, Usage: readUse},
		planUse:  planUse,
		vocab:    &app.Vocabulary{Tx: db, Clock: c, IDs: ids, Usage: planUse},
		tpls:     &app.Templates{Tx: db, Clock: c, IDs: ids},
		svcs:     &app.Services{Tx: db, Clock: c, IDs: ids},
		seed:     &app.Seed{Tx: db, Clock: c, IDs: ids},
		setup:    &app.Setup{Tx: db, Hasher: h, Clock: c, IDs: ids, Auth: auth},
		churches: &app.Churches{Tx: db, Clock: c},
		members:  &app.Members{Tx: db, Clock: c, Entitlements: ent},
		roles:    &app.Roles{Tx: db, Clock: c, IDs: ids},
		invites:  &app.Invites{Tx: db, Hasher: h, Clock: c, IDs: ids, URLs: urls, Entitlements: ent, Auth: auth},
		resets:   &app.Resets{Tx: db, Hasher: h, Clock: c, IDs: ids, URLs: urls, Auth: auth},
		operator: &app.Operator{Tx: db, Clock: c, IDs: ids},
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

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
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

// limitAdmin takes the liturgy and library scopes away from the Church admin
// role, so escalation can be tested with an actor who lacks some scopes.
func (e cenv) limitAdmin() {
	e.t.Helper()
	admin := e.role(domain.OriginChurchAdmin)
	limited := []domain.Scope{domain.ScopeChurchSettings, domain.ScopeMembersView, domain.ScopeMembersManage,
		domain.ScopeRolesManage, domain.ScopeTemplatesEdit, domain.ScopeLiturgyManage}
	if _, err := e.roles.Update(e.ctx, e.admin, admin.ID, app.RoleChange{Scopes: &limited}); err != nil {
		e.t.Fatal(err)
	}
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
		"create invite": func() error {
			_, _, err := e.invites.Create(e.ctx, team, app.InviteInput{Name: "A", Email: "a@example.org"})
			return err
		}(),
		"create role": func() error { _, err := e.roles.Create(e.ctx, team, app.RoleInput{Name: "R"}); return err }(),
		"list roles":  func() error { _, err := e.roles.List(e.ctx, team); return err }(),
		"list scopes": func() error { _, err := e.roles.Scopes(e.ctx, team); return err }(),
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
		if m.Actions != (app.MemberActions{}) || m.LastReset != nil {
			t.Errorf("members.view only must have no actions: %+v", m)
		}
	}
	if _, _, err := e.invites.Create(e.ctx, v, app.InviteInput{Name: "A", Email: "a@example.org"}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("viewer invites: %v", err)
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
	e.limitAdmin()
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
	e.limitAdmin()
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
	if a := actions[adminID]; a.Remove || !a.EditRoles || !a.CreateResetLink {
		t.Errorf("own row as the only admin: %+v", a)
	}
	if a := actions[teamID]; !a.Remove || !a.EditRoles || !a.CreateResetLink {
		t.Errorf("team member row: %+v", a)
	}
	if a := actions[litID]; a.Remove || a.EditRoles || a.CreateResetLink {
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

// The ready-made Church admin holds every scope, so it can invite with and
// assign every ready-made role.
func TestAdminGivesEveryRole(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	_, teamID := e.member("team@example.org")
	for i, rm := range domain.ReadyMadeRoles {
		r := e.role(rm.Origin)
		if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "N", Email: fmt.Sprintf("n%d@example.org", i),
			RoleIDs: []domain.RoleID{r.ID}}); err != nil {
			t.Errorf("invite as %s: %v", rm.Origin, err)
		}
		if _, err := e.members.SetRoles(e.ctx, e.admin, teamID, []domain.RoleID{r.ID}); err != nil {
			t.Errorf("assign %s: %v", rm.Origin, err)
		}
	}
}

// Deleting a role held by members needs its scopes and removes it from them.
func TestDeleteRole(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	e.limitAdmin()
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

// IT-A-004: invite a new user, with corrections at acceptance.
func TestInviteNewUser(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	e.limitAdmin()
	editor := e.role(domain.OriginEditor)
	if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Sari", Phone: "0812-0000-0000", RoleIDs: []domain.RoleID{editor.ID}}); !scopeNotHeld(err, domain.ScopeLibraryEdit, domain.ScopeLiturgyComment, domain.ScopeLiturgyEdit) {
		t.Fatalf("inviting with a role whose scopes aren't held: %v", err)
	}
	v, link, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: " Sari ", Phone: "0812-0000-0000"})
	if err != nil || v.Invite.Phone != "+6281200000000" || v.Invite.Name != "Sari" || v.Status != domain.InvitePending ||
		v.CreatedByName == nil || *v.CreatedByName != "Admin" || !strings.HasPrefix(link.Link, "https://liturgi.example.org/invite#t=") {
		t.Fatalf("create: %+v %+v %v", v, link, err)
	}
	if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Sari", Phone: "+62 812 0000 0000"}); !errors.Is(err, app.ErrInviteExists) {
		t.Errorf("second open invite: %v", err)
	}
	if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Self", Email: "ADMIN@example.org"}); !errors.Is(err, app.ErrAlreadyMember) {
		t.Errorf("inviting a member: %v", err)
	}
	token := strings.TrimPrefix(link.Link, "https://liturgi.example.org/invite#t=")

	info, err := e.invites.Inspect(ctx, token)
	if err != nil || info.OwnerExists || info.Phone != "+6281200000000" || info.ChurchName != "GKY Citragarden" {
		t.Errorf("inspect: %+v %v", info, err)
	}
	// The admin mistyped the phone; Sari corrects it and adds an email.
	in := app.AcceptInput{Token: token, Name: "Sari W", Phone: "0813-1111-2222", Email: "sari@example.org", Password: password, UserAgent: "test"}
	bad := in
	bad.Email = "admin@example.org"
	if _, err := e.invites.Accept(ctx, bad); !errors.Is(err, app.ErrIdentifierTaken) {
		t.Errorf("corrected email of another user: %v", err)
	}
	res, err := e.invites.Accept(ctx, in)
	if err != nil || res.Token == "" {
		t.Fatalf("accept: %v", err)
	}
	sari := &domain.Session{UserID: res.Session.UserID}
	me, err := e.account.MeView(e.ctx, sari)
	if err != nil || me.User.Phone != "+6281311112222" || me.User.Name != "Sari W" || me.Membership == nil || len(me.Membership.Roles) != 0 {
		t.Errorf("new member: %+v %v", me, err)
	}
	if _, err := e.invites.Accept(ctx, in); tokenReason(err) != domain.TokenUsed {
		t.Errorf("link reused: %v", err)
	}
	if _, err := e.invites.Inspect(ctx, "nonsense"); tokenReason(err) != domain.TokenUnknown {
		t.Errorf("unknown token: %v", err)
	}
	list, _ := e.invites.List(e.ctx, e.admin)
	if len(list) != 0 {
		t.Errorf("accepted invites are not listed: %+v", list)
	}
}

// IT-A-005: invite for an existing user.
func TestInviteExistingUser(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	now := e.clock.Now()
	u := domain.User{ID: domain.UserID(e.ids.NewID()), Name: "Udin", Phone: "+6285700000001", PasswordHash: "x", CreatedAt: now, UpdatedAt: now}
	vUser := domain.User{ID: domain.UserID(e.ids.NewID()), Name: "Vera", Email: "vera@example.org", PasswordHash: "x", CreatedAt: now, UpdatedAt: now}
	e.write(func(s app.Store) error {
		if err := s.Users().Create(ctx, u); err != nil {
			return err
		}
		return s.Users().Create(ctx, vUser)
	})
	_, link, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Udin", Phone: "+6285700000001"})
	if err != nil {
		t.Fatal(err)
	}
	token := link.Link[strings.Index(link.Link, "#t=")+3:]
	if info, _ := e.invites.Inspect(ctx, token); !info.OwnerExists {
		t.Error("owner_exists should be true")
	}
	if _, err := e.invites.AcceptExisting(ctx, &domain.Session{UserID: vUser.ID}, token); !errors.Is(err, app.ErrInviteMismatch) {
		t.Errorf("accepted by another account: %v", err)
	}
	if _, err := e.invites.Accept(ctx, app.AcceptInput{Token: token, Name: "Udin", Phone: "+6285700000009", Password: password}); !errors.Is(err, app.ErrIdentifierTaken) {
		t.Errorf("accept as new while an owner exists: %v", err)
	}
	if created, err := e.invites.AcceptExisting(ctx, &domain.Session{UserID: u.ID}, token); err != nil || !created {
		t.Errorf("owner accepts: %v %v", created, err)
	}
	if _, err := e.invites.AcceptExisting(ctx, nil, token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("without session: %v", err)
	}

	// An invite for an unused phone can be accepted by any logged-in user.
	_, link, _ = e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Vera", Phone: "+6285799999999"})
	token = link.Link[strings.Index(link.Link, "#t=")+3:]
	if created, err := e.invites.AcceptExisting(ctx, &domain.Session{UserID: vUser.ID}, token); err != nil || !created {
		t.Errorf("V accepts an unowned invite: %v %v", created, err)
	}
	// Already a member: idempotent.
	_, link, _ = e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Vera again", Phone: "+6285788888888"})
	token = link.Link[strings.Index(link.Link, "#t=")+3:]
	if created, err := e.invites.AcceptExisting(ctx, &domain.Session{UserID: vUser.ID}, token); err != nil || created {
		t.Errorf("already a member: %v %v", created, err)
	}
	// A tenant that differs from the invite's church → not found.
	_, link, _ = e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "W", Email: "w@example.org"})
	token = link.Link[strings.Index(link.Link, "#t=")+3:]
	if _, err := e.invites.Inspect(app.WithTenant(ctx, "01JOTHERCHURCH000000000000"), token); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("other tenant: %v", err)
	}
}

// IT-A-006, TC-A-008: limit, expiry, regenerate, cancel.
func TestInviteLifecycleAndLimit(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), limitStub{max: 2})
	_, link, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "A", Email: "a@example.org"})
	if err != nil {
		t.Fatal(err)
	}
	var lim *app.LimitReachedError
	if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "B", Email: "b@example.org"}); !errors.As(err, &lim) || lim.Used != 2 || lim.Max != 2 {
		t.Errorf("third team member: %v", err)
	}
	list, err := e.members.List(e.ctx, e.admin)
	if err != nil || list.Used != 2 || list.Max == nil || *list.Max != 2 {
		t.Errorf("usage: %+v %v", list, err)
	}
	invites, err := e.invites.List(e.ctx, e.admin)
	if err != nil || len(invites) != 1 || !invites[0].Actions.Cancel || !invites[0].Actions.Regenerate {
		t.Fatalf("list: %+v %v", invites, err)
	}
	id := invites[0].Invite.ID

	// Expired: no longer counts, can't be accepted, can be regenerated.
	e.clock.add(8 * 24 * time.Hour)
	old := link.Link[strings.Index(link.Link, "#t=")+3:]
	if _, err := e.invites.Inspect(ctx, old); tokenReason(err) != domain.TokenExpired {
		t.Errorf("expired: %v", err)
	}
	_, link2, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "B", Email: "b@example.org"})
	if err != nil {
		t.Fatalf("expired invites stop counting: %v", err)
	}
	if _, err := e.invites.Regenerate(e.ctx, e.admin, id); !errors.As(err, &lim) {
		t.Errorf("regenerating an expired invite re-checks the limit: %v", err)
	}
	if err := e.invites.Cancel(e.ctx, e.admin, invites[0].Invite.ID); err != nil {
		t.Errorf("cancel expired: %v", err)
	}
	if err := e.invites.Cancel(e.ctx, e.admin, invites[0].Invite.ID); !isNotFound(err, app.ReasonMissing) {
		t.Errorf("cancel twice: %v", err)
	}
	if _, err := e.invites.Inspect(ctx, old); tokenReason(err) != domain.TokenCancelled {
		t.Errorf("cancelled: %v", err)
	}
	// Re-inviting an address whose invite expired cancels the expired one.
	e.clock.add(8 * 24 * time.Hour)
	if _, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "B", Email: "b@example.org"}); err != nil {
		t.Errorf("re-invite after expiry: %v", err)
	}
	if _, err := e.invites.Inspect(ctx, link2.Link[strings.Index(link2.Link, "#t=")+3:]); tokenReason(err) != domain.TokenCancelled {
		t.Errorf("expired invite replaced: %v", err)
	}
	pending, _ := e.invites.List(e.ctx, e.admin)
	newLink, err := e.invites.Regenerate(e.ctx, e.admin, pending[0].Invite.ID)
	if err != nil || !newLink.ExpiresAt.Equal(e.clock.Now().Add(domain.InviteLifetime)) {
		t.Errorf("regenerate pending: %+v %v", newLink, err)
	}
	if _, err := e.invites.Inspect(ctx, newLink.Link[strings.Index(newLink.Link, "#t=")+3:]); err != nil {
		t.Errorf("new link works: %v", err)
	}
}

// IT-A-008: admin-created reset links.
func TestAdminReset(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	e.limitAdmin()
	m, mid := e.member("m@example.org")
	_, litID := e.member("lit@example.org", e.role(domain.OriginLiturgist).ID)

	first, err := e.resets.CreateForMember(e.ctx, e.admin, mid)
	if err != nil || !strings.HasPrefix(first.Link, "https://liturgi.example.org/reset#t=") {
		t.Fatalf("create: %+v %v", first, err)
	}
	second, err := e.resets.CreateForMember(e.ctx, e.admin, mid)
	if err != nil {
		t.Fatal(err)
	}
	tok := func(l app.ResetLink) string { return l.Link[strings.Index(l.Link, "#t=")+3:] }
	if _, err := e.resets.Inspect(ctx, tok(first)); tokenReason(err) != domain.TokenUsed {
		t.Errorf("second link cancels the first: %v", err)
	}
	info, err := e.resets.Inspect(ctx, tok(second))
	if err != nil || info.UserName != "m" || info.CreatedByName == nil || *info.CreatedByName != "Admin" {
		t.Errorf("inspect: %+v %v", info, err)
	}
	if _, err := e.resets.CreateForMember(e.ctx, e.admin, litID); !scopeNotHeld(err, domain.ScopeLiturgyApprove, domain.ScopeLiturgyComment, domain.ScopeLiturgyEdit) {
		t.Errorf("reset for a more powerful member: %v", err)
	}

	// m has two sessions; the reset ends both and starts a new one.
	e.write(func(s app.Store) error {
		for _, h := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
			if err := s.Sessions().Create(ctx, domain.Session{TokenHash: h, UserID: m.UserID, CreatedAt: e.clock.Now(),
				LastSeenAt: e.clock.Now(), ExpiresAt: e.clock.Now().Add(time.Hour)}); err != nil {
				return err
			}
		}
		return nil
	})
	var weak *domain.WeakPasswordError
	if _, err := e.resets.Use(ctx, app.ResetInput{Token: tok(second), NewPassword: "GKY Citragarden"}); !errors.As(err, &weak) { //nolint:gosec // test password
		t.Errorf("church name as new password: %v", err)
	}
	res, err := e.resets.Use(ctx, app.ResetInput{Token: tok(second), NewPassword: password})
	if err != nil || res.UserID != m.UserID || res.Token == "" {
		t.Fatalf("use: %+v %v", res, err)
	}
	if n := sqlstoretest.Count(t, e.db, "SELECT count(*) FROM sessions WHERE user_id = ?", m.UserID); n != 1 {
		t.Errorf("sessions after reset: %d, want only the new one", n)
	}
	if _, err := e.resets.Use(ctx, app.ResetInput{Token: tok(second), NewPassword: password}); tokenReason(err) != domain.TokenUsed {
		t.Errorf("link reused: %v", err)
	}
	if _, err := e.auth.Login(ctx, app.LoginInput{Identifier: "m@example.org", Password: password}); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	list, _ := e.members.List(e.ctx, e.admin)
	for _, mv := range list.Members {
		if mv.Member.ID == mid && (mv.LastReset == nil || mv.LastReset.UsedAt == nil || *mv.LastReset.CreatedByName != "Admin") {
			t.Errorf("last_reset: %+v", mv.LastReset)
		}
	}

	// A member of another church can't be reset by this church's admin.
	other := domain.ChurchID(e.ids.NewID())
	e.write(func(s app.Store) error {
		c := domain.Church{ID: other, Name: "Other", DefaultUILanguage: "en", DefaultLanguage: "en", DefaultTranslationID: "01M3XY2HBEKN8PETK6KK6A7NB2",
			TimeZone: "UTC", Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: e.clock.Now(), UpdatedAt: e.clock.Now()}
		if err := s.Churches().Create(ctx, c); err != nil {
			return err
		}
		cs, _ := s.ForChurch(ctx, other)
		return cs.Memberships().Create(ctx, domain.Membership{ID: domain.MembershipID(e.ids.NewID()), UserID: m.UserID, CreatedAt: e.clock.Now()})
	})
	if _, err := e.resets.CreateForMember(e.ctx, e.admin, mid); !errors.Is(err, app.ErrResetNotAllowed) {
		t.Errorf("member elsewhere: %v", err)
	}
	// The CLI has no such restriction.
	cli, err := e.resets.CreateForUser(ctx, "M@example.org")
	if err != nil || cli.User.ID != m.UserID {
		t.Errorf("CLI reset: %+v %v", cli, err)
	}
	info, _ = e.resets.Inspect(ctx, tok(cli))
	if info.CreatedByName != nil {
		t.Errorf("CLI links have no creator: %v", *info.CreatedByName)
	}
	if _, err := e.resets.CreateForUser(ctx, "nobody@example.org"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

// IT-A-012: operator recovery.
func TestGrantAdmin(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	m, _ := e.member("m@example.org")
	admin := e.role(domain.OriginChurchAdmin)

	if _, err := e.operator.GrantAdmin(ctx, e.church, "m@example.org"); err != nil {
		t.Fatal(err)
	}
	e.write(func(s app.Store) error { // simulate the role having been deleted
		cs, _ := s.ForChurch(ctx, e.church)
		return cs.Roles().Delete(ctx, admin.ID)
	})
	if _, err := e.operator.GrantAdmin(ctx, e.church, "m@example.org"); err != nil {
		t.Fatal(err)
	}
	me, err := e.account.MeView(e.ctx, m)
	if err != nil || !me.Membership.Scopes.CanAdminister() || len(me.Membership.Roles) != 1 || me.Membership.Roles[0].Origin != domain.OriginChurchAdmin {
		t.Errorf("after grant: %+v %v", me.Membership, err)
	}
	if _, err := e.operator.GrantAdmin(ctx, e.church, "nobody@example.org"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	now := e.clock.Now()
	e.write(func(s app.Store) error {
		return s.Users().Create(ctx, domain.User{ID: domain.UserID(e.ids.NewID()), Name: "Out", Email: "out@example.org", PasswordHash: "x", CreatedAt: now, UpdatedAt: now})
	})
	if _, err := e.operator.GrantAdmin(ctx, e.church, "out@example.org"); !errors.Is(err, app.ErrNotMember) {
		t.Errorf("not a member: %v", err)
	}
	lines, err := e.operator.ListUsers(ctx, e.church)
	if err != nil || len(lines) != 3 {
		t.Fatalf("list users: %+v %v", lines, err)
	}
	for _, l := range lines {
		if l.User.Email == "out@example.org" && l.Roles != nil {
			t.Errorf("non-member has roles: %+v", l)
		}
	}
}

// Changing one's own password rejects the church name (TC-A-003).
func TestChangePasswordChecksChurchName(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	var weak *domain.WeakPasswordError
	err := e.account.ChangePassword(ctx, &domain.Session{UserID: e.admin.UserID, TokenHash: strings.Repeat("c", 64)}, "203.0.113.5", password, "gky citragarden")
	if !errors.As(err, &weak) || weak.Reason != domain.PasswordMatchesIdentity {
		t.Errorf("church name: %v", err)
	}
}

// Cleanup removes expired rows only (03 §11).
func TestCleanup(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	cl := &app.Cleanup{Tx: e.db, Clock: e.clock}
	now := e.clock.Now()
	e.write(func(s app.Store) error {
		if err := s.Sessions().Create(ctx, domain.Session{TokenHash: strings.Repeat("a", 64), UserID: e.admin.UserID,
			CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			return err
		}
		for kind, rule := range domain.ThrottleRules {
			if err := s.AuthThrottle().RecordFailure(ctx, string(kind)+":k", rule, now); err != nil {
				return err
			}
		}
		return s.SetupTokens().Put(ctx, strings.Repeat("d", 64), now, now.Add(time.Hour))
	})
	if _, err := e.resets.CreateForUser(ctx, "admin@example.org"); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int { return sqlstoretest.Count(t, e.db, "SELECT count(*) FROM "+table) }
	e.clock.add(30 * time.Minute)
	if err := cl.Throttle(ctx); err != nil || count("auth_throttle") != 1 {
		t.Errorf("after 30 min only the 1-hour counter remains: %d %v", count("auth_throttle"), err)
	}
	if err := cl.Hourly(ctx); err != nil || count("sessions") != 1 || count("setup_tokens") != 1 || count("password_resets") != 1 {
		t.Errorf("nothing expired yet: %v", err)
	}
	e.clock.add(8 * 24 * time.Hour)
	if err := errors.Join(cl.Throttle(ctx), cl.Hourly(ctx)); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"auth_throttle", "sessions", "setup_tokens", "password_resets"} {
		if n := count(table); n != 0 {
			t.Errorf("%s: %d rows left", table, n)
		}
	}
}
