// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/internal/backup"
)

func ptr[T any](v T) *T { return &v }

// TC-610
func TestDiskLow(t *testing.T) {
	const gib = int64(1 << 30)
	for _, tc := range []struct {
		free, total int64
		low         bool
	}{
		{500 << 20, 10 * gib, true}, // under 1 GiB
		{2 * gib, 10 * gib, false},  // over 1 GiB and over 5 %
		{3 * gib, 100 * gib, true},  // over 1 GiB but under 5 % (5 GiB)
		{6 * gib, 100 * gib, false}, // over 5 %
		{1 * gib, 10 * gib, false},  // exactly 1 GiB is not below it
		{0, 0, true},                // nothing free
	} {
		if got := diskLow(tc.free, tc.total); got != tc.low {
			t.Errorf("free %d of %d: %v, want %v", tc.free, tc.total, got, tc.low)
		}
	}
}

// TC-610
func TestBackupWarning(t *testing.T) {
	now := at("2026-10-05 12:00")
	old := now.AddDate(0, -2, 0) // the church is two months old
	hours := func(h int) *time.Time { return ptr(now.Add(-time.Duration(h) * time.Hour)) }
	days := func(d int) *time.Time { return ptr(now.AddDate(0, 0, -d)) }
	for _, tc := range []struct {
		name          string
		last, copied  *time.Time
		churchCreated time.Time
		want          string
	}{
		{"all recent", hours(3), days(5), old, ""},
		{"backup 3 days old", days(3), days(5), old, "stale"},
		{"backup just under 48 hours", hours(47), days(5), old, ""},
		{"no backup ever, old church", nil, days(5), old, "stale"},
		{"no backup yet, church made an hour ago", nil, nil, now.Add(-time.Hour), ""},
		{"no copy in 31 days", hours(3), days(31), old, "not_copied"},
		{"no copy ever, old church", hours(3), nil, old, "not_copied"},
		{"no copy ever, church 10 days old", hours(3), nil, now.AddDate(0, 0, -10), ""},
		{"both missing: stale is the more urgent", days(3), days(40), old, "stale"},
	} {
		if got := backupWarning(now, tc.last, tc.copied, tc.churchCreated); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestInsideBackups(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	for p, want := range map[string]bool{
		filepath.Join(data, "backups", "x.zip"):        true,
		filepath.Join(data, "backups", "sub", "x.zip"): true,
		filepath.Join(data, "x.zip"):                   false,
		filepath.Join(data, "backups-other", "x.zip"):  false,
		filepath.Join(filepath.Dir(data), "x.zip"):     false,
	} {
		if got := insideBackups(data, p); got != want {
			t.Errorf("%s: %v, want %v", p, got, want)
		}
	}
}

func TestNewestBackup(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"auto-20261003T020000Z.zip", "manual-20261004T101500Z.zip", "auto-20261002T020000Z.zip",
		"pre-upgrade-v00003-20261005T000000Z-ab.db", "auto-20261005T020000Z.zip.partial", "manual-bad.zip"} {
		write(t, filepath.Join(dir, n), "x")
	}
	at, kind := newestBackup(dir)
	if at == nil || kind != "manual" || !at.Equal(time.Date(2026, 10, 4, 10, 15, 0, 0, time.UTC)) {
		t.Errorf("%v %q", at, kind)
	}
	if at, kind := newestBackup(t.TempDir()); at != nil || kind != "" {
		t.Errorf("empty folder: %v %q", at, kind)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"GKY Citragarden": "gky-citragarden", "  --  ": "church", "Gereja Kristus Yesus — Jakarta Barat!": "gereja-kristus-yesus-jakarta-barat", "": "church"} {
		if got := slug(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := slug(strings.Repeat("a", 100)); len(got) != 40 {
		t.Errorf("length %d", len(got))
	}
}

// IT-604: the status and the download, for the admin, a plain member and nobody.
func TestSystemRoutesHTTP(t *testing.T) {
	var logs bytes.Buffer
	cfg := testConfig(t, "http://localhost:8080", &logs)
	srv, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	h := harness{t: t, srv: srv, h: srv.Handler(), log: &logs}
	admin := h.setupChurch()
	member := h.invite(admin, "member@example.org") // no roles: a team member

	t.Run("status", func(t *testing.T) {
		rec := h.get("/api/v1/system/status", admin)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		b := decode(t, rec)
		db := b["database"].(map[string]any)
		disk := b["disk"].(map[string]any)
		bk := b["backup"].(map[string]any)
		hs := b["https"].(map[string]any)
		if b["version"] != "dev" || db["driver"] != "sqlite" || db["size_bytes"].(float64) <= 0 || db["schema_version"].(float64) < 1 {
			t.Errorf("version or database: %v", b)
		}
		if disk["free_bytes"].(float64) <= 0 || disk["total_bytes"].(float64) <= 0 {
			t.Errorf("disk: %v", disk)
		}
		if bk["supported"] != true || bk["scheduled"] != false || bk["last_at"] != nil || bk["warning"] != "" {
			t.Errorf("backup: %v", bk)
		}
		if hs["mode"] != "plain_http" || b["email_configured"] != false || b["update"].(map[string]any)["enabled"] != false {
			t.Errorf("https, email or update: %v", b)
		}
	})
	t.Run("only church.settings", func(t *testing.T) {
		for _, path := range []string{"/api/v1/system/status", "/api/v1/system/backup"} {
			if rec := h.get(path, member); rec.Code != http.StatusForbidden {
				t.Errorf("%s as a team member: %d", path, rec.Code)
			}
			if rec := h.do(req{method: "GET", path: path}); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s without a session: %d", path, rec.Code)
			}
		}
		if strings.Contains(logs.String(), "backup_downloaded") {
			t.Error("a refused download was logged as done")
		}
	})
	t.Run("download", func(t *testing.T) {
		write(t, filepath.Join(cfg.DataDir, "files", "logo.txt"), "a file")
		rec := h.get("/api/v1/system/backup", admin)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
			t.Errorf("content type %q", ct)
		}
		cd := rec.Header().Get("Content-Disposition")
		if !strings.HasPrefix(cd, `attachment; filename="liturgist-gky-citragarden-`) || !strings.HasSuffix(cd, `.zip"`) {
			t.Errorf("content disposition %q", cd)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("cache control %q", cc)
		}
		zipPath := filepath.Join(t.TempDir(), "d.zip")
		if err := os.WriteFile(zipPath, rec.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := backup.Open(zipPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = a.Close() }()
		dir := t.TempDir()
		if err := a.Extract(dir); err != nil {
			t.Fatal(err)
		}
		if got := churchName(t, filepath.Join(dir, "liturgist.db")); got != "GKY Citragarden" {
			t.Errorf("the downloaded database holds %q", got)
		}
		if got := read(t, filepath.Join(dir, "files", "logo.txt")); got != "a file" {
			t.Errorf("the downloaded file: %q", got)
		}
		// Logged with the actor and the size, never the content; no temp left; counted as a copy.
		if !strings.Contains(logs.String(), `"msg":"backup_downloaded"`) {
			t.Error("the download was not logged")
		}
		entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "backups"))
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), backupTempPrefix) {
				t.Errorf("temp folder left behind: %s", e.Name())
			}
		}
		b := decode(t, h.get("/api/v1/system/status", admin))
		if b["backup"].(map[string]any)["last_copied_at"] == nil {
			t.Error("the download is not shown as a copy")
		}
	})
	t.Run("a backup is already running", func(t *testing.T) {
		srv.backupMu.Lock()
		rec := h.get("/api/v1/system/backup", admin)
		srv.backupMu.Unlock()
		if rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "backup_running" {
			t.Errorf("%d %s", rec.Code, rec.Body.String())
		}
		if rec := h.get("/api/v1/system/backup", admin); rec.Code != 200 {
			t.Errorf("after it finished: %d", rec.Code)
		}
	})
	t.Run("the disk is full", func(t *testing.T) {
		old := freeBytes
		freeBytes = func(string) (int64, error) { return 1024, nil }
		defer func() { freeBytes = old }()
		rec := h.get("/api/v1/system/backup", admin)
		if rec.Code != http.StatusInsufficientStorage || problemCode(t, rec)["code"] != "storage_full" {
			t.Errorf("%d %s", rec.Code, rec.Body.String())
		}
		if srv.backupMu.TryLock() { // the lock was given back
			srv.backupMu.Unlock()
		} else {
			t.Error("the backup lock was not released")
		}
	})
}

// H-8: a server with a tenant resolver (the hosted edition) has no system routes.
func TestSystemRoutesAbsentWhenHosted(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	churchID := decode(t, h.get("/api/v1/me", admin))["church"].(map[string]any)["id"].(string)
	saas, err := New(context.Background(), h.srv.cfg, WithTenantResolver(headerResolver{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saas.Close() })
	sh := harness{t: t, srv: saas, h: saas.Handler()}
	for _, path := range []string{"/api/v1/system/status", "/api/v1/system/backup"} {
		rec := sh.do(req{method: "GET", path: path, cookies: []*http.Cookie{admin}, headers: map[string]string{"X-Test-Church": churchID}})
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	// And the community build documents them.
	doc, err := OpenAPI()
	if err != nil || !strings.Contains(string(doc), "/system/status") {
		t.Errorf("the OpenAPI document of the community edition lacks the system routes: %v", err)
	}
	hosted, err := OpenAPI(WithTenantResolver(headerResolver{}))
	if err != nil || strings.Contains(string(hosted), "/system/") {
		t.Errorf("the hosted OpenAPI document has system routes: %v", err)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.3.0", "v0.2.9", true}, {"v0.3.0", "v0.3.0", false}, {"v0.3.0", "v0.4.0", false},
		{"v1.0.0", "v0.9.9", true}, {"v0.10.0", "v0.9.0", true}, {"v0.3.1", "0.3.0", true},
		{"v0.3.0", "dev", false}, {"v0.3.0", "", false}, {"garbage", "v0.1.0", false},
		{"v0.3.0", "v0.2.0-5-gabc123", true}, {"v0.3.0", "v0.3.0-5-gabc123", false},
	} {
		if got := newerVersion(tc.latest, tc.current); got != tc.want {
			t.Errorf("%q vs %q: %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

// H-13: the update check sends one GET with the program's name and nothing else.
func TestUpdateCheck(t *testing.T) {
	var gotUA, gotCookie, gotAuth, gotQuery string
	status, body := http.StatusOK, `{"tag_name":"v9.9.9"}`
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotCookie, gotAuth, gotQuery = r.Header.Get("User-Agent"), r.Header.Get("Cookie"), r.Header.Get("Authorization"), r.URL.RawQuery
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer api.Close()
	cfg := testConfig(t, "http://localhost:8080", nil)
	cfg.UpdateCheck, cfg.UpdateURL = true, api.URL
	srv, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	h := harness{t: t, srv: srv, h: srv.Handler()}
	admin := h.setupChurch()

	if u := decode(t, h.get("/api/v1/system/status", admin))["update"].(map[string]any); u["enabled"] != true || u["latest"] != "" || u["available"] != false {
		t.Errorf("before the first check: %v", u)
	}
	if err := srv.updates.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotUA != "liturgist/dev" || gotCookie != "" || gotAuth != "" || gotQuery != "" {
		t.Errorf("the request carried more than the program name: UA %q cookie %q auth %q query %q", gotUA, gotCookie, gotAuth, gotQuery)
	}
	// "dev" is not a release, so nothing is out of date; with a release version it is.
	if u := decode(t, h.get("/api/v1/system/status", admin))["update"].(map[string]any); u["latest"] != "v9.9.9" || u["available"] != false {
		t.Errorf("a development build: %v", u)
	}
	old := Version
	Version = "v0.1.0"
	u := decode(t, h.get("/api/v1/system/status", admin))["update"].(map[string]any)
	Version = old
	if u["available"] != true {
		t.Errorf("an old release: %v", u)
	}

	for name, tc := range map[string]struct {
		status int
		body   string
	}{"server error": {500, "x"}, "not json": {200, "<html>"}, "no version": {200, `{"tag_name":"nightly"}`}} {
		status, body = tc.status, tc.body
		srv.updates.mu.Lock()
		srv.updates.latest = ""
		srv.updates.mu.Unlock()
		if err := srv.updates.check(context.Background()); err == nil {
			t.Errorf("%s: no error", name)
		}
		if srv.updates.Latest() != "" {
			t.Errorf("%s: latest %q", name, srv.updates.Latest())
		}
	}
}

// Without LITURGIST_UPDATE_CHECK there is no checker at all.
func TestUpdateCheckOffByDefault(t *testing.T) {
	srv, err := New(context.Background(), testConfig(t, "http://localhost:8080", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if srv.updates != nil {
		t.Error("a checker exists although the check is off")
	}
}
