// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func sectionIDsOf(song map[string]any) []string {
	var out []string
	for _, s := range song["sections"].([]any) {
		out = append(out, s.(map[string]any)["id"].(string))
	}
	return out
}

// IT-S-001 over HTTP: CRUD, search, permissions, optimistic versions, groups.
func TestSongsHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	post := func(c *http.Cookie, path string, body any) map[string]any {
		t.Helper()
		rec := h.do(req{method: "POST", path: path, cookies: []*http.Cookie{c}, body: body})
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}

	song := post(admin, "/api/v1/songs", map[string]any{
		"language": "id", "title": "Besar Setia-Mu", "hymnal_source": "KJ", "hymnal_number": "12",
		"alt_titles": []string{"Great Is Thy Faithfulness"},
		"sections": []map[string]any{
			{"key": "v1", "kind": "verse", "number": 1, "text": "besar setia-Mu ya Tuhan"},
			{"key": "c", "kind": "chorus", "text": "reff"},
		},
		"default_arrangement": []string{"v1", "c", "v1"},
	})
	id := song["id"].(string)
	secs := sectionIDsOf(song)
	arr := song["default_arrangement"].([]any)
	if song["version"] != 1.0 || len(secs) != 2 || len(arr) != 3 || arr[0] != secs[0] || arr[1] != secs[1] || arr[2] != secs[0] {
		t.Fatalf("created song: %v", song)
	}
	if song["actions"].(map[string]any)["edit"] != true {
		t.Errorf("admin actions: %v", song["actions"])
	}

	// Anyone can read; lyrics are not in lists.
	rec := h.get("/api/v1/songs?q=setia", team)
	list := decode(t, rec)
	if rec.Code != 200 || list["total"] != 1.0 || strings.Contains(rec.Body.String(), "besar setia-Mu ya Tuhan") {
		t.Errorf("team member list: %d %s", rec.Code, rec.Body.String())
	}
	if items := list["items"].([]any); items[0].(map[string]any)["actions"].(map[string]any)["edit"] != false {
		t.Errorf("team member must not get edit: %v", items[0])
	}
	if got := decode(t, h.get("/api/v1/songs/"+id, team)); len(got["sections"].([]any)) != 2 {
		t.Errorf("team member get: %v", got)
	}
	for q, want := range map[string]float64{"kj 12": 1, "great": 1, "reff": 1, "zzz": 0} {
		if got := decode(t, h.get("/api/v1/songs?q="+url.QueryEscape(q), team)); got["total"] != want {
			t.Errorf("q=%q: total %v, want %v", q, got["total"], want)
		}
	}

	// Writes need library.edit, a JSON body, and known fields.
	for _, r := range []req{
		{method: "POST", path: "/api/v1/songs", body: map[string]any{"language": "id", "title": "X"}},
		{method: "PATCH", path: "/api/v1/songs/" + id, body: map[string]any{"version": 1, "title": "X"}},
		{method: "DELETE", path: "/api/v1/songs/" + id},
		{method: "POST", path: "/api/v1/songs/" + id + "/link", body: map[string]any{"other_song_id": id}},
		{method: "DELETE", path: "/api/v1/songs/" + id + "/link"},
	} {
		r.cookies = []*http.Cookie{team}
		if rec := h.do(r); rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s %s by team member: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/songs", cookies: []*http.Cookie{admin}, body: map[string]any{"language": "id", "title": "X", "bogus": 1}}); rec.Code != 422 {
		t.Errorf("unknown field: %d", rec.Code)
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/songs", cookies: []*http.Cookie{admin}, body: map[string]any{"language": "id", "title": " "}}); rec.Code != 422 ||
		problemCode(t, rec)["code"] != "validation_failed" {
		t.Errorf("blank title: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/songs/01JNOSUCHSONG0000000000000", admin); rec.Code != 404 || problemCode(t, rec)["code"] != "not_found" {
		t.Errorf("unknown song: %d %s", rec.Code, rec.Body.String())
	}

	// IT-S-002: versions.
	rec = h.do(req{method: "PATCH", path: "/api/v1/songs/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{"version": 1, "title": "Besar Setia"}})
	if rec.Code != 200 || decode(t, rec)["version"] != 2.0 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(req{method: "PATCH", path: "/api/v1/songs/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{"version": 1, "title": "Stale"}})
	if rec.Code != http.StatusConflict || problemCode(t, rec)["code"] != "version_conflict" {
		t.Errorf("stale patch: %d %s", rec.Code, rec.Body.String())
	}
	// Dropping the chorus through sections keeps the rest and cleans the arrangement.
	rec = h.do(req{method: "PATCH", path: "/api/v1/songs/" + id, cookies: []*http.Cookie{admin}, body: map[string]any{
		"version": 2, "sections": []map[string]any{{"id": secs[0], "kind": "verse", "number": 1, "text": "bait satu"}}}})
	got := decode(t, rec)
	if rec.Code != 200 || len(got["default_arrangement"].([]any)) != 2 || got["sections"].([]any)[0].(map[string]any)["id"] != secs[0] {
		t.Errorf("sections patch: %d %s", rec.Code, rec.Body.String())
	}

	// Groups.
	other := post(admin, "/api/v1/songs", map[string]any{"language": "en", "title": "Great Is Thy Faithfulness"})
	linked := post(admin, "/api/v1/songs/"+id+"/link", map[string]any{"other_song_id": other["id"]})
	if v := linked["versions"].([]any); len(v) != 1 || v[0].(map[string]any)["id"] != other["id"] {
		t.Errorf("linked versions: %v", linked["versions"])
	}
	another := post(admin, "/api/v1/songs", map[string]any{"language": "en", "title": "Second English"})
	rec = h.do(req{method: "POST", path: "/api/v1/songs/" + id + "/link", cookies: []*http.Cookie{admin}, body: map[string]any{"other_song_id": another["id"]}})
	if p := problemCode(t, rec); rec.Code != http.StatusConflict || p["code"] != "group_conflict" || p["reason"] != "language_taken" {
		t.Errorf("language taken: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(req{method: "DELETE", path: "/api/v1/songs/" + id + "/link", cookies: []*http.Cookie{admin}}); rec.Code != 204 {
		t.Errorf("unlink: %d", rec.Code)
	}

	// Delete.
	if rec := h.do(req{method: "DELETE", path: "/api/v1/songs/" + id, cookies: []*http.Cookie{admin}}); rec.Code != 204 {
		t.Errorf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/songs/"+id, admin); rec.Code != 404 {
		t.Errorf("deleted song: %d", rec.Code)
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/songs"}); rec.Code != 401 {
		t.Errorf("anonymous list: %d", rec.Code)
	}
}
