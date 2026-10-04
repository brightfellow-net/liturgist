// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { CommentView, LiturgyView } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { fieldClass } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { isCode } from "@/lib/errors";
import { commentsQuery, liturgyQuery } from "@/lib/liturgy";

// useComments shares the comments of a liturgy between the item cards and the
// review panel (12 §6); it is off for a member without a liturgy scope.
export function useComments(liturgy: LiturgyView) {
  return useQuery({ ...commentsQuery(liturgy.id), enabled: liturgy.open_comments !== undefined });
}

function useCommentWrites(liturgy: LiturgyView, announce?: (message: string) => void) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: commentsQuery(liturgy.id).queryKey }),
      queryClient.invalidateQueries({ queryKey: liturgyQuery(liturgy.id).queryKey, exact: true }),
    ]);
  };
  const add = useMutation({
    mutationFn: ({ itemId, body }: { itemId: string; body: string }) =>
      call(api.POST("/liturgies/{id}/comments", { params: { path: { id: liturgy.id } }, body: { body, ...(itemId ? { item_id: itemId } : {}) } })),
    onSuccess: async () => { await refresh(); announce?.(t("liturgy.comments.done.added")); },
    // The liturgy was approved meanwhile: show it as it is now.
    onError: async (err) => { if (isCode(err, "liturgy_locked")) await refresh(); },
  });
  const resolve = useMutation({
    mutationFn: ({ id, resolved }: { id: string; resolved: boolean }) =>
      call(api.PUT("/liturgies/{id}/comments/{cid}/resolved", { params: { path: { id: liturgy.id, cid: id } }, body: { resolved } })),
    onSuccess: async (_, { resolved }) => { await refresh(); announce?.(t(resolved ? "liturgy.comments.done.resolved" : "liturgy.comments.done.reopened")); },
    onError: async (err) => { if (isCode(err, "liturgy_locked")) await refresh(); },
  });
  return { add, resolve };
}

// CommentForm writes one comment, on an item or on the whole liturgy.
function CommentForm({ liturgy, itemId, label, onClose, announce }: {
  liturgy: LiturgyView; itemId: string; label: string; onClose?: () => void; announce?: (message: string) => void;
}) {
  const { t } = useTranslation();
  const { add } = useCommentWrites(liturgy, announce);
  const [body, setBody] = useState("");
  return (
    <form noValidate role="group" aria-label={label} className="space-y-2 rounded-md border border-border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        add.mutate({ itemId, body: body.trim() }, { onSuccess: () => { setBody(""); onClose?.(); } });
      }}>
      <ErrorAlert error={add.error} />
      <Field label={t("liturgy.comments.label")} hint={t("liturgy.comments.hint")}>
        <textarea className={fieldClass} rows={3} value={body} maxLength={20000} onChange={(e) => setBody(e.target.value)} />
      </Field>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={add.isPending || body.trim() === ""}>{t("liturgy.comments.send")}</Button>
        {onClose && <Button type="button" variant="outline" onClick={onClose}>{t("liturgy.comments.cancel")}</Button>}
      </div>
    </form>
  );
}

function CommentRow({ liturgy, comment, where, announce }: { liturgy: LiturgyView; comment: CommentView; where?: string; announce?: (message: string) => void }) {
  const { t, i18n } = useTranslation();
  const { resolve } = useCommentWrites(liturgy, announce);
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });
  return (
    <li className="space-y-1 rounded-md border border-border p-2">
      {where && <p className="text-sm text-muted-foreground">{where}</p>}
      <p className="whitespace-pre-line">{comment.body}</p>
      <p className="text-sm text-muted-foreground">
        {comment.author.name} · {when.format(new Date(comment.created_at))}
        {comment.resolved && comment.resolved_by && <> · {t("liturgy.comments.resolved_by", { name: comment.resolved_by.name })}</>}
      </p>
      {liturgy.actions.comment && (
        <Button type="button" variant="outline" disabled={resolve.isPending}
          onClick={() => resolve.mutate({ id: comment.id, resolved: !comment.resolved })}>
          {comment.resolved ? t("liturgy.comments.reopen") : t("liturgy.comments.resolve")}
        </Button>
      )}
      <ErrorAlert error={resolve.error} />
    </li>
  );
}

// ItemComments are the comments of one item, under its card: the open ones, the
// resolved ones folded, and a button to write another.
export function ItemComments({ liturgy, itemId, title, announce }: { liturgy: LiturgyView; itemId: string; title: string; announce?: (message: string) => void }) {
  const { t } = useTranslation();
  const comments = useComments(liturgy);
  const [writing, setWriting] = useState(false);
  if (liturgy.open_comments === undefined) return null;
  const mine = (comments.data?.items ?? []).filter((c) => c.item_id === itemId);
  const open = mine.filter((c) => !c.resolved);
  const done = mine.filter((c) => c.resolved);
  if (mine.length === 0 && !liturgy.actions.comment) return null;
  return (
    <div className="mt-2 space-y-2" aria-label={t("liturgy.comments.on_item", { title })} role="group">
      {open.length > 0 && <p className="text-sm font-medium">{t("liturgy.comments.open_count", { count: open.length })}</p>}
      <ul className="space-y-2">
        {open.map((c) => <CommentRow key={c.id} liturgy={liturgy} comment={c} announce={announce} />)}
      </ul>
      {done.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm">{t("liturgy.comments.show_resolved", { count: done.length })}</summary>
          <ul className="mt-2 space-y-2">
            {done.map((c) => <CommentRow key={c.id} liturgy={liturgy} comment={c} announce={announce} />)}
          </ul>
        </details>
      )}
      {liturgy.actions.comment && !writing && (
        <Button type="button" variant="outline" onClick={() => setWriting(true)}>
          <span aria-hidden="true">{t("liturgy.comments.add")}</span>
          <span className="sr-only">{t("liturgy.comments.add_on", { title })}</span>
        </Button>
      )}
      {writing && <CommentForm liturgy={liturgy} itemId={itemId} label={t("liturgy.comments.add_on", { title })} onClose={() => setWriting(false)} announce={announce} />}
    </div>
  );
}

type Filter = "open" | "resolved" | "all";

// ReviewComments is the panel of all comments of the liturgy (12 §6): filters,
// the comments with where they belong, and a form for the whole liturgy.
export function ReviewComments({ liturgy, announce }: { liturgy: LiturgyView; announce?: (message: string) => void }) {
  const { t } = useTranslation();
  const comments = useComments(liturgy);
  const [filter, setFilter] = useState<Filter>("open");
  if (liturgy.open_comments === undefined) return null;
  const items = liturgy.items ?? [];
  const all = comments.data?.items ?? [];
  const shown = all.filter((c) => filter === "all" || (filter === "open" ? !c.resolved : c.resolved));
  const where = (c: CommentView) => {
    if (!c.item_id) return t("liturgy.comments.whole");
    const place = items.findIndex((i) => i.id === c.item_id);
    return place >= 0 ? `${place + 1}. ${items[place].title}` : t("liturgy.comments.removed_item", { title: c.item_title });
  };
  return (
    <div className="space-y-2">
      <h4 className="font-medium">{t("liturgy.comments.title")}</h4>
      <ErrorAlert error={comments.error} onRetry={() => void comments.refetch()} />
      <div role="group" aria-label={t("liturgy.comments.filter.label")} className="flex flex-wrap gap-2">
        {(["open", "resolved", "all"] as const).map((f) => (
          <Button key={f} type="button" variant={filter === f ? "default" : "outline"} aria-pressed={filter === f} onClick={() => setFilter(f)}>
            {t(`liturgy.comments.filter.${f}`)}
          </Button>
        ))}
      </div>
      {comments.data && shown.length === 0 && <p className="text-muted-foreground">{t("liturgy.comments.none")}</p>}
      <ul className="space-y-2">
        {shown.map((c) => <CommentRow key={c.id} liturgy={liturgy} comment={c} where={where(c)} announce={announce} />)}
      </ul>
      {liturgy.actions.comment && <CommentForm liturgy={liturgy} itemId="" label={t("liturgy.comments.whole")} announce={announce} />}
    </div>
  );
}
