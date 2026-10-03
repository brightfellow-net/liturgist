// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { ImportBatchView, ImportCandidateView, LookupView, Me, ReadingView, SongSummaryView, SongView } from "@liturgist/api-client";

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

export function reading(changes: Partial<ReadingView> = {}): ReadingView {
  return {
    id: "r1", reference: "JHN 3:16-21", canonical: "Yohanes 3:16-21", reference_display: "Yoh 3:16-21",
    translation: { code: "TB", name: "Terjemahan Baru (LAI)", language: "id" },
    text: "Karena begitu besar kasih Allah\nakan dunia ini", attribution: "© LAI", source_provider: "manual", version: 2,
    created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z", actions: { edit: true, delete: true }, ...changes,
  };
}

export function lookup(changes: Partial<LookupView> = {}): LookupView {
  return {
    reference: "JHN 3:16", canonical: "Yohanes 3:16", display: "Yoh 3:16",
    translation: { code: "TB", name: "Terjemahan Baru (LAI)", language: "id" },
    reading: null, provider: null, provider_error: false, suggested_attribution: "", ...changes,
  };
}

export const translations = [
  { code: "TB", name: "Terjemahan Baru (LAI)", language: "id" },
  { code: "BIS", name: "Bahasa Indonesia Sehari-hari (LAI)", language: "id" },
];

// editorOf is an editor whose church uses TB.
export const editorTB = (() => {
  const me = meWith(["library.edit"]);
  return { ...me, church: { ...me.church, default_translation_code: "TB" } } as Me;
})();

// candidate is an import candidate with a verse and a chorus.
export function candidate(id: string, title: string, changes: Partial<ImportCandidateView> = {}): ImportCandidateView {
  return {
    id, decision: "pending", remove_unmatched: false, warnings: [],
    draft: {
      language: "id", title, alt_titles: [], default_arrangement: [0, 1, 0],
      sections: [{ kind: "verse", number: 1, text: "Besar setia-Mu" }, { kind: "chorus", text: "Setiap pagi" }],
    },
    ...changes,
  };
}

export function batch(candidates: ImportCandidateView[], changes: Partial<ImportBatchView> = {}): ImportBatchView {
  return { id: "b1", source_format: "paste", status: "open", created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z", candidates, rejected: [], ...changes };
}
