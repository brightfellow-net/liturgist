// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useLocation, useNavigate, useOutletContext, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { ApplyResultView, ImportCandidateView, ImportRejectedView, Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { isCode } from "@/lib/errors";
import { chosen, failureText, importQuery, isDone } from "@/lib/imports";
import { hasScope } from "@/lib/scopes";
import { paths, songPath } from "../paths";
import { CandidatePanel, DuplicateNotice, Warnings } from "./ImportCandidate";
import { Rejected } from "./ImportPage";

// withoutDuplicate: the candidates "Add all without duplicates" accepts. A
// candidate that matches a song in the library or another candidate of the
// batch is left for the member to decide.
export function withoutDuplicate(list: ImportCandidateView[]): ImportCandidateView[] {
  return list.filter((c) => !isDone(c) && c.decision === "pending" && !c.duplicate_of && !(c.warnings ?? []).includes("duplicate_in_batch"));
}

// ImportReviewPage is step 2 of importing songs: the member looks at what was
// found, chooses what to do with each song, then imports the chosen ones (08 §6).
export function ImportReviewPage() {
  const { id = "" } = useParams();
  const me = useOutletContext<Me>();
  if (!hasScope(me, "library.edit")) return <Navigate to={paths.library} replace />;
  return <Review id={id} />;
}

function Review({ id }: { id: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const location = useLocation();
  const rejected = ((location.state as { rejected?: ImportRejectedView[] } | null)?.rejected ?? []);
  const batch = useQuery(importQuery(id));
  const [selected, setSelected] = useState("");
  const [result, setResult] = useState<ApplyResultView | null>(null);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["import", id] });

  const decideAll = useMutation({
    mutationFn: (ids: string[]) =>
      call(api.PATCH("/imports/{id}/candidates", { params: { path: { id } }, body: { decisions: ids.map((cid) => ({ id: cid, decision: "accept" as const })) } })),
    onSuccess: refresh,
  });
  const apply = useMutation({
    mutationFn: () => call(api.POST("/imports/{id}/apply", { params: { path: { id } } })),
    onSuccess: async (r) => {
      setResult(r);
      await refresh();
      await queryClient.invalidateQueries({ queryKey: ["imports"] });
      await queryClient.invalidateQueries({ queryKey: ["songs"] });
    },
  });
  const discard = useMutation({
    mutationFn: () => call(api.DELETE("/imports/{id}", { params: { path: { id } } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["imports"] });
      void navigate(paths.library);
    },
  });

  if (batch.error) {
    return isCode(batch.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("import.not_found")}</Alert>
        <Link className="underline" to={paths.import}>{t("import.start_again")}</Link>
      </div>
    ) : <ErrorAlert error={batch.error} onRetry={() => void batch.refetch()} />;
  }
  if (!batch.data) return <p role="status">{t("app.loading")}</p>;

  const list = batch.data.candidates ?? [];
  const open = batch.data.status === "open";
  const current = list.find((c) => c.id === selected) ?? list[0];
  const toApply = list.filter((c) => !isDone(c) && chosen(c));
  const failed = list.filter((c) => c.outcome === "failed" && chosen(c));
  const quick = withoutDuplicate(list);
  const applied = list.filter(isDone);

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">{t("import.review_title")}</h1>
        <p>{t("import.review_intro")}</p>
        <Link className="underline" to={paths.library}>{t("library.back")}</Link>
      </div>
      {rejected.length > 0 && <Rejected items={rejected} />}
      <ErrorAlert error={decideAll.error ?? apply.error ?? discard.error} />

      {(result || applied.length > 0) && <Summary result={result} applied={applied} failed={failed} onRetry={() => apply.mutate()} busy={apply.isPending}
        onCheck={(cid) => setSelected(cid)} />}

      <div className="grid gap-6 lg:grid-cols-[20rem_1fr]">
        <section aria-labelledby="found-title" className="space-y-3">
          <h2 id="found-title" className="text-xl font-semibold">{t("import.found", { count: list.length })}</h2>
          <ul className="divide-y divide-border rounded-md border border-border">
            {list.map((c) => (
              <li key={c.id} className="space-y-1 p-3">
                <button
                  type="button"
                  aria-current={c.id === current?.id ? "true" : undefined}
                  className="block min-h-12 w-full text-left font-semibold underline"
                  onClick={() => setSelected(c.id)}
                >
                  {c.draft.title}
                </button>
                <p className="text-sm text-muted-foreground">
                  {t("import.sections", { count: (c.draft.sections ?? []).length })} · {t(`import.state.${stateOf(c)}`)}
                </p>
                <DuplicateNotice c={c} />
                <Warnings c={c} />
              </li>
            ))}
          </ul>
        </section>
        {current && <CandidatePanel key={current.id + ":" + current.merge_target_version + current.error_code} batchId={id} c={current} canChange={open || !isDone(current)} />}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={quick.length === 0 || decideAll.isPending} onClick={() => decideAll.mutate(quick.map((c) => c.id))}>
          {t("import.add_all")}
        </Button>
        <Button disabled={toApply.length === 0 || apply.isPending} onClick={() => apply.mutate()}>{t("import.apply")}</Button>
        <span role="status" className="self-center">{t("import.chosen", { count: toApply.length })}</span>
      </div>
      <ConfirmButton
        label={t("import.discard")} question={t("import.discard_question")} confirmLabel={t("import.discard_confirm")}
        pending={discard.isPending} onConfirm={() => discard.mutate()}
      />
    </div>
  );
}

function stateOf(c: ImportCandidateView): string {
  if (c.outcome === "applied") return "applied";
  if (c.outcome === "failed") return "failed";
  return c.decision;
}

// Summary says what an import did: the songs now in the library with links,
// and the ones that failed with their reason (08 §6).
function Summary({ result, applied, failed, onRetry, busy, onCheck }: {
  result: ApplyResultView | null;
  applied: ImportCandidateView[];
  failed: ImportCandidateView[];
  onRetry: () => void;
  busy: boolean;
  onCheck: (candidateId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="summary-title" className="space-y-3 rounded-md border border-border p-4">
      <h2 id="summary-title" className="text-xl font-semibold">{t("import.summary_title")}</h2>
      {result && <p role="status">{t("import.summary", { created: result.created, merged: result.merged, skipped: result.skipped })}</p>}
      {applied.length > 0 && (
        <ul className="list-disc pl-6">
          {applied.map((c) => (
            <li key={c.id}>
              {c.applied_song_id ? <Link className="underline" to={songPath(c.applied_song_id)}>{c.draft.title}</Link> : c.draft.title}
              {" "}({t(c.decision === "merge" ? "import.state.merged" : "import.state.added")})
            </li>
          ))}
        </ul>
      )}
      {failed.length > 0 && (
        <Alert variant="error" className="space-y-2">
          <p>{t("import.failed_title", { count: failed.length })}</p>
          <ul className="list-disc space-y-2 pl-6">
            {failed.map((c) => (
              <li key={c.id}>
                {c.draft.title}: {failureText(t, c.error_code ?? "")}{" "}
                {c.error_code === "target_changed" && (
                  <Button variant="outline" aria-label={`${t("import.check_preview")} ${c.draft.title}`} onClick={() => onCheck(c.id)}>{t("import.check_preview")}</Button>
                )}
              </li>
            ))}
          </ul>
          <Button variant="outline" disabled={busy} onClick={onRetry}>{t("common.retry")}</Button>
        </Alert>
      )}
    </section>
  );
}
