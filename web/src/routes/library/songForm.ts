// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { z } from "zod";
import type { SongView } from "@liturgist/api-client";
import { licenceStatuses, limits, sectionKinds, songLanguages, type SectionKind } from "@/lib/library";

// The song form's values. Numbers are kept as text while typing. Every
// section has a "ref": its ID when the song already has it, otherwise a
// name that is local to the form, which the arrangement uses to point at it
// (06 §2.4 sends such names as "key").
export type SectionValues = {
  ref: string;
  sid?: string; // not "id": the field array reserves that name
  kind: SectionKind;
  number: string;
  label: string;
  text: string;
};

const required = "common.required";
const invalid = "common.field_invalid";

const sectionSchema = z.object({
  ref: z.string(),
  sid: z.string().optional(),
  kind: z.enum(sectionKinds),
  number: z.string(),
  label: z.string().trim().max(60, invalid),
  text: z.string().trim().min(1, "library.section_text_required").max(5000, invalid),
});

export const songSchema = z
  .object({
    title: z.string().trim().min(1, required).max(200, invalid),
    language: z.enum(songLanguages),
    alt_titles: z.string(),
    hymnal_source: z.string().trim().max(40, invalid),
    hymnal_number: z.string().trim().max(10, invalid),
    default_key: z.string().trim().regex(/^([A-G][#b]?m?)?$/, "library.key_invalid"),
    lyricist: z.string().trim().max(200, invalid),
    composer: z.string().trim().max(200, invalid),
    translator: z.string().trim().max(200, invalid),
    copyright_holder: z.string().trim().max(200, invalid),
    copyright_line: z.string().trim().max(300, invalid),
    ccli_song_number: z.string().trim().regex(/^([0-9]{1,12})?$/, "library.ccli_invalid"),
    licence_status: z.enum(licenceStatuses),
    licence_notes: z.string().trim().max(2000, invalid),
    sections: z.array(sectionSchema).max(limits.sections, "library.too_many_sections"),
    arrangement: z.array(z.string()).max(limits.arrangement, "library.arrangement_full"),
  })
  .superRefine((v, ctx) => {
    const fail = (path: (string | number)[], message: string) => ctx.addIssue({ code: "custom", path, message });
    if ((v.hymnal_source === "") !== (v.hymnal_number === "")) fail(["hymnal_number"], "library.hymnal_pair");
    const titles = splitTitles(v.alt_titles);
    if (titles.length > limits.altTitles || titles.some((x) => [...x].length > 200)) fail(["alt_titles"], invalid);
    const seen = new Set<number>();
    v.sections.forEach((s, i) => {
      if (s.kind !== "verse") return;
      const n = Number(s.number);
      if (!/^[0-9]{1,2}$/.test(s.number.trim()) || n < 1) fail(["sections", i, "number"], "library.section_number_required");
      else if (seen.has(n)) fail(["sections", i, "number"], "library.section_number_taken");
      else seen.add(n);
    });
  });

export type SongValues = z.infer<typeof songSchema>;

// splitTitles turns the "other titles" box (one per line) into a list.
export function splitTitles(text: string): string[] {
  return text.split("\n").map((l) => l.trim()).filter((l) => l !== "");
}

// nextVerseNumber is the smallest number above every verse number in use.
export function nextVerseNumber(sections: Pick<SectionValues, "kind" | "number">[]): string {
  const used = sections.filter((s) => s.kind === "verse").map((s) => Number(s.number) || 0);
  return String(Math.min(99, Math.max(0, ...used) + 1));
}

let counter = 0;
// newRef returns a name for a section that has no ID yet.
export function newRef(): string {
  counter += 1;
  return "new" + counter;
}

export function emptyValues(language: string): SongValues {
  return {
    title: "", language: (songLanguages as readonly string[]).includes(language) ? (language as SongValues["language"]) : "id",
    alt_titles: "", hymnal_source: "", hymnal_number: "", default_key: "", lyricist: "", composer: "", translator: "",
    copyright_holder: "", copyright_line: "", ccli_song_number: "", licence_status: "unknown", licence_notes: "",
    sections: [], arrangement: [],
  };
}

export function toValues(song: SongView): SongValues {
  return {
    title: song.title,
    language: song.language,
    alt_titles: (song.alt_titles ?? []).join("\n"),
    hymnal_source: song.hymnal_source,
    hymnal_number: song.hymnal_number,
    default_key: song.default_key,
    lyricist: song.lyricist,
    composer: song.composer,
    translator: song.translator,
    copyright_holder: song.copyright_holder,
    copyright_line: song.copyright_line,
    ccli_song_number: song.ccli_song_number,
    licence_status: song.licence_status,
    licence_notes: song.licence_notes,
    sections: (song.sections ?? []).map((s) => ({
      ref: s.id, sid: s.id, kind: s.kind, number: s.number ? String(s.number) : "", label: s.label ?? "", text: s.text,
    })),
    arrangement: song.default_arrangement ?? [],
  };
}

// toRequest builds the body of POST /songs, or of PATCH /songs/{id} when it
// is given the version that was loaded. It always sends the complete list of
// sections and the arrangement, as the form shows them.
export function toRequest(v: SongValues) {
  const refs = new Set(v.sections.map((s) => s.ref));
  return {
    title: v.title.trim(),
    language: v.language,
    alt_titles: splitTitles(v.alt_titles),
    hymnal_source: v.hymnal_source.trim(),
    hymnal_number: v.hymnal_number.trim(),
    default_key: v.default_key.trim(),
    lyricist: v.lyricist.trim(),
    composer: v.composer.trim(),
    translator: v.translator.trim(),
    copyright_holder: v.copyright_holder.trim(),
    copyright_line: v.copyright_line.trim(),
    ccli_song_number: v.ccli_song_number.trim(),
    licence_status: v.licence_status,
    licence_notes: v.licence_notes.trim(),
    sections: v.sections.map((s) => ({
      ...(s.sid ? { id: s.sid } : { key: s.ref }),
      kind: s.kind,
      ...(s.kind === "verse" ? { number: Number(s.number) } : {}),
      label: s.label.trim(),
      text: s.text,
    })),
    default_arrangement: v.arrangement.filter((r) => refs.has(r)),
  };
}
