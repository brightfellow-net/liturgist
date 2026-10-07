// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from "react-i18next";
import type { PublishedContentView, PublishedSongView } from "@liturgist/api-client";
import { ChurchLogo } from "@/components/ChurchLogo";
import { formatKey, keyChangeText, longDate } from "@/lib/liturgy";
import { firstLine, pageSize, type PrintOptions } from "@/lib/print";

type Render = { key_display: string; show_credits: boolean };

// PrintBody is the stored copy laid out for paper (13 §6). The text is plain
// text, escaped by React. The rules that keep a song heading with its first
// section and a row of the sequence whole are `break-*` classes; the browser
// applies them on a best-effort basis.
export function PrintBody({ content, render, opts, logoUrl }: { content: PublishedContentView; render: Render; opts: PrintOptions; logoUrl?: string | null }) {
  const { t, i18n } = useTranslation();
  const { liturgy } = content;
  const musician = opts.variant === "musician";
  const items = (content.items ?? []).filter((i) => !musician || (i.songs ?? []).length > 0);
  const assignments = content.assignments ?? [];
  const people = (dutyID: string) => assignments.filter((a) => a.duty.id === dutyID).map((a) => a.name).join(", ");
  const shown = new Set(items.map((i) => i.duty?.id).filter(Boolean));
  const others = assignments.filter((a) => !shown.has(a.duty.id));
  return (
    <article lang={liturgy.language} data-paper={opts.paper} data-variant={opts.variant} className={opts.size === "large" ? "text-xl" : "text-base"}>
      <style>{`@media print { @page { size: ${pageSize[opts.paper]}; margin: 15mm; } }`}</style>
      <header className="space-y-1 border-b border-border pb-3">
        <div className="flex items-center gap-2 text-muted-foreground">
          <ChurchLogo url={logoUrl} className="h-[14mm] w-auto max-w-[40mm] shrink-0 object-contain" />
          <p>{liturgy.church_name}</p>
        </div>
        <h1 className="text-2xl font-semibold">{liturgy.service_name}</h1>
        <p>{longDate(liturgy.date, i18n.language)}{liturgy.time && ` · ${liturgy.time}`}</p>
      </header>
      {items.map((item) => (
        <section key={item.id} className="space-y-1 border-b border-border py-3">
          {!musician && (
            <h2 className="break-after-avoid text-lg font-semibold">{item.title}</h2>
          )}
          {!musician && opts.assignments && item.duty && (
            <p className="break-after-avoid text-muted-foreground">
              {item.duty.name}{people(item.duty.id) && `: ${people(item.duty.id)}`}
            </p>
          )}
          {!musician && item.text && <p className="whitespace-pre-line">{item.text}</p>}
          {!musician && item.reading && (
            <div className="break-inside-avoid space-y-1">
              <p className="font-medium">
                {item.reading.reference_display}{item.reading.translation_code && ` (${item.reading.translation_code})`}
              </p>
              {opts.readings && <p className="whitespace-pre-line">{item.reading.text}</p>}
            </div>
          )}
          {(item.songs ?? []).map((song, i) => (
            <Song key={`${song.song_id}-${i}`} song={song} render={render} opts={opts} />
          ))}
        </section>
      ))}
      {!musician && opts.assignments && others.length > 0 && (
        <section className="break-inside-avoid space-y-1 border-b border-border py-3">
          <h2 className="text-lg font-semibold">{t("published.team")}</h2>
          <ul>{others.map((a, i) => <li key={i}>{a.duty.name}: {a.name}</li>)}</ul>
        </section>
      )}
      {render.show_credits && content.licence_footer && (
        <footer className="whitespace-pre-line pt-3 text-sm text-muted-foreground">{content.licence_footer}</footer>
      )}
    </article>
  );
}

function Song({ song, render, opts }: { song: PublishedSongView; render: Render; opts: PrintOptions }) {
  const { t } = useTranslation();
  const musician = opts.variant === "musician";
  const sections = new Map((song.sections ?? []).map((s) => [s.id, s]));
  const key = opts.keys ? formatKey(song.key, render.key_display) : "";
  const hymnal = [song.hymnal_source, song.hymnal_number].filter(Boolean).join(" ");
  const credit = [song.copyright_line || song.copyright_holder, song.ccli_song_number && t("published.ccli", { number: song.ccli_song_number })].filter(Boolean).join(" · ");
  const entries = song.entries ?? [];
  return (
    <div className="space-y-1 pt-1">
      {/* The heading stays with the first row of the sequence. */}
      <div className="break-inside-avoid">
        <h3 className="break-after-avoid font-medium">
          {song.title}
          {(hymnal || key) && <span className="font-normal text-muted-foreground"> — {[hymnal, key].filter(Boolean).join(" · ")}</span>}
        </h3>
        {opts.notes && song.note && <p className="break-after-avoid text-sm">{song.note}</p>}
        {entries.slice(0, 1).map((e, i) => <Row key={i} e={e} sections={sections} render={render} opts={opts} firstLines={musician} />)}
      </div>
      {entries.slice(1).map((e, i) => <Row key={i} e={e} sections={sections} render={render} opts={opts} firstLines={musician} />)}
      {!musician && render.show_credits && credit && <p className="text-sm text-muted-foreground">{credit}</p>}
    </div>
  );
}

// Row is one entry of the sequence; it is never split across pages.
function Row({ e, sections, render, opts, firstLines }: {
  e: NonNullable<PublishedSongView["entries"]>[number];
  sections: Map<string, NonNullable<PublishedSongView["sections"]>[number]>;
  render: Render; opts: PrintOptions; firstLines: boolean;
}) {
  const { t } = useTranslation();
  const sec = sections.get(e.section_id);
  const change = opts.keys ? keyChangeText(t, e.key_change, render.key_display) : "";
  const text = !sec ? "" : firstLines || opts.lyrics === "first_lines" ? firstLine(sec.text) : sec.text;
  return (
    <div className="break-inside-avoid space-y-0.5">
      <p className="text-sm font-medium">{[sec?.label, e.part?.name, change].filter(Boolean).join(" · ")}</p>
      {opts.notes && e.note && <p className="text-sm italic">{e.note}</p>}
      {text && <p className="whitespace-pre-line">{text}</p>}
    </div>
  );
}
