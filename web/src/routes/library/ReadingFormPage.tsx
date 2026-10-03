// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { ApiError, fieldErrors, isCode } from "@/lib/errors";
import { translationsQuery } from "@/lib/queries";
import { lookupQuery, parseQuery } from "@/lib/readings";
import { hasScope } from "@/lib/scopes";
import { useDebounced } from "@/lib/useDebounced";
import { paths, readingPath } from "../paths";

const schema = z.object({
  reference: z.string().trim().min(1, "common.required").max(100, "common.field_invalid"),
  translation: z.string().min(1, "common.required"),
  text: z.string().trim().min(1, "readings.text_required").max(20000, "common.field_invalid"),
  attribution: z.string().trim().max(300, "common.field_invalid"),
});
type Values = z.infer<typeof schema>;

// ReadingFormPage adds a reading: type a reference, see how it was
// understood, then paste the text once, or take it from a provider (07 §5).
export function ReadingFormPage() {
  const me = useOutletContext<Me>();
  if (!hasScope(me, "library.edit")) return <Navigate to={paths.readings} replace />;
  return <ReadingForm defaultTranslation={me.church?.default_translation_code ?? "TB"} />;
}

function ReadingForm({ defaultTranslation }: { defaultTranslation: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const translations = useQuery(translationsQuery);
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { reference: "", translation: defaultTranslation, text: "", attribution: "" },
  });
  const reference = form.watch("reference");
  const translation = form.watch("translation");

  // The preview waits until typing stops for 400 ms, and runs at once when
  // the field loses focus (07 §5).
  const typed = useDebounced(reference.trim(), 400);
  const [checked, setChecked] = useState("");
  useEffect(() => setChecked(typed), [typed]);
  const parsed = useQuery({ ...parseQuery(checked), enabled: checked !== "" });
  const understood = checked !== "" && checked === reference.trim() && parsed.data ? parsed.data : null;

  const lookup = useQuery({ ...lookupQuery(understood?.reference ?? "", translation), enabled: understood !== null && translation !== "" });
  const found = lookup.data;

  // The credit line typed last for this translation is offered again.
  const suggested = found?.suggested_attribution ?? "";
  useEffect(() => {
    if (suggested && !form.getValues("attribution") && !form.formState.dirtyFields.attribution) {
      form.setValue("attribution", suggested);
    }
  }, [suggested, form]);

  const saved = async (id: string) => {
    await queryClient.invalidateQueries({ queryKey: ["readings"] });
    await queryClient.invalidateQueries({ queryKey: ["reading-lookup"] });
    void navigate(readingPath(id));
  };
  const save = useMutation({
    mutationFn: (v: Values) => call(api.POST("/readings", { body: v })),
    onSuccess: (r) => saved(r.id),
    onError: (err) => {
      for (const f of fieldErrors(err)) {
        if (f === "text" || f === "attribution") form.setError(f, { message: "common.field_invalid" });
      }
    },
  });
  const fromProvider = useMutation({
    mutationFn: () =>
      call(api.POST("/readings/from-provider", { body: { reference, translation, provider: found?.provider?.source ?? "" } })),
    onSuccess: (r) => saved(r.id),
  });

  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  const error = save.error ?? fromProvider.error;
  const existingId = found?.reading?.id ?? (isCode(error, "reading_exists") ? (error as ApiError).problem.reading_id : undefined);
  const refField = form.register("reference");

  const preview = (() => {
    if (checked === "" || checked !== reference.trim()) return "";
    if (parsed.error) return null;
    return parsed.data ? t("readings.understood", { reference: parsed.data.canonical }) : "";
  })();

  return (
    <form noValidate className="max-w-3xl space-y-6" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <h1 className="text-2xl font-semibold">{t("readings.new_title")}</h1>
      {isCode(error, "reading_exists") ? null : <ErrorAlert error={error} />}

      <Field
        label={t("readings.reference")}
        hint={<span aria-live="polite">{preview === null ? "" : preview || t("readings.reference_hint")}</span>}
        error={msg(e.reference?.message)}
      >
        <Input
          autoComplete="off"
          {...refField}
          onBlur={(ev) => {
            void refField.onBlur(ev);
            setChecked(ev.target.value.trim());
          }}
        />
      </Field>
      {preview === null && (
        <Alert variant="error" className="!mt-2">{<ErrorAlertText error={parsed.error} />}</Alert>
      )}

      <Field label={t("readings.translation")}>
        <Select className="sm:max-w-sm" {...form.register("translation")}>
          {(translations.data ?? [{ code: defaultTranslation, name: "" }]).map((tr) => (
            <option key={tr.code} value={tr.code}>{tr.name ? `${tr.code}: ${tr.name}` : tr.code}</option>
          ))}
        </Select>
      </Field>

      {existingId ? (
        <Alert className="space-y-1">
          <p>{t("readings.exists")}</p>
          <p><Link className="underline" to={readingPath(existingId)}>{t("readings.open_existing")}</Link></p>
        </Alert>
      ) : (
        <>
          {found?.provider_error && <Alert>{t("readings.provider_error")}</Alert>}
          {found?.provider && (
            <section aria-labelledby="provider-title" className="space-y-3 rounded-md border border-border p-4">
              <h2 id="provider-title" className="text-lg font-semibold">{t("readings.provider_title")}</h2>
              <p className="whitespace-pre-line">{found.provider.text}</p>
              {found.provider.attribution && <p className="text-sm text-muted-foreground">{found.provider.attribution}</p>}
              {found.provider.may_store ? (
                <Button disabled={fromProvider.isPending} onClick={() => fromProvider.mutate()}>{t("readings.save_provider")}</Button>
              ) : (
                <p className="text-sm">{t("readings.provider_no_store")}</p>
              )}
            </section>
          )}

          <Field label={t("readings.text")} hint={t("readings.text_hint")} error={msg(e.text?.message)}>
            <textarea
              rows={8}
              className="block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive"
              {...form.register("text")}
            />
          </Field>
          <Field label={t("readings.attribution")} hint={t("readings.attribution_hint")} error={msg(e.attribution?.message)}>
            <Input autoComplete="off" {...form.register("attribution")} />
          </Field>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={save.isPending}>{t("readings.save")}</Button>
            <Link className="inline-flex min-h-12 items-center rounded-md border border-border px-4 hover:bg-muted" to={paths.readings}>
              {t("common.cancel")}
            </Link>
          </div>
        </>
      )}
    </form>
  );
}

// ErrorAlertText is the translated reason a reference was not understood.
function ErrorAlertText({ error }: { error: unknown }) {
  const { t } = useTranslation();
  const reason = error instanceof ApiError ? error.problem.reason : undefined;
  return <>{t(`readings.reasons.${reason ?? "unknown"}`, { defaultValue: t("readings.reasons.unknown") })}</>;
}
