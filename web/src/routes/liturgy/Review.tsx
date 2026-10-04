// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { LiturgyView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { fieldClass } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { ApiError, isCode } from "@/lib/errors";
import { liturgyQuery, problemText, stateChangesQuery } from "@/lib/liturgy";

type Action = "submit" | "approve" | "request_changes" | "reopen";

// Review is the review bar of a liturgy (12 §6): the state in words, the buttons
// the liturgy's actions allow, the note form, and the review history. Submit
// goes straight away; approving, requesting changes and reopening ask for an
// optional note first, and approving asks once more when comments are open.
export function Review({ liturgy, onChanged }: { liturgy: LiturgyView; onChanged?: (message: string) => void }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const key = liturgyQuery(liturgy.id).queryKey;
  const [asking, setAsking] = useState<Action | null>(null);
  const [note, setNote] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const reviewable = liturgy.open_comments !== undefined;
  const history = useQuery({ ...stateChangesQuery(liturgy.id), enabled: reviewable });
  const a = liturgy.actions;
  const open = liturgy.open_comments ?? 0;
  const items = liturgy.items ?? [];
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });

  const run = useMutation({
    mutationFn: ({ action, text }: { action: Action; text: string }) => {
      const params = { path: { id: liturgy.id } };
      const body = { ...(text ? { note: text } : {}) };
      switch (action) {
        case "submit": return call(api.POST("/liturgies/{id}/submit", { params, body }));
        case "approve": return call(api.POST("/liturgies/{id}/approve", { params, body: { ...body, edit_seq: liturgy.edit_seq } }));
        case "request_changes": return call(api.POST("/liturgies/{id}/request-changes", { params, body: { ...body, edit_seq: liturgy.edit_seq } }));
        case "reopen": return call(api.POST("/liturgies/{id}/reopen", { params, body }));
      }
    },
    onSuccess: async (l, { action }) => {
      queryClient.setQueryData<LiturgyView>(key, l);
      setAsking(null);
      setNote("");
      setConfirmed(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: stateChangesQuery(liturgy.id).queryKey }),
        queryClient.invalidateQueries({ queryKey: [...key, "edits"] }),
      ]);
      onChanged?.(t(`liturgy.review.done.${action}`));
    },
    // The page the person read is out of date: show it as it is now.
    onError: async (err) => {
      if (isCode(err, "review_stale") || isCode(err, "invalid_transition") || isCode(err, "liturgy_locked")) {
        setAsking(null);
        await queryClient.invalidateQueries({ queryKey: key, exact: true });
      }
    },
  });

  const start = (action: Action) => {
    run.reset();
    setConfirmed(false);
    setNote("");
    if (action === "submit") run.mutate({ action, text: "" });
    else setAsking(action);
  };
  const send = () => {
    if (asking === "approve" && open > 0 && !confirmed) {
      setConfirmed(true);
      return;
    }
    if (asking) run.mutate({ action: asking, text: note.trim() });
  };

  const last = liturgy.last_change;
  const banner =
    liturgy.state === "in_review" ? t("liturgy.review.banner.in_review")
    : liturgy.state === "approved" ? t("liturgy.review.banner.approved")
    : liturgy.state === "published" ? t("liturgy.locked_final")
    : liturgy.state === "needs_revision" ? t("liturgy.review.banner.needs_revision")
    : "";
  const problems = isCode(run.error, "has_problems") && run.error instanceof ApiError ? run.error.problem.problems ?? [] : [];

  if (!banner && !reviewable && !a.submit && !a.approve && !a.request_changes && !a.reopen) return null;
  return (
    <section aria-labelledby="review-title" className="space-y-3">
      <h3 id="review-title" className="text-lg font-semibold">{t("liturgy.review.title")}</h3>
      {banner && <Alert variant="info">{banner}</Alert>}
      {liturgy.state === "needs_revision" && last?.note && last.to_state === "needs_revision" && (
        <p className="rounded-md border border-border p-3">{t("liturgy.review.banner.needs_revision_note", { name: last.user.name, note: last.note })}</p>
      )}

      <div className="flex flex-wrap gap-2">
        {a.submit && <Button type="button" disabled={run.isPending} onClick={() => start("submit")}>{t("liturgy.review.submit")}</Button>}
        {a.approve && <Button type="button" variant={asking === "approve" ? "default" : "outline"} disabled={run.isPending} onClick={() => start("approve")}>{t("liturgy.review.approve")}</Button>}
        {a.request_changes && <Button type="button" variant="outline" disabled={run.isPending} onClick={() => start("request_changes")}>{t("liturgy.review.request_changes")}</Button>}
        {a.reopen && <Button type="button" variant="outline" disabled={run.isPending} onClick={() => start("reopen")}>{t("liturgy.review.reopen")}</Button>}
      </div>

      {asking && (
        <form noValidate role="group" aria-label={t(`liturgy.review.${asking}`)} className="space-y-3 rounded-md border border-border p-3"
          onSubmit={(e) => { e.preventDefault(); send(); }}>
          <Field label={t("liturgy.review.note")} hint={t("liturgy.review.note_hint")}>
            <textarea className={fieldClass} rows={3} value={note} maxLength={4000} onChange={(e) => setNote(e.target.value)} />
          </Field>
          {asking === "approve" && open > 0 && confirmed && <Alert variant="error">{t("liturgy.review.open_warning", { count: open })}</Alert>}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={run.isPending}>
              {asking === "approve" && open > 0 && confirmed ? t("liturgy.review.approve_anyway") : t("liturgy.review.send")}
            </Button>
            <Button type="button" variant="outline" onClick={() => { setAsking(null); setConfirmed(false); }}>{t("liturgy.review.cancel")}</Button>
          </div>
        </form>
      )}

      {problems.length > 0 ? (
        <div className="space-y-2 rounded-md border border-destructive p-3" role="alert">
          <p className="font-semibold">{t("liturgy.review.problems_title")}</p>
          <ul className="list-disc pl-6">
            {problems.map((p, i) => {
              const place = items.findIndex((x) => x.id === p.item_id);
              return <li key={i}><a className="underline" href={`#item-${p.item_id}`}>{problemText(t, p.code, place + 1, items[place]?.title ?? "")}</a></li>;
            })}
          </ul>
        </div>
      ) : <ErrorAlert error={run.error} />}

      {reviewable && (
        <div className="space-y-1">
          <h4 className="font-medium">{t("liturgy.review.history.title")}</h4>
          <ErrorAlert error={history.error} onRetry={() => void history.refetch()} />
          {history.data && (history.data.items ?? []).length === 0 && <p className="text-muted-foreground">{t("liturgy.review.history.none")}</p>}
          <ul className="space-y-1">
            {(history.data?.items ?? []).map((c) => (
              <li key={c.id}>
                {t("liturgy.review.history.row", { name: c.user.name, from: t(`liturgy.states.${c.from_state}`), to: t(`liturgy.states.${c.to_state}`) })}
                <span className="text-sm text-muted-foreground"> · {when.format(new Date(c.created_at))}</span>
                {c.note && <p className="ml-4 text-sm">{c.note}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
