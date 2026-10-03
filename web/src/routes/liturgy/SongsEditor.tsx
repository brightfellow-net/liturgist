// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Field } from "@/components/Field";
import { formatKey, liturgyLimits } from "@/lib/liturgy";
import { sectionName } from "@/lib/library";
import { songQuery } from "@/lib/library";
import { fillFromDefault, move, newUid, type EntryDraft, type SongDraft } from "./itemDraft";

type Part = { id: string; name: string };

// SongsEditor edits the songs of one song item and the sequence of each
// (11 §4). Every change stays in the draft until the card's Save item.
export function SongsEditor({ songs, onChange, parts, keyDisplay, disabled }: {
  songs: SongDraft[];
  onChange: (songs: SongDraft[]) => void;
  parts: Part[];
  keyDisplay: string | undefined;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const set = (i: number, next: SongDraft) => onChange(songs.map((s, j) => (j === i ? next : s)));
  return (
    <ol className="space-y-4">
      {songs.map((s, i) => (
        <li key={s.uid}>
          <SongBlock
            song={s} n={i + 1} count={songs.length} parts={parts} keyDisplay={keyDisplay} disabled={disabled}
            onChange={(next) => set(i, next)}
            onMove={(d) => onChange(move(songs, i, d))}
            onRemove={() => onChange(songs.filter((_, j) => j !== i))}
          />
        </li>
      ))}
      {songs.length === 0 && <li className="text-muted-foreground">{t("liturgy.song.none")}</li>}
    </ol>
  );
}

function SongBlock({ song, n, count, parts, keyDisplay, disabled, onChange, onMove, onRemove }: {
  song: SongDraft; n: number; count: number; parts: Part[]; keyDisplay: string | undefined; disabled?: boolean;
  onChange: (s: SongDraft) => void; onMove: (delta: number) => void; onRemove: () => void;
}) {
  const { t } = useTranslation();
  const title = song.song_title || t("liturgy.song.removed_title");
  const label = t("liturgy.song.n", { n });
  const sections = song.summary?.sections ?? [];
  const library = useQuery({ ...songQuery(song.song_id), enabled: false });
  const setEntries = (entries: EntryDraft[]) => onChange({ ...song, entries });
  const keyLabel = formatKey(song.key, keyDisplay);
  return (
    <fieldset className="space-y-3 rounded-md border border-border p-3">
      <legend className="px-1 font-semibold">{label}: {title}</legend>
      {!song.song_id && <p className="text-sm text-destructive">{t("liturgy.song.removed")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={disabled || n === 1} aria-label={`${t("planning.move_up")} ${label}`} onClick={() => onMove(-1)}>{t("planning.move_up")}</Button>
        <Button variant="outline" disabled={disabled || n === count} aria-label={`${t("planning.move_down")} ${label}`} onClick={() => onMove(1)}>{t("planning.move_down")}</Button>
        <Button variant="outline" disabled={disabled} aria-label={`${t("planning.remove")} ${label}`} onClick={onRemove}>{t("planning.remove")}</Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("liturgy.song.key")} hint={keyLabel && keyLabel !== song.key.trim() ? keyLabel : undefined}>
          <Input autoComplete="off" disabled={disabled} value={song.key} onChange={(e) => onChange({ ...song, key: e.target.value })} />
        </Field>
        <Field label={t("liturgy.song.note")}>
          <Input autoComplete="off" disabled={disabled} value={song.note} onChange={(e) => onChange({ ...song, note: e.target.value })} />
        </Field>
      </div>
      <h4 className="font-semibold">{t("liturgy.sequence.title")}</h4>
      <ol className="space-y-3">
        {song.entries.map((e, i) => (
          <li key={e.uid}>
            <EntryRow
              entry={e} n={i + 1} count={song.entries.length} songId={song.song_id} sections={sections} parts={parts}
              keyDisplay={keyDisplay} disabled={disabled}
              onChange={(next) => setEntries(song.entries.map((x, j) => (j === i ? next : x)))}
              onMove={(d) => setEntries(move(song.entries, i, d))}
              onRemove={() => setEntries(song.entries.filter((_, j) => j !== i))}
            />
          </li>
        ))}
      </ol>
      {song.entries.length === 0 && <p className="text-muted-foreground">{t("liturgy.sequence.none")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline" disabled={disabled || sections.length === 0 || song.entries.length >= liturgyLimits.entriesPerSong}
          onClick={() => setEntries([...song.entries, { uid: newUid(), section_id: sections[0]?.id ?? "", section_label: "", singing_part_id: "", key_change: "", note: "" }])}
        >
          {t("liturgy.sequence.add")}
        </Button>
        <Button
          variant="outline" disabled={disabled || !song.song_id}
          onClick={async () => {
            const result = library.data ?? (await library.refetch()).data;
            if (result) setEntries(fillFromDefault(result));
          }}
        >
          {t("liturgy.sequence.fill")}
        </Button>
      </div>
    </fieldset>
  );
}

function EntryRow({ entry, n, count, songId, sections, parts, keyDisplay, disabled, onChange, onMove, onRemove }: {
  entry: EntryDraft; n: number; count: number; songId: string;
  sections: { id: string; kind: string; number: number | null; label?: string | null }[];
  parts: Part[]; keyDisplay: string | undefined; disabled?: boolean;
  onChange: (e: EntryDraft) => void; onMove: (delta: number) => void; onRemove: () => void;
}) {
  const { t } = useTranslation();
  const [lyrics, setLyrics] = useState(false);
  const song = useQuery({ ...songQuery(songId), enabled: lyrics && songId !== "" });
  const known = sections.find((s) => s.id === entry.section_id);
  const name = known ? sectionName(t, { kind: known.kind as never, number: known.number, label: known.label }) : entry.section_label || t("liturgy.sequence.removed_section");
  const label = t("liturgy.sequence.entry_n", { n });
  const text = song.data?.sections?.find((s) => s.id === entry.section_id)?.text;
  const change = formatKey(entry.key_change, keyDisplay);
  return (
    <fieldset className="space-y-2 rounded-md bg-muted/30 p-3">
      <legend className="px-1 text-sm font-semibold">{label}: {name}</legend>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("liturgy.sequence.section")} error={!known ? t("liturgy.sequence.choose_section") : undefined}>
          <Select disabled={disabled} value={known ? entry.section_id : ""} onChange={(e) => onChange({ ...entry, section_id: e.target.value })}>
            {!known && <option value="">{entry.section_label || t("liturgy.sequence.removed_section")}</option>}
            {sections.map((s) => <option key={s.id} value={s.id}>{sectionName(t, { kind: s.kind as never, number: s.number, label: s.label })}</option>)}
          </Select>
        </Field>
        <Field label={t("liturgy.sequence.part")}>
          <Select disabled={disabled} value={entry.singing_part_id} onChange={(e) => onChange({ ...entry, singing_part_id: e.target.value })}>
            <option value="">{t("planning.none")}</option>
            {parts.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </Field>
        <Field label={t("liturgy.sequence.key_change")} hint={change && change !== entry.key_change.trim() ? t("liturgy.key_change_to", { key: change }) : undefined}>
          <Input autoComplete="off" disabled={disabled} value={entry.key_change} onChange={(e) => onChange({ ...entry, key_change: e.target.value })} />
        </Field>
        <Field label={t("liturgy.song.note")}>
          <Input autoComplete="off" disabled={disabled} value={entry.note} onChange={(e) => onChange({ ...entry, note: e.target.value })} />
        </Field>
      </div>
      {songId !== "" && known && (
        <div>
          <Button variant="ghost" aria-expanded={lyrics} onClick={() => setLyrics(!lyrics)}>
            {lyrics ? t("liturgy.sequence.hide_lyrics") : t("liturgy.sequence.show_lyrics")}
          </Button>
          {lyrics && (song.isPending ? <p role="status">{t("app.loading")}</p> : <p className="whitespace-pre-line rounded-md border border-border p-3">{text}</p>)}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={disabled || n === 1} aria-label={`${t("planning.move_up")} ${label}`} onClick={() => onMove(-1)}>{t("planning.move_up")}</Button>
        <Button variant="outline" disabled={disabled || n === count} aria-label={`${t("planning.move_down")} ${label}`} onClick={() => onMove(1)}>{t("planning.move_down")}</Button>
        <Button variant="outline" disabled={disabled} aria-label={`${t("planning.remove")} ${label}`} onClick={onRemove}>{t("planning.remove")}</Button>
      </div>
    </fieldset>
  );
}
