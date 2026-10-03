// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"strings"
	"testing"
)

// IT-L-001 to IT-L-005 and IT-L-008 over HTTP: liturgies, items, songs, assignments and history.
func TestLiturgyHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	role := func(name string, scopes ...string) string {
		rec := h.do(req{method: "POST", path: "/api/v1/roles", cookies: []*http.Cookie{admin}, body: map[string]any{"name": name, "scopes": scopes}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("role %s: %d %s", name, rec.Code, rec.Body.String())
		}
		return decode(t, rec)["id"].(string)
	}
	editor := h.invite(admin, "editor@example.org", role("Penyunting", "liturgy.edit"))
	commenter := h.invite(admin, "commenter@example.org", role("Komentator", "liturgy.comment"))

	send := func(c *http.Cookie, method, path string, body any) (int, map[string]any) {
		t.Helper()
		rec := h.do(req{method: method, path: "/api/v1" + path, cookies: []*http.Cookie{c}, body: body})
		if rec.Code == http.StatusNoContent {
			return rec.Code, nil
		}
		return rec.Code, decode(t, rec)
	}
	items := func(b map[string]any) []any { return b["items"].([]any) }
	obj := func(v any) map[string]any { return v.(map[string]any) }

	// Setup of the planning rows: the seeded template and a service with it.
	_, body := send(admin, "GET", "/templates", nil)
	tpl := obj(items(body)[0])["id"].(string)
	_, body = send(admin, "GET", "/duties", nil)
	duty := obj(items(body)[0])["id"].(string)
	code, body := send(admin, "POST", "/services", map[string]any{"name": "Ibadah Umum", "default_template_id": tpl,
		"times": []any{map[string]any{"weekday": 7, "time": "07:00"}, map[string]any{"weekday": 7, "time": "09:00"}}})
	if code != 201 {
		t.Fatalf("service: %d %v", code, body)
	}
	svc := body["id"].(string)

	// Prepare a week: list, create two, the slots are then flagged.
	code, body = send(editor, "GET", "/liturgies/prepare?week=2026-10-14", nil)
	if code != 200 || body["week"] != "2026-10-12" || len(body["occurrences"].([]any)) != 2 || obj(body["occurrences"].([]any)[0])["liturgy_id"] != nil ||
		obj(obj(body["limits"])["max_active_liturgies"])["unlimited"] != true {
		t.Fatalf("prepare week: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies/prepare", nil); code != 403 || body["code"] != "forbidden" {
		t.Errorf("team prepares: %d %v", code, body)
	}
	if code, body := send(editor, "GET", "/liturgies/prepare?week=2026-02-30", nil); code != 422 {
		t.Errorf("bad week: %d %v", code, body)
	}
	code, body = send(editor, "POST", "/liturgies/prepare", map[string]any{"occurrences": []any{
		map[string]any{"service_id": svc, "date": "2026-10-18", "time": "07:00"}, map[string]any{"service_id": svc, "date": "2026-10-18", "time": "09:00"}}})
	if code != 201 || len(items(body)) != 2 || obj(items(body)[0])["template_name"] != "Ibadah Minggu" {
		t.Fatalf("prepare: %d %v", code, body)
	}
	first := obj(items(body)[0])["liturgy_id"].(string)
	if code, body := send(editor, "POST", "/liturgies/prepare", map[string]any{"occurrences": []any{
		map[string]any{"service_id": svc, "date": "2026-10-18", "time": "07:00"}}}); code != 409 || body["code"] != "liturgy_exists" || body["liturgy_id"] != first {
		t.Errorf("prepare an existing slot: %d %v", code, body)
	}
	if code, body := send(editor, "POST", "/liturgies/prepare", map[string]any{"occurrences": []any{
		map[string]any{"service_id": svc, "date": "2026-10-19", "time": "07:00"}}}); code != 422 {
		t.Errorf("prepare an invented occurrence: %d %v", code, body)
	}

	// Visibility: lists and reads.
	code, body = send(editor, "GET", "/liturgies", nil)
	if code != 200 || body["total"] != 2.0 || obj(items(body)[0])["item_count"] != 7.0 || obj(obj(items(body)[0])["actions"])["edit"] != true ||
		obj(items(body)[0])["time"] != "09:00" { // newest date first, then later time first
		t.Fatalf("editor's list: %d %v", code, body)
	}
	if code, body := send(editor, "GET", "/liturgies?order=date_asc&limit=1&offset=1&from=2026-10-18&to=2026-10-18", nil); code != 200 || body["total"] != 2.0 ||
		len(items(body)) != 1 || obj(items(body)[0])["time"] != "09:00" {
		t.Errorf("paging: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies", nil); code != 200 || body["total"] != 0.0 || len(items(body)) != 0 {
		t.Errorf("team member's list: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies/"+first, nil); code != 404 || body["code"] != "not_found" {
		t.Errorf("team member reads: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies/"+first+"/edits", nil); code != 404 {
		t.Errorf("team member reads the history: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies/assignable", nil); code != 403 || body["code"] != "forbidden" {
		t.Errorf("team member lists assignable: %d %v", code, body)
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/liturgies"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	code, body = send(commenter, "GET", "/liturgies/"+first, nil)
	if code != 200 || body["version"] != 1.0 || body["state"] != "draft" || len(body["items"].([]any)) != 7 ||
		obj(body["actions"])["edit"] != false || body["service_id"] != svc || body["template_id"] != tpl {
		t.Fatalf("commenter reads: %d %v", code, body)
	}
	if code, body := send(commenter, "POST", "/liturgies/"+first+"/items", map[string]any{"liturgy_version": 1, "title": "x", "item_type": "prayer"}); code != 403 || body["code"] != "forbidden" {
		t.Errorf("commenter writes: %d %v", code, body)
	}
	if code, body := send(commenter, "POST", "/liturgies", map[string]any{"date": "2026-10-25", "service_name": "X"}); code != 403 {
		t.Errorf("commenter creates: %d %v", code, body)
	}

	// Create: the slot, the fields, unknown fields.
	if code, body := send(editor, "POST", "/liturgies", map[string]any{"date": "2026-10-18", "time": "07:00", "service_id": svc}); code != 409 ||
		body["code"] != "liturgy_exists" || body["liturgy_id"] != first {
		t.Errorf("same slot: %d %v", code, body)
	}
	for name, b := range map[string]map[string]any{
		"bad date": {"date": "2026-02-30", "service_name": "X"}, "no name": {"date": "2026-10-25"},
		"unknown field": {"date": "2026-10-25", "service_name": "X", "state": "published"},
	} {
		if code, body := send(editor, "POST", "/liturgies", b); code != 422 || body["code"] != "validation_failed" {
			t.Errorf("create with %s: %d %v", name, code, body)
		}
	}
	code, body = send(editor, "POST", "/liturgies", map[string]any{"date": "2026-10-25", "service_name": "Doa Malam", "template_id": ""})
	if code != 201 || len(body["items"].([]any)) != 0 || body["service_id"] != nil || body["language"] != "id" || body["actions"].(map[string]any)["delete"] != false {
		t.Fatalf("one-off: %d %v", code, body)
	}
	one := body["id"].(string)

	// Items: add, versions, conflicts.
	code, body = send(editor, "POST", "/liturgies/"+one+"/items", map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "prayer", "duty_id": duty, "text": "Bapa kami"})
	if code != 201 || body["liturgy_version"] != 2.0 || obj(body["item"])["version"] != 1.0 || obj(body["item"])["duty_id"] != duty || obj(body["item"])["position"] != 0.0 {
		t.Fatalf("add item: %d %v", code, body)
	}
	prayer := obj(body["item"])["id"].(string)
	if code, body := send(editor, "POST", "/liturgies/"+one+"/items", map[string]any{"liturgy_version": 1, "title": "Lain", "item_type": "prayer"}); code != 409 ||
		body["code"] != "version_conflict" || body["scope"] != "liturgy" {
		t.Errorf("stale add: %d %v", code, body)
	}
	code, body = send(editor, "POST", "/liturgies/"+one+"/items", map[string]any{"liturgy_version": 2, "title": "Lagu", "item_type": "song", "position": 0})
	if code != 201 || obj(body["item"])["position"] != 0.0 {
		t.Fatalf("add song item: %d %v", code, body)
	}
	songItem := obj(body["item"])["id"].(string)
	if code, body := send(editor, "PATCH", "/liturgies/"+one+"/items/"+prayer, map[string]any{"version": 1, "title": "Doa penutup"}); code != 200 ||
		obj(body["item"])["version"] != 2.0 || body["liturgy_version"] != 3.0 {
		t.Errorf("update item: %d %v", code, body)
	}
	if code, body := send(editor, "PATCH", "/liturgies/"+one+"/items/"+prayer, map[string]any{"version": 1, "title": "Basi"}); code != 409 ||
		body["scope"] != "item" || body["item_id"] != prayer {
		t.Errorf("stale item update: %d %v", code, body)
	}
	if code, body := send(editor, "PATCH", "/liturgies/"+one+"/items/"+prayer, map[string]any{"version": 2, "item_type": "song"}); code != 422 {
		t.Errorf("an item's type can't change: %d %v", code, body)
	}
	if code, body := send(editor, "PATCH", "/liturgies/"+one+"/items/"+songItem, map[string]any{"version": 1, "text": "lirik"}); code != 422 || body["code"] != "validation_failed" {
		t.Errorf("text on a song: %d %v", code, body)
	}

	// Songs: add with a filled sequence, then replace; no lyrics in the answer.
	code, body = send(admin, "POST", "/songs", map[string]any{"language": "id", "title": "Besar Setia-Mu", "default_key": "G", "sections": []any{
		map[string]any{"key": "v1", "kind": "verse", "number": 1, "text": "RAHASIA bait satu"}, map[string]any{"key": "c", "kind": "chorus", "text": "RAHASIA reff"}},
		"default_arrangement": []string{"v1", "c", "c"}})
	if code != 201 {
		t.Fatalf("song: %d %v", code, body)
	}
	song := body["id"].(string)
	secs := body["sections"].([]any)
	rec := h.do(req{method: "POST", path: "/api/v1/liturgies/" + one + "/items/" + songItem + "/songs", cookies: []*http.Cookie{editor},
		body: map[string]any{"version": 1, "song_id": song}})
	if rec.Code != 201 || strings.Contains(rec.Body.String(), "RAHASIA") {
		t.Fatalf("add song: %d %s", rec.Code, rec.Body.String())
	}
	body = decode(t, rec)
	s0 := obj(obj(body["item"])["songs"].([]any)[0])
	if len(s0["entries"].([]any)) != 3 || s0["key"] != "G" || obj(s0["song"])["title"] != "Besar Setia-Mu" || obj(obj(s0["song"])["sections"].([]any)[0])["id"] != obj(secs[0])["id"] ||
		obj(s0["entries"].([]any)[1])["section_label"] != "Chorus" || obj(body["item"])["version"] != 2.0 {
		t.Errorf("sequence: %v", s0)
	}
	rec = h.do(req{method: "PUT", path: "/api/v1/liturgies/" + one + "/items/" + songItem + "/songs", cookies: []*http.Cookie{editor},
		body: map[string]any{"version": 2, "songs": []any{map[string]any{"song_id": song, "key": "Bb", "note": "pelan", "entries": []any{
			map[string]any{"section_id": obj(secs[0])["id"], "key_change": "C", "note": "2x"}, map[string]any{"section_id": obj(secs[1])["id"]}}}}}})
	body = decode(t, rec)
	if rec.Code != 200 || obj(body["item"])["version"] != 3.0 || obj(obj(obj(body["item"])["songs"].([]any)[0])["entries"].([]any)[0])["key_change"] != "C" {
		t.Errorf("put songs: %d %v", rec.Code, body)
	}
	if code, body := send(editor, "PUT", "/liturgies/"+one+"/items/"+songItem+"/songs", map[string]any{"version": 3, "songs": []any{
		map[string]any{"song_id": song, "entries": []any{map[string]any{"section_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV"}}}}}); code != 422 {
		t.Errorf("a section of no song: %d %v", code, body)
	}
	if code, body := send(admin, "DELETE", "/songs/"+song, nil); code != 409 || body["code"] != "song_in_use" {
		t.Errorf("delete a song in a liturgy: %d %v", code, body)
	}

	// Reorder and remove; the answer lists the order.
	code, body = send(editor, "PUT", "/liturgies/"+one+"/items/order", map[string]any{"liturgy_version": 3, "item_ids": []string{prayer, songItem}})
	if code != 200 || body["liturgy_version"] != 4.0 || body["item_ids"].([]any)[0] != prayer {
		t.Fatalf("reorder: %d %v", code, body)
	}
	if code, body := send(editor, "PUT", "/liturgies/"+one+"/items/order", map[string]any{"liturgy_version": 4, "item_ids": []string{prayer}}); code != 409 || body["scope"] != "liturgy" {
		t.Errorf("reorder with a missing item: %d %v", code, body)
	}
	if code, body := send(editor, "DELETE", "/liturgies/"+one+"/items/"+songItem+"?liturgy_version=3", nil); code != 409 || body["scope"] != "liturgy" {
		t.Errorf("stale remove: %d %v", code, body)
	}
	if code, body := send(editor, "DELETE", "/liturgies/"+one+"/items/"+songItem, nil); code != 422 {
		t.Errorf("remove without a version: %d %v", code, body)
	}
	code, body = send(editor, "DELETE", "/liturgies/"+one+"/items/"+songItem+"?liturgy_version=4", nil)
	if code != 200 || body["liturgy_version"] != 5.0 || len(body["item_ids"].([]any)) != 1 {
		t.Errorf("remove: %d %v", code, body)
	}
	if code, _ := send(admin, "DELETE", "/songs/"+song, nil); code != 204 {
		t.Errorf("delete the song after its item is gone: %d", code)
	}

	// The liturgy's own fields.
	if code, body := send(editor, "PATCH", "/liturgies/"+one, map[string]any{"version": 5, "service_name": "Doa Pagi", "date": "2026-10-26"}); code != 200 ||
		body["version"] != 6.0 || body["service_name"] != "Doa Pagi" || body["date"] != "2026-10-26" {
		t.Errorf("update liturgy: %d %v", code, body)
	}
	if code, body := send(editor, "PATCH", "/liturgies/"+one, map[string]any{"version": 5, "service_name": "Basi"}); code != 409 || body["scope"] != "liturgy" {
		t.Errorf("stale liturgy update: %d %v", code, body)
	}

	// Assignments: the picker, add, duplicate, remove.
	code, body = send(editor, "GET", "/liturgies/assignable", nil)
	if code != 200 || len(items(body)) != 4 || len(obj(items(body)[0])) != 2 {
		t.Fatalf("assignable: %d %v", code, body)
	}
	member := obj(items(body)[0])["user_id"].(string)
	code, body = send(editor, "POST", "/liturgies/"+one+"/assignments", map[string]any{"duty_id": duty, "user_id": member})
	if code != 201 || body["user_id"] != member || body["former_member"] != false || body["name"] == "" {
		t.Fatalf("assign: %d %v", code, body)
	}
	assignment := body["id"].(string)
	if code, body := send(editor, "POST", "/liturgies/"+one+"/assignments", map[string]any{"duty_id": duty, "user_id": member}); code != 409 || body["code"] != "assignment_exists" {
		t.Errorf("assign twice: %d %v", code, body)
	}
	if code, body := send(editor, "POST", "/liturgies/"+one+"/assignments", map[string]any{"duty_id": duty, "name": "Pak Yan"}); code != 201 || body["user_id"] != nil || body["name"] != "Pak Yan" {
		t.Errorf("assign a name: %d %v", code, body)
	}
	if code, body := send(editor, "POST", "/liturgies/"+one+"/assignments", map[string]any{"duty_id": duty, "user_id": member, "name": "X"}); code != 422 {
		t.Errorf("both: %d %v", code, body)
	}
	if code, _ := send(editor, "DELETE", "/liturgies/"+one+"/assignments/"+assignment, nil); code != 204 {
		t.Errorf("unassign: %d", code)
	}
	if code, body := send(editor, "DELETE", "/liturgies/"+one+"/assignments/"+assignment, nil); code != 404 {
		t.Errorf("unassign twice: %d %v", code, body)
	}

	// History: newest first, one row per change.
	code, body = send(commenter, "GET", "/liturgies/"+one+"/edits?limit=3", nil)
	if code != 200 || len(items(body)) != 3 || obj(items(body)[0])["command"] != "assignment.remove" || obj(items(body)[0])["user_name"] != "Member" {
		t.Fatalf("history: %d %v", code, body)
	}
	code, body = send(commenter, "GET", "/liturgies/"+one+"/edits", nil)
	all := items(body)
	if last := obj(all[len(all)-1]); last["command"] != "liturgy.create" || last["seq"] != 1.0 || last["before"] != nil || last["after"] == nil {
		t.Errorf("first row: %v", last)
	}

	// Delete needs liturgy.manage, and the liturgy goes with its rows.
	if code, body := send(editor, "DELETE", "/liturgies/"+one, nil); code != 403 || body["code"] != "forbidden" {
		t.Errorf("editor deletes: %d %v", code, body)
	}
	if code, _ := send(admin, "DELETE", "/liturgies/"+one, nil); code != 204 {
		t.Errorf("admin deletes: %d", code)
	}
	if code, body := send(admin, "GET", "/liturgies/"+one, nil); code != 404 || body["code"] != "not_found" {
		t.Errorf("after delete: %d %v", code, body)
	}
}
