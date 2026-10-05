// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Alert } from "@/components/ui/alert";
import { Button, buttonVariants } from "@/components/ui/button";
import { Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { isCode } from "@/lib/errors";
import { printFlags, printParams, parsePrint, printVariants, type PrintOptions } from "@/lib/print";
import { knownPublishedFormat, publishedQuery } from "@/lib/published";
import { publishedPath, paths } from "../paths";
import { PrintBody } from "./PrintBody";

// PrintPage prints the stored copy with the print stylesheet (13 §6). Choosing
// an option changes this page only, never the church's defaults.
export function PrintPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
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
  if (c.content.format > knownPublishedFormat) return <Alert variant="error">{t("published.update_app")}</Alert>;
  const opts = parsePrint(params, c.render.print);
  const change = (next: Partial<PrintOptions>) => setParams(printParams({ ...opts, ...next }), { replace: true });
  const musician = opts.variant === "musician";
  return (
    <div className="space-y-4">
      <div className="space-y-3 print:hidden">
        <Link className="underline" to={publishedPath(id)}>{t("print.back")}</Link>
        <p className="text-lg font-semibold">{t("print.title")}</p>
        <div className="flex flex-wrap items-end gap-4">
          <label className="space-y-1">
            <span className="block font-medium">{t("print.variant")}</span>
            <Select value={opts.variant} onChange={(e) => change({ variant: e.target.value as PrintOptions["variant"] })}>
              {printVariants.map((v) => <option key={v} value={v}>{t(`print.variant_${v}`)}</option>)}
            </Select>
          </label>
          <label className="space-y-1">
            <span className="block font-medium">{t("print.paper")}</span>
            <Select value={opts.paper} onChange={(e) => change({ paper: e.target.value as PrintOptions["paper"] })}>
              {(["a4", "f4"] as const).map((v) => <option key={v} value={v}>{t(`print.paper_${v}`)}</option>)}
            </Select>
          </label>
          {!musician && (
            <label className="space-y-1">
              <span className="block font-medium">{t("print.lyrics")}</span>
              <Select value={opts.lyrics} onChange={(e) => change({ lyrics: e.target.value as PrintOptions["lyrics"] })}>
                {(["full", "first_lines"] as const).map((v) => <option key={v} value={v}>{t(`print.lyrics_${v}`)}</option>)}
              </Select>
            </label>
          )}
          <label className="space-y-1">
            <span className="block font-medium">{t("print.size")}</span>
            <Select value={opts.size} onChange={(e) => change({ size: e.target.value as PrintOptions["size"] })}>
              {(["normal", "large"] as const).map((v) => <option key={v} value={v}>{t(`print.size_${v}`)}</option>)}
            </Select>
          </label>
        </div>
        <fieldset className="flex flex-wrap gap-x-6">
          <legend className="sr-only">{t("print.include")}</legend>
          {printFlags.filter((f) => !musician || f === "keys" || f === "notes").map((f) => (
            <label key={f} className="flex min-h-12 items-center gap-3">
              <input type="checkbox" className="size-5" checked={opts[f]} onChange={(e) => change({ [f]: e.target.checked })} />
              {t(`print.${f}`)}
            </label>
          ))}
        </fieldset>
        <Button onClick={() => window.print()}>{t("print.print")}</Button>
      </div>
      <PrintBody content={c.content} render={c.render} opts={opts} />
    </div>
  );
}
