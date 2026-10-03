// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { type FormEvent } from "react";
import { Link, useOutletContext, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { readingPageSize, readingsQuery, type ReadingFilters } from "@/lib/readings";
import { translationsQuery } from "@/lib/queries";
import { hasScope } from "@/lib/scopes";
import { paths, readingPath } from "../paths";

// ReadingsPage is the second tab of the library: the saved readings, with a
// search box and a translation filter (07 §5). Search terms live in the
// address bar, as on the songs tab.
export function ReadingsPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const [params, setParams] = useSearchParams();
  const filters: ReadingFilters = {
    q: params.get("q") ?? "",
    translation: params.get("translation") ?? "",
    offset: Math.max(0, Number(params.get("offset")) || 0),
    limit: readingPageSize,
  };
  const readings = useQuery({ ...readingsQuery(filters), placeholderData: (previous) => previous });
  const translations = useQuery(translationsQuery);
  const canEdit = hasScope(me, "library.edit");
  const filtered = Boolean(filters.q || filters.translation);

  const update = (changes: Record<string, string>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries({ ...changes, offset: "" })) {
      if (v) next.set(k, v);
      else next.delete(k);
    }
    setParams(next);
  };
  const goTo = (offset: number) => {
    const next = new URLSearchParams(params);
    if (offset > 0) next.set("offset", String(offset));
    else next.delete("offset");
    setParams(next);
  };
  const search = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    update({ q: String(new FormData(e.currentTarget).get("q") ?? "").trim() });
  };

  const total = readings.data?.total ?? 0;
  const items = readings.data?.items ?? [];
  const from = (filters.offset ?? 0) + 1;
  const to = (filters.offset ?? 0) + items.length;
  const empty = readings.data !== undefined && total === 0 && !filtered;

  return (
    <div className="space-y-6">
      {canEdit && !empty && <div><Link className={buttonVariants()} to={paths.readingNew}>{t("readings.add")}</Link></div>}

      {empty ? (
        <section className="space-y-3 rounded-md border border-border p-4">
          <h2 className="text-xl font-semibold">{t("readings.empty_title")}</h2>
          {canEdit ? (
            <>
              <p>{t("readings.empty_text")}</p>
              <Link className={buttonVariants()} to={paths.readingNew}>{t("readings.add")}</Link>
            </>
          ) : (
            <p>{t("readings.empty_viewer")}</p>
          )}
        </section>
      ) : (
        <>
          <form role="search" className="space-y-4" onSubmit={search} key={filters.q}>
            <Field label={t("readings.search_label")} hint={t("readings.search_hint")}>
              <Input type="search" name="q" defaultValue={filters.q} autoComplete="off" />
            </Field>
            <Field label={t("readings.translation")}>
              <Select className="sm:max-w-sm" value={filters.translation} onChange={(e) => update({ translation: e.target.value })}>
                <option value="">{t("library.any")}</option>
                {(translations.data ?? []).map((tr) => <option key={tr.code} value={tr.code}>{tr.code}: {tr.name}</option>)}
              </Select>
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button type="submit">{t("library.search")}</Button>
              {filtered && <Button variant="outline" onClick={() => setParams(new URLSearchParams())}>{t("library.clear_filters")}</Button>}
            </div>
          </form>

          <ErrorAlert error={readings.error} onRetry={() => void readings.refetch()} />
          {readings.isPending && <p role="status">{t("app.loading")}</p>}
          {readings.data && (
            <div className="space-y-3">
              <p role="status">{total === 0 ? "" : t("readings.found", { count: total })}</p>
              {total === 0 ? <Alert>{t("readings.no_results")}</Alert> : (
                <ul className="divide-y divide-border rounded-md border border-border">
                  {items.map((r) => (
                    <li key={r.id}>
                      <Link to={readingPath(r.id)} className="block min-h-12 space-y-1 p-3 hover:bg-muted">
                        <span className="block font-semibold underline">{r.canonical} ({r.translation.code})</span>
                        <span className="block text-sm text-muted-foreground">{r.snippet}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              )}
              {total > readingPageSize && (
                <nav aria-label={t("library.pages")} className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" disabled={(filters.offset ?? 0) === 0} onClick={() => goTo(Math.max(0, (filters.offset ?? 0) - readingPageSize))}>
                    {t("library.previous")}
                  </Button>
                  <Button variant="outline" disabled={to >= total} onClick={() => goTo(to)}>{t("library.next")}</Button>
                  <span>{t("library.range", { from, to, total })}</span>
                </nav>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
