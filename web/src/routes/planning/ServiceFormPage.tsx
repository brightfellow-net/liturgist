// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { Me, ServiceView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { contentLanguages } from "@/lib/church";
import { fieldErrors, isCode } from "@/lib/errors";
import { planningLimits, serviceQuery, templatesQuery, weekdays } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { paths } from "../paths";
import { timesText } from "./ServicesPage";

const schema = z.object({
  name: z.string().trim().min(1, "planning.services.name_required").max(100, "common.field_invalid"),
  language: z.string().min(1),
  default_template_id: z.string(),
  times: z
    .array(z.object({ weekday: z.string(), time: z.string().regex(/^([01][0-9]|2[0-3]):[0-5][0-9]$/, "planning.services.time_invalid") }))
    .min(1, "planning.services.no_times")
    .max(planningLimits.serviceTimes),
});
type Values = z.infer<typeof schema>;

function toValues(s: ServiceView | undefined, language: string): Values {
  return {
    name: s?.name ?? "",
    language: s?.language ?? language,
    default_template_id: s?.default_template_id ?? "",
    times: (s?.times ?? [{ weekday: 7, time: "" }]).map((x) => ({ weekday: String(x.weekday), time: x.time })),
  };
}

// ServiceFormPage adds or edits a regular service: its name, language,
// default template and weekly times. Members without templates.edit see it
// read-only (09 §5).
export function ServiceFormPage() {
  const { id } = useParams();
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const service = useQuery({ ...serviceQuery(id ?? ""), enabled: id !== undefined });
  const [generation, setGeneration] = useState(0);
  if (id === undefined) {
    if (!hasScope(me, "templates.edit")) return <Navigate to={paths.services} replace />;
    return <ServiceForm key="new" language={me.church?.default_language ?? "id"} />;
  }
  if (service.error) {
    return isCode(service.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("planning.not_found")}</Alert>
        <Link className="underline" to={paths.services}>{t("planning.services.back")}</Link>
      </div>
    ) : <ErrorAlert error={service.error} onRetry={() => void service.refetch()} />;
  }
  if (!service.data) return <p role="status">{t("app.loading")}</p>;
  const reload = async () => {
    const result = await service.refetch();
    if (result.data) setGeneration((g) => g + 1);
  };
  return <ServiceForm key={generation + ":" + service.data.version} service={service.data} language="id" onReload={reload} />;
}

function ServiceForm({ service, language, onReload }: { service?: ServiceView; language: string; onReload?: () => Promise<void> }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const templates = useQuery(templatesQuery);
  const [conflict, setConflict] = useState(false);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: toValues(service, language) });
  const times = useFieldArray({ control: form.control, name: "times" });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);

  const done = async (saved?: ServiceView) => {
    if (saved) queryClient.setQueryData(serviceQuery(saved.id).queryKey, saved);
    else queryClient.removeQueries({ queryKey: ["service", service?.id] });
    await queryClient.invalidateQueries({ queryKey: ["services"] });
    void navigate(paths.services);
  };
  const body = (v: Values) => ({
    name: v.name, language: v.language as (typeof contentLanguages)[number], default_template_id: v.default_template_id,
    times: v.times.map((x) => ({ weekday: Number(x.weekday), time: x.time })),
  });
  const save = useMutation({
    mutationFn: (v: Values) =>
      service
        ? call(api.PATCH("/services/{id}", { params: { path: { id: service.id } }, body: { ...body(v), version: service.version } }))
        : call(api.POST("/services", { body: body(v) })),
    onSuccess: (saved) => done(saved),
    onError: (err) => {
      setConflict(isCode(err, "version_conflict"));
      for (const f of fieldErrors(err)) {
        if (f === "name") form.setError("name", { message: "common.field_invalid" });
      }
    },
  });
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/services/{id}", { params: { path: { id: service?.id ?? "" } } })),
    onSuccess: () => done(),
  });

  if (service && !service.actions.edit) {
    return (
      <article className="max-w-3xl space-y-4">
        <p><Link className="underline" to={paths.services}>{t("planning.services.back")}</Link></p>
        <h2 className="text-xl font-semibold">{service.name}</h2>
        <p className="text-muted-foreground">{t(`setup.content_languages.${service.language}`)} · {timesText(t, service.times)}</p>
        {service.default_template_name && <p>{t("planning.services.template")}: {service.default_template_name}</p>}
        <p className="text-sm text-muted-foreground">{t("planning.read_only")}</p>
      </article>
    );
  }

  // The template list must be there before the form, or its select would save the default template away.
  if (templates.error) return <ErrorAlert error={templates.error} onRetry={() => void templates.refetch()} />;
  if (!templates.data) return <p role="status">{t("app.loading")}</p>;

  return (
    <form noValidate className="max-w-3xl space-y-8" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <p><Link className="underline" to={paths.services}>{t("planning.services.back")}</Link></p>
      <h2 className="text-xl font-semibold">{service ? service.name : t("planning.services.new_title")}</h2>
      {conflict ? (
        <Alert variant="error" className="space-y-2">
          <p>{t("planning.conflict")}</p>
          <Button variant="outline" onClick={() => void onReload?.()}>{t("planning.reload")}</Button>
        </Alert>
      ) : <ErrorAlert error={save.error} />}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("planning.name")} hint={t("planning.services.name_hint")} error={msg(e.name?.message)}>
          <Input autoComplete="off" {...form.register("name")} />
        </Field>
        <Field label={t("planning.language")}>
          <Select {...form.register("language")}>
            {contentLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("planning.services.template")} hint={t("planning.services.template_hint")}>
          <Select {...form.register("default_template_id")}>
            <option value="">{t("planning.none")}</option>
            {(templates.data?.items ?? []).map((tp) => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </Select>
        </Field>
      </div>

      <section aria-labelledby="times-title" className="space-y-4">
        <div>
          <h3 id="times-title" className="text-lg font-semibold">{t("planning.services.times_title")}</h3>
          <p className="text-muted-foreground">{t("planning.services.times_intro")}</p>
        </div>
        {typeof e.times?.message === "string" && <p className="text-sm text-destructive">{t(e.times.message)}</p>}
        {typeof e.times?.root?.message === "string" && <p className="text-sm text-destructive">{t(e.times.root.message)}</p>}
        <ol className="space-y-3">
          {times.fields.map((field, i) => {
            const num = t("planning.services.time_n", { n: i + 1 });
            return (
              <li key={field.id}>
                <fieldset className="flex flex-wrap items-end gap-3 rounded-md border border-border p-3">
                  <legend className="px-1 text-sm font-semibold">{num}</legend>
                  <Field label={t("planning.services.weekday")}>
                    <Select {...form.register(`times.${i}.weekday`)}>
                      {weekdays.map((d) => <option key={d} value={String(d)}>{t(`planning.weekdays.${d}`)}</option>)}
                    </Select>
                  </Field>
                  <Field label={t("planning.services.time")} error={msg(e.times?.[i]?.time?.message)}>
                    <Input type="time" {...form.register(`times.${i}.time`)} />
                  </Field>
                  <Button aria-label={`${t("planning.remove")} ${num}`} variant="outline" onClick={() => times.remove(i)}>
                    {t("planning.remove")}
                  </Button>
                </fieldset>
              </li>
            );
          })}
        </ol>
        <Button variant="outline" disabled={times.fields.length >= planningLimits.serviceTimes} onClick={() => times.append({ weekday: "7", time: "" })}>
          {t("planning.services.add_time")}
        </Button>
      </section>

      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>{service ? t("planning.save_changes") : t("planning.services.save")}</Button>
        <Button variant="outline" onClick={() => void navigate(paths.services)}>{t("planning.cancel")}</Button>
      </div>
      {service?.actions.delete && (
        <div className="space-y-2 border-t border-border pt-4">
          <ErrorAlert error={remove.error} />
          <ConfirmButton
            label={t("planning.delete")}
            question={t("planning.services.delete_question", { name: service.name })}
            confirmLabel={t("planning.services.delete_confirm", { name: service.name })}
            pending={remove.isPending}
            onConfirm={() => remove.mutate()}
          />
        </div>
      )}
    </form>
  );
}
