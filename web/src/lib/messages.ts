// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { TFunction } from "i18next";
import type { PublishedSummaryView } from "@liturgist/api-client";
import i18n from "./i18n";
import { formatKey } from "./liturgy";

// The WhatsApp texts of 13 §8. The server sends structured data only; the
// texts are built here, in the liturgy's language, and never contain lyrics or
// Bible text because the data has none.

// The generated type has `| null` on every list (Go's nil slices); the server
// sends [] in practice. summaryOf makes that certain, once, here.
type Raw = PublishedSummaryView;
type RawItem = NonNullable<Raw["items"]>[number];
type RawChanges = NonNullable<Raw["changes"]>;
export type Summary = Omit<Raw, "items" | "assignments" | "recipients" | "changes"> & {
  items: (Omit<RawItem, "songs"> & { songs: NonNullable<RawItem["songs"]> })[];
  assignments: (Omit<NonNullable<Raw["assignments"]>[number], "names"> & { names: string[] })[];
  recipients: (Omit<NonNullable<Raw["recipients"]>[number], "duties"> & { duties: string[] })[];
  changes?: { [K in keyof RawChanges]: NonNullable<RawChanges[K]> };
};
export type Recipient = Summary["recipients"][number];

export function summaryOf(raw: Raw): Summary {
  const c = raw.changes;
  return {
    ...raw,
    items: (raw.items ?? []).map((i) => ({ ...i, songs: i.songs ?? [] })),
    assignments: (raw.assignments ?? []).map((a) => ({ ...a, names: a.names ?? [] })),
    recipients: (raw.recipients ?? []).map((r) => ({ ...r, duties: r.duties ?? [] })),
    changes: c && { items: c.items ?? [], songs: c.songs ?? [], reading: c.reading ?? [], assignments: c.assignments ?? [] },
  };
}

// What the sender chooses to include.
export type MessageOptions = { songs: boolean; keys: boolean; readings: boolean };
export const defaultMessageOptions: MessageOptions = { songs: true, keys: true, readings: true };

// maxMessage is the app's own limit for a message that stays easy to read on
// a phone (13 §8), counted in characters, not bytes.
export const maxMessage = 1500;

// messageLanguage is the language of the texts: the liturgy's when the app has
// it, otherwise the church's default interface language (13 §8).
export function messageLanguage(liturgyLanguage: string, churchDefault: string | undefined): "en" | "id" {
  if (liturgyLanguage === "id" || liturgyLanguage === "en") return liturgyLanguage;
  return churchDefault === "id" ? "id" : "en";
}

export function messageT(lang: "en" | "id"): TFunction {
  return i18n.getFixedT(lang);
}

// A line of the droppable list; a title line goes when nothing follows it.
type Line = { text: string; title?: boolean };

export type Composed = {
  text: string;
  trimmed: boolean; // whole item lines were dropped and "…" added
  long: boolean; // the fixed blocks alone exceed the limit
};

const length = (s: string) => [...s].length;

// compose joins the fixed blocks and as many item lines as fit. The item list
// is the only block that is ever shortened, from the bottom, by whole lines.
export function compose(before: string, lines: Line[], after: string, limit = maxMessage): Composed {
  const join = (kept: Line[], more: boolean) => {
    const body = [...kept.map((l) => l.text), ...(more ? ["…"] : [])].join("\n");
    return [before, body, after].filter((b) => b !== "").join("\n\n");
  };
  const full = join(lines, false);
  if (length(full) <= limit) return { text: full, trimmed: false, long: false };
  for (let n = lines.length - 1; n >= 0; n--) {
    let kept = lines.slice(0, n);
    while (kept.length > 0 && (kept[kept.length - 1].title || kept[kept.length - 1].text === "")) kept = kept.slice(0, -1);
    const text = join(kept, true);
    if (length(text) <= limit) return { text, trimmed: true, long: false };
  }
  return { text: join([], lines.length > 0), trimmed: lines.length > 0, long: true };
}

function dateLine(s: Summary, lang: "en" | "id"): string {
  const [y, m, d] = s.liturgy.date.split("-").map(Number);
  const date = y && m && d
    ? new Intl.DateTimeFormat(lang === "en" ? "en-GB" : "id-ID", { timeZone: "UTC", weekday: "long", day: "numeric", month: "long", year: "numeric" })
        .format(new Date(Date.UTC(y, m - 1, d)))
    : s.liturgy.date;
  return s.liturgy.time ? `${date} · ${s.liturgy.time}` : date;
}

function songLabel(s: Summary, song: Summary["items"][number]["songs"][number], opts: MessageOptions): string {
  const hymnal = [song.hymnal_source, song.hymnal_number].filter(Boolean).join(" ");
  const key = opts.keys ? formatKey(song.key, s.key_display) : "";
  return [hymnal, song.title, key].filter(Boolean).join(" · ");
}

function itemLines(s: Summary, t: TFunction, opts: MessageOptions, only?: (item: Summary["items"][number]) => boolean): Line[] {
  const items = s.items.filter((i) => !only || only(i));
  const lines: Line[] = [];
  const songs = opts.songs ? items.flatMap((i) => i.songs) : [];
  if (songs.length > 0) {
    lines.push({ text: t("msg.songs_title"), title: true });
    songs.forEach((song, n) => lines.push({ text: `${n + 1}. ${songLabel(s, song, opts)}` }));
  }
  const readings = opts.readings ? items.filter((i) => i.reading) : [];
  if (readings.length > 0) {
    if (lines.length > 0) lines.push({ text: "" });
    for (const i of readings) {
      const r = i.reading!;
      lines.push({ text: t("msg.reading_line", { reference: r.translation_code ? `${r.reference_display} (${r.translation_code})` : r.reference_display }) });
    }
  }
  return lines;
}

const linkBlock = (s: Summary, t: TFunction) => `${t("msg.full_link")}\n${s.url}`;

// teamText is the summary for the WhatsApp group.
export function teamText(s: Summary, opts: MessageOptions, t: TFunction, lang: "en" | "id"): Composed {
  const heading = `${t("msg.team_heading", { service: s.liturgy.service_name })}\n${dateLine(s, lang)}`;
  const duties = s.assignments.length === 0 ? "" : [t("msg.duties_title"), ...s.assignments.map((a) => `${a.duty.name}: ${a.names.join(", ")}`)].join("\n");
  return compose([heading, duties].filter(Boolean).join("\n\n"), itemLines(s, t, opts), linkBlock(s, t));
}

// personalText greets one person and names their duties and the parts of the
// liturgy that belong to them.
export function personalText(s: Summary, who: Recipient, opts: MessageOptions, t: TFunction, lang: "en" | "id"): Composed {
  const mine = new Set(who.duties);
  const own = [
    t("msg.greeting", { name: who.name }),
    t("msg.reminder"),
    t("msg.personal_head", { duties: who.duties.join(", "), service: s.liturgy.service_name }),
    dateLine(s, lang),
  ].join("\n");
  const parts = s.items.filter((i) => i.duty && mine.has(i.duty.name));
  const lines: Line[] = parts.length > 0 ? [{ text: t("msg.your_parts"), title: true }, ...parts.map((i) => ({ text: `• ${i.title}` }))] : [];
  const detail = itemLines(s, t, opts, (i) => !!i.duty && mine.has(i.duty.name));
  return compose(own, [...lines, ...(detail.length > 0 ? [{ text: "" }, ...detail] : [])], linkBlock(s, t));
}

// changeText lists what the new version changed for the team, or null when
// nothing did (the page then says so and offers no text).
export function changeText(s: Summary, opts: MessageOptions, t: TFunction, lang: "en" | "id"): Composed | null {
  const c = s.changes;
  if (!c) return null;
  const key = (k: string) => (k === "" ? "" : formatKey(k, s.key_display));
  const none = t("msg.change_nobody");
  const lines: Line[] = [];
  for (const x of c.items) {
    switch (x.kind) {
      case "added": lines.push({ text: t("msg.item_added", { title: x.title }) }); break;
      case "removed": lines.push({ text: t("msg.item_removed", { title: x.title }) }); break;
      case "moved": lines.push({ text: t("msg.item_moved", { title: x.title }) }); break;
      case "retitled": lines.push({ text: t("msg.item_retitled", { old: x.old_title, title: x.title }) }); break;
      case "duty_changed": lines.push({ text: t("msg.item_duty", { title: x.title, old: x.old_duty?.name ?? none, duty: x.duty?.name ?? none }) }); break;
    }
  }
  if (opts.songs) {
    for (const x of c.songs) {
      const label = [[x.hymnal_source, x.hymnal_number].filter(Boolean).join(" "), x.title].filter(Boolean).join(" · ");
      if (x.kind === "added") lines.push({ text: t("msg.song_added", { song: label, item: x.item_title }) });
      else if (x.kind === "removed") lines.push({ text: t("msg.song_removed", { song: label, item: x.item_title }) });
      else if (opts.keys) {
        lines.push({ text: x.old_key === x.new_key ? t("msg.song_keys_inside", { song: label }) : t("msg.song_key", { song: label, old: key(x.old_key), key: key(x.new_key) }) });
      }
    }
  }
  if (opts.readings) {
    for (const x of c.reading) {
      const text = x.old === "" ? t("msg.reading_added", { item: x.item_title, reference: x.new })
        : x.new === "" ? t("msg.reading_removed", { item: x.item_title, reference: x.old })
        : t("msg.reading_changed", { item: x.item_title, old: x.old, reference: x.new });
      lines.push({ text });
    }
  }
  for (const x of c.assignments) {
    lines.push({ text: t(x.kind === "added" ? "msg.person_added" : "msg.person_removed", { duty: x.duty.name, name: x.name }) });
  }
  if (lines.length === 0) return null;
  const heading = `${t("msg.change_heading", { service: s.liturgy.service_name, number: s.number })}\n${dateLine(s, lang)}`;
  return compose(heading, lines, linkBlock(s, t));
}

// whatsappLink opens WhatsApp with the text filled in; the sender presses send.
// Without a phone it opens the chat picker (13 §8).
export function whatsappLink(text: string, phone?: string): string {
  const digits = (phone ?? "").replace(/\D/g, "");
  return `https://wa.me/${digits}?text=${encodeURIComponent(text)}`;
}

// isLocalLink is true when the link points at this computer, which no other
// phone can open: LITURGIST_BASE_URL is not set (13 §8).
export function isLocalLink(url: string): boolean {
  try {
    const host = new URL(url).hostname;
    return host === "localhost" || host === "127.0.0.1" || host === "[::1]" || host === "::1";
  } catch {
    return true;
  }
}
