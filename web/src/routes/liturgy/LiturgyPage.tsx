// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, useNavigate, useOutletContext, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { LiturgyView, Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { ApiError, fieldErrors, isCode } from "@/lib/errors";
import { liturgyLimits, liturgyQuery, longDate, problemText, withOrder } from "@/lib/liturgy";
import { dutiesQuery, itemTypes, singingPartsQuery, type ItemType } from "@/lib/planning";
import { ItemCard } from "./ItemCard";
import { History } from "./History";
import { Review } from "./Review";
import { Team } from "./Team";
import { paths } from "../paths";

// LiturgyPage is the editor (11 §4): a column of cards. It is read-only when
// the liturgy's actions say the member cannot edit it.
export function LiturgyPage() {
  const { id = "" } = useParams();
  const { t } = useTranslation();
  const liturgy = useQuery(liturgyQuery(id));
  const duties = useQuery(dutiesQuery);
  const parts = useQuery(singingPartsQuery);
  if (liturgy.error) {
    return isCode(liturgy.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("liturgy.not_found")}</Alert>
        <Link className="underline" to={paths.planning}>{t("liturgy.back")}</Link>
      </div>
    ) : <ErrorAlert error={liturgy.error} onRetry={() => void liturgy.refetch()} />;
  }
  if (!liturgy.data || !duties.data || !parts.data) {
    return duties.error || parts.error ? <ErrorAlert error={duties.error ?? parts.error} /> : <p role="status">{t("app.loading")}</p>;
  }
  return <Editor liturgy={liturgy.data} duties={duties.data.items ?? []} parts={parts.data.items ?? []} />;
}

type Named = { id: string; name: string };

function Editor({ liturgy, duties, parts }: { liturgy: LiturgyView; duties: Named[]; parts: Named[] }) {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const key = liturgyQuery(liturgy.id).queryKey;
  const [status, setStatus] = useState("");
  const [structureError, setStructureError] = useState<unknown>(null);
  const items = liturgy.items ?? [];
  const editable = liturgy.actions.edit;
  const keyDisplay = me.church?.key_display;
  const touch = () => void queryClient.invalidateQueries({ queryKey: [...key, "edits"] });

  // A structure conflict (scope "liturgy") reloads the list and says so; unsaved
  // item text stays in the cards (11 §4).
  const structure = async (run: () => Promise<void>) => {
    setStructureError(null);
    try {
      await run();
    } catch (err) {
      if (err instanceof ApiError && isCode(err, "version_conflict")) {
        await queryClient.invalidateQueries({ queryKey: key, exact: true });
        setStatus(t("liturgy.list_conflict"));
      } else setStructureError(err);
    }
  };
  const focusLater = (id: string) => setTimeout(() => document.getElementById(id)?.focus(), 0);

  const moveItem = (index: number, delta: number) =>
    structure(async () => {
      const ids = items.map((i) => i.id);
      const to = index + delta;
      [ids[index], ids[to]] = [ids[to], ids[index]];
      const r = await call(api.PUT("/liturgies/{id}/items/order", { params: { path: { id: liturgy.id } }, body: { liturgy_version: liturgy.version, item_ids: ids } }));
      queryClient.setQueryData<LiturgyView>(key, (l) => (l ? withOrder(l, r.item_ids ?? ids, r.liturgy_version) : l));
      touch();
      const title = items[index].title;
      setStatus(t("liturgy.moved", { title, n: to + 1, total: items.length }));
      const dir = delta < 0 ? "up" : "down";
      const edge = delta < 0 ? to === 0 : to === items.length - 1;
      focusLater(`move-${edge ? (dir === "up" ? "down" : "up") : dir}-${items[index].id}`);
    });

  const removeItem = (index: number) =>
    structure(async () => {
      const item = items[index];
      const r = await call(api.DELETE("/liturgies/{id}/items/{iid}", { params: { path: { id: liturgy.id, iid: item.id }, query: { liturgy_version: liturgy.version } } }));
      queryClient.setQueryData<LiturgyView>(key, (l) => (l ? withOrder(l, r.item_ids ?? [], r.liturgy_version) : l));
      touch();
      setStatus(t("liturgy.removed", { title: item.title }));
    });

  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/liturgies/{id}", { params: { path: { id: liturgy.id } } })),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: key });
      await queryClient.invalidateQueries({ queryKey: ["liturgies"] });
      void navigate(paths.planning);
    },
  });

  const problems = liturgy.problems ?? [];
  const placeOf = (itemId: string) => items.findIndex((i) => i.id === itemId);

  return (
    <div className="max-w-3xl space-y-6">
      <p><Link className="underline" to={paths.planning}>{t("liturgy.back")}</Link></p>
      <Header liturgy={liturgy} lang={i18n.language} onSaved={() => { setStatus(t("liturgy.details.saved")); touch(); }} />
      <Review liturgy={liturgy} onChanged={(message) => { setStatus(message); touch(); }} />
      <div role="status" aria-live="polite" className="sr-only">{status}</div>
      {status && <p className="text-sm text-muted-foreground">{status}</p>}
      <ErrorAlert error={structureError} />

      {problems.length > 0 && (
        <section aria-labelledby="problems-title" className="space-y-2 rounded-md border border-destructive p-3">
          <h3 id="problems-title" className="font-semibold">{t("liturgy.problems.title")}</h3>
          <ul className="list-disc pl-6">
            {problems.map((p, i) => {
              const place = placeOf(p.item_id);
              return (
                <li key={i}>
                  <a className="underline" href={`#item-${p.item_id}`}>{problemText(t, p.code, place + 1, items[place]?.title ?? "")}</a>
                </li>
              );
            })}
          </ul>
        </section>
      )}

      <section aria-labelledby="items-title" className="space-y-4">
        <h3 id="items-title" className="text-lg font-semibold">{t("liturgy.items.title")}</h3>
        {items.length === 0 && <p className="text-muted-foreground">{t("liturgy.items.none")}</p>}
        <ol className="space-y-4">
          {items.map((item, i) => (
            <li key={item.id} id={`item-${item.id}`}>
              <ItemCard
                liturgy={liturgy} item={item} place={i + 1} count={items.length}
                duties={duties} parts={parts} keyDisplay={keyDisplay}
                onMove={(d) => void moveItem(i, d)} onRemove={() => void removeItem(i)}
              />
            </li>
          ))}
        </ol>
      </section>

      {editable && items.length < liturgyLimits.items && (
        <AddItem liturgy={liturgy} duties={duties} onAdded={(title) => { setStatus(t("liturgy.items.added", { title })); touch(); }} />
      )}
      {editable && items.length >= liturgyLimits.items && <p className="text-muted-foreground">{t("liturgy.items.max", { max: liturgyLimits.items })}</p>}

      <Team liturgy={liturgy} duties={duties} canEdit={editable} />
      <History id={liturgy.id} canEdit={editable} />

      {liturgy.actions.delete && (
        <div className="space-y-2 border-t border-border pt-4">
          <ErrorAlert error={remove.error} />
          <ConfirmButton
            label={t("liturgy.delete")}
            question={t("liturgy.delete_question", { name: liturgy.service_name, date: longDate(liturgy.date, i18n.language) })}
            confirmLabel={t("liturgy.delete_confirm")}
            pending={remove.isPending}
            onConfirm={() => remove.mutate()}
          />
        </div>
      )}
    </div>
  );
}

function Header({ liturgy, lang, onSaved }: { liturgy: LiturgyView; lang: string; onSaved: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(liturgy.service_name);
  const [date, setDate] = useState(liturgy.date);
  const [time, setTime] = useState(liturgy.time);
  const [invalid, setInvalid] = useState<string[]>([]);
  const [conflict, setConflict] = useState(false);
  const save = useMutation({
    mutationFn: () => call(api.PATCH("/liturgies/{id}", { params: { path: { id: liturgy.id } }, body: { version: liturgy.version, service_name: name.trim(), date, time } })),
    onSuccess: (l) => {
      // The response holds the whole liturgy; the cards keep their own copies of their items.
      queryClient.setQueryData<LiturgyView>(liturgyQuery(liturgy.id).queryKey, (old) =>
        old ? { ...old, version: l.version, date: l.date, time: l.time, service_name: l.service_name, updated_at: l.updated_at } : l);
      void queryClient.invalidateQueries({ queryKey: ["liturgies"] });
      setEditing(false);
      onSaved();
    },
    onError: (err) => {
      setInvalid(fieldErrors(err));
      setConflict(isCode(err, "version_conflict"));
    },
  });
  const bad = (f: string) => (invalid.includes(f) ? t("common.field_invalid") : undefined);
  return (
    <header className="space-y-2">
      <h2 className="text-2xl font-semibold">{liturgy.service_name}</h2>
      <p className="text-muted-foreground">
        {longDate(liturgy.date, lang)} {liturgy.time} · {t(`setup.content_languages.${liturgy.language}`)} · {t(`liturgy.states.${liturgy.state}`)}
      </p>
      {liturgy.actions.edit && !editing && <Button variant="outline" onClick={() => { setName(liturgy.service_name); setDate(liturgy.date); setTime(liturgy.time); setEditing(true); }}>{t("liturgy.details.edit")}</Button>}
      {editing && (
        <form noValidate className="space-y-3 rounded-md border border-border p-3" onSubmit={(e) => { e.preventDefault(); setInvalid([]); setConflict(false); save.mutate(); }}>
          {conflict ? <Alert variant="error">{t("liturgy.details.conflict")}</Alert> : <ErrorAlert error={save.error} />}
          <Field label={t("liturgy.new.name")} error={bad("service_name")}><Input autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t("liturgy.date")} error={bad("date")}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
            <Field label={t("liturgy.time")} error={bad("time")}><Input type="time" value={time} onChange={(e) => setTime(e.target.value)} /></Field>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={save.isPending || name.trim() === "" || date === "" || time === ""}>{t("liturgy.details.save")}</Button>
            <Button variant="outline" onClick={() => setEditing(false)}>{t("planning.cancel")}</Button>
          </div>
        </form>
      )}
    </header>
  );
}

function AddItem({ liturgy, duties, onAdded }: { liturgy: LiturgyView; duties: Named[]; onAdded: (title: string) => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [type, setType] = useState<ItemType>("song");
  const [duty, setDuty] = useState("");
  const add = useMutation({
    mutationFn: () => call(api.POST("/liturgies/{id}/items", {
      params: { path: { id: liturgy.id } },
      body: { liturgy_version: liturgy.version, title: title.trim(), item_type: type, ...(duty ? { duty_id: duty } : {}) },
    })),
    onSuccess: (r) => {
      queryClient.setQueryData<LiturgyView>(liturgyQuery(liturgy.id).queryKey, (l) =>
        l ? { ...l, version: r.liturgy_version, items: [...(l.items ?? []), r.item] } : l);
      const added = title.trim();
      setTitle("");
      onAdded(added);
    },
    onError: async (err) => {
      // Someone else changed the list: reload it so the next try uses the new version.
      if (isCode(err, "version_conflict")) await queryClient.invalidateQueries({ queryKey: liturgyQuery(liturgy.id).queryKey, exact: true });
    },
  });
  return (
    <form noValidate className="space-y-3 rounded-md border border-border p-4" onSubmit={(e) => { e.preventDefault(); add.mutate(); }}>
      <h3 className="text-lg font-semibold">{t("liturgy.add_item.title")}</h3>
      {isCode(add.error, "version_conflict") ? <Alert variant="error">{t("liturgy.list_conflict")}</Alert> : <ErrorAlert error={add.error} />}
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label={t("liturgy.item.title")}><Input autoComplete="off" value={title} onChange={(e) => setTitle(e.target.value)} /></Field>
        <Field label={t("planning.templates.item_type")} hint={t("liturgy.add_item.type_hint")}>
          <Select value={type} onChange={(e) => setType(e.target.value as ItemType)}>
            {itemTypes.map((x) => <option key={x} value={x}>{t(`planning.item_types.${x}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("liturgy.item.duty")}>
          <Select value={duty} onChange={(e) => setDuty(e.target.value)}>
            <option value="">{t("planning.none")}</option>
            {duties.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </Select>
        </Field>
      </div>
      <Button type="submit" disabled={title.trim() === "" || add.isPending}>{t("liturgy.add_item.add")}</Button>
    </form>
  );
}
