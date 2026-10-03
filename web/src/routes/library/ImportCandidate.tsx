// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslation } from "react-i18next";
import type { ImportCandidateView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { failureText, warningText } from "@/lib/imports";
import { hymnalText, sectionName, songLanguages } from "@/lib/library";
import { songPath } from "../paths";
import { ArrangementEditor } from "./ArrangementEditor";
import { draftToValues, valuesToDraft } from "./importDraft";
import { SectionsEditor } from "./SectionsEditor";
import { songSchema, type SongValues } from "./songForm";

const textarea = "block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive";

// candidateWarnings are the notes the import made about a candidate, in words.
export function Warnings({ c }: { c: ImportCandidateView }) {
  const { t } = useTranslation();
  const list = c.warnings ?? [];
  if (list.length === 0) return null;
  return (
    <ul className="list-disc space-y-1 pl-6 text-sm">
      {list.map((w) => <li key={w}>{warningText(t, w)}</li>)}
    </ul>
  );
}

// DuplicateNotice says the song may already be in the library, with a link.
export function DuplicateNotice({ c }: { c: ImportCandidateView }) {
  const { t } = useTranslation();
  const d = c.duplicate_of;
  if (!d) return null;
  return (
    <p className="text-sm">
      {t("import.duplicate")} <Link className="underline" to={songPath(d.id)}>{d.title}</Link>
      {hymnalText({ hymnal_source: d.hymnal_source ?? "", hymnal_number: d.hymnal_number ?? "" }) && ` (${hymnalText({ hymnal_source: d.hymnal_source ?? "", hymnal_number: d.hymnal_number ?? "" })})`}
    </p>
  );
}

// SectionsPreview shows a draft's sections as they would be saved.
function SectionsPreview({ c }: { c: ImportCandidateView }) {
  const { t } = useTranslation();
  const sections = c.draft.sections ?? [];
  return (
    <div className="space-y-3">
      {sections.map((s, i) => (
        <section key={i} className="space-y-1">
          <h4 className="font-semibold">{sectionName(t, { kind: s.kind, number: s.number ?? null, label: s.label ?? null })}</h4>
          <p className="whitespace-pre-wrap">{s.text}</p>
        </section>
      ))}
    </div>
  );
}

// CandidatePanel is the selected candidate: its preview, the choice of what
// to do with it, and the editor (08 §6).
export function CandidatePanel({ batchId, c, canChange }: { batchId: string; c: ImportCandidateView; canChange: boolean }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  // choice is what the radio buttons show. It moves at once; a merge is only
  // saved from its preview, and a choice the server refuses is taken back.
  const saved = c.decision === "merge" || c.error_code === "target_changed" ? "merge" : c.decision;
  const [choice, setChoice] = useState(saved);
  const done = c.outcome === "applied";
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["import", batchId] });

  const decide = useMutation({
    mutationFn: (d: { decision: "pending" | "accept" | "merge" | "skip"; merge_into?: string; merge_target_version?: number; remove_unmatched?: boolean }) =>
      call(api.PATCH("/imports/{id}/candidates", { params: { path: { id: batchId } }, body: { decisions: [{ id: c.id, ...d }] } })),
    onSuccess: refresh,
    // A target changed after the preview: show the preview as it is now.
    // A merge stays open so that the member sees the preview as it is now.
    onError: (_err, d) => {
      if (d.decision !== "merge") setChoice(saved);
      void refresh();
    },
  });
  const choose = (decision: "accept" | "skip") => {
    setChoice(decision);
    decide.mutate({ decision });
  };
  const current = choice;
  const merging = choice === "merge";

  return (
    <article aria-labelledby="candidate-title" className="space-y-4 rounded-md border border-border p-4">
      <h3 id="candidate-title" className="text-xl font-semibold">{c.draft.title}</h3>
      {done ? (
        <p>
          {t("import.done_song")}{" "}
          {c.applied_song_id && <Link className="underline" to={songPath(c.applied_song_id)}>{t("import.open_song")}</Link>}
        </p>
      ) : (
        <>
          {c.outcome === "failed" && c.error_code && <Alert variant="error">{failureText(t, c.error_code)}</Alert>}
          <ErrorAlert error={decide.error} />
          {canChange && (
            <fieldset className="space-y-2">
              <legend className="font-semibold">{t("import.choice")}</legend>
              <label className="flex min-h-12 items-center gap-3">
                <input type="radio" name={`decision-${c.id}`} className="size-5" checked={current === "accept"} onChange={() => choose("accept")} />
                {t("import.add_new")}
              </label>
              {c.duplicate_of && (
                <label className="flex min-h-12 items-center gap-3">
                  <input type="radio" name={`decision-${c.id}`} className="size-5" checked={current === "merge"} onChange={() => setChoice("merge")} />
                  {t("import.merge_into", { title: c.duplicate_of.title })}
                </label>
              )}
              <label className="flex min-h-12 items-center gap-3">
                <input type="radio" name={`decision-${c.id}`} className="size-5" checked={current === "skip"} onChange={() => choose("skip")} />
                {t("import.skip")}
              </label>
            </fieldset>
          )}
          {merging && c.duplicate_of && canChange && (
            <MergePreview
              batchId={batchId} c={c} target={c.merge_into ?? c.duplicate_of.id} pending={decide.isPending}
              onSave={(remove, version) => decide.mutate({ decision: "merge", merge_into: c.merge_into ?? c.duplicate_of?.id, merge_target_version: version, remove_unmatched: remove })}
            />
          )}
        </>
      )}

      {editing && !done ? (
        <DraftEditor batchId={batchId} c={c} onClose={() => setEditing(false)} />
      ) : (
        <>
          <SectionsPreview c={c} />
          {!done && canChange && <Button variant="outline" onClick={() => setEditing(true)}>{t("import.edit")}</Button>}
        </>
      )}
    </article>
  );
}

// MergePreview shows what merging would do, exactly as Apply will do it
// (08 §3.2), and saves the choice with the version it was shown for.
function MergePreview({ batchId, c, target, pending, onSave }: {
  batchId: string;
  c: ImportCandidateView;
  target: string;
  pending: boolean;
  onSave: (remove: boolean, targetVersion: number) => void;
}) {
  const { t } = useTranslation();
  const [remove, setRemove] = useState(c.remove_unmatched);
  const preview = useQuery({
    queryKey: ["import", batchId, "merge-preview", c.id, target, remove, c.error_code ?? ""],
    queryFn: () => call(api.GET("/imports/{id}/candidates/{cid}/merge-preview", {
      params: { path: { id: batchId, cid: c.id }, query: { merge_into: target, remove_unmatched: remove } },
    })),
    staleTime: 0,
  });
  const sections = preview.data?.sections ?? [];
  const unmatched = sections.filter((s) => s.status === "kept" || s.status === "removed").length;
  return (
    <section aria-labelledby={`merge-${c.id}`} className="space-y-3 rounded-md border border-border p-3">
      <h4 id={`merge-${c.id}`} className="font-semibold">{t("import.merge.title")}</h4>
      {c.error_code === "target_changed" && <Alert>{failureText(t, "target_changed")}</Alert>}
      <ErrorAlert error={preview.error} onRetry={() => void preview.refetch()} />
      {preview.isPending && <p role="status">{t("app.loading")}</p>}
      {preview.data && (
        <>
          <ul className="space-y-3">
            {sections.map((s, i) => (
              <li key={i} className="space-y-1">
                <p className="font-semibold">{s.label} <span className="font-normal text-muted-foreground">({t(`import.merge.status.${s.status}`)})</span></p>
                {s.status === "updated" ? (
                  <div className="grid gap-2 sm:grid-cols-2">
                    <div><p className="text-sm text-muted-foreground">{t("import.merge.before")}</p><p className="whitespace-pre-wrap">{s.old_text}</p></div>
                    <div><p className="text-sm text-muted-foreground">{t("import.merge.after")}</p><p className="whitespace-pre-wrap">{s.new_text}</p></div>
                  </div>
                ) : (
                  <p className="whitespace-pre-wrap">{s.status === "new" ? s.new_text : s.old_text}</p>
                )}
              </li>
            ))}
          </ul>
          {unmatched > 0 && (
            <label className="flex min-h-12 items-center gap-3">
              <input type="checkbox" className="size-5" checked={remove} onChange={(e) => setRemove(e.target.checked)} />
              {t("import.merge.remove", { count: unmatched })}
            </label>
          )}
          <Button disabled={pending} onClick={() => onSave(remove, preview.data.target_version)}>{t("import.merge.confirm")}</Button>
        </>
      )}
    </section>
  );
}

// DraftEditor edits a candidate before it is saved to the library: the same
// parts as the song form (06 §4) apart from the licence fields, which keep
// the values they have.
function DraftEditor({ batchId, c, onClose }: { batchId: string; c: ImportCandidateView; onClose: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const form = useForm<SongValues>({ resolver: zodResolver(songSchema), defaultValues: draftToValues(c.draft) });
  const save = useMutation({
    mutationFn: (v: SongValues) =>
      call(api.PATCH("/imports/{id}/candidates/{cid}", { params: { path: { id: batchId, cid: c.id } }, body: { draft: valuesToDraft(v) } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["import", batchId] });
      onClose();
    },
    onError: (err) => {
      for (const f of fieldErrors(err)) {
        if (f.startsWith("draft.")) {
          const name = f.slice(6) as keyof SongValues;
          if (name in form.getValues() && name !== "sections" && name !== "arrangement") form.setError(name, { message: "common.field_invalid" });
        }
      }
    },
  });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  const reg = form.register;
  return (
    <form noValidate className="space-y-6" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <h4 className="text-lg font-semibold">{t("import.edit_title")}</h4>
      <ErrorAlert error={save.error} />
      <Field label={t("library.title_label")} error={msg(e.title?.message)}><Input autoComplete="off" {...reg("title")} /></Field>
      <Field label={t("library.alt_titles")} hint={t("library.alt_titles_hint")} error={msg(e.alt_titles?.message)}>
        <textarea rows={2} className={textarea} {...reg("alt_titles")} />
      </Field>
      <Field label={t("library.language")}>
        <Select {...reg("language")}>
          {songLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
        </Select>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("library.hymnal_source")} error={msg(e.hymnal_source?.message)}><Input autoComplete="off" {...reg("hymnal_source")} /></Field>
        <Field label={t("library.hymnal_number")} error={msg(e.hymnal_number?.message)}><Input autoComplete="off" {...reg("hymnal_number")} /></Field>
      </div>
      <SectionsEditor form={form} />
      <ArrangementEditor form={form} />
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>{t("import.edit_save")}</Button>
        <Button variant="outline" onClick={onClose}>{t("common.cancel")}</Button>
      </div>
    </form>
  );
}
