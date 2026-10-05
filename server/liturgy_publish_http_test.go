// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"testing"
)

// IT-P-001, IT-P-007, IT-P-008, IT-P-015: publishing, archiving and who sees what, over HTTP.
func TestPublishHTTP(t *testing.T) {
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
	if code, body = send(admin, "POST", path("items"), map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "free_text"}); code != 201 {
		t.Fatalf("add item: %d %v", code, body)
	}
	if code, body = send(ruth, "POST", path("submit"), map[string]any{}); code != 200 {
		t.Fatalf("submit: %d %v", code, body)
	}
	if code, body = send(admin, "POST", path("approve"), map[string]any{"edit_seq": body["edit_seq"]}); code != 200 {
		t.Fatalf("approve: %d %v", code, body)
	}
	seq := body["edit_seq"]
	if obj(body["actions"])["publish"] != true || body["published"] != nil {
		t.Fatalf("approved: %v %v", body["actions"], body["published"])
	}

	// Who may publish, and in which shape.
	if code, body := send(team, "POST", path("publish"), map[string]any{"edit_seq": seq}); code != 404 {
		t.Errorf("team member: %d %v", code, body)
	}
	if code, body := send(ruth, "POST", path("publish"), map[string]any{"edit_seq": seq}); code != 403 {
		t.Errorf("editor: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("publish"), map[string]any{}); code != 422 {
		t.Errorf("no edit_seq: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("publish"), map[string]any{"edit_seq": 0}); code != 409 || body["code"] != "review_stale" {
		t.Errorf("stale: %d %v", code, body)
	}
	code, body = send(admin, "POST", path("publish"), map[string]any{"edit_seq": seq, "note": "Shalom"})
	pub := obj(body["published"])
	if code != 200 || body["state"] != "published" || pub["number"] != 1.0 || pub["published_at"] == "" || obj(body["actions"])["archive"] != true {
		t.Fatalf("publish: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("publish"), map[string]any{"edit_seq": seq}); code != 409 || body["code"] != "invalid_transition" || body["state"] != "published" {
		t.Errorf("publish twice: %d %v", code, body)
	}

	// Members without a scope reach nothing of the editable liturgy (13 §5, P-79).
	for _, p := range []string{"", "/edits", "/state-changes", "/comments"} {
		if code, body := send(team, "GET", "/liturgies/"+lid+p, nil); code != 404 {
			t.Errorf("team member GET %s: %d %v", p, code, body)
		}
	}
	if code, body := send(team, "GET", "/liturgies", nil); code != 200 || len(body["items"].([]any)) != 0 {
		t.Errorf("team member list: %d %v", code, body)
	}

	// Deleting and archiving.
	if code, body := send(ruth, "DELETE", "/liturgies/"+lid, nil); code != 403 {
		t.Errorf("editor deletes: %d %v", code, body)
	}
	if code, body := send(admin, "DELETE", "/liturgies/"+lid, nil); code != 409 || body["code"] != "liturgy_not_deletable" {
		t.Errorf("delete published: %d %v", code, body)
	}
	if code, body := send(ruth, "POST", path("archive"), map[string]any{}); code != 403 {
		t.Errorf("editor archives: %d %v", code, body)
	}
	code, body = send(admin, "POST", path("archive"), map[string]any{})
	if code != 200 || body["archived_at"] == nil || obj(body["actions"])["unarchive"] != true || obj(body["actions"])["reopen"] != false {
		t.Fatalf("archive: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("archive"), map[string]any{}); code != 409 || body["code"] != "liturgy_archived" {
		t.Errorf("archive twice: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("reopen"), map[string]any{}); code != 409 || body["code"] != "liturgy_archived" {
		t.Errorf("reopen archived: %d %v", code, body)
	}
	list := func(q string) int {
		code, body := send(admin, "GET", "/liturgies"+q, nil)
		if code != 200 {
			t.Fatalf("list %s: %d %v", q, code, body)
		}
		return len(body["items"].([]any))
	}
	if list("") != 0 || list("?archived=true") != 1 || list("?archived=all") != 1 {
		t.Errorf("list filters: %d %d %d", list(""), list("?archived=true"), list("?archived=all"))
	}
	if code, body := send(admin, "GET", "/liturgies?archived=maybe", nil); code != 422 {
		t.Errorf("bad filter: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("unarchive"), map[string]any{}); code != 200 || body["archived_at"] != nil {
		t.Fatalf("unarchive: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path("unarchive"), map[string]any{}); code != 409 || body["code"] != "not_archived" {
		t.Errorf("unarchive twice: %d %v", code, body)
	}

	// Reopen, and the version stays.
	code, body = send(admin, "POST", path("reopen"), map[string]any{"note": "typo"})
	if code != 200 || body["state"] != "draft" || obj(body["published"])["number"] != 1.0 || obj(body["actions"])["delete"] != false {
		t.Fatalf("reopen: %d %v", code, body)
	}
	if code, body := send(admin, "DELETE", "/liturgies/"+lid, nil); code != 409 || body["code"] != "liturgy_not_deletable" {
		t.Errorf("delete a reopened liturgy: %d %v", code, body)
	}
	// The list item says the same.
	_, lbody := send(admin, "GET", "/liturgies", nil)
	if a := obj(obj(lbody["items"].([]any)[0])["actions"]); a["delete"] != false {
		t.Errorf("list actions: %v", a)
	}
}
