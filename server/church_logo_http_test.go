// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func logoBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 180, 255
	}
	img.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func mustDecode(t *testing.T, b64 string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// IT-607: upload, serve, cache, remove, and who may do what.
func TestChurchLogoHTTP(t *testing.T) {
	h := harnessWith(t)
	const path = "/api/v1/church/logo"
	admin := h.setupChurch()
	if rec := h.do(req{method: "GET", path: path}); rec.Code != http.StatusUnauthorized {
		t.Errorf("get without a session: %d", rec.Code)
	}
	if rec := h.get(path, admin); rec.Code != http.StatusNotFound || problemCode(t, rec)["code"] != "not_found" {
		t.Errorf("get without a logo: %d %s", rec.Code, rec.Body.String())
	}
	if c := decode(t, h.get("/api/v1/church", admin)); c["logo_url"] != nil {
		t.Errorf("logo_url without a logo: %v", c["logo_url"])
	}

	rec := h.do(req{method: "PUT", path: path, cookies: []*http.Cookie{admin}, body: map[string]any{"image": logoBase64(t, 600, 300)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	url, _ := decode(t, rec)["logo_url"].(string)
	if !regexp.MustCompile(`^/api/v1/church/logo\?v=[0-9a-f]{16}$`).MatchString(url) {
		t.Fatalf("logo_url %q", url)
	}
	if me := decode(t, h.get("/api/v1/me", admin))["church"].(map[string]any); me["logo_url"] != url {
		t.Errorf("/me church logo_url %v, want %s", me["logo_url"], url)
	}
	version := url[strings.Index(url, "v=")+2:]
	if got := filesUnder(t, filepath.Join(h.srv.cfg.DataDir, "files")); len(got) != 1 || !strings.HasSuffix(got[0], "logo-"+version+".png") {
		t.Errorf("files on disk: %v", got)
	}

	// The file: a PNG that fits the square, cached for a year under its own version.
	rec = h.get(url, admin)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" || rec.Header().Get("ETag") != `"`+version+`"` {
		t.Fatalf("get: %d %v", rec.Code, rec.Header())
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(rec.Body.Bytes())); err != nil || cfg.Width != 512 || cfg.Height != 256 {
		t.Errorf("served image: %+v, %v", cfg, err)
	}
	// Without the version, or with an old one, it is not cached; a matching ETag gives 304.
	for _, p := range []string{path, path + "?v=0000000000000000"} {
		if rec := h.get(p, admin); rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, no-cache" {
			t.Errorf("get %s: %d %q", p, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
	rec = h.do(req{method: "GET", path: url, cookies: []*http.Cookie{admin}, headers: map[string]string{"If-None-Match": `"` + version + `"`}})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("conditional get: %d with %d bytes", rec.Code, rec.Body.Len())
	}

	// A member without church.settings can see the logo but not change it.
	var editorRole string
	var roles []map[string]any
	if err := json.Unmarshal(h.get("/api/v1/roles", admin).Body.Bytes(), &roles); err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r["origin"] == "editor" {
			editorRole, _ = r["id"].(string)
		}
	}
	member := h.invite(admin, "editor@example.org", editorRole)
	if rec := h.get(url, member); rec.Code != http.StatusOK {
		t.Errorf("member get: %d", rec.Code)
	}
	for _, m := range []string{"PUT", "DELETE"} {
		rec := h.do(req{method: m, path: path, cookies: []*http.Cookie{member}, body: map[string]any{"image": logoBase64(t, 10, 10)}})
		if rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s by a member: %d %s", m, rec.Code, rec.Body.String())
		}
	}

	// Unsafe requests must be JSON (03 §6): a raw image body is refused.
	rec = h.do(req{method: "PUT", path: path, cookies: []*http.Cookie{admin}, noJSON: true,
		headers: map[string]string{"Content-Type": "image/png"}})
	if rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "csrf_rejected" {
		t.Errorf("raw image put: %d %s", rec.Code, rec.Body.String())
	}

	// Remove: the setting and the file go; a second remove is fine.
	for i := 0; i < 2; i++ {
		rec = h.do(req{method: "DELETE", path: path, cookies: []*http.Cookie{admin}})
		if rec.Code != http.StatusOK || decode(t, rec)["logo_url"] != nil {
			t.Fatalf("delete %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if got := filesUnder(t, filepath.Join(h.srv.cfg.DataDir, "files")); len(got) != 0 {
		t.Errorf("files left after delete: %v", got)
	}
	if rec := h.get(url, admin); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete: %d", rec.Code)
	}
}

// IT-607: what the sender can fix is a 422 on "image" and nothing is stored; a body over the limit is 413.
func TestChurchLogoHTTPInvalid(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	put := func(body any) int {
		return h.do(req{method: "PUT", path: "/api/v1/church/logo", cookies: []*http.Cookie{admin}, body: body}).Code
	}
	for name, body := range map[string]any{
		"not base64":   map[string]any{"image": "%%%"},
		"not an image": map[string]any{"image": base64.StdEncoding.EncodeToString([]byte("hello"))},
		// A real PNG with padding after it, so only the size limit can refuse it.
		"too large": map[string]any{"image": base64.StdEncoding.EncodeToString(append(mustDecode(t, logoBase64(t, 64, 64)), make([]byte, 2<<20)...))},
		"empty":     map[string]any{"image": ""},
		"missing":   map[string]any{},
	} {
		if code := put(body); code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code := put(map[string]any{"image": strings.Repeat("A", 4<<20)}); code != http.StatusRequestEntityTooLarge {
		t.Errorf("a 4 MiB body: %d", code)
	}
	if got := filesUnder(t, filepath.Join(h.srv.cfg.DataDir, "files")); len(got) != 0 {
		t.Errorf("refused uploads left files: %v", got)
	}
}

// IT-608: a backup holds the logo; after a restore into an empty folder it is served again.
func TestChurchLogoBackupRestore(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	rec := h.do(req{method: "PUT", path: "/api/v1/church/logo", cookies: []*http.Cookie{admin}, body: map[string]any{"image": logoBase64(t, 300, 300)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	url := decode(t, rec)["logo_url"].(string)
	want := h.get(url, admin).Body.Bytes()

	ctx := context.Background()
	res, err := Backup(ctx, h.srv.cfg, filepath.Join(t.TempDir(), "church.zip"))
	if err != nil {
		t.Fatal(err)
	}
	_ = h.srv.Close()

	cfg := h.srv.cfg
	cfg.DataDir = t.TempDir()
	if _, err := Restore(ctx, cfg, res.Path, nil); err != nil {
		t.Fatal(err)
	}
	srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	restored := harness{t: t, srv: srv, h: srv.Handler(), log: &bytes.Buffer{}}
	cookie := sessionCookie(t, restored.login("admin@example.org", testPassword))
	if church := decode(t, restored.get("/api/v1/church", cookie)); church["logo_url"] != url {
		t.Errorf("restored logo_url %v, want %s", church["logo_url"], url)
	}
	if got := restored.get(url, cookie); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), want) {
		t.Errorf("restored logo: %d, %d bytes (want %d)", got.Code, got.Body.Len(), len(want))
	}
}
