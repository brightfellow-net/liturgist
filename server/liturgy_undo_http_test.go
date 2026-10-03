// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"testing"
)

// IT-E-001: undo and redo over HTTP, with two people and the people who may not.
func TestUndoHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	role := func(name string, scopes ...string) string {
		rec := h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin}, body: map[string]any{"name": name, "scopes": scopes}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("role %s: %d %s", name, rec.Code, rec.Body.String())
		}
		return decode(t, rec)["id"].(string)
	}
	editorRole := role("Penyunting", "liturgy.edit")
	ruth := h.invite(admin, "ruth@example.org", editorRole)
	budi := h.invite(admin, "budi@example.org", editorRole)
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

	code, body := send(admin, "POST", "/liturgies", map[string]any{"date": "2026-11-01", "service_name": "Ibadah Batal", "template_id": ""})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	lid := body["id"].(string)
	code, body = send(admin, "POST", "/liturgies/"+lid+"/items", map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "free_text"})
	if code != 201 {
		t.Fatalf("add item: %d %v", code, body)
	}
	item := obj(body["item"])["id"].(string)
	text := func(c *http.Cookie, version int, s string) {
		t.Helper()
		if code, body := send(c, "PATCH", "/liturgies/"+lid+"/items/"+item, map[string]any{"version": version, "text": s}); code != 200 {
			t.Fatalf("patch: %d %v", code, body)
		}
	}
	itemText := func() (string, float64) {
		_, body := send(admin, "GET", "/liturgies/"+lid, nil)
		it := obj(body["items"].([]any)[0])
		return it["text"].(string), it["version"].(float64)
	}

	// Ruth edits, undoes and redoes; the answer names the edit and the new versions.
	text(ruth, 1, "satu")
	code, body = send(ruth, "POST", "/liturgies/"+lid+"/undo", nil)
	if e := obj(body["edit"]); code != 200 || e["command"] != "item.update" || e["status"] != "undone" || e["item_id"] != item || body["item_version"] != 3.0 {
		t.Fatalf("undo: %d %v", code, body)
	}
	if s, v := itemText(); s != "" || v != 3 {
		t.Errorf("after undo: %q version %v", s, v)
	}
	code, body = send(ruth, "POST", "/liturgies/"+lid+"/redo", nil)
	if e := obj(body["edit"]); code != 200 || e["status"] != "done" || body["item_version"] != 4.0 {
		t.Fatalf("redo: %d %v", code, body)
	}
	if s, _ := itemText(); s != "satu" {
		t.Errorf("after redo: %q", s)
	}

	// The history shows the undo and redo rows and the status of what they acted on.
	_, body = send(ruth, "GET", "/liturgies/"+lid+"/edits", nil)
	cmds := map[string]int{}
	for _, e := range body["items"].([]any) {
		cmds[obj(e)["command"].(string)]++
	}
	if cmds["undo"] != 1 || cmds["redo"] != 1 {
		t.Errorf("history commands: %v", cmds)
	}

	// Budi changes the same item: Ruth's undo is refused, with the reason, and changes nothing.
	_, v := itemText()
	text(budi, int(v), "dari Budi")
	code, body = send(ruth, "POST", "/liturgies/"+lid+"/undo", nil)
	if code != 409 || body["code"] != "undo_refused" || body["reason"] != "changed_since" {
		t.Fatalf("undo after a colleague: %d %v", code, body)
	}
	if s, _ := itemText(); s != "dari Budi" {
		t.Errorf("a refused undo changed the text: %q", s)
	}
	// The refused edit is skipped, so there is nothing else to undo; nor is there anything to redo.
	code, body = send(ruth, "POST", "/liturgies/"+lid+"/undo", nil)
	if code != 409 || body["reason"] != "nothing_to_undo" {
		t.Errorf("second undo: %d %v", code, body)
	}
	code, body = send(ruth, "POST", "/liturgies/"+lid+"/redo", nil)
	if code != 409 || body["code"] != "undo_refused" || body["reason"] != "nothing_to_redo" {
		t.Errorf("redo: %d %v", code, body)
	}

	// Who may not: no session 401, a member without liturgy scopes 404, a commenter 403, an unknown liturgy 404.
	if code, body := send(nil, "POST", "/liturgies/"+lid+"/undo", nil); code != 401 || body["code"] != "unauthenticated" {
		t.Errorf("no session: %d %v", code, body)
	}
	if code, body := send(team, "POST", "/liturgies/"+lid+"/undo", nil); code != 404 {
		t.Errorf("team member: %d %v", code, body)
	}
	if code, body := send(commenter, "POST", "/liturgies/"+lid+"/redo", nil); code != 403 || body["code"] != "forbidden" {
		t.Errorf("commenter: %d %v", code, body)
	}
	if code, _ := send(ruth, "POST", "/liturgies/01ARZ3NDEKTSV4RRFFQ69G5FAV/undo", nil); code != 404 {
		t.Errorf("unknown liturgy: %d", code)
	}
}
