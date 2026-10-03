// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { EntryView, ItemSongView, LiturgyItemView, SongView } from "@liturgist/api-client";
import { liturgyLimits } from "@/lib/liturgy";
import { textItemTypes, type ItemType } from "@/lib/planning";

// What the user has typed into one item card, kept apart from the saved item
// so a change by someone else never replaces it (11 §4).
export type EntryDraft = { uid: string; section_id: string; section_label: string; singing_part_id: string; key_change: string; note: string };
export type SongDraft = {
  uid: string;
  song_id: string;
  song_title: string;
  key: string;
  note: string;
  entries: EntryDraft[];
  summary: ItemSongView["song"] | null;
};
// label is the passage as the server keeps it for the item: "Yohanes 3:16 (TB)".
export type ReadingChoice = { id: string; label: string; text: string };
export type ItemDraft = { title: string; duty_id: string; text: string; reading: ReadingChoice | null; songs: SongDraft[] };

let counter = 0;
// newUid names a row that has no server ID yet.
export const newUid = () => "new-" + ++counter;

function entryDraft(e: EntryView): EntryDraft {
  return { uid: e.id, section_id: e.section_id ?? "", section_label: e.section_label, singing_part_id: e.singing_part_id ?? "", key_change: e.key_change, note: e.note };
}

function songDraft(s: ItemSongView): SongDraft {
  return { uid: s.id, song_id: s.song_id ?? "", song_title: s.song_title, key: s.key, note: s.note, summary: s.song ?? null, entries: (s.entries ?? []).map(entryDraft) };
}

// draftOf is the draft that matches a saved item exactly.
export function draftOf(item: LiturgyItemView): ItemDraft {
  const r = item.reading;
  return {
    title: item.title,
    duty_id: item.duty_id ?? "",
    text: item.text,
    reading: item.reading_id && r ? { id: r.id, label: item.reading_label || r.reference, text: r.text } : null,
    songs: (item.songs ?? []).map(songDraft),
  };
}

// songsDirty tells whether the song list differs from what was saved.
export function songsDirty(d: ItemDraft, item: LiturgyItemView): boolean {
  return JSON.stringify(d.songs) !== JSON.stringify(draftOf(item).songs);
}

// fieldsDirty tells whether the title, duty, text or reading differ.
export function fieldsDirty(d: ItemDraft, item: LiturgyItemView): boolean {
  const saved = draftOf(item);
  return d.title !== saved.title || d.duty_id !== saved.duty_id || d.text !== saved.text || (d.reading?.id ?? "") !== (saved.reading?.id ?? "");
}

export const isDirty = (d: ItemDraft, item: LiturgyItemView) => fieldsDirty(d, item) || songsDirty(d, item);

// patchBody is the PATCH of the card's own fields: only what the item type has.
export function patchBody(d: ItemDraft, item: LiturgyItemView, version: number) {
  const type = item.item_type as ItemType;
  return {
    version,
    title: d.title.trim(),
    duty_id: d.duty_id,
    ...(textItemTypes.includes(type) ? { text: d.text } : {}),
    ...(type === "reading" ? { reading_id: d.reading?.id ?? "" } : {}),
  };
}

// songsBody is the complete song list for PUT …/songs (10 §4).
export function songsBody(d: ItemDraft, version: number) {
  return {
    version,
    songs: d.songs.map((s) => ({
      song_id: s.song_id,
      key: s.key.trim(),
      note: s.note,
      entries: s.entries.map((e) => ({
        section_id: e.section_id,
        ...(e.singing_part_id ? { singing_part_id: e.singing_part_id } : {}),
        key_change: e.key_change.trim(),
        note: e.note,
      })),
    })),
  };
}

// missingSections counts entries whose section was removed from the library
// and not yet replaced: the server refuses them, so Save waits (10 §2.5).
export const missingSections = (d: ItemDraft) => d.songs.reduce((n, s) => n + s.entries.filter((e) => e.section_id === "").length, 0);

// move returns a copy of list with the item at index moved by delta.
export function move<T>(list: T[], index: number, delta: number): T[] {
  const to = index + delta;
  if (to < 0 || to >= list.length) return list;
  const out = list.slice();
  const [x] = out.splice(index, 1);
  out.splice(to, 0, x);
  return out;
}

// fillFromDefault is the sequence a song gets when it is added: its default
// arrangement, or all its sections in order (10 §2.3, P-62).
export function fillFromDefault(song: Pick<SongView, "sections" | "default_arrangement">): EntryDraft[] {
  const sections = song.sections ?? [];
  const ids = (song.default_arrangement ?? []).length > 0 ? (song.default_arrangement ?? []) : sections.map((s) => s.id);
  return ids.slice(0, liturgyLimits.entriesPerSong).map((id) => ({
    uid: newUid(), section_id: id, section_label: "", singing_part_id: "", key_change: "", note: "",
  }));
}
