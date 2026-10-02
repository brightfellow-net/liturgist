// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/internal/envconfig"
	"github.com/brightfellow-net/liturgist/server"
)

func cli(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// IT-A-012 and the step-1 commands of 01 §6, with their exit codes.
func TestOperatorCommands(t *testing.T) {
	t.Setenv("LITURGIST_DATA_DIR", t.TempDir())
	t.Setenv("LITURGIST_BASE_URL", "https://liturgi.example.org")
	if code, _, errOut := cli("", "migrate"); code != exitOK {
		t.Fatalf("migrate: %d %s", code, errOut)
	}

	code, out, errOut := cli("", "setup-link")
	if code != exitOK || !strings.Contains(out, "https://liturgi.example.org/setup#t=") || !strings.Contains(out, "=====") {
		t.Errorf("setup-link before setup: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("", "user", "list"); code != exitOK {
		t.Errorf("user list before setup: %d", code)
	}

	setup := []string{"setup", "--church-name", "GKY Citragarden", "--admin-name", "Admin",
		"--admin-identifier", "admin@example.org", "--password-stdin"}
	if code, _, errOut := cli("short\n", setup...); code != exitError || !strings.Contains(errOut, "weak password") {
		t.Errorf("weak password: %d %q", code, errOut)
	}
	if code, out, errOut := cli("kopi susu pagi hari\n", setup...); code != exitOK || !strings.Contains(out, "set up") {
		t.Fatalf("setup: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("kopi susu pagi hari\n", setup...); code != exitAlreadySetUp {
		t.Errorf("second setup: %d", code)
	}
	if code, _, _ := cli("", "setup-link"); code != exitAlreadySetUp {
		t.Errorf("setup-link after setup: %d", code)
	}
	if code, _, _ := cli("", "setup", "--church-name", "X"); code != exitConfig {
		t.Errorf("setup without required flags: %d", code)
	}
	if code, _, _ := cli("", "setup", "--church-name", "X", "--admin-name", "A", "--admin-identifier", "a@example.org"); code != exitError {
		t.Errorf("setup without a terminal or --password-stdin: %d", code)
	}

	code, out, _ = cli("", "user", "list")
	if code != exitOK || !strings.Contains(out, "admin@example.org") || !strings.Contains(out, "Church admin") {
		t.Errorf("user list: %d %q", code, out)
	}
	code, out, errOut = cli("", "user", "reset-password", "ADMIN@example.org")
	if code != exitOK || !strings.Contains(out, "https://liturgi.example.org/reset#t=") || !strings.Contains(out, "WIB") ||
		strings.Contains(errOut, "LITURGIST_BASE_URL is not set") {
		t.Errorf("reset-password: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("", "user", "reset-password", "nobody@example.org"); code != exitNoSuchUser {
		t.Errorf("reset-password for an unknown user: %d", code)
	}
	if code, _, _ := cli("", "member", "grant-admin", "admin@example.org"); code != exitOK {
		t.Errorf("grant-admin: %d", code)
	}
	if code, _, _ := cli("", "member", "grant-admin", "nobody@example.org"); code != exitNoSuchUser {
		t.Errorf("grant-admin for an unknown user: %d", code)
	}
	for _, args := range [][]string{{"--all"}, {"--identifier", "admin@example.org"}, {"--ip", "203.0.113.5"}, {"--ip", "2001:db8::1"}} {
		if code, _, errOut := cli("", append([]string{"auth", "clear-throttle"}, args...)...); code != exitOK {
			t.Errorf("clear-throttle %v: %d %s", args, code, errOut)
		}
	}
	for _, args := range [][]string{{}, {"--all", "--ip", "203.0.113.5"}} {
		if code, _, _ := cli("", append([]string{"auth", "clear-throttle"}, args...)...); code != exitConfig {
			t.Errorf("clear-throttle %v: %d", args, code)
		}
	}
	if code, _, errOut := cli("", "auth", "clear-throttle", "--ip", "nonsense"); code != exitError || !strings.Contains(errOut, "not an IP") {
		t.Errorf("bad IP: %d %q", code, errOut)
	}
	if code, out, errOut := cli("", "search", "reindex"); code != exitOK || !strings.Contains(out, "rebuilt for 1 church") {
		t.Errorf("search reindex: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("", "search", "reindex", "extra"); code != exitConfig {
		t.Errorf("search reindex with an argument: %d", code)
	}

	t.Setenv("LITURGIST_BASE_URL", "")
	if _, _, errOut := cli("", "user", "reset-password", "admin@example.org"); !strings.Contains(errOut, "LITURGIST_BASE_URL is not set") {
		t.Errorf("missing base URL warning: %q", errOut)
	}
}

// Exit codes for each error class (01 §6), including wrapped errors.
func TestExitCodes(t *testing.T) {
	for err, want := range map[error]int{
		errors.New("boom"):                                    exitError,
		server.ErrAlreadySetUp:                                exitAlreadySetUp,
		fmt.Errorf("op: %w", server.ErrNoSuchUser):            exitNoSuchUser,
		fmt.Errorf("op: %w", server.ErrNotMember):             exitNotMember,
		fmt.Errorf("resolver: %w", server.ErrTooManyChurches): exitTooManyChurches,
	} {
		if got := exitFor(err, io.Discard); got != want {
			t.Errorf("%v: exit %d, want %d", err, got, want)
		}
	}
}

// Test passwords.
const (
	adminPassword  = "kopi susu pagi hari" //nolint:gosec // test password
	memberPassword = "es jeruk sore hari"  //nolint:gosec // test password
)

// Exit 6: grant-admin for someone who has an account but left the church.
func TestGrantAdminNotMember(t *testing.T) {
	t.Setenv("LITURGIST_DATA_DIR", t.TempDir())
	t.Setenv("LITURGIST_BASE_URL", "https://liturgi.example.org")
	if code, _, errOut := cli("", "migrate"); code != exitOK {
		t.Fatalf("migrate: %d %s", code, errOut)
	}
	if code, _, errOut := cli(adminPassword+"\n", "setup", "--church-name", "GKY Citragarden", "--admin-name", "Admin",
		"--admin-identifier", "admin@example.org", "--password-stdin"); code != exitOK {
		t.Fatalf("setup: %d %s", code, errOut)
	}

	cfg, _, err := envconfig.Load(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Logger = slog.New(slog.DiscardHandler)
	srv, err := server.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	api := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(b))
		r.Host = "liturgi.example.org"
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, r)
		if rec.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	cookie := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if strings.HasSuffix(c.Name, "liturgist_session") && c.Value != "" {
				return c
			}
		}
		t.Fatal("no session cookie")
		return nil
	}

	admin := cookie(api("POST", "/auth/login", map[string]string{"identifier": "admin@example.org", "password": adminPassword}, nil))
	var invite struct{ Link string }
	_ = json.Unmarshal(api("POST", "/invites", map[string]string{"name": "Sari", "email": "sari@example.org"}, admin).Body.Bytes(), &invite)
	api("POST", "/invites/accept", map[string]string{"token": invite.Link[strings.Index(invite.Link, "#t=")+3:],
		"name": "Sari", "email": "sari@example.org", "password": memberPassword}, nil)
	var members struct {
		Members []struct{ ID, Email string }
	}
	_ = json.Unmarshal(api("GET", "/members", nil, admin).Body.Bytes(), &members)
	for _, m := range members.Members {
		if m.Email == "sari@example.org" {
			api("DELETE", "/members/"+m.ID, nil, admin)
		}
	}

	if code, _, errOut := cli("", "member", "grant-admin", "sari@example.org"); code != exitNotMember {
		t.Errorf("grant-admin for a former member: %d %s", code, errOut)
	}
}
