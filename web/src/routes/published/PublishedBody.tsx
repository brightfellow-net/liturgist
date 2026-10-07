// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from "react-i18next";
import type { PublishedContentView, PublishedSongView } from "@liturgist/api-client";
import { ChurchLogo } from "@/components/ChurchLogo";
import { formatKey, keyChangeText, longDate } from "@/lib/liturgy";

type Render = { key_display: string; show_credits: boolean };

// PublishedBody shows a stored copy and nothing else (13 §3): every text is
// plain text, so it is escaped by React and never taken as HTML.
export function PublishedBody({ content, render, mine, logoUrl }: { content: PublishedContentView; render: Render; mine?: ReadonlySet<string>; logoUrl?: string | null }) {
  const { t, i18n } = useTranslation();
  const { liturgy } = content;
  const items = content.items ?? [];
  const assignments = content.assignments ?? [];
  const people = (dutyID: string) => assignments.filter((a) => a.duty.id === dutyID).map((a) => a.name).join(", ");
  // The ids of the duties that have items, so the team list shows only the rest.
  const shown = new Set(items.map((i) => i.duty?.id).filter(Boolean));
  const others = assignments.filter((a) => !shown.has(a.duty.id));
  return (
    <div lang={liturgy.language} className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold">{liturgy.service_name}</h1>
        <p>{longDate(liturgy.date, i18n.language)}{liturgy.time && ` · ${liturgy.time}`}</p>
        {/* The logo is the church's current one, not part of the stored copy (15 §2, L-5). */}
        <div className="flex items-center gap-2 text-muted-foreground">
          <ChurchLogo url={logoUrl} className="h-8 w-auto max-w-24 shrink-0 object-contain" />
          <p>{liturgy.church_name}</p>
        </div>
      </header>
      {items.map((item) => {
        const yours = !!item.duty && !!mine?.has(item.duty.id);
        return (
        <section
          key={item.id}
          aria-labelledby={`item-${item.id}`}
          className={"space-y-2 border-t border-border pt-4" + (yours ? " border-l-4 border-l-primary pl-3" : "")}
        >
          {/* Marked in words as well as by the bar, so colour is not the only cue. */}
          {yours && <p className="text-sm font-semibold uppercase">{t("reading.your_part")}</p>}
          <h2 id={`item-${item.id}`} tabIndex={yours ? -1 : undefined} className="text-lg font-semibold">{item.title}</h2>
          {item.duty && (
            <p className="text-muted-foreground">
              {item.duty.name}{people(item.duty.id) && `: ${people(item.duty.id)}`}
            </p>
          )}
          {item.text && <p className="whitespace-pre-line">{item.text}</p>}
          {item.reading && (
            <div className="space-y-1">
              <p className="font-medium">
                {item.reading.reference_display}{item.reading.translation_code && ` (${item.reading.translation_code})`}
              </p>
              <p className="whitespace-pre-line">{item.reading.text}</p>
              {item.reading.attribution && <p className="text-sm text-muted-foreground">{item.reading.attribution}</p>}
            </div>
          )}
          {(item.songs ?? []).map((song, i) => (
            <Song key={`${song.song_id}-${i}`} song={song} render={render} />
          ))}
        </section>
        );
      })}
      {others.length > 0 && (
        <section aria-labelledby="team" className="space-y-2 border-t border-border pt-4">
          <h2 id="team" className="text-lg font-semibold">{t("published.team")}</h2>
          <ul className="space-y-1">
            {others.map((a, i) => <li key={i}>{a.duty.name}: {a.name}</li>)}
          </ul>
        </section>
      )}
      {render.show_credits && content.licence_footer && (
        <footer className="border-t border-border pt-4 text-sm text-muted-foreground whitespace-pre-line">{content.licence_footer}</footer>
      )}
    </div>
  );
}

function Song({ song, render }: { song: PublishedSongView; render: Render }) {
  const { t } = useTranslation();
  const sections = new Map((song.sections ?? []).map((s) => [s.id, s]));
  const key = formatKey(song.key, render.key_display);
  const hymnal = [song.hymnal_source, song.hymnal_number].filter(Boolean).join(" ");
  const credit = [song.copyright_line || song.copyright_holder, song.ccli_song_number && t("published.ccli", { number: song.ccli_song_number })].filter(Boolean).join(" · ");
  return (
    <div className="space-y-2">
      <h3 className="font-medium">
        {song.title}
        {(hymnal || key) && <span className="font-normal text-muted-foreground"> — {[hymnal, key].filter(Boolean).join(" · ")}</span>}
      </h3>
      {song.note && <p className="text-sm">{song.note}</p>}
      {(song.entries ?? []).map((e, i) => {
        const sec = sections.get(e.section_id);
        const change = keyChangeText(t, e.key_change, render.key_display);
        return (
          <div key={i} className="space-y-0.5">
            <p className="text-sm font-medium">
              {[sec?.label, e.part?.name, change].filter(Boolean).join(" · ")}
            </p>
            {e.note && <p className="text-sm italic">{e.note}</p>}
            {sec && <p className="whitespace-pre-line">{sec.text}</p>}
          </div>
        );
      })}
      {render.show_credits && credit && <p className="text-sm text-muted-foreground">{credit}</p>}
    </div>
  );
}
