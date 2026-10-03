// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { SongDraft } from "@liturgist/api-client";
import { licenceStatuses, sectionKinds, songLanguages } from "@/lib/library";
import { emptyValues, splitTitles, type SongValues } from "./songForm";

// A draft has no section IDs and its arrangement is a list of positions. The
// editor works on the song form's values, where sections have a "ref": a
// draft section is named "d<position>".
const refOf = (i: number) => "d" + i;

function oneOf<T extends string>(list: readonly T[], value: string | undefined, fallback: T): T {
  return list.includes(value as T) ? (value as T) : fallback;
}

// draftToValues fills the song form from a candidate's draft.
export function draftToValues(d: SongDraft): SongValues {
  const base = emptyValues(d.language);
  const sections = d.sections ?? [];
  return {
    ...base,
    title: d.title,
    language: oneOf(songLanguages, d.language, base.language),
    alt_titles: (d.alt_titles ?? []).join("\n"),
    hymnal_source: d.hymnal_source ?? "",
    hymnal_number: d.hymnal_number ?? "",
    default_key: d.default_key ?? "",
    lyricist: d.lyricist ?? "",
    composer: d.composer ?? "",
    translator: d.translator ?? "",
    copyright_holder: d.copyright_holder ?? "",
    copyright_line: d.copyright_line ?? "",
    ccli_song_number: d.ccli_song_number ?? "",
    licence_status: oneOf(licenceStatuses, d.licence_status, "unknown"),
    licence_notes: d.licence_notes ?? "",
    sections: sections.map((s, i) => ({
      ref: refOf(i), kind: oneOf(sectionKinds, s.kind, "other"), number: s.number ? String(s.number) : "",
      label: s.label ?? "", text: s.text,
    })),
    arrangement: (d.default_arrangement ?? []).filter((n) => n >= 0 && n < sections.length).map(refOf),
  };
}

// valuesToDraft is the body of PATCH /imports/{id}/candidates/{cid}: the
// complete draft, as the form shows it.
export function valuesToDraft(v: SongValues): SongDraft {
  const position = new Map(v.sections.map((s, i) => [s.ref, i]));
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
      kind: s.kind,
      ...(s.kind === "verse" ? { number: Number(s.number) } : {}),
      label: s.label.trim(),
      text: s.text,
    })),
    default_arrangement: v.arrangement.flatMap((r) => (position.has(r) ? [position.get(r) as number] : [])),
  };
}
