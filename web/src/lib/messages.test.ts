// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import {
  changeText, compose, defaultMessageOptions, isLocalLink, maxMessage, messageLanguage, messageT, personalText, summaryOf, teamText, whatsappLink,
  type Summary,
} from "./messages";

// WT-P-006: the texts for given data.
const raw = {
  number: 2,
  url: "https://liturgi.example.org/published/L1",
  key_display: "do",
  liturgy: { date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum 1", language: "id", church_name: "GKY" },
  items: [
    { title: "Pujian", type: "song", duty: { id: "d2", name: "Pemandu Pujian" }, songs: [{ title: "Haleluya, Pujilah", hymnal_source: "KJ", hymnal_number: "1", key: "G" }] },
    { title: "Pembacaan", type: "reading", duty: { id: "d1", name: "Liturgis" }, songs: [], reading: { reference_display: "Yoh 3:16-21", translation_code: "" } },
  ],
  assignments: [
    { duty: { id: "d1", name: "Liturgis" }, names: ["Andreas"] },
    { duty: { id: "d2", name: "Pemandu Pujian" }, names: ["Budi"] },
    { duty: { id: "d3", name: "Pemusik" }, names: ["Clara", "Daniel"] },
  ],
  recipients: [
    { name: "Budi", duties: ["Pemandu Pujian"], phone: "+6281234567890", member: true },
    { name: "Pak Joko", duties: ["Pemusik"], member: false },
  ],
} as unknown as Parameters<typeof summaryOf>[0];
const s: Summary = summaryOf(raw);
const id = messageT("id");
const en = messageT("en");

describe("team text", () => {
  it("is the example of the spec, in the liturgy's language", () => {
    expect(teamText(s, defaultMessageOptions, id, "id").text).toBe(
      [
        "*Liturgi Ibadah Umum 1*", "Minggu, 11 Oktober 2026 · 07:00", "",
        "*Petugas*", "Liturgis: Andreas", "Pemandu Pujian: Budi", "Pemusik: Clara, Daniel", "",
        "*Lagu*", "1. KJ 1 · Haleluya, Pujilah · Do = G", "",
        "*Bacaan:* Yoh 3:16-21", "",
        "Liturgi lengkap:", "https://liturgi.example.org/published/L1",
      ].join("\n"),
    );
  });

  it("leaves out songs, keys and readings as chosen, and writes English for an English service", () => {
    const text = (o: Partial<typeof defaultMessageOptions>) => teamText(s, { ...defaultMessageOptions, ...o }, en, "en").text;
    expect(text({ keys: false })).toContain("1. KJ 1 · Haleluya, Pujilah\n");
    expect(text({ songs: false })).not.toContain("Haleluya");
    expect(text({ readings: false })).not.toContain("Yoh 3:16");
    expect(text({}).startsWith("*Liturgy Ibadah Umum 1*\nSunday, 11 October 2026 · 07:00")).toBe(true);
    expect(text({})).toContain("Full liturgy:");
  });

  it("shows keys as letters when the church does", () => {
    expect(teamText({ ...s, key_display: "letter" }, defaultMessageOptions, id, "id").text).toContain("1. KJ 1 · Haleluya, Pujilah · G\n");
  });
});

describe("personal text", () => {
  it("greets by name, names the duty, and lists only that person's parts", () => {
    const text = personalText(s, s.recipients[0], defaultMessageOptions, id, "id").text;
    expect(text.startsWith("Shalom Budi 🙏\nMengingatkan tugas pelayanan:\n*Pemandu Pujian* · Ibadah Umum 1\nMinggu, 11 Oktober 2026 · 07:00")).toBe(true);
    expect(text).toContain("• Pujian");
    expect(text).toContain("1. KJ 1 · Haleluya, Pujilah · Do = G");
    expect(text).not.toContain("Pembacaan");
    expect(text).not.toMatch(/kamu|Bapak|Ibu/);
    expect(text.endsWith("Liturgi lengkap:\nhttps://liturgi.example.org/published/L1")).toBe(true);
  });
});

describe("change text", () => {
  it("is null for a first version and when nothing relevant changed", () => {
    expect(changeText(s, defaultMessageOptions, id, "id")).toBeNull();
    expect(changeText({ ...s, changes: { items: [], songs: [], reading: [], assignments: [] } }, defaultMessageOptions, id, "id")).toBeNull();
  });

  it("lists every kind of change, in the church's key form", () => {
    const withChanges: Summary = {
      ...s,
      changes: {
        items: [
          { kind: "added", item_id: "a", title: "Penutup" }, { kind: "removed", item_id: "b", title: "Kolekte" }, { kind: "moved", item_id: "c", title: "Doa" },
          { kind: "retitled", item_id: "d", title: "Doa Syafaat", old_title: "Doa" },
          { kind: "duty_changed", item_id: "e", title: "Berita", duty: { id: "d2", name: "Budi" } },
        ],
        songs: [
          { kind: "added", item_id: "p", item_title: "Pujian", title: "Besar", hymnal_source: "KJ", hymnal_number: "9", old_key: "", new_key: "G", entry_keys: false },
          { kind: "key_changed", item_id: "p", item_title: "Pujian", title: "Haleluya", hymnal_source: "KJ", hymnal_number: "1", old_key: "G", new_key: "A", entry_keys: false },
        ],
        reading: [{ item_id: "r", item_title: "Pembacaan", old: "Yoh 3:16", new: "Yoh 3:17" }],
        assignments: [{ kind: "added", duty: { id: "d1", name: "Liturgis" }, name: "Eka" }, { kind: "removed", duty: { id: "d1", name: "Liturgis" }, name: "Andreas" }],
      },
    };
    const text = changeText(withChanges, defaultMessageOptions, id, "id")!.text;
    expect(text.split("\n").slice(0, 2)).toEqual(["*Perubahan liturgi Ibadah Umum 1* (versi 2)", "Minggu, 11 Oktober 2026 · 07:00"]);
    for (const line of [
      "+ Penutup ditambahkan", "– Kolekte dihapus", "Doa dipindah urutannya", "Doa menjadi Doa Syafaat", "Berita: petugas belum ada → Budi",
      "+ Lagu KJ 9 · Besar (Pujian)", "KJ 1 · Haleluya: Do = G → Do = A", "Pembacaan: Yoh 3:16 → Yoh 3:17", "+ Liturgis: Eka", "– Liturgis: Andreas",
    ]) expect(text).toContain(line);
    const noSongs = changeText(withChanges, { ...defaultMessageOptions, songs: false, readings: false }, id, "id")!.text;
    expect(noSongs).not.toContain("Haleluya");
    expect(noSongs).not.toContain("Yoh");
  });
});

describe("length", () => {
  const lines = Array.from({ length: 40 }, (_, i) => ({ text: `${i + 1}. A song with a long enough title to fill the line number ${i + 1}` }));

  it("drops whole item lines from the bottom and says so, keeping the fixed blocks", () => {
    const c = compose("HEAD\nline two", [{ text: "*Lagu*", title: true }, ...lines], "LINK\nhttps://x", 600);
    expect(c.trimmed).toBe(true);
    expect(c.long).toBe(false);
    expect([...c.text].length).toBeLessThanOrEqual(600);
    expect(c.text.startsWith("HEAD\nline two\n\n*Lagu*\n1. ")).toBe(true);
    expect(c.text.endsWith("…\n\nLINK\nhttps://x")).toBe(true);
    const kept = c.text.split("\n").filter((l) => /^\d+\. /.test(l));
    expect(kept[kept.length - 1]).toMatch(new RegExp(`^${kept.length}\\. `)); // whole lines, in order
  });

  it("counts characters, not bytes", () => {
    const wide = [{ text: "界".repeat(100) }, { text: "界".repeat(100) }];
    expect(compose("H", wide, "L", 207).trimmed).toBe(false);
    expect(compose("H", wide, "L", 206).trimmed).toBe(true);
  });

  it("shows the fixed blocks whole, and says it is long, when they alone are too long", () => {
    const c = compose("H".repeat(maxMessage), lines, "LINK");
    expect(c.long).toBe(true);
    expect(c.text.startsWith("H".repeat(maxMessage))).toBe(true);
    expect(c.text.endsWith("LINK")).toBe(true);
  });

  it("drops a title line that would be left with nothing under it", () => {
    const c = compose("H", [{ text: "*Lagu*", title: true }, { text: "x".repeat(300) }], "L", 100);
    expect(c.text).toBe("H\n\n…\n\nL");
  });
});

describe("links and language", () => {
  it("encodes the text and keeps only the digits of the phone", () => {
    expect(whatsappLink("Halo & *tebal*\nbaris", "+62 812-3456")).toBe(`https://wa.me/628123456?text=${encodeURIComponent("Halo & *tebal*\nbaris")}`);
    expect(whatsappLink("x")).toBe("https://wa.me/?text=x");
  });

  it("uses the liturgy's language, or the church's default when the app has none", () => {
    expect(messageLanguage("id", "en")).toBe("id");
    expect(messageLanguage("en", "id")).toBe("en");
    expect(messageLanguage("zh-Hans", "id")).toBe("id");
    expect(messageLanguage("zh-Hant", undefined)).toBe("en");
  });

  it("knows a link that only works on this computer", () => {
    expect(isLocalLink("http://localhost:8080/published/x")).toBe(true);
    expect(isLocalLink("http://127.0.0.1:8080/x")).toBe(true);
    expect(isLocalLink("https://liturgi.example.org/x")).toBe(false);
  });
});
