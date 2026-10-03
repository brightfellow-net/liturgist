// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useOutletContext } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { templatesQuery } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { paths, templatePath } from "../paths";

// TemplatesPage lists the templates; each opens its editor (09 §5).
export function TemplatesPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const templates = useQuery(templatesQuery);
  const canEdit = hasScope(me, "templates.edit");
  const items = templates.data?.items ?? [];
  const empty = templates.data !== undefined && items.length === 0;
  return (
    <section aria-labelledby="templates-title" className="max-w-2xl space-y-4">
      <h2 id="templates-title" className="text-xl font-semibold">{t("planning.templates.title")}</h2>
      <p className="text-muted-foreground">{t("planning.templates.intro")}</p>
      <ErrorAlert error={templates.error} onRetry={() => void templates.refetch()} />
      {templates.isPending && <p role="status">{t("app.loading")}</p>}
      {canEdit && !empty && templates.data && (
        <Link className={buttonVariants()} to={paths.templateNew}>{t("planning.templates.add")}</Link>
      )}
      {empty && (
        <div className="space-y-3 rounded-md border border-border p-4">
          <h3 className="text-lg font-semibold">{t("planning.templates.empty_title")}</h3>
          {canEdit ? (
            <>
              <p>{t("planning.templates.empty_text")}</p>
              <Link className={buttonVariants()} to={paths.templateNew}>{t("planning.templates.add")}</Link>
            </>
          ) : (
            <p>{t("planning.templates.empty_viewer")}</p>
          )}
        </div>
      )}
      {items.length > 0 && (
        <ul className="space-y-2">
          {items.map((tpl) => (
            <li key={tpl.id} className="rounded-md border border-border p-3">
              <Link className="font-medium underline" to={templatePath(tpl.id)}>{tpl.name}</Link>
              <p className="text-sm text-muted-foreground">
                {t(`setup.content_languages.${tpl.language}`)} · {t("planning.templates.items", { count: tpl.item_count })}
              </p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
