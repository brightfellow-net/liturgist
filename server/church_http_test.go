// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

const newPassword = "es jeruk sore hari" //nolint:gosec // test password

func harnessWith(t *testing.T, opts ...Option) harness {
	t.Helper()
	var buf bytes.Buffer
	srv, err := New(context.Background(), testConfig(t, "http://localhost:8080", &buf), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return harness{t: t, srv: srv, h: srv.Handler(), log: &buf}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("not JSON (%d): %q", rec.Code, rec.Body.String())
	}
	return v
}

// setupChurch runs the web setup with the link the server announces and
// returns the admin's cookie.
func (h harness) setupChurch() *http.Cookie {
	h.t.Helper()
	var notices bytes.Buffer
	h.srv.cfg.Notices = &notices
	if err := h.srv.announceSetup(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	link := notices.String()[strings.Index(notices.String(), "http://"):]
	link = link[:strings.Index(link, "\n")]
	if !strings.HasPrefix(link, "http://localhost:8080/setup#t=") {
		h.t.Fatalf("setup link %q", link)
	}
	rec := h.do(req{method: "POST", path: "/api/v1/setup", body: map[string]any{
		"token": strings.TrimPrefix(link, "http://localhost:8080/setup#t="),
		"church": map[string]string{"name": "GKY Citragarden", "default_ui_language": "en", "default_language": "id",
			"default_translation_code": "TB", "time_zone": "Asia/Jakarta", "key_display": "do"},
		"admin": map[string]string{"name": "Admin", "identifier": "admin@example.org", "password": testPassword},
	}})
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	return sessionCookie(h.t, rec)
}

// invite creates an invite as admin and accepts it as a new user; returns the new user's cookie.
func (h harness) invite(admin *http.Cookie, email string, roleIDs ...string) *http.Cookie {
	h.t.Helper()
	if roleIDs == nil {
		roleIDs = []string{}
	}
	rec := h.do(req{method: "POST", path: "/api/v1/invites", cookies: []*http.Cookie{admin},
		body: map[string]any{"name": "Member", "email": email, "role_ids": roleIDs}})
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("invite: %d %s", rec.Code, rec.Body.String())
	}
	link := decode(h.t, rec)["link"].(string)
	token := link[strings.Index(link, "#t=")+3:]
	rec = h.do(req{method: "POST", path: "/api/v1/invites/accept",
		body: map[string]any{"token": token, "name": "Member", "email": email, "password": testPassword}})
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("accept: %d %s", rec.Code, rec.Body.String())
	}
	return sessionCookie(h.t, rec)
}

func (h harness) get(path string, c *http.Cookie) *httptest.ResponseRecorder {
	return h.do(req{method: "GET", path: path, cookies: []*http.Cookie{c}})
}

// IT-A-001 and IT-T-003 over HTTP.
func TestSetupHTTP(t *testing.T) {
	h := harnessWith(t)
	if rec := h.do(req{method: "GET", path: "/api/v1/setup/status"}); rec.Code != 200 || decode(t, rec)["set_up"] != false {
		t.Errorf("status: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/church"}); rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "not_set_up" {
		t.Errorf("church before setup: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/translations"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":"TB"`) {
		t.Errorf("translations: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "GET", path: "/readyz"}); rec.Code != 200 {
		t.Errorf("ready before setup: %d", rec.Code)
	}
	admin := h.setupChurch()
	if !strings.Contains(h.log.String(), "/setup#t=") {
		t.Error("setup link must be in the log (accepted risk P-42)")
	}

	me := decode(t, h.get("/api/v1/me", admin))
	church, _ := me["church"].(map[string]any)
	membership, _ := me["membership"].(map[string]any)
	if church == nil || church["name"] != "GKY Citragarden" || church["default_translation_code"] != "TB" ||
		membership == nil || len(membership["scopes"].([]any)) != 10 {
		t.Errorf("me after setup: %v", me)
	}
	rec := h.do(req{method: "POST", path: "/api/v1/setup", body: map[string]any{"token": "x",
		"church": map[string]string{"name": "Again", "default_ui_language": "en", "default_language": "id",
			"default_translation_code": "TB", "time_zone": "Asia/Jakarta", "key_display": "do"},
		"admin": map[string]string{"name": "Admin", "identifier": "other@example.org", "password": testPassword}}})
	if rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "already_set_up" {
		t.Errorf("second setup: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/church", admin); rec.Code != 200 || decode(t, rec)["actions"].(map[string]any)["edit"] != true {
		t.Errorf("church: %d %s", rec.Code, rec.Body.String())
	}
}

// IT-T-001: a logged-in non-member gets byte-identical 404s everywhere.
func TestNotMemberHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	missing := h.do(req{method: "DELETE", path: "/api/v1/members/01JNOSUCHMEMBER00000000000", cookies: []*http.Cookie{admin}})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing member: %d %s", missing.Code, missing.Body.String())
	}
	h.user("01JOUTSIDER000000000000000", "out@example.org", "")
	out := sessionCookie(t, h.login("out@example.org", testPassword))

	for _, r := range []req{
		{method: "GET", path: "/api/v1/church"},
		{method: "PATCH", path: "/api/v1/church", body: map[string]any{"name": "X"}},
		{method: "GET", path: "/api/v1/members"},
		{method: "PATCH", path: "/api/v1/members/01JNOSUCHMEMBER00000000000", body: map[string]any{"role_ids": []string{}}},
		{method: "DELETE", path: "/api/v1/members/01JNOSUCHMEMBER00000000000"},
		{method: "POST", path: "/api/v1/members/01JNOSUCHMEMBER00000000000/password-reset"},
		{method: "GET", path: "/api/v1/roles"},
		{method: "POST", path: "/api/v1/roles", body: map[string]any{"name": "R", "scopes": []string{}}},
		{method: "PATCH", path: "/api/v1/roles/01JNOSUCHROLE0000000000000", body: map[string]any{}},
		{method: "DELETE", path: "/api/v1/roles/01JNOSUCHROLE0000000000000"},
		{method: "GET", path: "/api/v1/scopes"},
		{method: "GET", path: "/api/v1/invites"},
		{method: "POST", path: "/api/v1/invites", body: map[string]any{"name": "A", "email": "a@example.org"}},
		{method: "POST", path: "/api/v1/invites/01JNOSUCHINVITE00000000000/regenerate"},
		{method: "DELETE", path: "/api/v1/invites/01JNOSUCHINVITE00000000000"},
	} {
		r.cookies = []*http.Cookie{out}
		rec := h.do(r)
		if rec.Code != http.StatusNotFound || rec.Body.String() != missing.Body.String() {
			t.Errorf("%s %s: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	if !strings.Contains(h.log.String(), `"not_found_reason":"not_member"`) || !strings.Contains(h.log.String(), `"not_found_reason":"missing"`) {
		t.Error("404 reasons must be logged")
	}
	me := decode(t, h.get("/api/v1/me", out))
	if me["membership"] != nil || me["church"] != nil {
		t.Errorf("me of a non-member: %v", me)
	}
}

// IT-T-002, TC-T-001 (part), IT-T-004: missing scopes and removed members.
func TestScopesHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")

	for _, r := range []req{
		{method: "PATCH", path: "/api/v1/church", body: map[string]any{"name": "X"}},
		{method: "GET", path: "/api/v1/members"},
		{method: "POST", path: "/api/v1/invites", body: map[string]any{"name": "A", "email": "a@example.org"}},
		{method: "POST", path: "/api/v1/roles", body: map[string]any{"name": "R", "scopes": []string{}}},
	} {
		r.cookies = []*http.Cookie{team}
		if rec := h.do(r); rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s %s by team member: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	if rec := h.get("/api/v1/church", team); rec.Code != 200 {
		t.Errorf("baseline GET /church: %d", rec.Code)
	}

	// A custom role with only members.view.
	rec := h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin},
		body: map[string]any{"name": "Viewer", "scopes": []string{"members.view"}}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create role: %d %s", rec.Code, rec.Body.String())
	}
	viewerRole := decode(t, rec)["id"].(string)
	viewer := h.invite(admin, "viewer@example.org", viewerRole)
	if rec := h.get("/api/v1/members", viewer); rec.Code != 200 {
		t.Errorf("viewer lists members: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/invites", cookies: []*http.Cookie{viewer},
		body: map[string]any{"name": "A", "email": "a@example.org"}}); rec.Code != http.StatusForbidden {
		t.Errorf("viewer invites: %d", rec.Code)
	}
	// Escalation is 403 scope_not_held with the scopes: first take
	// liturgy.approve away from the admin's own role.
	var roles []map[string]any
	_ = json.Unmarshal(h.get("/api/v1/roles", admin).Body.Bytes(), &roles)
	var adminRole string
	for _, r := range roles {
		if r["origin"] == "church_admin" {
			adminRole = r["id"].(string)
		}
	}
	if rec := h.do(req{method: "PATCH", path: "/api/v1/roles/" + adminRole, cookies: []*http.Cookie{admin},
		body: map[string]any{"scopes": []string{"church.settings", "members.view", "members.manage", "roles.manage"}}}); rec.Code != 200 {
		t.Fatalf("limit admin role: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin},
		body: map[string]any{"name": "Approver", "scopes": []string{"liturgy.approve"}}})
	if p := problemCode(t, rec); rec.Code != http.StatusForbidden || p["code"] != "scope_not_held" || p["scopes"].([]any)[0] != "liturgy.approve" {
		t.Errorf("escalation: %d %s", rec.Code, rec.Body.String())
	}

	// IT-T-004: the removed member's next request is 404; the session still works for /me.
	members := decode(t, h.get("/api/v1/members", admin))["members"].([]any)
	var teamID string
	for _, m := range members {
		if mm := m.(map[string]any); mm["email"] == "team@example.org" {
			teamID = mm["id"].(string)
		}
	}
	rec = h.do(req{method: "PATCH", path: "/api/v1/members/" + teamID, cookies: []*http.Cookie{admin},
		body: map[string]any{"role_ids": []string{viewerRole}}})
	if rec.Code != 200 || len(decode(t, rec)["roles"].([]any)) != 1 {
		t.Errorf("assign roles: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "DELETE", path: "/api/v1/members/" + teamID, cookies: []*http.Cookie{admin}}); rec.Code != 204 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/church", team); rec.Code != http.StatusNotFound {
		t.Errorf("removed member: %d", rec.Code)
	}
	if rec := h.get("/api/v1/me", team); rec.Code != 200 || decode(t, rec)["membership"] != nil {
		t.Errorf("removed member's /me: %d %s", rec.Code, rec.Body.String())
	}
	// The admin can't remove themselves (lock-out).
	me := decode(t, h.get("/api/v1/me", admin))
	adminID := me["membership"].(map[string]any)["id"].(string)
	if rec := h.do(req{method: "DELETE", path: "/api/v1/members/" + adminID, cookies: []*http.Cookie{admin}}); rec.Code != http.StatusConflict ||
		problemCode(t, rec)["code"] != "lockout_prevented" {
		t.Errorf("only admin leaves: %d %s", rec.Code, rec.Body.String())
	}
}

// IT-A-008 over HTTP: reset link created by the admin, inspected and used.
func TestResetHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	member := h.invite(admin, "m@example.org")
	me := decode(t, h.get("/api/v1/me", member))
	mid := me["membership"].(map[string]any)["id"].(string)

	rec := h.do(req{method: "POST", path: "/api/v1/members/" + mid + "/password-reset", cookies: []*http.Cookie{admin}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	link := decode(t, rec)["link"].(string)
	token := link[strings.Index(link, "#t=")+3:]
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/reset/inspect", body: map[string]any{"token": token}}); rec.Code != 200 ||
		decode(t, rec)["created_by_name"] != "Admin" {
		t.Errorf("inspect: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(req{method: "POST", path: "/api/v1/auth/reset", body: map[string]any{"token": token, "new_password": newPassword}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("use: %d %s", rec.Code, rec.Body.String())
	}
	fresh := sessionCookie(t, rec)
	if h.get("/api/v1/me", member).Code != http.StatusUnauthorized || h.get("/api/v1/me", fresh).Code != 200 {
		t.Error("a reset ends all sessions and starts a new one")
	}
	rec = h.do(req{method: "POST", path: "/api/v1/auth/reset", body: map[string]any{"token": token, "new_password": newPassword}})
	if p := problemCode(t, rec); rec.Code != http.StatusBadRequest || p["code"] != "invalid_token" || p["reason"] != "used" {
		t.Errorf("reuse: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(h.log.String(), "reset_link_created") || !strings.Contains(h.log.String(), "reset_link_used") {
		t.Error("reset events must be logged")
	}
	members := decode(t, h.get("/api/v1/members", admin))["members"].([]any)
	for _, m := range members {
		mm := m.(map[string]any)
		if mm["id"] == mid && (mm["last_reset"] == nil || mm["last_reset"].(map[string]any)["used_at"] == nil) {
			t.Errorf("last_reset: %v", mm)
		}
	}
}

// IT-T-005: the team-member limit comes from Entitlements.
func TestLimitHTTP(t *testing.T) {
	h := harnessWith(t, WithEntitlements(limitOne{}))
	admin := h.setupChurch()
	usage := decode(t, h.get("/api/v1/members", admin))["usage"].(map[string]any)["team_members"].(map[string]any)
	if usage["used"] != 1.0 || usage["max"] != 1.0 {
		t.Errorf("usage: %v", usage)
	}
	rec := h.do(req{method: "POST", path: "/api/v1/invites", cookies: []*http.Cookie{admin}, body: map[string]any{"name": "A", "email": "a@example.org"}})
	if p := problemCode(t, rec); rec.Code != http.StatusForbidden || p["code"] != "limit_reached" || p["limit"] != "max_team_members" ||
		p["used"] != 1.0 || p["max"] != 1.0 {
		t.Errorf("limit: %d %s", rec.Code, rec.Body.String())
	}
}

type limitOne struct{}

func (limitOne) Has(context.Context, domain.ChurchID, app.Feature) (bool, error) { return true, nil }
func (limitOne) Limit(context.Context, domain.ChurchID, app.LimitName) (app.Limit, error) {
	return app.Limit{Max: 1}, nil
}

// IT-T-008: every operation's tenancy; undeclared operations count as church.
func TestTenancyDeclarations(t *testing.T) {
	want := map[string]string{
		"login": "platform", "logout": "platform", "getMe": "optional", "updateMe": "platform", "changePassword": "platform",
		"endOtherSessions": "platform", "getSetupStatus": "platform", "setup": "platform", "listTranslations": "platform",
		"inspectReset": "platform", "resetPassword": "platform",
		"inspectInvite": "optional", "acceptInvite": "optional", "acceptInviteExisting": "optional",
		"getChurch": "church", "updateChurch": "church", "listMembers": "church", "setMemberRoles": "church",
		"removeMember": "church", "createResetLink": "church", "listRoles": "church", "createRole": "church",
		"updateRole": "church", "deleteRole": "church", "listScopes": "church", "listInvites": "church",
		"createInvite": "church", "regenerateInvite": "church", "cancelInvite": "church",
		"listSongs": "church", "getSong": "church", "createSong": "church", "updateSong": "church", "deleteSong": "church",
		"linkSong": "church", "unlinkSong": "church",
		"parseReference": "church", "lookupReading": "church", "listReadings": "church", "getReading": "church",
		"createReading": "church", "createReadingFromProvider": "church", "updateReading": "church", "deleteReading": "church",
		"saasExtra": "church", // added through WithRoutes without a declaration
	}
	extra := WithRoutes(func(api huma.API) {
		huma.Register(api, huma.Operation{OperationID: "saasExtra", Method: http.MethodGet, Path: "/extra"},
			func(context.Context, *struct{}) (*struct{}, error) { return nil, nil })
	})
	var o options
	extra(&o)
	api := newAPI(nil, o, deps{})
	got := map[string]string{}
	for _, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Patch, item.Delete, item.Put} {
			if op != nil {
				got[op.OperationID] = httpapi.TenancyOf(op)
			}
		}
	}
	for id, tenancy := range want {
		if got[id] != tenancy {
			t.Errorf("%s: tenancy %q, want %q", id, got[id], tenancy)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("operation %s missing from the expected table", id)
		}
	}
	// And it is enforced: before setup the undeclared operation is refused.
	h := harnessWith(t, extra)
	if rec := h.do(req{method: "GET", path: "/api/v1/extra"}); rec.Code != http.StatusConflict {
		t.Errorf("undeclared operation before setup: %d", rec.Code)
	}
}

// headerResolver is a SaaS-style resolver: the church comes from the request.
type headerResolver struct{}

func (headerResolver) Resolve(r *http.Request) (domain.ChurchID, error) {
	if c := r.Header.Get("X-Test-Church"); c != "" {
		return domain.ChurchID(c), nil
	}
	return "", app.ErrNotSetUp
}

// IT-T-006: with two churches, a member of A gets 404 for B's endpoints.
func TestOtherTenantHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	me := decode(t, h.get("/api/v1/me", admin))
	churchA := me["church"].(map[string]any)["id"].(string)
	const churchB = "01JCHURCHB0000000000000000"
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := h.srv.db.Write(context.Background(), func(s app.Store) error {
		return s.Churches().Create(context.Background(), domain.Church{ID: churchB, Name: "B", DefaultUILanguage: "en",
			DefaultLanguage: "en", DefaultTranslationID: "01M3XY2HBEKN8PETK6KK6A7NB2", TimeZone: "UTC",
			Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	// The community server now refuses to start (exit 7).
	if _, err := New(context.Background(), h.srv.cfg); !IsTooManyChurches(err) {
		t.Errorf("start with two churches: %v", err)
	}
	saas := harness{t: t, srv: h.srv}
	srv, err := New(context.Background(), h.srv.cfg, WithTenantResolver(headerResolver{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	saas.h = srv.Handler()
	if rec := saas.do(req{method: "GET", path: "/api/v1/church", cookies: []*http.Cookie{admin}, headers: map[string]string{"X-Test-Church": churchA}}); rec.Code != 200 {
		t.Errorf("own church: %d", rec.Code)
	}
	for _, path := range []string{"/api/v1/church", "/api/v1/members", "/api/v1/roles"} {
		if rec := saas.do(req{method: "GET", path: path, cookies: []*http.Cookie{admin}, headers: map[string]string{"X-Test-Church": churchB}}); rec.Code != http.StatusNotFound {
			t.Errorf("%s of church B: %d", path, rec.Code)
		}
	}
}

// Invite inspection, existing-account acceptance and role editing over HTTP.
func TestInvitesAndRolesHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	list := h.get("/api/v1/roles", admin)
	var rs []map[string]any
	_ = json.Unmarshal(list.Body.Bytes(), &rs)
	if len(rs) != 3 {
		t.Fatalf("roles: %s", list.Body.String())
	}
	var adminRole map[string]any
	for _, r := range rs {
		if r["origin"] == "church_admin" {
			adminRole = r
		}
	}
	if adminRole["member_count"] != 1.0 || adminRole["actions"].(map[string]any)["delete"] != false {
		t.Errorf("admin role: %v", adminRole)
	}
	rec := h.do(req{method: "PATCH", path: "/api/v1/roles/" + adminRole["id"].(string), cookies: []*http.Cookie{admin},
		body: map[string]any{"scopes": []string{"members.view"}}})
	if rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "lockout_prevented" {
		t.Errorf("lock-out: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(req{method: "PATCH", path: "/api/v1/roles/" + adminRole["id"].(string), cookies: []*http.Cookie{admin},
		body: map[string]any{"name": "Pengurus"}})
	if rec.Code != 200 || decode(t, rec)["origin"] != "church_admin" {
		t.Errorf("rename keeps origin: %d %s", rec.Code, rec.Body.String())
	}

	h.user("01JEXISTING000000000000000", "", "+6285700000001")
	existing := sessionCookie(t, h.login("0857-0000-0001", testPassword))
	rec = h.do(req{method: "POST", path: "/api/v1/invites", cookies: []*http.Cookie{admin}, body: map[string]any{"name": "Udin", "phone": "0857-0000-0001"}})
	link := decode(t, rec)["link"].(string)
	token := link[strings.Index(link, "#t=")+3:]
	if rec := h.do(req{method: "POST", path: "/api/v1/invites/inspect", body: map[string]any{"token": token}}); rec.Code != 200 ||
		decode(t, rec)["owner_exists"] != true {
		t.Errorf("inspect: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/invites/accept-existing", cookies: []*http.Cookie{admin}, body: map[string]any{"token": token}}); rec.Code != http.StatusForbidden ||
		problemCode(t, rec)["code"] != "invite_identifier_mismatch" {
		t.Errorf("wrong account: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/invites/accept-existing", cookies: []*http.Cookie{existing}, body: map[string]any{"token": token}}); rec.Code != http.StatusCreated {
		t.Errorf("owner accepts: %d %s", rec.Code, rec.Body.String())
	}
	invites := h.get("/api/v1/invites", admin)
	if invites.Code != 200 || invites.Body.String() != "[]\n" && invites.Body.String() != "[]" {
		t.Errorf("open invites: %d %q", invites.Code, invites.Body.String())
	}
	if rec := h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{admin},
		body: map[string]any{"feedback_url": nil, "unknown": 1}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown field: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{admin},
		body: map[string]any{"privacy_contact": "office@example.org", "default_translation_code": "KJV"}})
	if c := decode(t, rec); rec.Code != 200 || c["privacy_contact"] != "office@example.org" || c["default_translation_code"] != "KJV" || c["feedback_url"] != nil {
		t.Errorf("patch church: %d %s", rec.Code, rec.Body.String())
	}
	scopes := []string{}
	var sc []map[string]any
	_ = json.Unmarshal(h.get("/api/v1/scopes", admin).Body.Bytes(), &sc)
	for _, s := range sc {
		scopes = append(scopes, s["scope"].(string))
	}
	if !slices.Contains(scopes, "liturgy.manage") || len(scopes) != 10 {
		t.Errorf("scopes: %v", scopes)
	}
}

// Start-up refusal and setup link handling (03 §10).
func TestAnnounceSetupOnlyBeforeSetup(t *testing.T) {
	h := harnessWith(t)
	h.setupChurch()
	var notices bytes.Buffer
	h.srv.cfg.Notices = &notices
	if err := h.srv.announceSetup(context.Background()); err != nil || notices.Len() != 0 {
		t.Errorf("after setup: %q %v", notices.String(), err)
	}
	if _, _, err := h.srv.uc.setup.IssueToken(context.Background()); !errors.Is(err, app.ErrAlreadySetUp) {
		t.Errorf("token after setup: %v", err)
	}
}
