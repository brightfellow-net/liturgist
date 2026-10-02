// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

const testPassword = "kopi susu pagi hari" //nolint:gosec // test password

type harness struct {
	t   *testing.T
	srv *Server
	h   http.Handler
	log *bytes.Buffer
}

func newHarness(t *testing.T, base string) harness {
	t.Helper()
	var buf bytes.Buffer
	srv, err := New(context.Background(), testConfig(t, base, &buf))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return harness{t: t, srv: srv, h: srv.Handler(), log: &buf}
}

func (h harness) user(id, email, phone string) {
	h.t.Helper()
	hash, err := argon2pw.New(argon2pw.Default).Hash(context.Background(), testPassword)
	if err != nil {
		h.t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := domain.User{ID: domain.UserID(id), Name: "Budi", Email: email, Phone: phone, PasswordHash: hash, CreatedAt: now, UpdatedAt: now}
	if err := h.srv.db.Write(context.Background(), func(s app.Store) error { return s.Users().Create(context.Background(), u) }); err != nil {
		h.t.Fatal(err)
	}
}

type req struct {
	method, path string
	body         any
	cookies      []*http.Cookie
	headers      map[string]string
	noJSON       bool
}

func (h harness) do(r req) *httptest.ResponseRecorder {
	h.t.Helper()
	var body *bytes.Reader
	if r.body != nil {
		b, _ := json.Marshal(r.body)
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	hr := httptest.NewRequest(r.method, r.path, body)
	hr.Host = "localhost:8080"
	hr.RemoteAddr = "203.0.113.5:4000"
	if !r.noJSON && r.method != http.MethodGet {
		hr.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.headers {
		if k == "Host" {
			hr.Host = v
			continue
		}
		hr.Header.Set(k, v)
	}
	for _, c := range r.cookies {
		hr.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, hr)
	return rec
}

func (h harness) login(identifier, pw string) *httptest.ResponseRecorder {
	return h.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]string{"identifier": identifier, "password": pw}})
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if strings.HasSuffix(c.Name, "liturgist_session") && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("not a problem body: %q", rec.Body.String())
	}
	return p
}

// IT-A-002 over HTTP
func TestLoginHTTP(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000001", "budi@example.org", "+6281234567890")

	wrong := h.login("budi@example.org", "wrong password!")
	unknown := h.login("nobody@example.org", "wrong password!")
	if wrong.Code != 401 || problemCode(t, wrong)["code"] != "invalid_credentials" || wrong.Body.String() != unknown.Body.String() {
		t.Errorf("wrong %d %s / unknown %s", wrong.Code, wrong.Body, unknown.Body)
	}

	ok := h.login("0812 3456 7890", testPassword)
	if ok.Code != 204 {
		t.Fatalf("login: %d %s", ok.Code, ok.Body)
	}
	c := sessionCookie(t, ok)
	if c.Name != "liturgist_session" || !c.HttpOnly || c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" ||
		c.MaxAge < 2160*3600-5 || c.MaxAge > 2160*3600 {
		t.Errorf("cookie attributes: %+v", c)
	}
	cleared := false
	for _, x := range ok.Result().Cookies() {
		if x.Name == "__Host-liturgist_session" && x.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the other cookie name must be cleared")
	}

	me := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{c}})
	var body map[string]any
	_ = json.Unmarshal(me.Body.Bytes(), &body)
	if me.Code != 200 || body["user"].(map[string]any)["email"] != "budi@example.org" || body["membership"] != nil {
		t.Errorf("me: %d %s", me.Code, me.Body)
	}
	if !strings.Contains(h.log.String(), `"user_id":"01JUSER0000000000000000001"`) {
		t.Error("access log must contain user_id")
	}
}

func TestLoginHTTPSCookie(t *testing.T) {
	h := newHarness(t, "https://localhost")
	h.user("01JUSER0000000000000000002", "budi@example.org", "")
	r := h.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]string{"identifier": "budi@example.org", "password": testPassword},
		headers: map[string]string{"Host": "localhost"}})
	if r.Code != 204 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if c := sessionCookie(t, r); c.Name != "__Host-liturgist_session" || !c.Secure {
		t.Errorf("https cookie: %+v", c)
	}
}

func TestTooManyAttemptsHTTP(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000003", "budi@example.org", "")
	for range 5 {
		h.login("budi@example.org", "wrong password!")
	}
	r := h.login("budi@example.org", testPassword)
	if r.Code != 429 || problemCode(t, r)["code"] != "too_many_attempts" || r.Header().Get("Retry-After") != "900" {
		t.Errorf("%d %s Retry-After=%q", r.Code, r.Body, r.Header().Get("Retry-After"))
	}
}

func TestMeRequiresSession(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	r := h.do(req{method: "GET", path: "/api/v1/me"})
	if r.Code != 401 || problemCode(t, r)["code"] != "unauthenticated" {
		t.Errorf("%d %s", r.Code, r.Body)
	}
}

// IT-A-016
func TestCookieNames(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000004", "budi@example.org", "")
	good := sessionCookie(t, h.login("budi@example.org", testPassword))
	stale := &http.Cookie{Name: "__Host-liturgist_session", Value: "old"} //nolint:gosec // test request cookie

	r := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{good, stale}})
	if r.Code != 200 {
		t.Errorf("current-scheme cookie must be used: %d", r.Code)
	}
	bogus := &http.Cookie{Name: "liturgist_session", Value: "bogus"} //nolint:gosec // test request cookie
	invalid := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{bogus}})
	names := map[string]bool{}
	for _, c := range invalid.Result().Cookies() {
		if c.MaxAge < 0 {
			names[c.Name] = true
		}
	}
	if invalid.Code != 401 || !names["liturgist_session"] || !names["__Host-liturgist_session"] {
		t.Errorf("invalid cookie must clear both names: %d %v", invalid.Code, invalid.Header().Values("Set-Cookie"))
	}
}

func TestLogoutHTTP(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000005", "budi@example.org", "")
	c := sessionCookie(t, h.login("budi@example.org", testPassword))
	if r := h.do(req{method: "POST", path: "/api/v1/auth/logout", cookies: []*http.Cookie{c}}); r.Code != 204 {
		t.Fatalf("logout: %d %s", r.Code, r.Body)
	}
	if r := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{c}}); r.Code != 401 {
		t.Errorf("session must be gone: %d", r.Code)
	}
	if r := h.do(req{method: "POST", path: "/api/v1/auth/logout"}); r.Code != 204 {
		t.Errorf("logout without session: %d", r.Code)
	}
}

func TestPatchMe(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000006", "budi@example.org", "")
	c := []*http.Cookie{sessionCookie(t, h.login("budi@example.org", testPassword))}
	patch := func(body string) *httptest.ResponseRecorder {
		var v any
		_ = json.Unmarshal([]byte(body), &v)
		return h.do(req{method: "PATCH", path: "/api/v1/me", body: v, cookies: c})
	}
	r := patch(`{"name":"  Budi S  ","preferences":{"text_size":"large","ui_language":"id"}}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"name":"Budi S"`) || !strings.Contains(r.Body.String(), `"ui_language":"id"`) {
		t.Fatalf("patch: %d %s", r.Code, r.Body)
	}
	r = patch(`{"preferences":{"ui_language":null}}`)
	if r.Code != 200 || strings.Contains(r.Body.String(), "ui_language") || !strings.Contains(r.Body.String(), `"text_size":"large"`) ||
		!strings.Contains(r.Body.String(), `"name":"Budi S"`) {
		t.Errorf("null clears only ui_language: %d %s", r.Code, r.Body)
	}
	r = patch(`{"name":"   "}`)
	if p := problemCode(t, r); r.Code != 422 || p["code"] != "validation_failed" || !strings.Contains(r.Body.String(), "body.name") {
		t.Errorf("empty name: %d %s", r.Code, r.Body)
	}
	if r := patch(`{"preferences":{"text_size":"huge"}}`); r.Code != 422 {
		t.Errorf("bad text size: %d %s", r.Code, r.Body)
	}
	if r := patch(`{"nickname":"x"}`); r.Code != 422 {
		t.Errorf("unknown field: %d %s", r.Code, r.Body)
	}
}

// IT-A-009
func TestChangePasswordHTTP(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000007", "budi@example.org", "")
	a := sessionCookie(t, h.login("budi@example.org", testPassword))
	b := sessionCookie(t, h.login("budi@example.org", testPassword))
	change := func(cur, next string) *httptest.ResponseRecorder {
		return h.do(req{method: "POST", path: "/api/v1/me/password", cookies: []*http.Cookie{a},
			body: map[string]string{"current_password": cur, "new_password": next}})
	}
	if r := change("wrong password!", "a much better passphrase"); r.Code != 401 || problemCode(t, r)["code"] != "invalid_credentials" {
		t.Errorf("wrong current: %d %s", r.Code, r.Body)
	}
	if r := change(testPassword, "password123"); r.Code != 422 || problemCode(t, r)["reason"] != "common" {
		t.Errorf("weak new: %d %s", r.Code, r.Body)
	}
	if r := change(testPassword, "a much better passphrase"); r.Code != 204 {
		t.Fatalf("change: %d %s", r.Code, r.Body)
	}
	if r := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{a}}); r.Code != 200 {
		t.Errorf("current session must stay: %d", r.Code)
	}
	if r := h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{b}}); r.Code != 401 {
		t.Errorf("other session must end: %d", r.Code)
	}
	if r := h.login("budi@example.org", "a much better passphrase"); r.Code != 204 {
		t.Errorf("new password must work: %d", r.Code)
	}
}

func TestEndOtherSessionsHTTP(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	h.user("01JUSER0000000000000000008", "budi@example.org", "")
	a := sessionCookie(t, h.login("budi@example.org", testPassword))
	b := sessionCookie(t, h.login("budi@example.org", testPassword))
	if r := h.do(req{method: "POST", path: "/api/v1/me/sessions/end-others", cookies: []*http.Cookie{a}}); r.Code != 204 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{a}}).Code != 200 ||
		h.do(req{method: "GET", path: "/api/v1/me", cookies: []*http.Cookie{b}}).Code != 401 {
		t.Error("only the current session may remain")
	}
}

// IT-A-010 / TC-A-006
func TestCSRF(t *testing.T) {
	h := newHarness(t, "http://localhost:8080")
	cases := []struct {
		name    string
		headers map[string]string
		noJSON  bool
		want    int
	}{
		{"cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}, false, 403},
		{"same-site subdomain", map[string]string{"Sec-Fetch-Site": "same-site"}, false, 403},
		{"same-origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, false, 204},
		{"matching Origin, no fetch metadata", map[string]string{"Origin": "http://localhost:8080"}, false, 204},
		{"foreign Origin", map[string]string{"Origin": "https://evil.example"}, false, 403},
		{"neither header (curl)", nil, false, 204},
		{"text/plain", map[string]string{"Content-Type": "text/plain"}, true, 403},
		{"no Content-Type", nil, true, 403},
	}
	for _, c := range cases {
		r := h.do(req{method: "POST", path: "/api/v1/auth/logout", headers: c.headers, noJSON: c.noJSON})
		if r.Code != c.want {
			t.Errorf("%s: got %d, want %d (%s)", c.name, r.Code, c.want, r.Body)
		}
		if c.want == 403 && problemCode(t, r)["code"] != "csrf_rejected" {
			t.Errorf("%s: code %s", c.name, r.Body)
		}
	}
}

func TestCSRFBehindProxy(t *testing.T) {
	h := newHarness(t, "https://liturgi.example.org")
	r := h.do(req{method: "POST", path: "/api/v1/auth/logout",
		headers: map[string]string{"Host": "liturgi.example.org", "Origin": "https://liturgi.example.org"}})
	if r.Code != 204 {
		t.Errorf("Origin equal to BaseURL must pass: %d %s", r.Code, r.Body)
	}
}
