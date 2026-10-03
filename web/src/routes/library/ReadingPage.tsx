// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { ReadingView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { fieldErrors, isCode } from "@/lib/errors";
import { readingQuery } from "@/lib/readings";
import { paths } from "../paths";

// ReadingPage shows one reading with its attribution; editors can change the
// text and the attribution or delete it (07 §5).
export function ReadingPage() {
  const { id = "" } = useParams();
  const { t } = useTranslation();
  const reading = useQuery(readingQuery(id));
  const [generation, setGeneration] = useState(0); // a reload after a conflict remounts the form
  if (reading.error) {
    return isCode(reading.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("readings.not_found")}</Alert>
        <Link className="underline" to={paths.readings}>{t("readings.back")}</Link>
      </div>
    ) : <ErrorAlert error={reading.error} onRetry={() => void reading.refetch()} />;
  }
  if (!reading.data) return <p role="status">{t("app.loading")}</p>;
  const reload = async () => {
    const result = await reading.refetch();
    if (result.data) setGeneration((g) => g + 1);
  };
  return <ReadingDetails key={generation + ":" + reading.data.version} reading={reading.data} onReload={reload} />;
}

const schema = z.object({
  text: z.string().trim().min(1, "readings.text_required").max(20000, "common.field_invalid"),
  attribution: z.string().trim().max(300, "common.field_invalid"),
});
type Values = z.infer<typeof schema>;

function ReadingDetails({ reading, onReload }: { reading: ReadingView; onReload: () => Promise<void> }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [conflict, setConflict] = useState(false);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { text: reading.text, attribution: reading.attribution } });

  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ["readings"] }),
    queryClient.invalidateQueries({ queryKey: ["reading-lookup"] }),
  ]);
  const save = useMutation({
    mutationFn: (v: Values) => call(api.PATCH("/readings/{id}", { params: { path: { id: reading.id } }, body: { ...v, version: reading.version } })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(readingQuery(saved.id).queryKey, saved);
      await refresh();
      setEditing(false);
    },
    onError: (err) => {
      setConflict(isCode(err, "version_conflict"));
      for (const f of fieldErrors(err)) {
        if (f === "text" || f === "attribution") form.setError(f, { message: "common.field_invalid" });
      }
    },
  });
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/readings/{id}", { params: { path: { id: reading.id } } })),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: ["reading", reading.id] });
      await refresh();
      void navigate(paths.readings);
    },
  });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);

  return (
    <article className="space-y-6">
      <p><Link className="underline" to={paths.readings}>{t("readings.back")}</Link></p>
      <header className="space-y-2">
        <h1 className="text-2xl font-semibold">{reading.canonical} ({reading.translation.code})</h1>
        <p className="text-muted-foreground">{reading.translation.name}</p>
      </header>

      {editing ? (
        <form noValidate className="max-w-3xl space-y-4" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
          {conflict ? (
            <Alert variant="error" className="space-y-2">
              <p>{t("readings.conflict")}</p>
              <Button variant="outline" onClick={() => void onReload()}>{t("library.reload")}</Button>
            </Alert>
          ) : <ErrorAlert error={save.error} />}
          <Field label={t("readings.text")} error={msg(e.text?.message)}>
            <textarea
              rows={10}
              className="block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive"
              {...form.register("text")}
            />
          </Field>
          <Field label={t("readings.attribution")} error={msg(e.attribution?.message)}>
            <Input autoComplete="off" {...form.register("attribution")} />
          </Field>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={save.isPending}>{t("readings.save_changes")}</Button>
            <Button variant="outline" onClick={() => { form.reset(); setConflict(false); setEditing(false); }}>{t("common.cancel")}</Button>
          </div>
        </form>
      ) : (
        <>
          <p className="max-w-3xl whitespace-pre-line text-lg">{reading.text}</p>
          {reading.attribution && <p className="text-sm text-muted-foreground">{reading.attribution}</p>}
          {reading.source_provider !== "manual" && (
            <p className="text-sm text-muted-foreground">{t("readings.source_provider", { provider: reading.source_provider })}</p>
          )}
          <ErrorAlert error={remove.error} />
          {(reading.actions.edit || reading.actions.delete) && (
            <div className="flex flex-wrap gap-2">
              {reading.actions.edit && <Button variant="outline" onClick={() => setEditing(true)}>{t("library.edit")}</Button>}
              {reading.actions.delete && (
                <ConfirmButton
                  label={t("library.delete")}
                  question={t("readings.delete_question", { reference: reading.canonical, translation: reading.translation.code })}
                  confirmLabel={t("readings.delete_confirm", { reference: reading.canonical })}
                  pending={remove.isPending}
                  onConfirm={() => remove.mutate()}
                />
              )}
            </div>
          )}
        </>
      )}
    </article>
  );
}
