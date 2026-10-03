// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { LiturgyView } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { assignableQuery, liturgyQuery } from "@/lib/liturgy";

type Named = { id: string; name: string };

// Team shows, for each duty that has people or an item using it, the people
// assigned and a form to add one (11 §4). Assignments are not versioned and
// save at once.
export function Team({ liturgy, duties, canEdit }: { liturgy: LiturgyView; duties: Named[]; canEdit: boolean }) {
  const { t } = useTranslation();
  const used = new Set([...(liturgy.assignments ?? []).map((a) => a.duty_id), ...(liturgy.items ?? []).flatMap((i) => (i.duty_id ? [i.duty_id] : []))]);
  const shown = duties.filter((d) => used.has(d.id));
  const free = duties.filter((d) => !used.has(d.id));
  return (
    <section aria-labelledby="team-title" className="space-y-3">
      <h3 id="team-title" className="text-lg font-semibold">{t("liturgy.team.title")}</h3>
      {shown.length === 0 && <p className="text-muted-foreground">{t("liturgy.team.none")}</p>}
      <ul className="space-y-3">
        {shown.map((d) => <DutyRow key={d.id} liturgy={liturgy} duty={d} canEdit={canEdit} />)}
      </ul>
      {canEdit && free.length > 0 && (
        <details className="rounded-md border border-border p-3">
          <summary className="min-h-12 cursor-pointer py-3">{t("liturgy.team.other_duties")}</summary>
          <ul className="space-y-3">
            {free.map((d) => <DutyRow key={d.id} liturgy={liturgy} duty={d} canEdit={canEdit} />)}
          </ul>
        </details>
      )}
    </section>
  );
}

function DutyRow({ liturgy, duty, canEdit }: { liturgy: LiturgyView; duty: Named; canEdit: boolean }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const people = (liturgy.assignments ?? []).filter((a) => a.duty_id === duty.id);
  const members = useQuery({ ...assignableQuery, enabled: canEdit });
  const [who, setWho] = useState(""); // a member's ID, or "" for someone without an account
  const [name, setName] = useState("");
  const key = liturgyQuery(liturgy.id).queryKey;
  const add = useMutation({
    mutationFn: () => call(api.POST("/liturgies/{id}/assignments", {
      params: { path: { id: liturgy.id } },
      body: who !== "" ? { duty_id: duty.id, user_id: who } : { duty_id: duty.id, name: name.trim() },
    })),
    onSuccess: (a) => {
      queryClient.setQueryData<LiturgyView>(key, (l) => (l ? { ...l, assignments: [...(l.assignments ?? []), a] } : l));
      void queryClient.invalidateQueries({ queryKey: [...key, "edits"] });
      setName("");
    },
  });
  const remove = useMutation({
    mutationFn: (aid: string) => call(api.DELETE("/liturgies/{id}/assignments/{aid}", { params: { path: { id: liturgy.id, aid } } })),
    onSuccess: (_, aid) => {
      queryClient.setQueryData<LiturgyView>(key, (l) => (l ? { ...l, assignments: (l.assignments ?? []).filter((a) => a.id !== aid) } : l));
      void queryClient.invalidateQueries({ queryKey: [...key, "edits"] });
    },
  });
  const ready = who !== "" || name.trim() !== "";
  return (
    <li className="space-y-2 rounded-md border border-border p-3">
      <h4 className="font-semibold">{duty.name}</h4>
      {people.length === 0 && <p className="text-muted-foreground">{t("liturgy.team.nobody")}</p>}
      <ul className="space-y-1">
        {people.map((a) => (
          <li key={a.id} className="flex flex-wrap items-center justify-between gap-2">
            <span>{a.name}{a.former_member ? ` ${t("liturgy.team.former")}` : ""}</span>
            {canEdit && (
              <Button variant="outline" disabled={remove.isPending} aria-label={`${t("planning.remove")} ${a.name} (${duty.name})`} onClick={() => remove.mutate(a.id)}>
                {t("planning.remove")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      <ErrorAlert error={add.error ?? remove.error} />
      {canEdit && (
        <div className="flex flex-wrap items-end gap-2">
          <Field label={`${t("liturgy.team.person")} (${duty.name})`}>
            <Select value={who} onChange={(e) => setWho(e.target.value)}>
              <option value="">{t("liturgy.team.someone_else")}</option>
              {(members.data?.items ?? []).map((m) => <option key={m.user_id} value={m.user_id}>{m.name}</option>)}
            </Select>
          </Field>
          {who === "" && (
            <Field label={`${t("liturgy.team.name")} (${duty.name})`}>
              <Input autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
          )}
          <Button disabled={!ready || add.isPending} aria-label={`${t("liturgy.team.add")} (${duty.name})`} onClick={() => add.mutate()}>{t("liturgy.team.add")}</Button>
        </div>
      )}
    </li>
  );
}
