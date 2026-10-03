// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useRef, useState, type FormEvent } from "react";
import { useOutletContext } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { isCode } from "@/lib/errors";
import type { NameEntry, NameList } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";

// focusSoon puts the keyboard focus back on a button after React has moved
// its row: moving a node in the page drops the focus.
function focusSoon(ids: string[]) {
  requestAnimationFrame(() => {
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el instanceof HTMLButtonElement && !el.disabled) {
        el.focus();
        return;
      }
    }
  });
}

// NameListPage edits one of the two short ordered lists, the duties or the
// singing parts: rename in the row, Move up / Move down with text, delete
// after a question, and add at the end (09 §5).
export function NameListPage({ list }: { list: NameList }) {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const queryClient = useQueryClient();
  const query = useQuery(list.query);
  const canEdit = hasScope(me, "templates.edit");
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [newName, setNewName] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [stale, setStale] = useState(false);
  const addRef = useRef<HTMLInputElement>(null);
  const key = list.query.queryKey;
  const refresh = () => queryClient.invalidateQueries({ queryKey: key });
  const items: NameEntry[] = (query.data?.items as NameEntry[] | undefined) ?? [];

  const add = useMutation({
    mutationFn: (name: string) => list.create(name),
    onSuccess: async () => {
      setNewName("");
      await refresh();
      addRef.current?.focus();
    },
  });
  const rename = useMutation({
    mutationFn: (v: { id: string; name: string }) => list.rename(v.id, v.name),
    onSuccess: async () => {
      setEditing(null);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (e: NameEntry) => list.remove(e.id),
    onSuccess: async (_, e) => {
      setAnnouncement(t("planning.removed", { name: e.name }));
      await refresh();
      addRef.current?.focus();
    },
  });
  const move = useMutation({
    mutationFn: (v: { ids: string[] }) => list.reorder(v.ids),
    onSuccess: (data) => queryClient.setQueryData(key, data),
    onError: (err) => {
      if (isCode(err, "version_conflict")) {
        setStale(true);
        void refresh();
      }
    },
  });

  const shift = (i: number, to: number) => {
    const ids = items.map((e) => e.id);
    const [moved] = ids.splice(i, 1);
    ids.splice(to, 0, moved);
    setStale(false);
    setAnnouncement(t("planning.moved", { name: items[i].name, n: to + 1, total: items.length }));
    move.mutate({ ids });
    focusSoon([`${items[i].id}-${to < i ? "up" : "down"}`, `${items[i].id}-${to < i ? "down" : "up"}`]);
  };
  const submitAdd = (e: FormEvent) => {
    e.preventDefault();
    if (newName.trim()) add.mutate(newName);
  };
  const submitRename = (e: FormEvent, entry: NameEntry) => {
    e.preventDefault();
    if (draft.trim()) rename.mutate({ id: entry.id, name: draft });
  };

  const text = (k: string, opts?: Record<string, unknown>) => t(`planning.${list.key}.${k}`, opts);
  const full = items.length >= list.max;
  return (
    <section aria-labelledby="list-title" className="max-w-2xl space-y-4">
      <h2 id="list-title" className="text-xl font-semibold">{text("title")}</h2>
      <p className="text-muted-foreground">{text("intro")}</p>
      {!canEdit && query.data && <p className="text-sm text-muted-foreground">{t("planning.read_only")}</p>}
      <p role="status" className="sr-only">{announcement}</p>
      <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />
      {stale && <Alert variant="error">{t("planning.order_changed")}</Alert>}
      <ErrorAlert error={rename.error ?? remove.error ?? (isCode(move.error, "version_conflict") ? null : move.error)} />
      {query.isPending && <p role="status">{t("app.loading")}</p>}
      {query.data && items.length === 0 && <p>{canEdit ? text("empty") : text("empty_viewer")}</p>}
      {items.length > 0 && (
        <ol className="space-y-2">
          {items.map((entry, i) => (
            <li key={entry.id} className="rounded-md border border-border p-3">
              {editing === entry.id ? (
                <form noValidate className="flex flex-wrap items-end gap-2" onSubmit={(e) => submitRename(e, entry)}>
                  <div className="min-w-48 flex-1">
                    <Field label={t("planning.name")}>
                      <Input autoFocus autoComplete="off" value={draft} onChange={(e) => setDraft(e.target.value)} />
                    </Field>
                  </div>
                  <Button type="submit" disabled={rename.isPending}>{t("planning.save_name")}</Button>
                  <Button variant="outline" onClick={() => { setEditing(null); rename.reset(); }}>{t("planning.cancel")}</Button>
                </form>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="mr-auto min-w-32 font-medium">{entry.name}</span>
                  {entry.actions.edit && (
                    <>
                      <Button variant="outline" aria-label={`${t("planning.rename")} ${entry.name}`} onClick={() => { setEditing(entry.id); setDraft(entry.name); rename.reset(); }}>
                        {t("planning.rename")}
                      </Button>
                      <Button id={`${entry.id}-up`} variant="outline" aria-label={`${t("planning.move_up")} ${entry.name}`} disabled={i === 0 || move.isPending} onClick={() => shift(i, i - 1)}>
                        {t("planning.move_up")}
                      </Button>
                      <Button id={`${entry.id}-down`} variant="outline" aria-label={`${t("planning.move_down")} ${entry.name}`} disabled={i === items.length - 1 || move.isPending} onClick={() => shift(i, i + 1)}>
                        {t("planning.move_down")}
                      </Button>
                    </>
                  )}
                  {entry.actions.delete && (
                    <ConfirmButton
                      label={t("planning.delete")}
                      question={text("delete_question", { name: entry.name })}
                      confirmLabel={text("delete_confirm", { name: entry.name })}
                      pending={remove.isPending}
                      onConfirm={() => remove.mutate(entry)}
                    />
                  )}
                </div>
              )}
            </li>
          ))}
        </ol>
      )}
      {canEdit && query.data && (
        <form noValidate className="space-y-2" onSubmit={submitAdd}>
          <ErrorAlert error={add.error} />
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-48 flex-1">
              <Field label={text("add_label")}>
                <Input ref={addRef} autoComplete="off" value={newName} disabled={full} onChange={(e) => { setNewName(e.target.value); add.reset(); }} />
              </Field>
            </div>
            <Button type="submit" disabled={full || add.isPending || newName.trim() === ""}>{text("add")}</Button>
          </div>
          <p className="text-sm text-muted-foreground">{t("planning.used_of", { used: items.length, max: list.max })}</p>
        </form>
      )}
    </section>
  );
}
