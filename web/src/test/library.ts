// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { Me, SongSummaryView, SongView } from "@liturgist/api-client";

export function meWith(scopes: string[]): Me {
  return {
    user: { id: "u1", name: "Ruth", email: "ruth@example.org", phone: null, preferences: {} },
    church: { name: "GKY Uji", time_zone: "Asia/Jakarta" },
    membership: { id: "m1", roles: [], scopes, actions: {} },
  } as unknown as Me;
}

export const editor = meWith(["library.edit"]);
export const viewer = meWith([]);

// song is a stored song with a verse, a chorus and a second verse.
export function song(changes: Partial<SongView> = {}): SongView {
  return {
    id: "s1", language: "id", title: "Besar Setia-Mu", alt_titles: ["Great Is Thy Faithfulness"],
    hymnal_source: "KJ", hymnal_number: "12", lyricist: "Thomas Chisholm", composer: "", translator: "",
    default_key: "G", copyright_holder: "", copyright_line: "Public domain", ccli_song_number: "",
    licence_status: "public_domain", licence_notes: "",
    sections: [
      { id: "a1", kind: "verse", number: 1, label: null, text: "Besar setia-Mu\nBapa" },
      { id: "a2", kind: "chorus", number: null, label: null, text: "Setiap pagi" },
      { id: "a3", kind: "verse", number: 2, label: null, text: "Musim dingin" },
    ],
    default_arrangement: ["a1", "a2", "a3", "a2"],
    versions: [], version: 3, created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z",
    actions: { edit: true, delete: true },
    ...changes,
  };
}

export function summary(id: string, title: string, changes: Partial<SongSummaryView> = {}): SongSummaryView {
  return {
    id, title, alt_titles: [], language: "id", hymnal_source: "", hymnal_number: "", licence_status: "unknown",
    has_group: false, actions: { edit: true, delete: true }, ...changes,
  };
}
