// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useOutletContext, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { LiturgySummaryView, Me } from "@liturgist/api-client";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { addDays, dateInZone, liturgiesQuery, liturgyLimits, longDate, type LiturgyFilters } from "@/lib/liturgy";
import { servicesQuery } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { liturgyPath, paths } from "../paths";

// LiturgiesPage lists the liturgies: Upcoming (today or later, oldest first)
// and Past (newest first), as tabs kept in the address bar (11 §3).
export function LiturgiesPage() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const [params, setParams] = useSearchParams();
  const past = params.get("tab") === "past";
  const showArchived = params.get("archived") === "1";
  const page = Math.max(0, Number(params.get("page")) || 0);
  const today = dateInZone(me.church?.time_zone);
  const archivedFilter = showArchived ? { archived: "all" as const } : {};
  const filters: LiturgyFilters = past
    ? { to: addDays(today, -1), order: "date_desc" as const, limit: liturgyLimits.pageSize, offset: page * liturgyLimits.pageSize, ...archivedFilter }
    : { from: today, order: "date_asc" as const, limit: liturgyLimits.pageSize, offset: page * liturgyLimits.pageSize, ...archivedFilter };
  const list = useQuery(liturgiesQuery(filters));
  const upcomingProbe = useQuery({ ...liturgiesQuery({ from: today, order: "date_asc", limit: 1 }), enabled: past });
  const services = useQuery(servicesQuery);
  const canEdit = hasScope(me, "liturgy.edit");
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const noLiturgiesAtAll = list.data !== undefined && total === 0 && !past && (upcomingProbe.data?.total ?? 0) === 0;
  const tab = (name: "upcoming" | "past") => {
    const active = (name === "past") === past;
    return (
      <button
        type="button"
        aria-pressed={active}
        onClick={() => setParams({ ...(name === "past" ? { tab: "past" } : {}), ...(showArchived ? { archived: "1" } : {}) }, { replace: true })}
        className={"inline-flex min-h-12 items-center border-b-2 px-3 " + (active ? "border-primary font-semibold" : "border-transparent")}
      >
        {t(`liturgy.list.${name}`)}
      </button>
    );
  };
  return (
    <section aria-labelledby="liturgies-title" className="max-w-3xl space-y-4">
      <h2 id="liturgies-title" className="text-xl font-semibold">{t("liturgy.list.title")}</h2>
      <p className="text-muted-foreground">{t("liturgy.list.intro")}</p>
      {canEdit && (
        <div className="flex flex-wrap gap-2">
          <Link className={buttonVariants()} to={paths.liturgyPrepare}>{t("liturgy.list.prepare")}</Link>
          <Link className={buttonVariants({ variant: "outline" })} to={paths.liturgyNew}>{t("liturgy.list.new")}</Link>
        </div>
      )}
      <div role="group" aria-label={t("liturgy.list.title")} className="flex flex-wrap gap-2 border-b border-border">
        {tab("upcoming")}
        {tab("past")}
      </div>
      <label className="flex min-h-12 items-center gap-2">
        <input type="checkbox" className="size-6" checked={showArchived}
          onChange={(e) => setParams({ ...(past ? { tab: "past" } : {}), ...(e.target.checked ? { archived: "1" } : {}) }, { replace: true })} />
        {t("liturgy.list.show_archived")}
      </label>
      <ErrorAlert error={list.error} onRetry={() => void list.refetch()} />
      {list.isPending && <p role="status">{t("app.loading")}</p>}
      {noLiturgiesAtAll && (
        <div className="space-y-3 rounded-md border border-border p-4">
          <h3 className="text-lg font-semibold">{t("liturgy.list.empty_title")}</h3>
          <p>{t("liturgy.list.empty_text")}</p>
          {canEdit ? (
            <div className="flex flex-wrap gap-2">
              <Link className={buttonVariants()} to={paths.liturgyPrepare}>{t("liturgy.list.prepare")}</Link>
              <Link className={buttonVariants({ variant: "outline" })} to={paths.liturgyNew}>{t("liturgy.list.new")}</Link>
            </div>
          ) : (
            <p>{t("liturgy.list.empty_viewer")}</p>
          )}
          {services.data && (services.data.items ?? []).length === 0 && (
            <p>
              {t("liturgy.list.no_services")} <Link className="underline" to={paths.services}>{t("liturgy.list.to_services")}</Link>
            </p>
          )}
        </div>
      )}
      {list.data && items.length === 0 && !noLiturgiesAtAll && <p>{t(past ? "liturgy.list.none_past" : "liturgy.list.none_upcoming")}</p>}
      {items.length > 0 && (
        <ul className="space-y-2">
          {items.map((l) => <LiturgyRow key={l.id} l={l} lang={i18n.language} />)}
        </ul>
      )}
      {total > liturgyLimits.pageSize && (
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className={buttonVariants({ variant: "outline" })} disabled={page === 0}
            onClick={() => setParams({ ...(past ? { tab: "past" } : {}), ...(showArchived ? { archived: "1" } : {}), page: String(page - 1) }, { replace: true })}>
            {t("liturgy.list.previous")}
          </button>
          <button type="button" className={buttonVariants({ variant: "outline" })} disabled={(page + 1) * liturgyLimits.pageSize >= total}
            onClick={() => setParams({ ...(past ? { tab: "past" } : {}), ...(showArchived ? { archived: "1" } : {}), page: String(page + 1) }, { replace: true })}>
            {t("liturgy.list.next")}
          </button>
        </div>
      )}
    </section>
  );
}

function LiturgyRow({ l, lang }: { l: LiturgySummaryView; lang: string }) {
  const { t } = useTranslation();
  return (
    <li className="rounded-md border border-border p-3">
      <Link className="font-medium underline" to={liturgyPath(l.id)}>
        {longDate(l.date, lang)} {l.time} · {l.service_name}
      </Link>
      <p className="text-sm text-muted-foreground">
        {t(`setup.content_languages.${l.language}`)} · {t(`liturgy.states.${l.state}`)}{l.archived ? ` · ${t("liturgy.list.archived")}` : ""} · {t("liturgy.list.items", { count: l.item_count })}
      </p>
    </li>
  );
}
