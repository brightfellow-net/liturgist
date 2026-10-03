// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { song } from "@/test/library";
import { emptyValues, nextVerseNumber, songSchema, splitTitles, toRequest, toValues, type SongValues } from "./songForm";

const base = (changes: Partial<SongValues> = {}): SongValues => ({ ...emptyValues("id"), title: "Lagu", ...changes });
const verse = (ref: string, number: string, text = "Syair") => ({ ref, kind: "verse" as const, number, label: "", text });

// problems lists "path: message" for every rule the values break.
function problems(v: SongValues): string[] {
  const r = songSchema.safeParse(v);
  return r.success ? [] : r.error.issues.map((i) => i.path.join(".") + ": " + i.message);
}

describe("song form rules (06 §2)", () => {
  it("accepts a song with a title only", () => {
    expect(problems(base())).toEqual([]);
  });

  it("needs a title and a hymnal number together with its hymnal", () => {
    expect(problems(base({ title: "  " }))).toEqual(["title: common.required"]);
    expect(problems(base({ hymnal_source: "KJ" }))).toEqual(["hymnal_number: library.hymnal_pair"]);
    expect(problems(base({ hymnal_number: "12" }))).toEqual(["hymnal_number: library.hymnal_pair"]);
    expect(problems(base({ hymnal_source: "KJ", hymnal_number: "12a" }))).toEqual([]);
  });

  it("checks the key and the CCLI number", () => {
    for (const k of ["G", "Bb", "F#m", ""]) expect(problems(base({ default_key: k }))).toEqual([]);
    for (const k of ["H", "f#m", "Gb#"]) expect(problems(base({ default_key: k }))).toEqual(["default_key: library.key_invalid"]);
    expect(problems(base({ ccli_song_number: "1234567890123" }))).toEqual(["ccli_song_number: library.ccli_invalid"]);
    expect(problems(base({ ccli_song_number: "123456789012" }))).toEqual([]);
  });

  it("allows at most 10 other titles of 200 characters", () => {
    const ten = Array.from({ length: 10 }, (_, i) => "Judul " + i).join("\n");
    expect(problems(base({ alt_titles: ten }))).toEqual([]);
    expect(problems(base({ alt_titles: ten + "\nSatu lagi" }))).toEqual(["alt_titles: common.field_invalid"]);
    expect(problems(base({ alt_titles: "x".repeat(201) }))).toEqual(["alt_titles: common.field_invalid"]);
  });

  it("needs a verse number from 1 to 99, different for every verse", () => {
    expect(problems(base({ sections: [verse("r1", "1"), verse("r2", "99")] }))).toEqual([]);
    expect(problems(base({ sections: [verse("r1", "")] }))).toEqual(["sections.0.number: library.section_number_required"]);
    expect(problems(base({ sections: [verse("r1", "0")] }))).toEqual(["sections.0.number: library.section_number_required"]);
    expect(problems(base({ sections: [verse("r1", "100")] }))).toEqual(["sections.0.number: library.section_number_required"]);
    expect(problems(base({ sections: [verse("r1", "2"), verse("r2", "2")] }))).toEqual(["sections.1.number: library.section_number_taken"]);
  });

  it("needs lyrics in every section, at most 5000 characters", () => {
    expect(problems(base({ sections: [verse("r1", "1", "  ")] }))).toEqual(["sections.0.text: library.section_text_required"]);
    expect(problems(base({ sections: [verse("r1", "1", "x".repeat(5001))] }))).toEqual(["sections.0.text: common.field_invalid"]);
  });

  it("does not ask a chorus for a number", () => {
    const chorus = { ref: "r1", kind: "chorus" as const, number: "", label: "", text: "Reff" };
    expect(problems(base({ sections: [chorus] }))).toEqual([]);
  });

  it("allows at most 60 sections and 100 arrangement entries", () => {
    const sixtyOne = Array.from({ length: 61 }, (_, i) => ({ ref: "r" + i, kind: "chorus" as const, number: "", label: "", text: "x" }));
    expect(problems(base({ sections: sixtyOne }))).toEqual(["sections: library.too_many_sections"]);
    expect(problems(base({ arrangement: Array(101).fill("r1") }))).toEqual(["arrangement: library.arrangement_full"]);
  });
});

describe("song form helpers", () => {
  it("splits other titles by line", () => {
    expect(splitTitles(" A \n\nB\n  \n")).toEqual(["A", "B"]);
  });

  it("proposes the next free verse number", () => {
    expect(nextVerseNumber([])).toBe("1");
    expect(nextVerseNumber([{ kind: "verse", number: "1" }, { kind: "chorus", number: "" }, { kind: "verse", number: "3" }])).toBe("4");
    expect(nextVerseNumber([{ kind: "verse", number: "99" }])).toBe("99");
  });

  it("loads a stored song into the form and back, keeping IDs", () => {
    const values = toValues(song());
    expect(values.sections.map((s) => [s.ref, s.sid, s.kind, s.number])).toEqual([
      ["a1", "a1", "verse", "1"], ["a2", "a2", "chorus", ""], ["a3", "a3", "verse", "2"],
    ]);
    expect(values.alt_titles).toBe("Great Is Thy Faithfulness");
    const body = toRequest(values);
    expect(body.sections).toEqual([
      { id: "a1", kind: "verse", number: 1, label: "", text: "Besar setia-Mu\nBapa" },
      { id: "a2", kind: "chorus", label: "", text: "Setiap pagi" },
      { id: "a3", kind: "verse", number: 2, label: "", text: "Musim dingin" },
    ]);
    expect(body.default_arrangement).toEqual(["a1", "a2", "a3", "a2"]);
  });

  it("sends new sections with a key and drops arrangement entries of removed sections", () => {
    const body = toRequest(base({
      sections: [verse("new1", "1"), { ref: "new2", kind: "chorus", number: "", label: " Reff ", text: "Teks" }],
      arrangement: ["new1", "new2", "gone", "new1"],
    }));
    expect(body.sections).toEqual([
      { key: "new1", kind: "verse", number: 1, label: "", text: "Syair" },
      { key: "new2", kind: "chorus", label: "Reff", text: "Teks" },
    ]);
    expect(body.default_arrangement).toEqual(["new1", "new2", "new1"]);
  });
});
