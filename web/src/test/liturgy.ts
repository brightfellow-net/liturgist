// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { LiturgyItemView, LiturgyView } from "@liturgist/api-client";
import { meWith } from "./library";

export const editorMe = meWith(["liturgy.edit"]);

type Section = { id: string; kind: string; number: number | null; label: string | null };
export const sections: Section[] = [
  { id: "a1", kind: "verse", number: 1, label: null },
  { id: "a2", kind: "chorus", number: null, label: null },
  { id: "a3", kind: "verse", number: 2, label: null },
];

// songItem is a song item with one song and the sequence V1, Chorus.
export function songItem(changes: Partial<LiturgyItemView> = {}): LiturgyItemView {
  return {
    id: "i2", position: 1, title: "Pujian", item_type: "song", duty_id: null, text: "", reading_id: null, reading_label: "", reading: undefined,
    version: 3,
    songs: [{
      id: "is1", position: 0, song_id: "s1", song_title: "Besar Setia-Mu", key: "G", note: "",
      song: { title: "Besar Setia-Mu", language: "id", hymnal_source: "KJ", hymnal_number: "12", default_key: "G", sections },
      entries: [
        { id: "e1", position: 0, section_id: "a1", singing_part_id: null, key_change: "", note: "", section_label: "Verse 1" },
        { id: "e2", position: 1, section_id: "a2", singing_part_id: null, key_change: "", note: "", section_label: "Chorus" },
      ],
    }],
    ...changes,
  } as unknown as LiturgyItemView;
}

export function textItem(changes: Partial<LiturgyItemView> = {}): LiturgyItemView {
  return {
    id: "i1", position: 0, title: "Votum", item_type: "free_text", duty_id: null, text: "Selamat pagi", reading_id: null, reading_label: "",
    reading: undefined, songs: [], version: 2, ...changes,
  } as unknown as LiturgyItemView;
}

export function liturgy(changes: Partial<LiturgyView> = {}): LiturgyView {
  return {
    id: "l1", date: "2026-10-11", time: "07:00", service_id: "sv1", service_name: "Ibadah Umum", language: "id", template_id: null,
    state: "draft", version: 5, items: [textItem(), songItem()], assignments: [], problems: [],
    edit_seq: 7, open_comments: 0,
    actions: { edit: true, delete: true, submit: true, approve: false, request_changes: false, reopen: false }, created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z", ...changes,
  } as unknown as LiturgyView;
}

export const duties = { status: 200, body: { items: [{ id: "d1", name: "Liturgis", position: 0, actions: { edit: true, delete: true } }] } };
export const noParts = { status: 200, body: { items: [{ id: "p1", name: "Semua", position: 0, actions: { edit: true, delete: true } }] } };
export const noEdits = { status: 200, body: { items: [] } };
