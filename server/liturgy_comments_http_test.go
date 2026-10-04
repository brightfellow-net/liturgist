// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"strings"
	"testing"
)

// IT-R-007, IT-R-009: comments over HTTP, with the people who may not.
func TestCommentsHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	role := func(name string, scopes ...string) string {
		rec := h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin}, body: map[string]any{"name": name, "scopes": scopes}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("role %s: %d %s", name, rec.Code, rec.Body.String())
		}
		return decode(t, rec)["id"].(string)
	}
	commenter := h.invite(admin, "commenter@example.org", role("Komentator", "liturgy.comment"))
	editOnly := h.invite(admin, "editonly@example.org", role("Penyunting", "liturgy.edit"))
	team := h.invite(admin, "team@example.org")
	send := func(c *http.Cookie, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var cookies []*http.Cookie
		if c != nil {
			cookies = []*http.Cookie{c}
		}
		rec := h.do(req{method: method, path: "/api/v1" + path, cookies: cookies, body: body})
		return rec.Code, decode(t, rec)
	}
	obj := func(v any) map[string]any { return v.(map[string]any) }

	_, body := send(admin, "POST", "/liturgies", map[string]any{"date": "2026-11-01", "service_name": "Ibadah", "template_id": ""})
	lid := body["id"].(string)
	_, body = send(admin, "POST", "/liturgies/"+lid+"/items", map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "free_text"})
	item := obj(body["item"])["id"].(string)
	path := "/liturgies/" + lid + "/comments"

	code, body := send(commenter, "POST", path, map[string]any{"item_id": item, "body": "  Ganti doa  "})
	if code != 201 || body["body"] != "Ganti doa" || body["item_id"] != item || body["item_title"] != "Doa" || body["resolved"] != false ||
		obj(body["author"])["name"] == "" {
		t.Fatalf("add: %d %v", code, body)
	}
	cid := body["id"].(string)
	if code, body = send(admin, "POST", path, map[string]any{"body": "Umum"}); code != 201 || body["item_id"] != nil {
		t.Fatalf("whole liturgy: %d %v", code, body)
	}
	code, body = send(admin, "GET", "/liturgies/"+lid, nil)
	if body["open_comments"] != 2.0 || obj(body["actions"])["comment"] != true {
		t.Errorf("view: %d %v %v", code, body["open_comments"], body["actions"])
	}

	code, body = send(admin, "PUT", path+"/"+cid+"/resolved", map[string]any{"resolved": true})
	if code != 200 || body["resolved"] != true || obj(body["resolved_by"])["name"] == "" || body["resolved_at"] == nil {
		t.Fatalf("resolve: %d %v", code, body)
	}
	code, body = send(commenter, "GET", path+"?resolved=false", nil)
	if items := body["items"].([]any); code != 200 || len(items) != 1 || body["open"] != 1.0 || obj(items[0])["body"] != "Umum" {
		t.Fatalf("open filter: %d %v", code, body)
	}
	code, body = send(commenter, "GET", path, nil)
	if items := body["items"].([]any); code != 200 || len(items) != 2 || obj(items[0])["id"] != cid {
		t.Fatalf("list: %d %v", code, body)
	}
	if code, body = send(admin, "PUT", path+"/"+cid+"/resolved", map[string]any{"resolved": false}); code != 200 || body["resolved"] != false || body["resolved_by"] != nil {
		t.Errorf("reopen: %d %v", code, body)
	}

	// Rules: validation, scopes, unknown ids, states.
	for name, b := range map[string]map[string]any{"blank": {"body": "  "}, "long": {"body": strings.Repeat("x", 2001)}, "foreign item": {"item_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV", "body": "x"}} {
		if code, body := send(commenter, "POST", path, b); code != 422 {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}
	if code, body := send(nil, "GET", path, nil); code != 401 {
		t.Errorf("no session: %d %v", code, body)
	}
	for _, c := range []*http.Cookie{team} {
		if code, _ := send(c, "GET", path, nil); code != 404 {
			t.Errorf("team member list: %d", code)
		}
		if code, _ := send(c, "POST", path, map[string]any{"body": "x"}); code != 404 {
			t.Errorf("team member add: %d", code)
		}
	}
	if code, body := send(editOnly, "POST", path, map[string]any{"body": "x"}); code != 403 || body["code"] != "forbidden" {
		t.Errorf("edit-only add: %d %v", code, body)
	}
	if code, _ := send(editOnly, "GET", path, nil); code != 200 {
		t.Errorf("edit-only list: %d", code)
	}
	if code, _ := send(admin, "PUT", path+"/01ARZ3NDEKTSV4RRFFQ69G5FAV/resolved", map[string]any{"resolved": true}); code != 404 {
		t.Errorf("unknown comment: %d", code)
	}
	send(admin, "POST", "/liturgies/"+lid+"/submit", map[string]any{})
	_, body = send(admin, "GET", "/liturgies/"+lid, nil)
	if code, body := send(admin, "POST", path, map[string]any{"body": "Saat tinjauan"}); code != 201 {
		t.Errorf("comment in review: %d %v", code, body)
	}
	if code, body := send(admin, "POST", "/liturgies/"+lid+"/approve", map[string]any{"edit_seq": body["edit_seq"]}); code != 200 || body["open_comments"] != 3.0 {
		t.Fatalf("approve: %d %v", code, body)
	}
	if code, body := send(admin, "POST", path, map[string]any{"body": "Terlambat"}); code != 409 || body["code"] != "liturgy_locked" {
		t.Errorf("comment when approved: %d %v", code, body)
	}
}
