// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm, type UseFormReturn } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslation } from "react-i18next";
import type { Me, SongView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { fieldErrors, isCode } from "@/lib/errors";
import { licenceStatuses, songLanguages, songQuery, songsQuery } from "@/lib/library";
import { useDebounced } from "@/lib/useDebounced";
import { hasScope } from "@/lib/scopes";
import { paths, songPath } from "../paths";
import { ArrangementEditor } from "./ArrangementEditor";
import { SectionsEditor } from "./SectionsEditor";
import { emptyValues, songSchema, toRequest, toValues, type SongValues } from "./songForm";

const textarea = "block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive";

// SongFormPage adds a song (/library/songs/new) or edits one
// (/library/songs/{id}/edit) (06 §4).
export function SongFormPage() {
  const { id } = useParams();
  const me = useOutletContext<Me>();
  if (!hasScope(me, "library.edit")) return <Navigate to={id ? songPath(id) : paths.library} replace />;
  return id ? <EditSong id={id} /> : <SongForm />;
}

function EditSong({ id }: { id: string }) {
  const { t } = useTranslation();
  const song = useQuery(songQuery(id));
  // Reloading after a version conflict remounts the form with the new copy.
  const [generation, setGeneration] = useState(0);
  if (song.error) {
    return isCode(song.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("library.not_found")}</Alert>
        <Link className="underline" to={paths.library}>{t("library.back")}</Link>
      </div>
    ) : <ErrorAlert error={song.error} onRetry={() => void song.refetch()} />;
  }
  if (!song.data) return <p role="status">{t("app.loading")}</p>;
  if (!song.data.actions.edit) return <Navigate to={songPath(id)} replace />;
  const reload = async () => {
    const result = await song.refetch();
    if (result.data) setGeneration((g) => g + 1);
  };
  return <SongForm key={generation + ":" + song.data.version} song={song.data} onReload={reload} />;
}

function SongForm({ song, onReload }: { song?: SongView; onReload?: () => Promise<void> }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const form = useForm<SongValues>({
    resolver: zodResolver(songSchema),
    defaultValues: song ? toValues(song) : emptyValues(i18n.resolvedLanguage ?? "id"),
  });
  const [conflict, setConflict] = useState(false);

  const save = useMutation({
    mutationFn: (v: SongValues) =>
      song
        ? call(api.PATCH("/songs/{id}", { params: { path: { id: song.id } }, body: { ...toRequest(v), version: song.version } }))
        : call(api.POST("/songs", { body: toRequest(v) })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(songQuery(saved.id).queryKey, saved);
      await queryClient.invalidateQueries({ queryKey: ["songs"] });
      void navigate(songPath(saved.id));
    },
    onError: (err) => {
      setConflict(isCode(err, "version_conflict"));
      for (const f of fieldErrors(err)) {
        if (f in form.getValues() && f !== "sections" && f !== "arrangement") {
          form.setError(f as keyof SongValues, { message: "common.field_invalid" });
        }
      }
    },
  });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  const reg = form.register;

  return (
    <form noValidate className="max-w-3xl space-y-8" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <h1 className="text-2xl font-semibold">{song ? t("library.edit_title", { title: song.title }) : t("library.new_title")}</h1>
      {conflict ? (
        <Alert variant="error" className="space-y-2">
          <p>{t("library.conflict")}</p>
          <Button variant="outline" onClick={() => void onReload?.()}>{t("library.reload")}</Button>
        </Alert>
      ) : (
        <ErrorAlert error={save.error} />
      )}

      <fieldset className="space-y-4">
        <legend className="mb-2 text-xl font-semibold">{t("library.group_title")}</legend>
        <Field label={t("library.title_label")} error={msg(e.title?.message)}>
          <Input autoComplete="off" {...reg("title")} />
        </Field>
        <Field label={t("library.alt_titles")} hint={t("library.alt_titles_hint")} error={msg(e.alt_titles?.message)}>
          <textarea rows={2} className={textarea} {...reg("alt_titles")} />
        </Field>
        <Field label={t("library.language")}>
          <Select {...reg("language")}>
            {songLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="mb-2 text-xl font-semibold">{t("library.group_hymnal")}</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("library.hymnal_source")} hint={t("library.hymnal_source_hint")} error={msg(e.hymnal_source?.message)}>
            <Input autoComplete="off" {...reg("hymnal_source")} />
          </Field>
          <Field label={t("library.hymnal_number")} hint={t("library.hymnal_number_hint")} error={msg(e.hymnal_number?.message)}>
            <Input autoComplete="off" {...reg("hymnal_number")} />
          </Field>
        </div>
        <HymnalWarning form={form} ownId={song?.id} />
        <Field label={t("library.default_key")} error={msg(e.default_key?.message)}>
          <Input autoComplete="off" className="sm:max-w-40" {...reg("default_key")} />
        </Field>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="mb-2 text-xl font-semibold">{t("library.group_credits")}</legend>
        <Field label={t("library.lyricist")} error={msg(e.lyricist?.message)}><Input autoComplete="off" {...reg("lyricist")} /></Field>
        <Field label={t("library.composer")} error={msg(e.composer?.message)}><Input autoComplete="off" {...reg("composer")} /></Field>
        <Field label={t("library.translator")} error={msg(e.translator?.message)}><Input autoComplete="off" {...reg("translator")} /></Field>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="mb-2 text-xl font-semibold">{t("library.group_licence")}</legend>
        <Field label={t("library.copyright_holder")} error={msg(e.copyright_holder?.message)}>
          <Input autoComplete="off" {...reg("copyright_holder")} />
        </Field>
        <Field label={t("library.copyright_line")} error={msg(e.copyright_line?.message)}>
          <Input autoComplete="off" {...reg("copyright_line")} />
        </Field>
        <Field label={t("library.ccli")} error={msg(e.ccli_song_number?.message)}>
          <Input inputMode="numeric" autoComplete="off" className="sm:max-w-60" {...reg("ccli_song_number")} />
        </Field>
        <Field label={t("library.licence_status")}>
          <Select {...reg("licence_status")}>
            {licenceStatuses.map((s) => <option key={s} value={s}>{t(`library.licence.${s}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("library.licence_notes")} error={msg(e.licence_notes?.message)}>
          <textarea rows={3} className={textarea} {...reg("licence_notes")} />
        </Field>
      </fieldset>

      <SectionsEditor form={form} />
      <ArrangementEditor form={form} />

      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>{t("library.save")}</Button>
        <Link className="inline-flex min-h-12 items-center rounded-md border border-border px-4 hover:bg-muted" to={song ? songPath(song.id) : paths.library}>
          {t("common.cancel")}
        </Link>
      </div>
    </form>
  );
}

// HymnalWarning asks whether the hymnal number is already in the library
// and says so with links; the song can still be saved (06 §3, P-53).
function HymnalWarning({ form, ownId }: { form: UseFormReturn<SongValues>; ownId?: string }) {
  const { t } = useTranslation();
  const source = useDebounced(form.watch("hymnal_source").trim());
  const number = useDebounced(form.watch("hymnal_number").trim());
  const found = useQuery({
    ...songsQuery({ hymnal_source: source, hymnal_number: number, limit: 5 }),
    enabled: source !== "" && number !== "",
  });
  const others = (found.data?.items ?? []).filter((s) => s.id !== ownId);
  if (source === "" || number === "" || others.length === 0) return null;
  return (
    <Alert className="space-y-1">
      <p>{t("library.hymnal_exists")}</p>
      <ul className="list-disc pl-6">
        {others.map((s) => <li key={s.id}><Link className="underline" to={songPath(s.id)}>{s.title}</Link></li>)}
      </ul>
    </Alert>
  );
}
