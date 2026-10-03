// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { LiturgyItemView, LiturgyView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { ApiError, isCode } from "@/lib/errors";
import { liturgyLimits, liturgyQuery, withItem } from "@/lib/liturgy";
import { textItemTypes, type ItemType } from "@/lib/planning";
import { ReadingPicker, SongPicker } from "./Pickers";
import { SongsEditor } from "./SongsEditor";
import {
  draftOf, fieldsDirty, isDirty, missingSections, patchBody, songsBody, songsDirty,
  type ItemDraft,
} from "./itemDraft";

type Named = { id: string; name: string };

// ItemCard is one item of the liturgy as its own form with its own Save item
// (11 §4): the version it sends is the one it last loaded, and what the user
// typed is never replaced by someone else's change.
export function ItemCard({ liturgy, item, place, count, duties, parts, keyDisplay, onMove, onRemove }: {
  liturgy: LiturgyView;
  item: LiturgyItemView;
  place: number;
  count: number;
  duties: Named[];
  parts: Named[];
  keyDisplay: string | undefined;
  onMove: (delta: number) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const editable = liturgy.actions.edit;
  const type = item.item_type as ItemType;
  const [base, setBase] = useState(item);
  const [draft, setDraft] = useState<ItemDraft>(() => draftOf(item));
  const [saved, setSaved] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);
  const [theirs, setTheirs] = useState<LiturgyItemView | null>(null);
  const dirty = isDirty(draft, base);

  // A clean card follows the saved item; a dirty one keeps its own copy of the
  // version, so a save is checked against what the user was looking at.
  if (!dirty && !pending && item !== base && item.version !== base.version) {
    setBase(item);
    setDraft(draftOf(item));
  }

  const patch = (changes: Partial<ItemDraft>) => {
    setDraft({ ...draft, ...changes });
    setSaved(false);
  };
  const store = (next: LiturgyItemView, liturgyVersion: number) => {
    queryClient.setQueryData<LiturgyView>(liturgyQuery(liturgy.id).queryKey, (l) => (l ? withItem(l, next, liturgyVersion) : l));
    setBase(next);
    void queryClient.invalidateQueries({ queryKey: [...liturgyQuery(liturgy.id).queryKey, "edits"] });
  };

  // freshItem re-reads the liturgy and returns this item as it is saved now.
  const freshItem = async (): Promise<LiturgyItemView | null> => {
    const l = await queryClient.fetchQuery({ ...liturgyQuery(liturgy.id), staleTime: 0 });
    return (l.items ?? []).find((x) => x.id === item.id) ?? null;
  };

  const save = async (from: LiturgyItemView) => {
    setPending(true);
    setError(null);
    setConflict(false);
    setTheirs(null);
    let current = from;
    try {
      if (fieldsDirty(draft, current)) {
        const r = await call(api.PATCH("/liturgies/{id}/items/{iid}", {
          params: { path: { id: liturgy.id, iid: item.id } }, body: patchBody(draft, current, current.version),
        }));
        current = r.item;
        store(r.item, r.liturgy_version);
      }
      if (type === "song" && songsDirty(draft, current)) {
        const r = await call(api.PUT("/liturgies/{id}/items/{iid}/songs", {
          params: { path: { id: liturgy.id, iid: item.id } }, body: songsBody(draft, current.version),
        }));
        current = r.item;
        store(r.item, r.liturgy_version);
      }
      setDraft(draftOf(current));
      setSaved(true);
    } catch (err) {
      if (err instanceof ApiError && isCode(err, "version_conflict")) setConflict(true);
      else setError(err);
    } finally {
      setPending(false);
    }
  };

  const showTheirs = async () => {
    try {
      const fresh = await freshItem();
      if (fresh) setTheirs(fresh);
      else setError(new ApiError(404, { code: "not_found" }));
    } catch (err) {
      setError(err);
    }
  };
  const keepMine = async () => {
    try {
      const fresh = await freshItem();
      if (!fresh) return setError(new ApiError(404, { code: "not_found" }));
      setBase(fresh);
      await save(fresh);
    } catch (err) {
      setError(err);
    }
  };

  const addSong = async (songId: string) => {
    setPending(true);
    setError(null);
    try {
      const r = await call(api.POST("/liturgies/{id}/items/{iid}/songs", {
        params: { path: { id: liturgy.id, iid: item.id } }, body: { version: base.version, song_id: songId },
      }));
      store(r.item, r.liturgy_version);
      setDraft((d) => ({ ...d, songs: draftOf(r.item).songs }));
    } catch (err) {
      if (err instanceof ApiError && isCode(err, "version_conflict")) setConflict(true);
      else setError(err);
    } finally {
      setPending(false);
    }
  };

  const heading = `${t("liturgy.item.n", { n: place })}: ${draft.title || item.title}`;
  const missing = missingSections(draft);
  const duty = duties.find((d) => d.id === draft.duty_id);

  if (!editable) {
    return (
      <article aria-label={heading} className="space-y-2 rounded-md border border-border p-4">
        <h3 className="text-lg font-semibold">{heading}</h3>
        <p className="text-sm text-muted-foreground">{t(`planning.item_types.${type}`)}{duty ? ` · ${duty.name}` : ""}</p>
        {textItemTypes.includes(type) && item.text && <p className="whitespace-pre-line">{item.text}</p>}
        {type === "reading" && item.reading_label && <p className="whitespace-pre-line">{item.reading_label}{item.reading ? `\n${item.reading.text}` : ""}</p>}
        {type === "song" && (
          <ol className="list-decimal pl-6">
            {(item.songs ?? []).map((s) => <li key={s.id}>{s.song_title}{s.key ? ` (${s.key})` : ""}</li>)}
          </ol>
        )}
      </article>
    );
  }

  return (
    <form
      noValidate aria-label={heading}
      className="space-y-3 rounded-md border border-border p-4"
      onSubmit={(e) => { e.preventDefault(); if (missing === 0) void save(base); }}
    >
      <h3 className="text-lg font-semibold">{heading}</h3>
      <p className="text-sm text-muted-foreground">{t(`planning.item_types.${type}`)}</p>
      <div className="flex flex-wrap gap-2">
        <Button id={`move-up-${item.id}`} variant="outline" disabled={place === 1} aria-label={`${t("planning.move_up")} ${heading}`} onClick={() => onMove(-1)}>{t("planning.move_up")}</Button>
        <Button id={`move-down-${item.id}`} variant="outline" disabled={place === count} aria-label={`${t("planning.move_down")} ${heading}`} onClick={() => onMove(1)}>{t("planning.move_down")}</Button>
        <ConfirmButton
          label={`${t("planning.remove")}`}
          question={t("liturgy.item.remove_question", { title: item.title })}
          confirmLabel={t("liturgy.item.remove_confirm", { title: item.title })}
          onConfirm={onRemove}
        />
      </div>
      {conflict && (
        <Alert variant="error" className="space-y-2">
          <p>{t("liturgy.item.conflict")}</p>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => void showTheirs()}>{t("liturgy.item.show_theirs")}</Button>
            <Button onClick={() => void keepMine()}>{t("liturgy.item.keep_mine")}</Button>
          </div>
        </Alert>
      )}
      {theirs && (
        <div className="space-y-1 rounded-md border border-border bg-muted/30 p-3">
          <h4 className="font-semibold">{t("liturgy.item.their_version")}</h4>
          <p>{theirs.title}{theirs.duty_id ? ` · ${duties.find((d) => d.id === theirs.duty_id)?.name ?? ""}` : ""}</p>
          {textItemTypes.includes(type) && <p className="whitespace-pre-line">{theirs.text}</p>}
          {type === "song" && <p>{(theirs.songs ?? []).map((s) => s.song_title).join(", ")}</p>}
          {type === "reading" && <p>{theirs.reading_label}</p>}
          <Button variant="outline" onClick={() => { setBase(theirs); setDraft(draftOf(theirs)); setConflict(false); setTheirs(null); }}>{t("liturgy.item.use_theirs")}</Button>
        </div>
      )}
      <ErrorAlert error={error} />
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("liturgy.item.title")}>
          <Input autoComplete="off" value={draft.title} onChange={(e) => patch({ title: e.target.value })} />
        </Field>
        <Field label={t("liturgy.item.duty")}>
          <Select value={draft.duty_id} onChange={(e) => patch({ duty_id: e.target.value })}>
            <option value="">{t("planning.none")}</option>
            {duties.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </Select>
        </Field>
      </div>

      {textItemTypes.includes(type) && (
        <Field label={t("liturgy.item.text")}>
          <textarea
            rows={4} value={draft.text} onChange={(e) => patch({ text: e.target.value })}
            className="min-h-24 w-full rounded-md border border-border bg-background px-3 py-2 text-base"
          />
        </Field>
      )}

      {type === "reading" && (
        <section aria-label={t("liturgy.reading.title")} className="space-y-2">
          {draft.reading ? (
            <div className="space-y-1 rounded-md border border-border p-3">
              <p className="font-medium">{draft.reading.label}</p>
              <p className="whitespace-pre-line">{draft.reading.text}</p>
              <Button variant="outline" onClick={() => patch({ reading: null })}>{t("liturgy.reading.clear")}</Button>
            </div>
          ) : (
            <p className="text-muted-foreground">{item.reading_label && item.reading_id === null ? t("liturgy.reading.removed", { label: item.reading_label }) : t("liturgy.reading.none")}</p>
          )}
          <ReadingPicker onPick={(r) => patch({ reading: r })} />
        </section>
      )}

      {type === "song" && (
        <section aria-label={t("liturgy.song.title")} className="space-y-3">
          <SongsEditor songs={draft.songs} onChange={(songs) => patch({ songs })} parts={parts} keyDisplay={keyDisplay} />
          {draft.songs.length >= liturgyLimits.songsPerItem ? (
            <p className="text-muted-foreground">{t("liturgy.song.max", { max: liturgyLimits.songsPerItem })}</p>
          ) : songsDirty(draft, base) ? (
            <p className="text-muted-foreground">{t("liturgy.song.save_first")}</p>
          ) : (
            <SongPicker onPick={(id) => void addSong(id)} pending={pending} />
          )}
        </section>
      )}

      {missing > 0 && <p className="text-destructive">{t("liturgy.sequence.missing", { count: missing })}</p>}
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={pending || !dirty || missing > 0 || draft.title.trim() === ""}>{t("liturgy.item.save")}</Button>
        <span aria-live="polite" className="text-sm">
          {dirty ? <strong>{t("liturgy.item.unsaved")}</strong> : saved ? t("liturgy.item.saved") : ""}
        </span>
      </div>
    </form>
  );
}
