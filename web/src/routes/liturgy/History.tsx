// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useOutletContext } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { api, call } from "@/lib/api";
import { ApiError } from "@/lib/errors";
import { editsQuery, editText, liturgyQuery, undoWhat } from "@/lib/liturgy";
import type { LiturgyView, Me } from "@liturgist/api-client";

// History lists the last changes in words and holds Undo and Redo (11 §4, §7.2).
// Undo takes back the caller's own newest change that nobody else has touched
// since; Ctrl/Cmd+Z and Ctrl/Cmd+Shift+Z (or Ctrl+Y) do the same, but only when
// the focus is not in a text field, where the browser's own undo is used.
export function History({ id, canEdit }: { id: string; canEdit: boolean }) {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me | undefined>();
  const queryClient = useQueryClient();
  const edits = useQuery(editsQuery(id));
  const items = edits.data?.items ?? [];
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");
  const [error, setError] = useState<unknown>(null);
  // A response that said nothing applies switches the button off until a new change appears.
  const [off, setOff] = useState<{ undo: boolean; redo: boolean; top: string }>({ undo: false, redo: false, top: "" });
  const top = items[0]?.id ?? "";
  const gate = off.top === top ? off : { undo: false, redo: false, top };
  const mineUndone = items.some((e) => e.user_id === me?.user?.id && e.status === "undone");
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });

  const run = async (kind: "undo" | "redo") => {
    if (busy || !canEdit) return;
    setBusy(true);
    setError(null);
    setStatus("");
    const key = liturgyQuery(id).queryKey;
    const cached = queryClient.getQueryData<LiturgyView>(key);
    try {
      const r = await call(api.POST(kind === "undo" ? "/liturgies/{id}/undo" : "/liturgies/{id}/redo", { params: { path: { id } } }));
      const title = cached?.items?.find((i) => i.id === r.edit.item_id)?.title ?? "";
      const what = undoWhat(t, r.edit.command, title);
      setStatus(t(kind === "undo" ? "liturgy.undo.undone" : "liturgy.undo.redone", { what }));
      await Promise.all([queryClient.invalidateQueries({ queryKey: key, exact: true }), queryClient.invalidateQueries({ queryKey: [...key, "edits"] })]);
    } catch (err) {
      if (err instanceof ApiError && err.code === "undo_refused") {
        const reason = err.problem.reason ?? "changed_since";
        setStatus(t(`liturgy.undo.refused.${kind}.${reason}`, { defaultValue: t(`liturgy.undo.refused.${kind}.changed_since`) }));
        if (reason === "nothing_to_undo") setOff({ ...gate, undo: true, top });
        if (reason === "nothing_to_redo") setOff({ ...gate, redo: true, top });
        await queryClient.invalidateQueries({ queryKey: [...key, "edits"] });
      } else {
        setError(err);
        if (err instanceof ApiError && err.code === "liturgy_locked") await queryClient.invalidateQueries({ queryKey: key, exact: true });
      }
    } finally {
      setBusy(false);
    }
  };

  const latest = useRef(run);
  latest.current = run;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing || !(e.ctrlKey || e.metaKey) || e.altKey) return;
      const target = e.target instanceof Element ? e.target : null;
      if (target?.closest("input, textarea, select, [contenteditable]:not([contenteditable='false'])")) return;
      const k = e.key.toLowerCase();
      if (k === "z" && !e.shiftKey) {
        e.preventDefault();
        void latest.current("undo");
      } else if ((k === "z" && e.shiftKey) || (k === "y" && !e.shiftKey)) {
        e.preventDefault();
        void latest.current("redo");
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <section aria-labelledby="history-title" className="space-y-2">
      <h3 id="history-title" className="text-lg font-semibold">{t("liturgy.history.title")}</h3>
      {canEdit && (
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={busy || gate.undo} onClick={() => void run("undo")}>{t("liturgy.undo.button")}</Button>
          <Button type="button" variant="outline" disabled={busy || gate.redo || !mineUndone} onClick={() => void run("redo")}>{t("liturgy.undo.redo_button")}</Button>
        </div>
      )}
      <p role="status" className="min-h-6">{status}</p>
      <ErrorAlert error={error} />
      <ErrorAlert error={edits.error} onRetry={() => void edits.refetch()} />
      {edits.isPending && <p role="status">{t("app.loading")}</p>}
      {edits.data && items.length === 0 && <p className="text-muted-foreground">{t("liturgy.history.none")}</p>}
      <ul className="space-y-1">
        {items.map((e) => (
          <li key={e.id}>
            {editText(t, e)} <span className="text-sm text-muted-foreground">· {when.format(new Date(e.created_at))}</span>
            {e.status !== "done" && <span className="text-sm text-muted-foreground"> · {t("liturgy.history.undone")}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}
