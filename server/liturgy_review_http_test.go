// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"strings"
	"testing"
)

// IT-R-001, IT-R-002, IT-R-009: the review round over HTTP, with the people who may not.
func TestReviewHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	role := func(name string, scopes ...string) string {
		rec := h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin}, body: map[string]any{"name": name, "scopes": scopes}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("role %s: %d %s", name, rec.Code, rec.Body.String())
		}
		return decode(t, rec)["id"].(string)
	}
	ruth := h.invite(admin, "ruth@example.org", role("Penyunting", "liturgy.edit"))
	commenter := h.invite(admin, "commenter@example.org", role("Komentator", "liturgy.comment"))
	team := h.invite(admin, "team@example.org")

	send := func(c *http.Cookie, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var cookies []*http.Cookie
		if c != nil {
			cookies = []*http.Cookie{c}
		}
		rec := h.do(req{method: method, path: "/api/v1" + path, cookies: cookies, body: body})
		if rec.Code == http.StatusNoContent {
			return rec.Code, nil
		}
		return rec.Code, decode(t, rec)
	}
	obj := func(v any) map[string]any { return v.(map[string]any) }

	code, body := send(admin, "POST", "/liturgies", map[string]any{"date": "2026-11-01", "service_name": "Ibadah", "template_id": ""})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	lid := body["id"].(string)
	path := func(action string) string { return "/liturgies/" + lid + "/" + action }

	// An empty liturgy cannot be submitted; an unfinished song item lists the problem.
	if code, body := send(ruth, "POST", path("submit"), map[string]any{}); code != 422 || body["code"] != "empty_liturgy" {
		t.Fatalf("empty: %d %v", code, body)
	}
	code, body = send(admin, "POST", path("items"), map[string]any{"liturgy_version": 1, "title": "Lagu", "item_type": "song"})
	if code != 201 {
		t.Fatalf("add item: %d %v", code, body)
	}
	song := obj(body["item"])["id"].(string)
	code, body = send(ruth, "POST", path("submit"), map[string]any{})
	if probs, _ := body["problems"].([]any); code != 422 || body["code"] != "has_problems" || len(probs) != 1 || obj(probs[0])["item_id"] != song {
		t.Fatalf("problems: %d %v", code, body)
	}
	if code, body = send(admin, "DELETE", path("items/"+song)+"?liturgy_version=2", nil); code != 200 {
		t.Fatalf("remove: %d %v", code, body)
	}
	if code, body = send(admin, "POST", path("items"), map[string]any{"liturgy_version": 3, "title": "Doa", "item_type": "free_text"}); code != 201 {
		t.Fatalf("add prayer: %d %v", code, body)
	}

	// Submit: the answer is the liturgy, with the review data and the new actions.
	code, body = send(ruth, "POST", path("submit"), map[string]any{"note": "siap"})
	if code != 200 || body["state"] != "in_review" || obj(body["last_change"])["note"] != "siap" || obj(obj(body["last_change"])["user"])["name"] == "" {
		t.Fatalf("submit: %d %v", code, body)
	}
	if body["open_comments"] != 0.0 {
		t.Errorf("open_comments: %v", body["open_comments"])
	}
	if a := obj(body["actions"]); a["edit"] != false || a["submit"] != false || a["approve"] != false {
		t.Errorf("editor's actions in review: %v", a)
	}
	seq := body["edit_seq"].(float64)

	// Wrong people, wrong shape, wrong state, wrong sequence.
	if code, body := send(nil, "POST", path("approve"), map[string]any{"edit_seq": seq}); code != 401 {
		t.Errorf("no session: %d %v", code, body)
	}
	if code, body := send(team, "POST", path("approve"), map[string]any{"edit_seq": seq}); code != 404 {
		t.Errorf("team member: %d %v", code, body)
	}
	for who, c := range map[string]*http.Cookie{"editor": ruth, "commenter": commenter} {
		if code, body := send(c, "POST", path("approve"), map[string]any{"edit_seq": seq}); code != 403 || body["code"] != "forbidden" {
			t.Errorf("%s approving: %d %v", who, code, body)
		}
	}
	if code, body := send(admin, "POST", path("approve"), map[string]any{}); code != 422 {
		t.Errorf("no edit_seq: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("submit"), map[string]any{}); code != 409 || body["code"] != "invalid_transition" || body["state"] != "in_review" {
		t.Errorf("second submit: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("approve"), map[string]any{"edit_seq": seq - 1}); code != 409 || body["code"] != "review_stale" {
		t.Errorf("stale: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("request-changes"), map[string]any{"edit_seq": seq, "note": strings.Repeat("x", 4001)}); code != 422 {
		t.Errorf("huge note: %d %v", code, body)
	}
	if code, body := send(ruth, "POST", path("items"), map[string]any{"liturgy_version": 5, "title": "x", "item_type": "free_text"}); code != 409 || body["code"] != "liturgy_locked" {
		t.Errorf("edit in review: %d %v", code, body)
	}

	// Request changes, resubmit, approve, reopen.
	code, body = send(admin, "POST", path("request-changes"), map[string]any{"edit_seq": seq, "note": "Ganti doa"})
	if code != 200 || body["state"] != "needs_revision" || obj(body["actions"])["edit"] != true {
		t.Fatalf("request changes: %d %v", code, body)
	}
	if code, body = send(ruth, "POST", path("submit"), map[string]any{}); code != 200 {
		t.Fatalf("resubmit: %d %v", code, body)
	}
	code, body = send(admin, "POST", path("approve"), map[string]any{"edit_seq": body["edit_seq"]})
	if code != 200 || body["state"] != "approved" || obj(body["actions"])["reopen"] != true {
		t.Fatalf("approve: %d %v", code, body)
	}
	if code, body = send(admin, "POST", path("reopen"), map[string]any{}); code != 200 || body["state"] != "draft" {
		t.Fatalf("reopen: %d %v", code, body)
	}

	// The history, newest first, paged; the people without a liturgy scope get 404.
	code, body = send(commenter, "GET", path("state-changes")+"?limit=2", nil)
	items := body["items"].([]any)
	if code != 200 || body["total"] != 5.0 || len(items) != 2 || obj(items[0])["to_state"] != "draft" || obj(items[1])["to_state"] != "approved" {
		t.Fatalf("history: %d %v", code, body)
	}
	code, body = send(commenter, "GET", path("state-changes")+"?limit=2&offset=4", nil)
	if items = body["items"].([]any); len(items) != 1 || obj(items[0])["from_state"] != "draft" || obj(items[0])["note"] != "siap" {
		t.Fatalf("last page: %d %v", code, body)
	}
	if code, _ := send(team, "GET", path("state-changes"), nil); code != 404 {
		t.Errorf("team member history: %d", code)
	}
	if code, body := send(admin, "POST", "/liturgies/01ARZ3NDEKTSV4RRFFQ69G5FAV/submit", map[string]any{}); code != 404 {
		t.Errorf("unknown liturgy: %d %v", code, body)
	}
}
