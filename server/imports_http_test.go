// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// IT-I-001, IT-I-004, IT-I-005 and IT-I-009 over HTTP.
func TestImportsHTTP(t *testing.T) {
	h := harnessWith(t)
	admin := h.setupChurch()
	team := h.invite(admin, "team@example.org")
	post := func(c *http.Cookie, path string, body any) *httptest.ResponseRecorder {
		return h.do(req{method: "POST", path: path, cookies: []*http.Cookie{c}, body: body})
	}
	create := func(c *http.Cookie, body any) map[string]any {
		t.Helper()
		rec := post(c, "/api/v1/imports", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /imports: %d %s", rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}
	paste := func(title, text string) map[string]any {
		return create(admin, map[string]any{"format": "paste", "files": []map[string]string{{"name": title, "text": text}}})
	}

	// IT-I-001: create, review, apply; nothing in the library before Apply.
	b := paste("Besar Setia-Mu", "1. Besar setia-Mu\n\nReff\nTuhan setia\n\n2. Pagi demi pagi\n\nReff")
	id := b["id"].(string)
	cands := b["candidates"].([]any)
	if b["status"] != "open" || b["source_format"] != "paste" || len(cands) != 1 || len(b["rejected"].([]any)) != 0 {
		t.Fatalf("batch: %v", b)
	}
	c := cands[0].(map[string]any)
	cid := c["id"].(string)
	draft := c["draft"].(map[string]any)
	if draft["title"] != "Besar Setia-Mu" || draft["language"] != "id" || len(draft["sections"].([]any)) != 3 ||
		len(draft["default_arrangement"].([]any)) != 4 || c["decision"] != "pending" || c["duplicate_of"] != nil {
		t.Errorf("candidate: %v", c)
	}
	if rec := h.get("/api/v1/songs", admin); decode(t, rec)["total"] != 0.0 {
		t.Errorf("songs before apply: %s", rec.Body.String())
	}
	if rec := h.get("/api/v1/imports", admin); len(decode(t, rec)["items"].([]any)) != 1 {
		t.Errorf("open imports: %s", rec.Body.String())
	}
	if rec := h.get("/api/v1/imports/"+id, admin); rec.Code != 200 || decode(t, rec)["id"] != id {
		t.Errorf("GET: %d %s", rec.Code, rec.Body.String())
	}

	// A draft edit goes through the song rules.
	patch := func(path string, body any) *httptest.ResponseRecorder {
		return h.do(req{method: "PATCH", path: path, cookies: []*http.Cookie{admin}, body: body})
	}
	bad := cloneMap(draft)
	bad["title"] = ""
	if rec := patch("/api/v1/imports/"+id+"/candidates/"+cid, map[string]any{"draft": bad}); rec.Code != 422 || problemCode(t, rec)["code"] != "validation_failed" {
		t.Errorf("empty title: %d %s", rec.Code, rec.Body.String())
	}
	good := cloneMap(draft)
	good["lyricist"] = "Thomas Chisholm"
	if rec := patch("/api/v1/imports/"+id+"/candidates/"+cid, map[string]any{"draft": good}); rec.Code != 200 ||
		decode(t, rec)["draft"].(map[string]any)["lyricist"] != "Thomas Chisholm" {
		t.Errorf("edit: %d %s", rec.Code, rec.Body.String())
	}

	// IT-I-004: decisions are checked.
	for name, d := range map[string]map[string]any{
		"merge without target":  {"id": cid, "decision": "merge", "merge_target_version": 1},
		"merge without version": {"id": cid, "decision": "merge", "merge_into": "01JNOSUCHSONG000000000000"},
		"unknown decision":      {"id": cid, "decision": "maybe"},
	} {
		if rec := patch("/api/v1/imports/"+id+"/candidates", map[string]any{"decisions": []any{d}}); rec.Code != 422 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := patch("/api/v1/imports/"+id+"/candidates", map[string]any{"decisions": []any{
		map[string]any{"id": cid, "decision": "merge", "merge_into": "01JNOSUCHSONG000000000000", "merge_target_version": 1}}}); rec.Code != 404 {
		t.Errorf("merge into an unknown song: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/imports/"+id+"/candidates/"+cid+"/merge-preview?merge_into=01JNOSUCHSONG000000000000", admin); rec.Code != 404 {
		t.Errorf("preview of an unknown song: %d", rec.Code)
	}
	rec := patch("/api/v1/imports/"+id+"/candidates", map[string]any{"decisions": []any{map[string]any{"id": cid, "decision": "accept"}}})
	if rec.Code != 200 || decode(t, rec)["candidates"].([]any)[0].(map[string]any)["decision"] != "accept" {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body.String())
	}
	rec = post(admin, "/api/v1/imports/"+id+"/apply", nil)
	if r := decode(t, rec); rec.Code != 200 || r["created"] != 1.0 || r["merged"] != 0.0 || r["status"] != "closed" || len(r["failed"].([]any)) != 0 {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.get("/api/v1/songs?q="+"besar+setia", admin); decode(t, rec)["total"] != 1.0 {
		t.Errorf("song after apply: %s", rec.Body.String())
	}
	if rec := h.get("/api/v1/imports", admin); len(decode(t, rec)["items"].([]any)) != 0 {
		t.Errorf("a closed batch is not listed: %s", rec.Body.String())
	}
	// An applied candidate cannot be changed any more.
	rec = patch("/api/v1/imports/"+id+"/candidates/"+cid, map[string]any{"draft": good})
	if p := problemCode(t, rec); rec.Code != http.StatusConflict || p["code"] != "import_conflict" || p["reason"] != "already_applied" {
		t.Errorf("edit after apply: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(admin, "/api/v1/imports/"+id+"/apply", nil); decode(t, rec)["created"] != 0.0 {
		t.Errorf("second apply: %s", rec.Body.String())
	}

	// IT-I-009: a multi-song file with one malformed song.
	multi := create(admin, map[string]any{"format": "chordpro", "files": []map[string]string{{"name": "set.cho",
		"text": "{title: Satu}\nlirik satu\n{new_song}\n{title: Rusak}\n{capo: 1}\n{new_song}\n{title: Tiga}\nlirik tiga\n"}}})
	rej := multi["rejected"].([]any)
	if len(multi["candidates"].([]any)) != 2 || len(rej) != 1 || rej[0].(map[string]any)["song_index"] != 1.0 ||
		rej[0].(map[string]any)["reason"] != "no_song" || rej[0].(map[string]any)["name"] != "set.cho" {
		t.Errorf("multi-song: %v", multi)
	}

	// OpenLyrics over HTTP, and a hostile DOCTYPE.
	ol := create(admin, map[string]any{"format": "openlyrics", "files": []map[string]string{{"name": "a.xml", "text": `<song><properties>
		<titles><title>Grace</title></titles></properties><lyrics><verse name="v1"><lines>Amazing<br/>grace</lines></verse></lyrics></song>`}}})
	if c := ol["candidates"].([]any)[0].(map[string]any); c["draft"].(map[string]any)["sections"].([]any)[0].(map[string]any)["text"] != "Amazing\ngrace" {
		t.Errorf("openlyrics: %v", ol)
	}
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "openlyrics", "files": []map[string]string{{"name": "x.xml",
		"text": `<!DOCTYPE s [<!ENTITY e SYSTEM "file:///etc/passwd">]><song><properties><titles><title>&e;</title></titles></properties></song>`}}})
	if p := problemCode(t, rec); rec.Code != 422 || p["code"] != "import_unreadable" || p["reason"] != "not_xml" {
		t.Errorf("entity: %d %s", rec.Code, rec.Body.String())
	}

	// IT-I-005: permissions, including reading.
	for _, r := range []req{
		{method: "POST", path: "/api/v1/imports", body: map[string]any{"format": "paste", "files": []map[string]string{{"name": "x", "text": "y"}}}},
		{method: "GET", path: "/api/v1/imports"},
		{method: "GET", path: "/api/v1/imports/" + id},
		{method: "PATCH", path: "/api/v1/imports/" + id + "/candidates/" + cid, body: map[string]any{"draft": good}},
		{method: "PATCH", path: "/api/v1/imports/" + id + "/candidates", body: map[string]any{"decisions": []any{}}},
		{method: "GET", path: "/api/v1/imports/" + id + "/candidates/" + cid + "/merge-preview?merge_into=x"},
		{method: "POST", path: "/api/v1/imports/" + id + "/apply"},
		{method: "DELETE", path: "/api/v1/imports/" + id},
	} {
		r.cookies = []*http.Cookie{team}
		if rec := h.do(r); rec.Code != http.StatusForbidden || problemCode(t, rec)["code"] != "forbidden" {
			t.Errorf("%s %s by a member without library.edit: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	// Unknown batches are 404.
	if rec := h.get("/api/v1/imports/01JNOSUCHBATCH00000000000", admin); rec.Code != 404 {
		t.Errorf("unknown batch: %d", rec.Code)
	}

	// Limits: body size, number of files, file size, complexity.
	files := func(n int) []map[string]string {
		out := make([]map[string]string, n)
		for i := range out {
			out[i] = map[string]string{"name": fmt.Sprintf("%d.cho", i), "text": "{title: T}\nx"}
		}
		return out
	}
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "chordpro", "files": []map[string]string{{"name": "big", "text": strings.Repeat("x", 6<<20)}}})
	if p := problemCode(t, rec); rec.Code != http.StatusRequestEntityTooLarge || p["code"] != "validation_failed" {
		t.Errorf("body over 6 MiB: %d %.200s", rec.Code, rec.Body.String())
	}
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "chordpro", "files": files(201)})
	if p := problemCode(t, rec); rec.Code != 422 || p["code"] != "validation_failed" {
		t.Errorf("201 files: %d %.200s", rec.Code, rec.Body.String())
	}
	// A body over 1 MiB is fine for imports and refused elsewhere.
	big := strings.Repeat("x", 1<<20+1)
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "chordpro", "files": append(files(1), map[string]string{"name": "big.cho", "text": big})})
	if r := decode(t, rec); rec.Code != http.StatusCreated || len(r["candidates"].([]any)) != 1 ||
		r["rejected"].([]any)[0].(map[string]any)["reason"] != "file_too_large" {
		t.Errorf("a 1 MiB + 1 file among valid ones: %d %.300s", rec.Code, rec.Body.String())
	}
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "chordpro", "files": []map[string]string{{"name": "big.cho", "text": big}}})
	if p := problemCode(t, rec); rec.Code != 422 || p["code"] != "import_unreadable" || p["reason"] != "file_too_large" {
		t.Errorf("only a big file: %d %.200s", rec.Code, rec.Body.String())
	}
	if rec := post(admin, "/api/v1/readings", map[string]any{"reference": "Yoh 3:16", "text": big}); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("1 MiB body elsewhere: %d", rec.Code)
	}
	xml := `<song><properties><titles><title>T</title></titles></properties><lyrics><verse name="v1"><lines>` +
		strings.Repeat("<br/>", 100_001) + `</lines></verse></lyrics></song>`
	rec = post(admin, "/api/v1/imports", map[string]any{"format": "openlyrics", "files": []map[string]string{{"name": "x.xml", "text": xml}}})
	if p := problemCode(t, rec); rec.Code != 422 || p["reason"] != "too_complex" {
		t.Errorf("100,001 tokens: %d %.200s", rec.Code, rec.Body.String())
	}
	if rec := post(admin, "/api/v1/imports", map[string]any{"format": "paste", "files": []map[string]string{{"name": "T", "text": strings.Repeat("é", 200_001)}}}); rec.Code != 422 {
		t.Errorf("pasted text over 200,000 characters: %d", rec.Code)
	}

	// Discarding deletes at once.
	if rec := h.do(req{method: "DELETE", path: "/api/v1/imports/" + multi["id"].(string), cookies: []*http.Cookie{admin}}); rec.Code != http.StatusNoContent {
		t.Errorf("discard: %d", rec.Code)
	}
	if rec := h.get("/api/v1/imports/"+multi["id"].(string), admin); rec.Code != 404 {
		t.Errorf("discarded batch: %d", rec.Code)
	}

	// Logging: IDs and counts, never lyrics.
	logs := h.log.String()
	if !strings.Contains(logs, "import_created") || !strings.Contains(logs, "import_applied") {
		t.Errorf("log lines missing: %s", logs)
	}
	for _, secret := range []string{"Tuhan setia", "Pagi demi pagi", "lirik satu", "Amazing"} {
		if strings.Contains(logs, secret) {
			t.Errorf("lyrics in the log: %q", secret)
		}
	}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
