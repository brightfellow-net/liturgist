// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

func testConfig(t *testing.T, base string, logBuf *bytes.Buffer) Config {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	if logBuf == nil {
		logBuf = &bytes.Buffer{}
	}
	return Config{
		Listen:      "127.0.0.1:0",
		BaseURL:     u,
		DataDir:     t.TempDir(),
		DBDriver:    "sqlite",
		AutoMigrate: true,
		Logger:      slog.New(slog.NewJSONHandler(logBuf, nil)),
	}
}

func testHandler(t *testing.T, cfg Config, opts ...Option) http.Handler {
	t.Helper()
	srv, err := New(context.Background(), cfg, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv.Handler()
}

func do(h http.Handler, method, target, host string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TC-F-004
func TestRequestID(t *testing.T) {
	h := testHandler(t, testConfig(t, "http://localhost:8080", nil))

	rec := do(h, http.MethodGet, "/healthz", "localhost:8080", map[string]string{"X-Request-ID": "abc"})
	if got := rec.Header().Get("X-Request-ID"); got == "abc" || len(got) != 32 {
		t.Errorf("too-short ID should be replaced by a 32-char ID, got %q", got)
	}
	rec = do(h, http.MethodGet, "/healthz", "localhost:8080", map[string]string{"X-Request-ID": "valid-request-id-1"})
	if got := rec.Header().Get("X-Request-ID"); got != "valid-request-id-1" {
		t.Errorf("valid ID should be kept, got %q", got)
	}
}

// TC-F-005
func TestSecurityHeaders(t *testing.T) {
	for base, wantHSTS := range map[string]bool{"http://localhost:8080": false, "https://liturgi.example.org": true} {
		h := testHandler(t, testConfig(t, base, nil))
		host := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
		rec := do(h, http.MethodGet, "/healthz", host, nil)
		for _, k := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy"} {
			if rec.Header().Get(k) == "" {
				t.Errorf("%s: missing %s", base, k)
			}
		}
		if got := rec.Header().Get("Strict-Transport-Security") != ""; got != wantHSTS {
			t.Errorf("%s: HSTS present = %v, want %v", base, got, wantHSTS)
		}
	}
}

// TC-F-009
func TestAllowedHosts(t *testing.T) {
	cfg := testConfig(t, "https://liturgi.example.org", nil)
	cfg.ExtraHosts = []string{"192.168.1.10"}
	h := testHandler(t, cfg)

	cases := []struct {
		host, path string
		want       int
	}{
		{"evil.example", "/", http.StatusMisdirectedRequest},
		{"evil.example", "/api/v1/openapi.json", http.StatusMisdirectedRequest},
		{"liturgi.example.org", "/api/v1/openapi.json", http.StatusOK},
		{"LITURGI.example.org:443", "/api/v1/openapi.json", http.StatusOK},
		{"192.168.1.10:8080", "/api/v1/openapi.json", http.StatusOK},
		{"127.0.0.1", "/api/v1/openapi.json", http.StatusMisdirectedRequest},
		{"evil.example", "/healthz", http.StatusOK},
		{"evil.example", "/readyz", http.StatusOK},
	}
	for _, c := range cases {
		if rec := do(h, http.MethodGet, c.path, c.host, nil); rec.Code != c.want {
			t.Errorf("host %q path %s: status %d, want %d", c.host, c.path, rec.Code, c.want)
		}
	}

	local := testHandler(t, testConfig(t, "http://localhost:8080", nil))
	for _, host := range []string{"localhost:8080", "127.0.0.1:8080", "[::1]:8080"} {
		if rec := do(local, http.MethodGet, "/api/v1/openapi.json", host, nil); rec.Code != http.StatusOK {
			t.Errorf("localhost base, host %q: status %d", host, rec.Code)
		}
	}
}

// TC-F-007
func TestAccessLogOmitsQuery(t *testing.T) {
	var buf bytes.Buffer
	h := testHandler(t, testConfig(t, "http://localhost:8080", &buf))
	do(h, http.MethodGet, "/healthz?secret=xyz", "localhost", nil)
	out := buf.String()
	if !strings.Contains(out, `"path":"/healthz"`) || strings.Contains(out, "xyz") {
		t.Errorf("access log should contain the path without the query: %s", out)
	}
}

func withDist(dist fs.FS) Option { return func(o *options) { o.dist = dist } }

// IT-F-002
func TestFrontendBuilt(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"sw.js":           {Data: []byte("self.skipWaiting()")},
	}
	h := testHandler(t, testConfig(t, "http://localhost:8080", nil), withDist(dist))

	rec := do(h, http.MethodGet, "/members", "localhost", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>app</title>") ||
		rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("SPA route: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
	}
	rec = do(h, http.MethodGet, "/assets/app-1.js", "localhost", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("asset: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	// The service worker is a real file at the root, never the index page, and
	// is revalidated on every start so a new build reaches the phones (13 §7).
	rec = do(h, http.MethodGet, "/sw.js", "localhost", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "self.skipWaiting()" ||
		rec.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("sw.js: %d %q %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"), rec.Header().Get("Content-Type"))
	}
	// A root file the build did not produce is a 404, not the index page.
	if rec = do(h, http.MethodGet, "/manifest.webmanifest", "localhost", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing manifest: got %d, want 404", rec.Code)
	}
}

// IT-F-003
func TestFrontendNotBuilt(t *testing.T) {
	h := testHandler(t, testConfig(t, "http://localhost:8080", nil), withDist(fstest.MapFS{".keep": {}}))
	rec := do(h, http.MethodGet, "/members", "localhost", nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Frontend not built") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("index must not be cached")
	}
	if rec := do(h, http.MethodGet, "/api/v1/openapi.json", "localhost", nil); rec.Code != http.StatusOK {
		t.Errorf("API must keep working without a frontend, got %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/members", "localhost", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST outside /api: got %d, want 405", rec.Code)
	}
}

// IT-F-006
func TestNoCORS(t *testing.T) {
	h := testHandler(t, testConfig(t, "http://localhost:8080", nil))
	rec := do(h, http.MethodOptions, "/api/v1/openapi.json", "localhost",
		map[string]string{"Origin": "https://other.example", "Access-Control-Request-Method": "POST"})
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("OPTIONS: got %d, want 405", rec.Code)
	}
	for k := range rec.Header() {
		if strings.HasPrefix(k, "Access-Control-") {
			t.Errorf("unexpected CORS header %s", k)
		}
	}
}

func TestAPINotFoundIsProblem(t *testing.T) {
	h := testHandler(t, testConfig(t, "http://localhost:8080", nil))
	rec := do(h, http.MethodGet, "/api/v1/nope", "localhost", nil)
	var p httpapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	if rec.Code != http.StatusNotFound || p.Code != "not_found" ||
		rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("got %d %+v %s", rec.Code, p, rec.Header().Get("Content-Type"))
	}
}

// TC-F-006 (part): panics become 500 `internal` without details.
func TestPanicBecomesProblem(t *testing.T) {
	var buf bytes.Buffer
	h := testHandler(t, testConfig(t, "http://localhost:8080", &buf), WithRoutes(func(api huma.API) {
		huma.Register(api, huma.Operation{OperationID: "boom", Method: http.MethodGet, Path: "/boom",
			Metadata: map[string]any{httpapi.TenancyKey: httpapi.TenancyPlatform}},
			func(context.Context, *struct{}) (*struct{}, error) { panic("kaboom") })
	}))
	rec := do(h, http.MethodGet, "/api/v1/boom", "localhost", nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal"`) ||
		strings.Contains(rec.Body.String(), "kaboom") {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(buf.String(), "kaboom") {
		t.Errorf("panic not logged")
	}
}

// TC-F-006 (part): Huma's own errors carry a code; 5xx details are hidden.
func TestHumaErrorsCarryCode(t *testing.T) {
	p, ok := httpapi.NewProblem(http.StatusUnprocessableEntity, "bad input").(*httpapi.Problem)
	if !ok || p.Code != "validation_failed" || p.Detail != "bad input" {
		t.Errorf("422: %+v", p)
	}
	p = httpapi.NewProblem(http.StatusInternalServerError, "db password wrong").(*httpapi.Problem)
	if p.Code != "internal" || p.Detail != "" {
		t.Errorf("500 must hide details: %+v", p)
	}
}
