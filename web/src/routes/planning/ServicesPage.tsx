// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useOutletContext } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { servicesQuery } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { paths, servicePath } from "../paths";

// timesText writes the weekly times in words: "Sunday 07:00; Wednesday 19:00" (09 §5).
export function timesText(t: (key: string) => string, times: { weekday: number; time: string }[] | null): string {
  return (times ?? []).map((x) => `${t(`planning.weekdays.${x.weekday}`)} ${x.time}`).join("; ");
}

// ServicesPage lists the regular services with their weekly times (09 §5).
export function ServicesPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const services = useQuery(servicesQuery);
  const canEdit = hasScope(me, "templates.edit");
  const items = services.data?.items ?? [];
  const empty = services.data !== undefined && items.length === 0;
  return (
    <section aria-labelledby="services-title" className="max-w-2xl space-y-4">
      <h2 id="services-title" className="text-xl font-semibold">{t("planning.services.title")}</h2>
      <p className="text-muted-foreground">{t("planning.services.intro")}</p>
      <ErrorAlert error={services.error} onRetry={() => void services.refetch()} />
      {services.isPending && <p role="status">{t("app.loading")}</p>}
      {canEdit && !empty && services.data && (
        <Link className={buttonVariants()} to={paths.serviceNew}>{t("planning.services.add")}</Link>
      )}
      {empty && (
        <div className="space-y-3 rounded-md border border-border p-4">
          <h3 className="text-lg font-semibold">{t("planning.services.empty_title")}</h3>
          {canEdit ? (
            <>
              <p>{t("planning.services.empty_text")}</p>
              <Link className={buttonVariants()} to={paths.serviceNew}>{t("planning.services.add")}</Link>
            </>
          ) : (
            <p>{t("planning.services.empty_viewer")}</p>
          )}
        </div>
      )}
      {items.length > 0 && (
        <ul className="space-y-2">
          {items.map((s) => (
            <li key={s.id} className="rounded-md border border-border p-3">
              <Link className="font-medium underline" to={servicePath(s.id)}>{s.name}</Link>
              <p className="text-sm text-muted-foreground">
                {t(`setup.content_languages.${s.language}`)} · {timesText(t, s.times)}
                {s.default_template_name ? ` · ${t("planning.services.template_in_list", { name: s.default_template_name })}` : ""}
              </p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
