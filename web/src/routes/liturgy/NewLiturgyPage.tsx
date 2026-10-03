// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { contentLanguages } from "@/lib/church";
import { ApiError, fieldErrors, isCode } from "@/lib/errors";
import { dateInZone } from "@/lib/liturgy";
import { servicesQuery, templatesQuery } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { liturgyPath, paths } from "../paths";

// NewLiturgyPage creates one liturgy: a regular service (its template and
// language preselected) or a one-off (11 §3).
export function NewLiturgyPage() {
  const me = useOutletContext<Me>();
  if (!hasScope(me, "liturgy.edit")) return <Navigate to={paths.planning} replace />;
  return <NewLiturgy me={me} />;
}

function NewLiturgy({ me }: { me: Me }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const services = useQuery(servicesQuery);
  const templates = useQuery(templatesQuery);
  const [kind, setKind] = useState<"regular" | "oneoff">("regular");
  const [serviceId, setServiceId] = useState("");
  const [name, setName] = useState("");
  const [date, setDate] = useState(dateInZone(me.church?.time_zone));
  const [time, setTime] = useState("");
  // "" follows the service; a choice of the user overrides it.
  const [templateId, setTemplateId] = useState<string | undefined>(undefined);
  const [language, setLanguage] = useState<string | undefined>(undefined);
  const [invalid, setInvalid] = useState<string[]>([]);

  const list = services.data?.items ?? [];
  const service = kind === "regular" ? list.find((s) => s.id === (serviceId || list[0]?.id)) : undefined;
  const effectiveTemplate = templateId ?? service?.default_template_id ?? "";
  const effectiveLanguage = language ?? service?.language ?? me.church?.default_language ?? "id";
  // A liturgy never holds items worded in another language than itself (10 §3):
  // only templates of the chosen language are offered, and none is sent otherwise.
  const usable = (templates.data?.items ?? []).filter((tp) => tp.language === effectiveLanguage);
  const template = usable.some((tp) => tp.id === effectiveTemplate) ? effectiveTemplate : "";
  const effectiveTime = time || (kind === "regular" ? service?.times?.[0]?.time ?? "" : "");

  const create = useMutation({
    mutationFn: () =>
      call(api.POST("/liturgies", {
        body: {
          date, time: effectiveTime,
          ...(kind === "regular" && service ? { service_id: service.id } : { service_name: name.trim() }),
          template_id: template,
          language: effectiveLanguage as (typeof contentLanguages)[number],
        },
      })),
    onSuccess: async (l) => {
      queryClient.setQueryData(["liturgy", l.id], l);
      await queryClient.invalidateQueries({ queryKey: ["liturgies"] });
      void navigate(liturgyPath(l.id));
    },
    onError: (err) => setInvalid(fieldErrors(err)),
  });
  const existing = create.error instanceof ApiError && isCode(create.error, "liturgy_exists") ? create.error.problem.liturgy_id : undefined;
  const missing = (field: string) => (invalid.includes(field) ? t("common.field_invalid") : undefined);
  const ready = date !== "" && effectiveTime !== "" && (kind === "regular" ? !!service : name.trim() !== "");

  if (services.error) return <ErrorAlert error={services.error} onRetry={() => void services.refetch()} />;
  if (templates.error) return <ErrorAlert error={templates.error} onRetry={() => void templates.refetch()} />;
  if (!services.data || !templates.data) return <p role="status">{t("app.loading")}</p>;

  return (
    <form
      noValidate
      className="max-w-2xl space-y-4"
      onSubmit={(e) => { e.preventDefault(); setInvalid([]); create.mutate(); }}
    >
      <p><Link className="underline" to={paths.planning}>{t("liturgy.back")}</Link></p>
      <h2 className="text-xl font-semibold">{t("liturgy.new.title")}</h2>
      {existing ? (
        <Alert variant="error">
          {t("errors.liturgy_exists")} <Link className="underline" to={liturgyPath(existing)}>{t("liturgy.prepare.open")}</Link>
        </Alert>
      ) : <ErrorAlert error={create.error} />}
      <Field label={t("liturgy.new.kind")}>
        <Select value={kind} onChange={(e) => { setKind(e.target.value as "regular" | "oneoff"); setTemplateId(undefined); setLanguage(undefined); }}>
          <option value="regular">{t("liturgy.new.regular")}</option>
          <option value="oneoff">{t("liturgy.new.oneoff")}</option>
        </Select>
      </Field>
      {kind === "regular" ? (
        <Field label={t("liturgy.new.service")} hint={list.length === 0 ? t("liturgy.new.no_services") : undefined}>
          <Select value={service?.id ?? ""} onChange={(e) => { setServiceId(e.target.value); setTemplateId(undefined); setLanguage(undefined); setTime(""); }}>
            {list.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </Select>
        </Field>
      ) : (
        <Field label={t("liturgy.new.name")} error={missing("service_name")}>
          <Input autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("liturgy.date")} error={missing("date")}>
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <Field label={t("liturgy.time")} error={missing("time")}>
          <Input type="time" value={effectiveTime} onChange={(e) => setTime(e.target.value)} />
        </Field>
        <Field label={t("liturgy.new.template")}>
          <Select value={template} onChange={(e) => setTemplateId(e.target.value)}>
            <option value="">{t("planning.none")}</option>
            {usable.map((tp) => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </Select>
        </Field>
        <Field label={t("planning.language")}>
          <Select value={effectiveLanguage} onChange={(e) => setLanguage(e.target.value)}>
            {contentLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={!ready || create.isPending}>{t("liturgy.new.create")}</Button>
        <Button variant="outline" onClick={() => void navigate(paths.planning)}>{t("planning.cancel")}</Button>
      </div>
    </form>
  );
}
