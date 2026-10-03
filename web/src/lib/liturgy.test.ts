// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import i18n from "@/lib/i18n";
import type { EditView } from "@liturgist/api-client";
import { addDays, dateInZone, editText, formatKey, weekdayOf, withOrder } from "./liturgy";
import { liturgy } from "@/test/liturgy";

describe("TC-E-001 formatKey", () => {
  it("writes letters unchanged and Do-mode keys with the relative-major convention", () => {
    expect(formatKey("G", "letter")).toBe("G");
    expect(formatKey("G", "do")).toBe("Do = G");
    expect(formatKey("Bb", "do")).toBe("Do = Bb");
    expect(formatKey("F#m", "do")).toBe("La = F#m");
    expect(formatKey("Em", "do")).toBe("La = Em");
  });
  it("keeps an empty key empty and unknown text as typed", () => {
    expect(formatKey("", "do")).toBe("");
    expect(formatKey("  ", "do")).toBe("");
    expect(formatKey("capo 2", "do")).toBe("capo 2");
    expect(formatKey("G", undefined)).toBe("G");
  });
});

describe("dates", () => {
  it("takes today in the church's zone, not the browser's", () => {
    const at = new Date("2026-10-10T20:00:00Z"); // 11 October 03:00 in Jakarta
    expect(dateInZone("Asia/Jakarta", at)).toBe("2026-10-11");
    expect(dateInZone("UTC", at)).toBe("2026-10-10");
  });
  it("moves whole days across a month end and finds the weekday", () => {
    expect(addDays("2026-10-30", 3)).toBe("2026-11-02");
    expect(addDays("2026-10-05", -7)).toBe("2026-09-28");
    expect(weekdayOf("2026-10-11")).toBe(7); // a Sunday
    expect(weekdayOf("2026-10-12")).toBe(1);
  });
});

describe("withOrder", () => {
  it("puts the items in the confirmed order and takes the new version", () => {
    const l = withOrder(liturgy(), ["i2", "i1"], 6);
    expect(l.version).toBe(6);
    expect((l.items ?? []).map((i) => [i.id, i.position])).toEqual([["i2", 0], ["i1", 1]]);
  });
});

describe("editText", () => {
  const edit = (changes: Partial<EditView>): EditView => ({
    id: "x", seq: 1, user_id: "u", user_name: "Budi", command: "item.add", item_id: "i1", before: null, after: { title: "Pujian" },
    liturgy_version_after: 2, item_version_after: 1, status: "done", created_at: "2026-10-01T00:00:00Z", ...changes,
  } as EditView);
  void i18n.changeLanguage("en");
  const t = i18n.t.bind(i18n);
  it("names who did what to which item", () => {
    expect(editText(t, edit({}))).toBe("Budi added Pujian");
    expect(editText(t, edit({ command: "item.remove", before: { title: "Pujian" } as never, after: null }))).toBe("Budi removed Pujian");
  });
  it("names a changed key", () => {
    const before = { title: "Pujian", songs: [{ song_title: "Besar Setia-Mu", key: "G" }] } as never;
    const after = { title: "Pujian", songs: [{ song_title: "Besar Setia-Mu", key: "A" }] } as never;
    expect(editText(t, edit({ command: "item.songs", before, after }))).toBe("Budi changed the key of Besar Setia-Mu to A");
  });
  it("falls back to a plain sentence for a free-text assignment without a name", () => {
    expect(editText(t, edit({ command: "assignment.add", after: { user_id: "u2" } as never }))).toBe("Budi assigned someone");
  });
  it("reads images that arrive as JSON text", () => {
    expect(editText(t, edit({ after: '{"title":"Doa"}' as never }))).toBe("Budi added Doa");
  });
});
