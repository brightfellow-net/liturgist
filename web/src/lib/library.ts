// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import type { SongSummaryView } from "@liturgist/api-client";
import { api, call } from "./api";
import { contentLanguages } from "./church";

// The values the song API accepts (06 §2).
export const songLanguages = contentLanguages;
export const licenceStatuses = ["unknown", "public_domain", "church_licence", "permission_obtained"] as const;
export const sectionKinds = ["verse", "pre_chorus", "chorus", "bridge", "tag", "intro", "ending", "other"] as const;
export type SectionKind = (typeof sectionKinds)[number];

// Limits the form checks before sending; the server checks them again (06 §2).
export const limits = { sections: 60, arrangement: 100, altTitles: 10, pageSize: 50 } as const;

// The parts of a section that decide its name.
export type Nameable = { kind: SectionKind; number: number | null | undefined; label: string | null | undefined };

// sectionName is the custom label, or the standard name for the kind in the
// viewer's language: "Verse 1", "Bait 1", "Chorus", "Reff" (06 §2.2).
export function sectionName(t: TFunction, s: Nameable): string {
  const label = s.label?.trim();
  if (label) return label;
  if (s.kind === "verse" && s.number) return t("library.verse_n", { number: s.number });
  return t(`library.kinds.${s.kind}`);
}

// hymnalText is "KJ 12", or "" when the song has no hymnal number.
export function hymnalText(s: { hymnal_source: string; hymnal_number: string }): string {
  return s.hymnal_source && s.hymnal_number ? `${s.hymnal_source} ${s.hymnal_number}` : "";
}

// SongFilters are the list's search terms, kept in the address bar.
export type SongFilters = {
  q?: string;
  language?: string;
  licence_status?: string;
  hymnal_source?: string;
  hymnal_number?: string;
  offset?: number;
  limit?: number;
};

// Query keys are fixed (05 §4): the list under ["songs", filters], one song
// under ["song", id].
export const songsQuery = (filters: SongFilters) =>
  queryOptions({
    queryKey: ["songs", filters],
    queryFn: () => {
      const query = Object.fromEntries(Object.entries(filters).filter(([, v]) => v !== undefined && v !== "")) as SongFilters;
      return call(api.GET("/songs", { params: { query } }));
    },
  });

export const songQuery = (id: string) =>
  queryOptions({
    queryKey: ["song", id],
    queryFn: () => call(api.GET("/songs/{id}", { params: { path: { id } } })),
  });

export type SongSummary = SongSummaryView;
