// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useOutletContext, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { TextSizeControl } from "@/components/TextSizeControl";
import { formatDateTime } from "@/lib/format";
import { isCode } from "@/lib/errors";
import { knownPublishedFormat, publishedQuery } from "@/lib/published";
import { paths } from "../paths";
import { PublishedBody } from "./PublishedBody";

// PublishedPage is the view every member reads: the newest published copy of a
// liturgy, shown from the stored copy only (13 §5).
export function PublishedPage() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const { id = "" } = useParams();
  const copy = useQuery(publishedQuery(id));
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
  return (
    <article className="max-w-3xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link className="underline" to={paths.published}>{t("published.back")}</Link>
        <TextSizeControl me={me} />
      </div>
      {c.revising && <Alert>{t("published.revising", { date: when })}</Alert>}
      {c.archived && <Alert>{t("published.archived")}</Alert>}
      <p className="text-sm text-muted-foreground">{t("published.version", { number: c.number, date: when, name: c.published_by.name })}</p>
      {c.content.format > knownPublishedFormat ? (
        <Alert variant="error">{t("published.update_app")}</Alert>
      ) : (
        <PublishedBody content={c.content} render={c.render} />
      )}
    </article>
  );
}
