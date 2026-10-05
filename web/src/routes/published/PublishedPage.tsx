// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useMemo } from "react";
import { Link, useOutletContext, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button, buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { KeepScreenOn } from "@/components/KeepScreenOn";
import { OfflineNotice } from "@/components/OfflineNotice";
import { TextSizeControl } from "@/components/TextSizeControl";
import { formatDateTime } from "@/lib/format";
import { isCode } from "@/lib/errors";
import { knownPublishedFormat, publishedQuery } from "@/lib/published";
import { paths, publishedPath, publishedPrintPath, publishedReadPath } from "../paths";
import { PublishedBody } from "./PublishedBody";

// PublishedPage is the view every member reads: the newest published copy of a
// liturgy, shown from the stored copy only (13 §5).
export function PublishedPage() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const { id = "" } = useParams();
  const copy = useQuery(publishedQuery(id));
  const reading = useSearchParams()[0].get("read") === "1";
  // The duties of the viewer in this copy: their items are marked (13 §7).
  const mine = useMemo(
    () => new Set((copy.data?.content.assignments ?? []).filter((a) => a.user_id === me.user.id).map((a) => a.duty.id)),
    [copy.data, me.user.id],
  );
  const goToMyPart = () => {
    const first = (copy.data?.content.items ?? []).find((i) => i.duty && mine.has(i.duty.id));
    const heading = first && document.getElementById(`item-${first.id}`);
    if (!heading) return;
    heading.scrollIntoView?.({ block: "start" });
    heading.focus();
  };
  if (copy.isPending) return <p role="status">{t("app.loading")}</p>;
  if (isCode(copy.error, "not_found")) {
    return (
      <div className="max-w-xl space-y-4">
        <Alert>{t("published.not_published")}</Alert>
        <Link className={buttonVariants({ variant: "outline" })} to={paths.published}>{t("published.back")}</Link>
      </div>
    );
  }
  if (copy.error || !copy.data) return <ErrorAlert error={copy.error} onRetry={() => void copy.refetch()} />;
  const c = copy.data;
  const when = formatDateTime(c.published_at, me.church?.time_zone ?? "UTC", i18n.language);
  const known = c.content.format <= knownPublishedFormat;
  return (
    <article className={"space-y-4 " + (reading ? "reading-mode max-w-none text-xl leading-relaxed" : "max-w-3xl")}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        {reading ? (
          <Link className="underline" to={publishedPath(id)}>{t("reading.exit")}</Link>
        ) : (
          <Link className="underline" to={paths.published}>{t("published.back")}</Link>
        )}
        <div className="flex flex-wrap items-center gap-2">
          {known && !reading && (
            <>
              <Link className={buttonVariants({ variant: "outline" })} to={publishedReadPath(id)}>{t("reading.enter")}</Link>
              <Link className={buttonVariants({ variant: "outline" })} to={publishedPrintPath(id)}>{t("published.print")}</Link>
            </>
          )}
          {known && reading && (
            <>
              <KeepScreenOn />
              {mine.size > 0 && <Button variant="outline" onClick={goToMyPart}>{t("reading.go_to_my_part")}</Button>}
            </>
          )}
          <TextSizeControl me={me} />
        </div>
      </div>
      <OfflineNotice path={`/liturgies/${id}/published`} savedAt={when} />
      {c.revising && <Alert>{t("published.revising", { date: when })}</Alert>}
      {c.archived && <Alert>{t("published.archived")}</Alert>}
      <p className="text-sm text-muted-foreground">{t("published.version", { number: c.number, date: when, name: c.published_by.name })}</p>
      {c.content.format > knownPublishedFormat ? (
        <Alert variant="error">{t("published.update_app")}</Alert>
      ) : (
        <PublishedBody content={c.content} render={c.render} mine={reading ? mine : undefined} />
      )}
    </article>
  );
}
