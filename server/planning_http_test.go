// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/http"
	"testing"
)

// IT-P-001 to IT-P-005 over HTTP: the planning setup, its error codes and its seeded defaults.
func TestPlanningHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")

	send := func(c *http.Cookie, method, path string, body any) (int, map[string]any) {
		t.Helper()
		rec := h.do(req{method: method, path: "/api/v1" + path, cookies: []*http.Cookie{c}, body: body})
		if rec.Code == http.StatusNoContent {
			return rec.Code, nil
		}
		return rec.Code, decode(t, rec)
	}
	items := func(body map[string]any) []any { return body["items"].([]any) }

	// The church was seeded at setup: any member sees the duties and parts, in order.
	code, body := send(team, "GET", "/duties", nil)
	duties := items(body)
	if code != 200 || len(duties) != 7 || duties[0].(map[string]any)["name"] != "Liturgis" || duties[0].(map[string]any)["position"] != 0.0 ||
		duties[0].(map[string]any)["actions"].(map[string]any)["edit"] != false {
		t.Fatalf("team duties: %d %v", code, body)
	}
	if code, body := send(team, "GET", "/singing-parts", nil); code != 200 || len(items(body)) != 6 {
		t.Errorf("team parts: %d %v", code, body)
	}
	// Templates and services need templates.edit or liturgy.edit.
	for _, path := range []string{"/templates", "/services"} {
		if code, body := send(team, "GET", path, nil); code != 403 || body["code"] != "forbidden" {
			t.Errorf("team GET %s: %d %v", path, code, body)
		}
	}
	for _, c := range []req{
		{method: "POST", path: "/duties", body: map[string]any{"name": "X"}},
		{method: "PATCH", path: "/duties/" + duties[0].(map[string]any)["id"].(string), body: map[string]any{"name": "X"}},
		{method: "PUT", path: "/duties/order", body: map[string]any{"ids": []string{}}},
		{method: "DELETE", path: "/singing-parts/" + duties[0].(map[string]any)["id"].(string)},
		{method: "POST", path: "/templates", body: map[string]any{"name": "X", "items": []any{}}},
		{method: "POST", path: "/services", body: map[string]any{"name": "X", "times": []any{map[string]any{"weekday": 1, "time": "07:00"}}}},
	} {
		c.path = "/api/v1" + c.path
		c.cookies = []*http.Cookie{team}
		if rec := h.do(c); rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s %s by team member: %d %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	// No session: 401.
	if rec := h.do(req{method: "GET", path: "/api/v1/duties"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}

	// Duties: create, name_taken, rename, reorder, delete.
	code, body = send(admin, "POST", "/duties", map[string]any{"name": "Penerima Tamu"})
	if code != 201 || body["position"] != 7.0 || body["actions"].(map[string]any)["delete"] != true {
		t.Fatalf("create duty: %d %v", code, body)
	}
	newDuty := body["id"].(string)
	if code, body := send(admin, "POST", "/duties", map[string]any{"name": "penerima  tamu"}); code != 409 || body["code"] != "name_taken" || body["reason"] != "duty" {
		// "penerima  tamu" folds to the same key as "Penerima Tamu"
		t.Errorf("duplicate duty: %d %v", code, body)
	}
	if code, body := send(admin, "POST", "/singing-parts", map[string]any{"name": "Liturgis"}); code != 201 {
		t.Errorf("the same word as a part is fine: %d %v", code, body) // lists are separate
	}
	if code, body := send(admin, "PATCH", "/duties/"+newDuty, map[string]any{"name": "Usher"}); code != 200 || body["name"] != "Usher" {
		t.Errorf("rename: %d %v", code, body)
	}
	if code, body := send(admin, "PUT", "/duties/order", map[string]any{"ids": []string{newDuty}}); code != 409 || body["code"] != "version_conflict" {
		t.Errorf("reorder with a missing ID: %d %v", code, body)
	}
	ids := []string{newDuty}
	for _, d := range duties {
		ids = append(ids, d.(map[string]any)["id"].(string))
	}
	if code, body := send(admin, "PUT", "/duties/order", map[string]any{"ids": ids}); code != 200 || items(body)[0].(map[string]any)["name"] != "Usher" || items(body)[7].(map[string]any)["position"] != 7.0 {
		t.Errorf("reorder: %d %v", code, body)
	}
	if code, _ := send(admin, "DELETE", "/duties/"+newDuty, nil); code != 204 {
		t.Errorf("delete duty: %d", code)
	}
	if code, body := send(admin, "DELETE", "/duties/"+newDuty, nil); code != 404 || body["code"] != "not_found" {
		t.Errorf("delete twice: %d %v", code, body)
	}
	if code, body := send(admin, "POST", "/duties", map[string]any{"name": ""}); code != 422 || body["code"] != "validation_failed" {
		t.Errorf("empty name: %d %v", code, body)
	}

	// The limit has its own reason and numbers.
	for i := 7; i < 50; i++ {
		if code, body := send(admin, "POST", "/duties", map[string]any{"name": fmt.Sprintf("Duty %d", i)}); code != 201 {
			t.Fatalf("fill %d: %d %v", i, code, body)
		}
	}
	code, body = send(admin, "POST", "/duties", map[string]any{"name": "One too many"})
	if code != 422 || body["code"] != "validation_failed" || body["reason"] != "limit" || body["max"] != 50.0 || body["used"] != 50.0 {
		t.Errorf("limit: %d %v", code, body)
	}

	// Templates: the seeded one, create, version conflict, template_in_use.
	code, body = send(admin, "GET", "/templates", nil)
	if code != 200 || len(items(body)) != 1 || items(body)[0].(map[string]any)["name"] != "Ibadah Minggu" || items(body)[0].(map[string]any)["item_count"] != 7.0 {
		t.Fatalf("templates: %d %v", code, body)
	}
	seeded := items(body)[0].(map[string]any)["id"].(string)
	code, body = send(admin, "GET", "/templates/"+seeded, nil)
	first := body["items"].([]any)[0].(map[string]any)
	if code != 200 || first["title"] != "Votum dan Salam" || first["item_type"] != "free_text" || first["default_duty_id"] != duties[0].(map[string]any)["id"] ||
		body["items"].([]any)[1].(map[string]any)["default_duty_id"] == nil {
		t.Errorf("seeded template: %d %v", code, body)
	}
	code, body = send(admin, "POST", "/templates", map[string]any{"name": "Doa Malam", "language": "id", "items": []any{
		map[string]any{"title": "Lagu", "item_type": "song"}, map[string]any{"title": "Doa", "item_type": "prayer", "default_text": "Bapa kami"}}})
	if code != 201 || body["version"] != 1.0 || len(body["items"].([]any)) != 2 || body["items"].([]any)[0].(map[string]any)["default_duty_id"] != nil {
		t.Fatalf("create template: %d %v", code, body)
	}
	tpl := body["id"].(string)
	if code, body := send(admin, "POST", "/templates", map[string]any{"name": "doa malam", "items": []any{}}); code != 409 || body["code"] != "name_taken" || body["reason"] != "template" {
		t.Errorf("duplicate template: %d %v", code, body)
	}
	if code, body := send(admin, "POST", "/templates", map[string]any{"name": "Salah", "items": []any{map[string]any{"title": "X", "item_type": "song", "default_text": "words"}}}); code != 422 || body["code"] != "validation_failed" {
		t.Errorf("text on a song item: %d %v", code, body)
	}
	if code, body := send(admin, "POST", "/templates", map[string]any{"name": "Salah", "items": []any{map[string]any{"title": "X", "item_type": "dance"}}}); code != 422 {
		t.Errorf("unknown item type: %d %v", code, body)
	}
	if code, body := send(admin, "PATCH", "/templates/"+tpl, map[string]any{"version": 1, "name": "Doa Malam Baru"}); code != 200 || body["version"] != 2.0 || len(body["items"].([]any)) != 2 {
		t.Errorf("rename template keeps the items: %d %v", code, body)
	}
	if code, body := send(admin, "PATCH", "/templates/"+tpl, map[string]any{"version": 1, "name": "Basi"}); code != 409 || body["code"] != "version_conflict" {
		t.Errorf("stale template: %d %v", code, body)
	}

	// Services: times, default template, template_in_use.
	code, body = send(admin, "POST", "/services", map[string]any{"name": "Ibadah Umum", "default_template_id": tpl, "times": []any{
		map[string]any{"weekday": 7, "time": "09:00"}, map[string]any{"weekday": 7, "time": "07:00"}}})
	if code != 201 || body["default_template_name"] != "Doa Malam Baru" || body["language"] != "id" ||
		body["times"].([]any)[0].(map[string]any)["time"] != "07:00" {
		t.Fatalf("create service: %d %v", code, body)
	}
	svc := body["id"].(string)
	for name, times := range map[string][]any{
		"none": {}, "bad weekday": {map[string]any{"weekday": 8, "time": "07:00"}}, "bad time": {map[string]any{"weekday": 1, "time": "7:00"}},
	} {
		if code, body := send(admin, "POST", "/services", map[string]any{"name": "Lain " + name, "times": times}); code != 422 || body["code"] != "validation_failed" {
			t.Errorf("times %s: %d %v", name, code, body)
		}
	}
	if code, body := send(admin, "PATCH", "/services/"+svc, map[string]any{"version": 1, "times": []any{}}); code != 422 || body["reason"] != "required" {
		t.Errorf("empty times: %d %v", code, body)
	}
	if code, body := send(admin, "PATCH", "/services/"+svc, map[string]any{"version": 1, "name": "Ibadah Raya"}); code != 200 || len(body["times"].([]any)) != 2 {
		t.Errorf("rename keeps the times: %d %v", code, body)
	}
	if code, body := send(admin, "DELETE", "/templates/"+tpl, nil); code != 409 || body["code"] != "template_in_use" || body["service_ids"].([]any)[0] != svc {
		t.Errorf("template in use: %d %v", code, body)
	}
	if code, body := send(admin, "PATCH", "/services/"+svc, map[string]any{"version": 2, "default_template_id": ""}); code != 200 || body["default_template_id"] != nil {
		t.Errorf("clear the default template: %d %v", code, body)
	}
	if code, _ := send(admin, "DELETE", "/templates/"+tpl, nil); code != 204 {
		t.Errorf("delete the template: %d", code)
	}
	if code, _ := send(admin, "DELETE", "/services/"+svc, nil); code != 204 {
		t.Errorf("delete the service: %d", code)
	}
}
