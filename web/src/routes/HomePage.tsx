// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useOutletContext } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { OfflineNotice } from "@/components/OfflineNotice";
import { longDate } from "@/lib/liturgy";
import { assignmentsQuery } from "@/lib/published";
import { paths, publishedPath } from "./paths";

// HomePage is "My assignments" (13 §10): the viewer's duties in upcoming
// published liturgies, with the welcome text and how to add the app to the
// home screen (SPEC §5.7).
export function HomePage() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const mine = useQuery(assignmentsQuery);
  const cards = mine.data?.items ?? [];
  return (
    <div className="max-w-3xl space-y-6">
      <h1 className="text-2xl font-semibold">{me.user.name ? t("home.title", { name: me.user.name }) : t("nav.assignments")}</h1>
      <section aria-labelledby="assignments-title" className="space-y-3">
        <h2 id="assignments-title" className="text-xl font-semibold">{t("home.assignments_title")}</h2>
        <OfflineNotice path="/me/assignments" />
        <ErrorAlert error={mine.error} onRetry={() => void mine.refetch()} />
        {mine.isPending && <p role="status">{t("app.loading")}</p>}
        {mine.data && cards.length === 0 && <p>{t("home.assignments_empty")}</p>}
        <ul className="space-y-3">
          {cards.map((c) => (
            <li key={c.liturgy.id} className="space-y-2 rounded-md border border-border p-4">
              <h3 className="font-semibold" lang={c.liturgy.language}>
                {longDate(c.liturgy.date, i18n.language)}{c.liturgy.time && ` · ${c.liturgy.time}`} · {c.liturgy.service_name}
              </h3>
              {c.liturgy.revising && <p className="text-sm text-muted-foreground">{t("published.revising_tag")}</p>}
              <p>{(c.duties ?? []).map((d) => d.name).join(", ")}</p>
              {(c.items ?? []).length > 0 && (
                <ul className="list-disc pl-5">
                  {(c.items ?? []).map((it) => <li key={it.id}>{it.title}</li>)}
                </ul>
              )}
              <Link className={buttonVariants({ variant: "outline" })} to={publishedPath(c.liturgy.id)}>{t("home.open")}</Link>
            </li>
          ))}
        </ul>
        {mine.data?.more && <p className="text-sm text-muted-foreground">{t("home.more")}</p>}
        <Link className="underline" to={paths.published}>{t("home.all_published")}</Link>
      </section>
      <details className="rounded-md border border-border p-4">
        <summary className="min-h-12 cursor-pointer font-medium">{t("home.install_title")}</summary>
        <div className="mt-2 space-y-2">
          <p>{t("home.install_intro")}</p>
          <h3 className="font-medium">{t("home.install_android")}</h3>
          <p>{t("home.install_android_steps")}</p>
          <h3 className="font-medium">{t("home.install_iphone")}</h3>
          <p>{t("home.install_iphone_steps")}</p>
        </div>
      </details>
    </div>
  );
}
