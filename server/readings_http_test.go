// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// httpProvider offers one text; storing is allowed when mayStore is set.
type httpProvider struct {
	id       string
	mayStore bool
}

func (p httpProvider) ID() string { return p.id }

func (p httpProvider) Lookup(_ context.Context, ref domain.Reference, _ string) (app.BibleText, error) {
	if ref.Book != "JHN" {
		return app.BibleText{}, app.ErrNotAvailable
	}
	return app.BibleText{Text: "Text from " + p.id, Attribution: "© " + p.id, Source: p.id, MayStore: p.mayStore}, nil
}

// IT-R-001 to IT-R-006 over HTTP.
func TestReadingsHTTP(t *testing.T) {
	h := harnessWith(t, WithBibleTextProvider(httpProvider{"readonly", false}), WithBibleTextProvider(httpProvider{"licensed", true}))
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	send := func(c *http.Cookie, method, path string, body any) *http.Response {
		t.Helper()
		rec := h.do(req{method: method, path: path, cookies: []*http.Cookie{c}, body: body})
		return rec.Result()
	}
	post := func(c *http.Cookie, path string, body any) map[string]any {
		t.Helper()
		rec := h.do(req{method: "POST", path: path, cookies: []*http.Cookie{c}, body: body})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}

	// parse: one parser for the browser and the server.
	rec := h.get("/api/v1/readings/parse?input="+url.QueryEscape("Kej. 1:1–2:3"), team)
	if p := decode(t, rec); rec.Code != 200 || p["reference"] != "GEN 1:1-2:3" || p["canonical"] != "Kejadian 1:1-2:3" {
		t.Errorf("parse: %d %v", rec.Code, p)
	}
	for in, reason := range map[string]string{"": "empty", "Foo 1": "unknown_book", "Yoh": "missing_chapter", "Yoh 3:21-16": "bad_range",
		"Yoh 3;4": "unsupported", "Yoh 3.16": "bad_number", strings.Repeat("a", 101): "too_long"} {
		rec := h.get("/api/v1/readings/parse?input="+url.QueryEscape(in), team)
		if p := problemCode(t, rec); rec.Code != 422 || p["code"] != "invalid_reference" || p["reason"] != reason {
			t.Errorf("parse %q: %d %s", in, rec.Code, rec.Body.String())
		}
	}

	// create: typed text is "manual"; the permission is library.edit.
	r := post(admin, "/api/v1/readings", map[string]any{"reference": "Yoh 3:16-21", "translation": "TB", "text": "Karena begitu besar", "attribution": "LAI"})
	id := r["id"].(string)
	if r["reference"] != "JHN 3:16-21" || r["canonical"] != "Yohanes 3:16-21" || r["source_provider"] != "manual" || r["version"] != 1.0 ||
		r["reference_display"] != "Yoh 3:16-21" || r["translation"].(map[string]any)["code"] != "TB" || r["actions"].(map[string]any)["edit"] != true {
		t.Fatalf("created: %v", r)
	}
	for _, c := range []req{
		{method: "POST", path: "/api/v1/readings", body: map[string]any{"reference": "Mzm 1", "text": "x"}},
		{method: "POST", path: "/api/v1/readings/from-provider", body: map[string]any{"reference": "Yoh 3:16", "provider": "licensed"}},
		{method: "PATCH", path: "/api/v1/readings/" + id, body: map[string]any{"version": 1, "text": "x"}},
		{method: "DELETE", path: "/api/v1/readings/" + id},
	} {
		c.cookies = []*http.Cookie{team}
		if rec := h.do(c); rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s %s by team member: %d %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	// A client cannot choose the source of a reading.
	if rec := h.do(req{method: "POST", path: "/api/v1/readings", cookies: []*http.Cookie{admin},
		body: map[string]any{"reference": "Mzm 1", "text": "x", "source_provider": "licensed"}}); rec.Code != 422 {
		t.Errorf("source_provider in the body: %d %s", rec.Code, rec.Body.String())
	}
	// Existing: 409 with the existing reading's ID, however it is spelled.
	rec = h.do(req{method: "POST", path: "/api/v1/readings", cookies: []*http.Cookie{admin},
		body: map[string]any{"reference": "Yohanes 3 : 16 - 21", "translation": "TB", "text": "lain"}})
	if p := problemCode(t, rec); rec.Code != http.StatusConflict || p["code"] != "reading_exists" || p["reading_id"] != id {
		t.Errorf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	post(admin, "/api/v1/readings", map[string]any{"reference": "Yoh 3:16-21", "translation": "BIS", "text": "BIS"}) // another translation
	if rec := h.do(req{method: "POST", path: "/api/v1/readings", cookies: []*http.Cookie{admin}, body: map[string]any{"reference": "Mzm 2", "translation": "NOPE", "text": "x"}}); rec.Code != 422 {
		t.Errorf("unknown translation: %d", rec.Code)
	}

	// lookup: the stored reading wins; otherwise the providers in order.
	lk := decode(t, h.get("/api/v1/readings/lookup?translation=TB&reference="+url.QueryEscape("Yoh 3:16-21"), team))
	if lk["reading"] == nil || lk["reading"].(map[string]any)["id"] != id || lk["provider"] != nil || lk["suggested_attribution"] != "LAI" {
		t.Errorf("stored lookup: %v", lk)
	}
	lk = decode(t, h.get("/api/v1/readings/lookup?translation=TB&reference=Yoh+3:16", team))
	pr, _ := lk["provider"].(map[string]any)
	if lk["reading"] != nil || pr["source"] != "readonly" || pr["may_store"] != false || pr["text"] != "Text from readonly" || lk["provider_error"] != false {
		t.Errorf("provider lookup: %v", lk)
	}
	lk = decode(t, h.get("/api/v1/readings/lookup?reference=Mzm+23", team)) // no translation: the church's
	if lk["reading"] != nil || lk["provider"] != nil || lk["translation"].(map[string]any)["code"] != "TB" || lk["canonical"] != "Mazmur 23" {
		t.Errorf("nothing found: %v", lk)
	}
	if rec := h.get("/api/v1/readings/lookup?reference=Foo+1", team); rec.Code != 422 || problemCode(t, rec)["code"] != "invalid_reference" {
		t.Errorf("lookup of a bad reference: %d", rec.Code)
	}

	// from-provider: the server saves the provider's text, never the client's.
	rec = h.do(req{method: "POST", path: "/api/v1/readings/from-provider", cookies: []*http.Cookie{admin},
		body: map[string]any{"reference": "Yoh 3:16", "translation": "TB", "provider": "readonly"}})
	if p := problemCode(t, rec); rec.Code != 422 || p["code"] != "validation_failed" {
		t.Errorf("may_store false: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/readings/from-provider", cookies: []*http.Cookie{admin},
		body: map[string]any{"reference": "Yoh 3:16", "translation": "TB", "provider": "licensed", "text": "my own text"}}); rec.Code != 422 {
		t.Errorf("text in from-provider: %d", rec.Code)
	}
	saved := post(admin, "/api/v1/readings/from-provider", map[string]any{"reference": "Yoh 3:16", "translation": "TB", "provider": "licensed"})
	if saved["source_provider"] != "licensed" || saved["text"] != "Text from licensed" || saved["attribution"] != "© licensed" {
		t.Errorf("from-provider: %v", saved)
	}
	rec = h.do(req{method: "POST", path: "/api/v1/readings/from-provider", cookies: []*http.Cookie{admin},
		body: map[string]any{"reference": "Yoh 3:16", "translation": "TB", "provider": "licensed"}})
	if p := problemCode(t, rec); rec.Code != http.StatusConflict || p["code"] != "reading_exists" || p["reading_id"] != saved["id"] {
		t.Errorf("saving twice: %d %s", rec.Code, rec.Body.String())
	}

	// list and get: anyone may read; order by book, chapter, verse.
	rec = h.get("/api/v1/readings", team)
	list := decode(t, rec)
	var refs []string
	for _, it := range list["items"].([]any) {
		m := it.(map[string]any)
		refs = append(refs, m["reference"].(string)+"/"+m["translation"].(map[string]any)["code"].(string))
		if m["actions"].(map[string]any)["edit"] != false {
			t.Errorf("team member must not get edit: %v", m)
		}
	}
	if got := strings.Join(refs, " "); rec.Code != 200 || list["total"] != 3.0 || got != "JHN 3:16-21/BIS JHN 3:16/TB JHN 3:16-21/TB" {
		t.Errorf("list: %d %s %v", rec.Code, got, list)
	}
	if got := decode(t, h.get("/api/v1/readings?q=begitu+besar", team)); got["total"] != 1.0 || got["items"].([]any)[0].(map[string]any)["snippet"] != "Karena begitu besar" {
		t.Errorf("search: %v", got)
	}
	if got := decode(t, h.get("/api/v1/readings?translation=BIS", team)); got["total"] != 1.0 {
		t.Errorf("filter: %v", got)
	}
	if got := decode(t, h.get("/api/v1/readings/"+id, team)); got["text"] != "Karena begitu besar" || got["attribution"] != "LAI" {
		t.Errorf("get: %v", got)
	}
	if rec := h.get("/api/v1/readings/01JNOSUCHREADING000000000", admin); rec.Code != 404 || problemCode(t, rec)["code"] != "not_found" {
		t.Errorf("unknown reading: %d", rec.Code)
	}

	// patch: versions; the reference cannot change.
	rec = h.do(req{method: "PATCH", path: "/api/v1/readings/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{"version": 1, "text": "Baru", "attribution": "TB"}})
	if got := decode(t, rec); rec.Code != 200 || got["version"] != 2.0 || got["text"] != "Baru" {
		t.Errorf("patch: %d %v", rec.Code, got)
	}
	rec = h.do(req{method: "PATCH", path: "/api/v1/readings/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{"version": 1, "text": "Stale"}})
	if rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "version_conflict" {
		t.Errorf("stale patch: %d %s", rec.Code, rec.Body.String())
	}
	for _, field := range []string{"reference", "translation", "source_provider"} {
		if rec := h.do(req{method: "PATCH", path: "/api/v1/readings/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{"version": 2, field: "X"}}); rec.Code != 422 {
			t.Errorf("patching %s: %d", field, rec.Code)
		}
	}

	// delete.
	if resp := send(admin, "DELETE", "/api/v1/readings/"+id, nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete: %d", resp.StatusCode)
	}
	if rec := h.get("/api/v1/readings/"+id, admin); rec.Code != 404 {
		t.Errorf("after delete: %d", rec.Code)
	}
}
