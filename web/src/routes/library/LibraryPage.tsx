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
import { hasScope } from "@/lib/scopes";
import { hymnalText, licenceStatuses, limits, songLanguages, songsQuery, type SongFilters } from "@/lib/library";
import { paths, songPath } from "../paths";

// LibraryPage is the song list: a search box, filters, and the results a page
// at a time. The search terms live in the address bar, so a result list can
// be bookmarked and the back button works (06 §4).
export function LibraryPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const [params, setParams] = useSearchParams();
  const filters: SongFilters = {
    q: params.get("q") ?? "",
    language: params.get("language") ?? "",
    licence_status: params.get("licence_status") ?? "",
    hymnal_source: params.get("hymnal_source") ?? "",
    offset: Math.max(0, Number(params.get("offset")) || 0),
    limit: limits.pageSize,
  };
  const songs = useQuery({ ...songsQuery(filters), placeholderData: (previous) => previous });
  const canEdit = hasScope(me, "library.edit");
  const filtered = Boolean(filters.q || filters.language || filters.licence_status || filters.hymnal_source);

  // update changes some search terms and goes back to the first page.
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

  const total = songs.data?.total ?? 0;
  const items = songs.data?.items ?? [];
  const from = (filters.offset ?? 0) + 1;
  const to = (filters.offset ?? 0) + items.length;
  const emptyLibrary = songs.data !== undefined && total === 0 && !filtered;

  return (
    <div className="space-y-6">
      {canEdit && !emptyLibrary && (
        <div><Link className={buttonVariants()} to={paths.songNew}>{t("library.add_song")}</Link></div>
      )}

      {emptyLibrary ? (
        <section className="space-y-3 rounded-md border border-border p-4">
          <h2 className="text-xl font-semibold">{t("library.empty_title")}</h2>
          {canEdit ? (
            <>
              <p>{t("library.empty_text")}</p>
              <Link className={buttonVariants()} to={paths.songNew}>{t("library.add_song")}</Link>
            </>
          ) : (
            <p>{t("library.empty_viewer")}</p>
          )}
        </section>
      ) : (
        <>
          <form role="search" className="space-y-4" onSubmit={search} key={filters.q}>
            <Field label={t("library.search_label")} hint={t("library.search_hint")}>
              <Input type="search" name="q" defaultValue={filters.q} autoComplete="off" />
            </Field>
            <div className="grid gap-4 sm:grid-cols-3">
              <Field label={t("library.filter_language")}>
                <Select value={filters.language} onChange={(e) => update({ language: e.target.value })}>
                  <option value="">{t("library.any")}</option>
                  {songLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
                </Select>
              </Field>
              <Field label={t("library.filter_licence")}>
                <Select value={filters.licence_status} onChange={(e) => update({ licence_status: e.target.value })}>
                  <option value="">{t("library.any")}</option>
                  {licenceStatuses.map((s) => <option key={s} value={s}>{t(`library.licence.${s}`)}</option>)}
                </Select>
              </Field>
              <Field label={t("library.filter_source")} hint={t("library.filter_source_hint")}>
                <Input
                  key={filters.hymnal_source}
                  defaultValue={filters.hymnal_source}
                  autoComplete="off"
                  onBlur={(e) => e.target.value.trim() !== filters.hymnal_source && update({ hymnal_source: e.target.value.trim() })}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      update({ hymnal_source: e.currentTarget.value.trim() });
                    }
                  }}
                />
              </Field>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="submit">{t("library.search")}</Button>
              {filtered && <Button variant="outline" onClick={() => setParams(new URLSearchParams())}>{t("library.clear_filters")}</Button>}
            </div>
          </form>

          <ErrorAlert error={songs.error} onRetry={() => void songs.refetch()} />
          {songs.isPending && <p role="status">{t("app.loading")}</p>}
          {songs.data && (
            <div className="space-y-3">
              <p role="status">{total === 0 ? "" : t("library.found", { count: total })}</p>
              {total === 0 ? <Alert>{t("library.no_results")}</Alert> : (
                <ul className="divide-y divide-border rounded-md border border-border">
                  {items.map((s) => (
                    <li key={s.id}>
                      <Link to={songPath(s.id)} className="block min-h-12 space-y-1 p-3 hover:bg-muted">
                        <span className="block font-semibold underline">{s.title}</span>
                        <span className="block text-sm text-muted-foreground">
                          {[hymnalText(s), t(`setup.content_languages.${s.language}`), t(`library.licence.${s.licence_status}`)]
                            .filter(Boolean).join(" · ")}
                          {s.has_group && " · " + t("library.has_group")}
                        </span>
                        {(s.alt_titles ?? []).length > 0 && (
                          <span className="block text-sm text-muted-foreground">{(s.alt_titles ?? []).join(" · ")}</span>
                        )}
                      </Link>
                    </li>
                  ))}
                </ul>
              )}
              {total > limits.pageSize && (
                <nav aria-label={t("library.pages")} className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" disabled={(filters.offset ?? 0) === 0} onClick={() => goTo(Math.max(0, (filters.offset ?? 0) - limits.pageSize))}>
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
