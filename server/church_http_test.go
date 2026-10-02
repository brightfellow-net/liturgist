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
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

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

// member adds a user with a membership holding roleIDs and logs them in
// (invites arrive in slice 4c); returns the user's cookie.
func (h harness) member(email string, roleIDs ...string) *http.Cookie {
	h.t.Helper()
	ids := ulidgen.New()
	h.user(ids.NewID(), email, "")
	ctx := context.Background()
	church, err := h.srv.single.ChurchID(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	err = h.srv.db.Write(ctx, func(s app.Store) error {
		u, err := s.Users().ByIdentifier(ctx, domain.Identifier{Kind: domain.Email, Value: email})
		if err != nil {
			return err
		}
		cs, err := s.ForChurch(ctx, church)
		if err != nil {
			return err
		}
		m := domain.Membership{ID: domain.MembershipID(ids.NewID()), UserID: u.ID, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
		for _, r := range roleIDs {
			m.RoleIDs = append(m.RoleIDs, domain.RoleID(r))
		}
		return cs.Memberships().Create(ctx, m)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return sessionCookie(h.t, h.login(email, testPassword))
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
		membership == nil || len(membership["scopes"].([]any)) != 6 {
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
	h.setupChurch()
	h.user("01JOUTSIDER000000000000000", "out@example.org", "")
	out := sessionCookie(t, h.login("out@example.org", testPassword))

	var first string
	for _, r := range []req{
		{method: "GET", path: "/api/v1/church"},
		{method: "PATCH", path: "/api/v1/church", body: map[string]any{"name": "X"}},
	} {
		r.cookies = []*http.Cookie{out}
		rec := h.do(r)
		if first == "" {
			first = rec.Body.String()
		}
		if rec.Code != http.StatusNotFound || rec.Body.String() != first {
			t.Errorf("%s %s: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	if !strings.Contains(h.log.String(), `"not_found_reason":"not_member"`) {
		t.Error("404 reasons must be logged")
	}
	me := decode(t, h.get("/api/v1/me", out))
	if me["membership"] != nil || me["church"] != nil {
		t.Errorf("me of a non-member: %v", me)
	}
}

// IT-T-002: a team member without church.settings gets 403 forbidden.
func TestScopesHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.member("team@example.org")
	rec := h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{team}, body: map[string]any{"name": "X"}})
	if rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
		t.Errorf("PATCH /church by team member: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/church", team); rec.Code != 200 || decode(t, rec)["actions"].(map[string]any)["edit"] != false {
		t.Errorf("baseline GET /church: %d %s", rec.Code, rec.Body.String())
	}
	me := decode(t, h.get("/api/v1/me", team))
	if m, _ := me["membership"].(map[string]any); m == nil || len(m["scopes"].([]any)) != 0 {
		t.Errorf("team member's /me: %v", me)
	}
	if me := decode(t, h.get("/api/v1/me", admin)); me["membership"].(map[string]any)["actions"].(map[string]any)["remove"] != false {
		t.Errorf("the only admin can't be removed: %v", me)
	}
}

// IT-T-008: every operation's tenancy; undeclared operations count as church.
func TestTenancyDeclarations(t *testing.T) {
	want := map[string]string{
		"login": "platform", "logout": "platform", "getMe": "optional", "updateMe": "platform", "changePassword": "platform",
		"endOtherSessions": "platform", "getSetupStatus": "platform", "setup": "platform", "listTranslations": "platform",
		"getChurch": "church", "updateChurch": "church",
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
	for _, path := range []string{"/api/v1/church"} {
		if rec := saas.do(req{method: "GET", path: path, cookies: []*http.Cookie{admin}, headers: map[string]string{"X-Test-Church": churchB}}); rec.Code != http.StatusNotFound {
			t.Errorf("%s of church B: %d", path, rec.Code)
		}
	}
}

// Church settings over HTTP.
func TestChurchSettingsHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	if rec := h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{admin},
		body: map[string]any{"feedback_url": nil, "unknown": 1}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown field: %d %s", rec.Code, rec.Body.String())
	}
	rec := h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{admin},
		body: map[string]any{"privacy_contact": "office@example.org", "default_translation_code": "KJV"}})
	if c := decode(t, rec); rec.Code != 200 || c["privacy_contact"] != "office@example.org" || c["default_translation_code"] != "KJV" || c["feedback_url"] != nil {
		t.Errorf("patch church: %d %s", rec.Code, rec.Body.String())
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
