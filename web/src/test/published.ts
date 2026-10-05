// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { PublishedCopyView } from "@liturgist/api-client";

// copy is a published copy with a prayer by a duty, a reading and a song whose
// sequence repeats the chorus.
export function copy(changes: Partial<PublishedCopyView> = {}): PublishedCopyView {
  return {
    number: 2, published_at: "2026-10-09T03:00:00Z", published_by: { id: "u9", name: "Admin" }, revising: false, archived: false,
    content: {
      format: 1,
      liturgy: { date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum", language: "id", church_name: "GKY Uji" },
      items: [
        { id: "i1", position: 0, type: "prayer", title: "Doa Pembuka", duty: { id: "d1", name: "Liturgis" }, text: "Ya Tuhan,\nterima kasih", songs: [] },
        { id: "i2", position: 1, type: "reading", title: "Pembacaan", text: "", songs: [],
          reading: { reference_display: "Yoh 3:16", translation_code: "TB", text: "Karena begitu besar kasih Allah", attribution: "© LAI" } },
        { id: "i3", position: 2, type: "song", title: "Pujian", text: "",
          songs: [{
            song_id: "s1", title: "Besar Setia-Mu", hymnal_source: "KJ", hymnal_number: "12", key: "G", note: "pelan",
            copyright_holder: "", copyright_line: "Public domain", ccli_song_number: "123",
            sections: [{ id: "a1", kind: "verse", number: 1, label: "Bait 1", text: "Besar setia-Mu" }, { id: "a2", kind: "chorus", number: 0, label: "Refren", text: "Setiap pagi" }],
            entries: [
              { section_id: "a1", key_change: "", note: "" },
              { section_id: "a2", part: { id: "p1", name: "Jemaat" }, key_change: "A", note: "" },
              { section_id: "a2", key_change: "", note: "" },
            ],
          }] },
      ],
      assignments: [
        { duty: { id: "d1", name: "Liturgis" }, user_id: "u1", name: "Ruth" },
        { duty: { id: "d2", name: "Kolektan" }, user_id: null, name: "Pak Budi" },
      ],
      licence_footer: "CCLI License #1234567",
    },
    render: { key_display: "do", show_credits: true },
    url: "https://liturgi.example.org/published/l1",
    ...changes,
  };
}
