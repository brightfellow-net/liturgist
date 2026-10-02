// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

// register adds a platform operation (no tenant needed).
func register(id, path string) Option {
	return WithRoutes(func(api huma.API) {
		huma.Register(api, huma.Operation{OperationID: id, Method: http.MethodGet, Path: path,
			Metadata: map[string]any{httpapi.TenancyKey: httpapi.TenancyPlatform}},
			func(context.Context, *struct{}) (*struct{}, error) { return nil, nil })
	})
}

// TC-F-008
func TestWithRoutesCannotReplace(t *testing.T) {
	cfg := testConfig(t, "http://localhost:8080", nil)

	// A new operation works.
	h := testHandler(t, cfg, register("extra", "/extra"))
	if rec := do(h, http.MethodGet, "/api/v1/extra", "localhost", nil); rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Errorf("new operation: got %d", rec.Code)
	}

	for name, opts := range map[string][]Option{
		"same operation ID": {register("dup", "/a"), register("dup", "/b")},
		"same method+path":  {register("one", "/same"), register("two", "/same")},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected a panic")
				}
			}()
			_, _ = New(context.Background(), cfg, opts...)
		})
	}
}

// TC-F-011
func TestStartupWarnings(t *testing.T) {
	cfg := testConfig(t, "http://liturgi.local:8080", nil)

	cfg.Listen = ":8080"
	if w := StartupWarnings(cfg); len(w) != 1 || !strings.Contains(w[0], "plain HTTP") {
		t.Errorf("all interfaces over http: %v", w)
	}
	cfg.Listen = "127.0.0.1:8080"
	if w := StartupWarnings(cfg); len(w) != 0 {
		t.Errorf("loopback: %v", w)
	}
	cfg.Listen = ":8080"
	cfg.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	if w := StartupWarnings(cfg); len(w) != 0 {
		t.Errorf("behind a trusted proxy: %v", w)
	}

	https := testConfig(t, "https://liturgi.example.org", nil)
	if w := StartupWarnings(https); len(w) != 1 || !strings.Contains(w[0], "terminates TLS") {
		t.Errorf("https without proxy: %v", w)
	}
}

// IT-F-005 (part): the document can be produced without a database.
func TestOpenAPI(t *testing.T) {
	doc, err := OpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !strings.HasPrefix(v["openapi"].(string), "3.1") {
		t.Errorf("openapi version = %v", v["openapi"])
	}
}
