// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"strings"
	"testing"
)

// IT-P-008, IT-P-009, IT-P-010, IT-P-014 over HTTP: the three read routes of the published copy.
func TestReadPublishedHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
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
	list := func(v any) []any { return v.([]any) }

	_, body := send(team, "GET", "/me", nil)
	teamID := obj(body["user"])["id"].(string)
	_, body = send(admin, "GET", "/duties", nil)
	duty := obj(list(body["items"])[0])["id"].(string)

	// Without a session.
	for _, p := range []string{"/published", "/liturgies/01JNOSUCHLITURGY0000000000/published", "/me/assignments"} {
		if code, _ := send(nil, "GET", p, nil); code != 401 {
			t.Errorf("%s without a session: %d", p, code)
		}
	}

	code, body := send(admin, "POST", "/liturgies", map[string]any{"date": "2099-11-01", "time": "09:00", "service_name": "Ibadah", "template_id": ""})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	lid := body["id"].(string)
	path := func(action string) string { return "/liturgies/" + lid + "/" + action }
	if code, body = send(admin, "GET", path("published"), nil); code != 404 {
		t.Errorf("no version, admin: %d %v", code, body)
	}
	if code, body = send(admin, "POST", path("items"), map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "prayer", "duty_id": duty}); code != 201 {
		t.Fatalf("add item: %d %v", code, body)
	}
	if code, body = send(admin, "POST", path("assignments"), map[string]any{"duty_id": duty, "user_id": teamID}); code != 201 {
		t.Fatalf("assign: %d %v", code, body)
	}
	_, body = send(admin, "POST", path("submit"), map[string]any{})
	_, body = send(admin, "POST", path("approve"), map[string]any{"edit_seq": body["edit_seq"]})
	if code, body = send(admin, "POST", path("publish"), map[string]any{"edit_seq": body["edit_seq"]}); code != 200 {
		t.Fatalf("publish: %d %v", code, body)
	}

	// The copy, for a member without any scope, and what they can no longer reach.
	rec := h.do(req{method: "GET", path: "/api/v1" + path("published"), cookies: []*http.Cookie{team}})
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("ETag") != "" {
		t.Fatalf("copy: %d %v", rec.Code, rec.Header())
	}
	got := decode(t, rec)
	content := obj(got["content"])
	items := list(content["items"])
	if got["number"] != 1.0 || got["revising"] != false || got["archived"] != false || obj(got["published_by"])["name"] == "" ||
		content["format"] != 1.0 || obj(content["liturgy"])["church_name"] == "" || len(items) != 1 || obj(items[0])["title"] != "Doa" ||
		obj(obj(items[0])["duty"])["id"] != duty || obj(got["render"])["key_display"] == "" ||
		!strings.HasSuffix(got["url"].(string), "/published/"+lid) {
		t.Errorf("copy: %v", got)
	}
	for _, leak := range []string{"@example.org", "phone", "email"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("the copy mentions %q", leak)
		}
	}
	if code, body := send(team, "GET", "/liturgies/"+lid, nil); code != 404 {
		t.Errorf("team member, editable liturgy: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/liturgies", nil); code != 200 || len(list(body["items"])) != 0 {
		t.Errorf("team member, liturgy list: %d %v", code, body)
	}

	// The published list.
	rec = h.do(req{method: "GET", path: "/api/v1/published", cookies: []*http.Cookie{team}})
	listed := decode(t, rec)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("ETag") != "" || listed["total"] != 1.0 ||
		obj(list(listed["items"])[0])["id"] != lid || obj(list(listed["items"])[0])["revising"] != false {
		t.Errorf("list: %d %v", rec.Code, listed)
	}
	for _, q := range []string{"from=2026-13-01", "to=tomorrow", "archived=maybe", "limit=101", "limit=-1", "offset=-1"} {
		if code, body := send(team, "GET", "/published?"+q, nil); code != 422 {
			t.Errorf("published?%s: %d %v", q, code, body)
		}
	}
	if code, body := send(team, "GET", "/published?limit=100&archived=all&from=2099-01-01&to=2099-12-31", nil); code != 200 || body["total"] != 1.0 {
		t.Errorf("valid parameters: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/published?from=2100-01-01", nil); code != 200 || body["total"] != 0.0 {
		t.Errorf("from after: %d %v", code, body)
	}

	// My assignments.
	rec = h.do(req{method: "GET", path: "/api/v1/me/assignments", cookies: []*http.Cookie{team}})
	mine := decode(t, rec)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("ETag") != "" || mine["more"] != false || len(list(mine["items"])) != 1 {
		t.Fatalf("mine: %d %v", rec.Code, mine)
	}
	card := obj(list(mine["items"])[0])
	if obj(card["liturgy"])["id"] != lid || obj(card["liturgy"])["revising"] != false || len(list(card["duties"])) != 1 || obj(list(card["items"])[0])["title"] != "Doa" {
		t.Errorf("card: %v", card)
	}
	if strings.Contains(rec.Body.String(), "@example.org") {
		t.Error("my assignments mentions an e-mail address")
	}
	if code, body := send(admin, "GET", "/me/assignments", nil); code != 200 || len(list(body["items"])) != 0 {
		t.Errorf("admin has no duties: %d %v", code, body)
	}

	// Reopen and archive are visible on the very next read (no validator, no cache).
	if code, body := send(admin, "POST", path("reopen"), map[string]any{}); code != 200 {
		t.Fatalf("reopen: %d %v", code, body)
	}
	if code, body := send(team, "GET", path("published"), nil); code != 200 || body["revising"] != true || body["number"] != 1.0 {
		t.Errorf("revising: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/me/assignments", nil); code != 200 || obj(list(body["items"])[0])["liturgy"].(map[string]any)["revising"] != true {
		t.Errorf("revising card: %d %v", code, body)
	}
}

// IT-P-016: PATCH /church takes the print settings; the print object is
// complete or refused, and GET /church always shows the defaults filled in.
func TestPrintSettingsHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	patch := func(c *http.Cookie, body any) (int, map[string]any) {
		rec := h.do(req{method: "PATCH", path: "/api/v1/church", cookies: []*http.Cookie{c}, body: body})
		return rec.Code, decode(t, rec)
	}
	code, church := patch(admin, map[string]any{})
	pr, _ := church["print"].(map[string]any)
	if code != 200 || church["show_credits"] != true || church["licence_footer"] != "" || pr["paper"] != "a4" || pr["lyrics"] != "full" ||
		pr["readings"] != true || pr["size"] != "normal" {
		t.Fatalf("defaults: %d %v", code, church)
	}
	full := map[string]any{"paper": "f4", "lyrics": "first_lines", "readings": false, "assignments": true, "keys": true, "notes": false, "size": "large"}
	code, church = patch(admin, map[string]any{"show_credits": false, "licence_footer": "CCLI License #1234567", "print": full})
	pr, _ = church["print"].(map[string]any)
	if code != 200 || church["show_credits"] != false || church["licence_footer"] != "CCLI License #1234567" || pr["paper"] != "f4" || pr["notes"] != false {
		t.Fatalf("set: %d %v", code, church)
	}
	// A sub-key missing, or a value outside the list: 422, nothing changes.
	for name, p := range map[string]map[string]any{
		"missing size": {"paper": "a4", "lyrics": "full", "readings": true, "assignments": true, "keys": true, "notes": true},
		"bad paper":    {"paper": "a3", "lyrics": "full", "readings": true, "assignments": true, "keys": true, "notes": true, "size": "normal"},
		"bad lyrics":   {"paper": "a4", "lyrics": "some", "readings": true, "assignments": true, "keys": true, "notes": true, "size": "normal"},
		"bad size":     {"paper": "a4", "lyrics": "full", "readings": true, "assignments": true, "keys": true, "notes": true, "size": "huge"},
	} {
		if code, _ := patch(admin, map[string]any{"print": p}); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := patch(admin, map[string]any{"licence_footer": strings.Repeat("x", 201)}); code != 422 {
		t.Errorf("footer of 201 characters: %d", code)
	}
	code, church = patch(admin, map[string]any{"licence_footer": ""})
	pr, _ = church["print"].(map[string]any)
	if code != 200 || church["licence_footer"] != "" || church["show_credits"] != false || pr["paper"] != "f4" {
		t.Errorf("clear the footer, keep the rest: %d %v", code, church)
	}
	if code, _ := patch(team, map[string]any{"show_credits": true}); code != 403 {
		t.Errorf("team member: %d", code)
	}
	// A member reads the settings through GET /church.
	if rec := h.get("/api/v1/church", team); rec.Code != 200 || decode(t, rec)["show_credits"] != false {
		t.Errorf("team member reads: %d %s", rec.Code, rec.Body.String())
	}
}

// IT-P-010: the summary route needs liturgy.approve, is never cached, names
// songs and readings but carries no lyrics or Bible text, and shows the change
// summary only once there is a previous version.
func TestPublishedSummaryHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	send := func(c *http.Cookie, method, path string, body any) (int, map[string]any, string) {
		t.Helper()
		var cookies []*http.Cookie
		if c != nil {
			cookies = []*http.Cookie{c}
		}
		rec := h.do(req{method: method, path: "/api/v1" + path, cookies: cookies, body: body})
		if rec.Body.Len() == 0 {
			return rec.Code, nil, ""
		}
		return rec.Code, decode(t, rec), rec.Body.String()
	}
	_, me, _ := send(team, "GET", "/me", nil)
	teamID := me["user"].(map[string]any)["id"].(string)
	_, duties, _ := send(admin, "GET", "/duties", nil)
	duty := duties["items"].([]any)[0].(map[string]any)["id"].(string)

	code, body, _ := send(admin, "POST", "/liturgies", map[string]any{"date": "2099-11-01", "time": "09:00", "service_name": "Ibadah", "template_id": ""})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	lid := body["id"].(string)
	path := func(a string) string { return "/liturgies/" + lid + "/" + a }
	if code, _, _ := send(admin, "GET", path("published/summary"), nil); code != 404 {
		t.Errorf("no version: %d", code)
	}
	if code, body, _ = send(admin, "POST", path("items"), map[string]any{"liturgy_version": 1, "title": "Doa", "item_type": "prayer", "duty_id": duty, "text": "LYRICS-AND-WORDS-MUST-NOT-LEAK"}); code != 201 {
		t.Fatalf("item: %d %v", code, body)
	}
	if code, body, _ = send(admin, "POST", path("assignments"), map[string]any{"duty_id": duty, "user_id": teamID}); code != 201 {
		t.Fatalf("assign: %d %v", code, body)
	}
	publish := func() {
		t.Helper()
		_, b, _ := send(admin, "POST", path("submit"), map[string]any{})
		_, b, _ = send(admin, "POST", path("approve"), map[string]any{"edit_seq": b["edit_seq"]})
		if code, b, _ = send(admin, "POST", path("publish"), map[string]any{"edit_seq": b["edit_seq"]}); code != 200 {
			t.Fatalf("publish: %d %v", code, b)
		}
	}
	publish()

	if code, _, _ := send(nil, "GET", path("published/summary"), nil); code != 401 {
		t.Errorf("no session: %d", code)
	}
	if code, body, _ := send(team, "GET", path("published/summary"), nil); code != 403 {
		t.Errorf("member with no scope: %d %v", code, body)
	}
	rec := h.do(req{method: "GET", path: "/api/v1" + path("published/summary"), cookies: []*http.Cookie{admin}})
	sum := decode(t, rec)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("ETag") != "" {
		t.Fatalf("summary: %d %v", rec.Code, rec.Header())
	}
	items := sum["items"].([]any)
	if sum["number"] != 1.0 || len(items) != 1 || items[0].(map[string]any)["title"] != "Doa" || sum["changes"] != nil ||
		sum["liturgy"].(map[string]any)["service_name"] != "Ibadah" || !strings.HasSuffix(sum["url"].(string), "/published/"+lid) {
		t.Errorf("summary: %v", sum)
	}
	if strings.Contains(rec.Body.String(), "LYRICS-AND-WORDS-MUST-NOT-LEAK") {
		t.Error("the summary carries the text of an item")
	}
	recips := sum["recipients"].([]any)
	if len(recips) != 1 || recips[0].(map[string]any)["name"] != "Member" || recips[0].(map[string]any)["phone"] != nil {
		t.Errorf("recipients: %v", recips)
	}

	// A phone number appears nowhere else.
	for _, p := range []string{path("published"), "/published", "/me/assignments"} {
		if _, _, raw := send(admin, "GET", p, nil); strings.Contains(raw, "phone") {
			t.Errorf("%s mentions a phone number", p)
		}
	}

	// Republished with another item: the summary now compares with version 1.
	send(admin, "POST", path("reopen"), map[string]any{})
	_, cur, _ := send(admin, "GET", "/liturgies/"+lid, nil)
	if code, body, _ = send(admin, "POST", path("items"), map[string]any{"liturgy_version": cur["version"], "title": "Penutup", "item_type": "prayer"}); code != 201 {
		t.Fatalf("second item: %d %v", code, body)
	}
	publish()
	_, sum, _ = send(admin, "GET", path("published/summary"), nil)
	changes, _ := sum["changes"].(map[string]any)
	added := changes["items"].([]any)
	if sum["number"] != 2.0 || len(added) != 1 || added[0].(map[string]any)["kind"] != "added" || added[0].(map[string]any)["title"] != "Penutup" ||
		len(changes["songs"].([]any))+len(changes["reading"].([]any))+len(changes["assignments"].([]any)) != 0 {
		t.Errorf("changes: %v", sum)
	}
}
