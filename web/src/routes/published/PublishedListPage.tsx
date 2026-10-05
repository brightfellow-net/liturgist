// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { longDate } from "@/lib/liturgy";
import { publishedListQuery, publishedPageSize } from "@/lib/published";
import { publishedPath } from "../paths";

// PublishedListPage lists the published liturgies, newest first (13 §10).
export function PublishedListPage() {
  const { t, i18n } = useTranslation();
  const [params, setParams] = useSearchParams();
  const archived = params.get("archived") === "1";
  const page = Math.max(0, Number(params.get("page")) || 0);
  const list = useQuery(publishedListQuery(archived, page));
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const go = (next: { archived?: boolean; page?: number }) => {
    const a = next.archived ?? archived;
    const p = next.page ?? page;
    setParams({ ...(a ? { archived: "1" } : {}), ...(p > 0 ? { page: String(p) } : {}) }, { replace: true });
  };
  return (
    <section aria-labelledby="published-title" className="max-w-3xl space-y-4">
      <h1 id="published-title" className="text-2xl font-semibold">{t("published.title")}</h1>
      <label className="flex min-h-12 items-center gap-2">
        <input type="checkbox" className="size-6" checked={archived} onChange={(e) => go({ archived: e.target.checked, page: 0 })} />
        {t("liturgy.list.show_archived")}
      </label>
      <ErrorAlert error={list.error} onRetry={() => void list.refetch()} />
      {list.isPending && <p role="status">{t("app.loading")}</p>}
      {list.data && items.length === 0 && <p>{t("published.empty")}</p>}
      {items.length > 0 && (
        <ul className="divide-y divide-border rounded-md border border-border">
          {items.map((it) => (
            <li key={it.id}>
              <Link to={publishedPath(it.id)} lang={it.language} className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-muted">
                <span className="font-medium">{it.service_name}</span>
                <span>{longDate(it.date, i18n.language)}{it.time && ` · ${it.time}`}</span>
                {it.revising && <span className="rounded bg-muted px-2 py-0.5 text-sm">{t("published.revising_tag")}</span>}
                {it.archived && <span className="rounded bg-muted px-2 py-0.5 text-sm">{t("liturgy.list.archived")}</span>}
              </Link>
            </li>
          ))}
        </ul>
      )}
      {total > publishedPageSize && (
        <nav aria-label={t("published.pages")} className="flex gap-2">
          <button type="button" className={buttonVariants({ variant: "outline" })} disabled={page === 0} onClick={() => go({ page: page - 1 })}>{t("liturgy.list.previous")}</button>
          <button type="button" className={buttonVariants({ variant: "outline" })} disabled={(page + 1) * publishedPageSize >= total} onClick={() => go({ page: page + 1 })}>{t("liturgy.list.next")}</button>
        </nav>
      )}
    </section>
  );
}
