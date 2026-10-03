// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import type { EditView, LiturgyItemView, LiturgyView } from "@liturgist/api-client";
import { api, call } from "./api";

// Limits the editor checks before sending; the server checks them again (10 §2).
export const liturgyLimits = { items: 60, songsPerItem: 10, entriesPerSong: 100, pageSize: 50, recentChanges: 20 } as const;

// The states of a liturgy in words come from "liturgy.states.<state>".
export const liturgyStates = ["draft", "in_review", "needs_revision", "approved", "published", "archived"] as const;

// Query keys are fixed (05 §4, 11 §5).
export type LiturgyFilters = { state?: (typeof liturgyStates)[number] & ("draft" | "in_review" | "needs_revision" | "approved" | "published"); from?: string; to?: string; order?: "date_asc" | "date_desc"; limit?: number; offset?: number };

export const liturgiesQuery = (filters: LiturgyFilters) =>
  queryOptions({
    queryKey: ["liturgies", filters],
    queryFn: () => {
      const query = Object.fromEntries(Object.entries(filters).filter(([, v]) => v !== undefined && v !== "")) as LiturgyFilters;
      return call(api.GET("/liturgies", { params: { query } }));
    },
  });

export const liturgyQuery = (id: string) =>
  queryOptions({
    queryKey: ["liturgy", id],
    queryFn: () => call(api.GET("/liturgies/{id}", { params: { path: { id } } })),
  });

export const prepareQuery = (week: string) =>
  queryOptions({
    queryKey: ["prepare", week],
    queryFn: () => call(api.GET("/liturgies/prepare", { params: { query: week ? { week } : {} } })),
  });

export const assignableQuery = queryOptions({
  queryKey: ["assignable"],
  queryFn: () => call(api.GET("/liturgies/assignable")),
});

export const editsQuery = (id: string) =>
  queryOptions({
    queryKey: ["liturgy", id, "edits"],
    queryFn: () => call(api.GET("/liturgies/{id}/edits", { params: { path: { id }, query: { limit: liturgyLimits.recentChanges } } })),
  });

// keyShape is the shape of a key the editor can say in "do" words: a letter
// A to G, an optional # or b, and an "m" for minor (06 §2).
const keyShape = /^([A-G])([#b]?)(m?)$/;

// formatKey writes a stored key (letters) as the church wants to see it
// (11 §2): `letter` shows it unchanged; `do` shows "Do = G", and for a minor
// key the relative-major convention "La = Em". Text that is not a key is shown
// as typed, and an empty key stays empty.
export function formatKey(key: string, display: string | undefined): string {
  const k = key.trim();
  if (k === "" || display !== "do") return k;
  const m = keyShape.exec(k);
  if (!m) return k;
  return `${m[3] ? "La" : "Do"} = ${k}`;
}

// keyChangeText says a key change in words: "change to Do = A" (11 §2).
export function keyChangeText(t: TFunction, key: string, display: string | undefined): string {
  return key.trim() === "" ? "" : t("liturgy.key_change_to", { key: formatKey(key, display) });
}

// dateInZone is today's calendar date (YYYY-MM-DD) in an IANA time zone, so
// "upcoming" means the church's today, not the browser's (11 §3).
export function dateInZone(zone: string | undefined, now: Date = new Date()): string {
  try {
    return new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  } catch {
    return new Intl.DateTimeFormat("en-CA", { year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  }
}

// addDays moves a calendar date (YYYY-MM-DD) by whole days, with no time zone
// in the way.
export function addDays(date: string, days: number): string {
  const [y, m, d] = date.split("-").map(Number);
  const moved = new Date(Date.UTC(y, m - 1, d + days));
  return moved.toISOString().slice(0, 10);
}

// longDate writes a date the way the viewer's language does ("12 October 2026").
export function longDate(date: string, lang: string): string {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return date;
  return new Intl.DateTimeFormat(lang === "en" ? "en-GB" : lang, { timeZone: "UTC", day: "numeric", month: "long", year: "numeric" }).format(new Date(Date.UTC(y, m - 1, d)));
}

// weekdayOf is the ISO weekday (1 = Monday … 7 = Sunday) of a calendar date.
export function weekdayOf(date: string): number {
  const [y, m, d] = date.split("-").map(Number);
  const day = new Date(Date.UTC(y, m - 1, d)).getUTCDay();
  return day === 0 ? 7 : day;
}

// withItem replaces one item of a loaded liturgy (a saved item or a song
// change) and takes the liturgy version the server reported.
export function withItem(l: LiturgyView, item: LiturgyItemView, liturgyVersion: number): LiturgyView {
  return { ...l, version: liturgyVersion, items: (l.items ?? []).map((x) => (x.id === item.id ? item : x)) };
}

// withOrder puts the items in the order the server confirmed, dropping any
// that are no longer there.
export function withOrder(l: LiturgyView, ids: string[], liturgyVersion: number): LiturgyView {
  const byId = new Map((l.items ?? []).map((x) => [x.id, x]));
  const items = ids.flatMap((id, position) => {
    const it = byId.get(id);
    return it ? [{ ...it, position }] : [];
  });
  return { ...l, version: liturgyVersion, items };
}

// problemText says one problem of GET /liturgies/{id} in words, naming the
// item by its place and title (11 §4).
export function problemText(t: TFunction, code: string, place: number, title: string): string {
  return t(`liturgy.problems.${code}`, { n: place, title, defaultValue: t("liturgy.problems.unknown", { n: place, title }) });
}

type Image = Record<string, unknown> | null;

function parse(raw: unknown): Image {
  if (raw === null || raw === undefined) return null;
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw) as Image;
    } catch {
      return null;
    }
  }
  return typeof raw === "object" ? (raw as Image) : null;
}

const str = (v: unknown): string => (typeof v === "string" ? v : "");

// itemTitle finds the title an edit is about in its images (10 §7); the
// images hold IDs and field values, so a title is there for item commands.
function itemTitle(e: EditView): string {
  const after = parse(e.after), before = parse(e.before);
  return str(after?.title) || str(before?.title) || str(after?.song_title) || str(before?.song_title);
}

// editText says one history row in words: "Budi moved Pujian up" (11 §4).
// The images never hold lyrics, so only titles and names appear.
export function editText(t: TFunction, e: EditView): string {
  const who = e.user_name;
  const title = itemTitle(e);
  const key = `liturgy.edits.${e.command.replace(".", "_")}`;
  const after = parse(e.after), before = parse(e.before);
  if (e.command === "item.update") {
    // A changed key is the one detail worth naming; any other change is "edited".
    return t(key, { who, title });
  }
  if (e.command === "item.songs") {
    const song = Array.isArray(after?.songs) ? (after.songs as Image[])[0] : undefined;
    const prev = Array.isArray(before?.songs) ? (before.songs as Image[])[0] : undefined;
    if (song && prev && str(song.key) !== str(prev.key)) {
      return t("liturgy.edits.item_key", { who, title: str(song.song_title) || title, key: str(song.key) });
    }
    return t(key, { who, title });
  }
  if (e.command === "assignment.add" || e.command === "assignment.remove") {
    const a = e.command === "assignment.add" ? after : before;
    return t(key, { who, name: str(a?.name) || t("liturgy.edits.someone") });
  }
  return t(key, { who, title, name: str(after?.service_name) || str(before?.service_name), defaultValue: t("liturgy.edits.other", { who }) });
}

// undoWhat says what an undo or redo acted on, after "Undid: " (11 §7.2).
export function undoWhat(t: TFunction, command: string, title: string): string {
  const key = `liturgy.undo.what.${command.replace(".", "_")}`;
  return t(key, { title: title || t("liturgy.undo.an_item"), defaultValue: t("liturgy.undo.what.other") });
}
