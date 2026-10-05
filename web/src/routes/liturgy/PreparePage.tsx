// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me, OccurrenceView } from "@liturgist/api-client";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { isCode, ApiError } from "@/lib/errors";
import { addDays, liturgiesQuery, longDate, prepareQuery, weekdayOf } from "@/lib/liturgy";
import { hasScope } from "@/lib/scopes";
import { liturgyPath, paths } from "../paths";

const keyOf = (o: Pick<OccurrenceView, "service_id" | "date" | "time">) => `${o.service_id}|${o.date}|${o.time}`;

// PreparePage creates next week's liturgies in one step (11 §3): the
// occurrences of the regular services as a checklist.
export function PreparePage() {
  const me = useOutletContext<Me>();
  if (!hasScope(me, "liturgy.edit")) return <Navigate to={paths.planning} replace />;
  return <Prepare />;
}

function Prepare() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [week, setWeek] = useState(""); // "" = the week after the current one, chosen by the server
  const data = useQuery(prepareQuery(week));
  // Everything that is free is ticked until the user unticks it; a week change starts afresh.
  const [unticked, setUnticked] = useState<Set<string>>(new Set());
  const free = (data.data?.occurrences ?? []).filter((o) => !o.liturgy_id);
  const ticked = free.filter((o) => !unticked.has(keyOf(o)));
  const limits = data.data?.limits;
  // The tightest limit decides how many may be created (10 §3.1).
  const room = limits
    ? Math.min(...[limits.max_unpublished_liturgies, limits.max_active_liturgies].filter((l) => !l.unlimited).map((l) => l.max - l.used), Infinity)
    : Infinity;
  const tooMany = ticked.length > room;
  // Archiving the oldest published liturgies can make room under max_active_liturgies,
  // never under max_unpublished_liturgies (13 §4); the server checks it all again.
  const roomOf = (l?: { unlimited: boolean; max: number; used: number }) => (!l || l.unlimited ? Infinity : l.max - l.used);
  const roomActive = roomOf(limits?.max_active_liturgies);
  const need = ticked.length - roomActive;
  const canArchive = hasScope(me, "liturgy.manage") && tooMany && ticked.length <= roomOf(limits?.max_unpublished_liturgies) && need > 0;
  const oldest = useQuery({
    ...liturgiesQuery({ state: "published", to: data.data?.week ? addDays(data.data.week, -1) : "", order: "date_asc", limit: 50 }),
    enabled: canArchive && !!data.data?.week,
  });
  const archivable = (oldest.data?.items ?? []).filter((l) => l.actions.archive).slice(0, need);
  const archiveAndCreate = canArchive && archivable.length === need;

  const create = useMutation({
    mutationFn: (archiveIds: string[]) =>
      call(api.POST("/liturgies/prepare", {
        body: { occurrences: ticked.map((o) => ({ service_id: o.service_id, date: o.date, time: o.time })), ...(archiveIds.length > 0 ? { archive_ids: archiveIds } : {}) },
      })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["liturgies"] });
      await queryClient.invalidateQueries({ queryKey: ["prepare"] });
      await queryClient.invalidateQueries({ queryKey: ["liturgy"] });
      void navigate(paths.planning);
    },
  });
  const existing = create.error instanceof ApiError && isCode(create.error, "liturgy_exists") ? create.error.problem.liturgy_id : undefined;

  const move = (days: number) => {
    const base = data.data?.week ?? "";
    if (!base) return;
    setWeek(addDays(base, days));
    setUnticked(new Set());
    create.reset();
  };
  const toggle = (o: OccurrenceView) => {
    const next = new Set(unticked);
    if (next.has(keyOf(o))) next.delete(keyOf(o));
    else next.add(keyOf(o));
    setUnticked(next);
  };

  return (
    <div className="max-w-3xl space-y-4">
      <p><Link className="underline" to={paths.planning}>{t("liturgy.back")}</Link></p>
      <h2 className="text-xl font-semibold">{t("liturgy.prepare.title")}</h2>
      <p className="text-muted-foreground">{t("liturgy.prepare.intro")}</p>
      <div className="flex flex-wrap items-end gap-2">
        <Button variant="outline" disabled={!data.data} onClick={() => move(-7)}>{t("liturgy.prepare.previous")}</Button>
        <Field label={t("liturgy.prepare.pick")}>
          <Input type="date" value={data.data?.week ?? week} onChange={(e) => { if (e.target.value) { setWeek(e.target.value); setUnticked(new Set()); create.reset(); } }} />
        </Field>
        <Button variant="outline" disabled={!data.data} onClick={() => move(7)}>{t("liturgy.prepare.next")}</Button>
      </div>
      {data.data && <p className="font-medium" role="status">{t("liturgy.prepare.week_of", { date: longDate(data.data.week, i18n.language) })}</p>}
      <ErrorAlert error={data.error} onRetry={() => void data.refetch()} />
      {data.isPending && <p role="status">{t("app.loading")}</p>}
      {data.data && (data.data.occurrences ?? []).length === 0 && (
        <div className="space-y-3 rounded-md border border-border p-4">
          <p>{t("liturgy.prepare.no_services")}</p>
          <Link className={buttonVariants({ variant: "outline" })} to={paths.services}>{t("liturgy.list.to_services")}</Link>
        </div>
      )}
      {data.data && (data.data.occurrences ?? []).length > 0 && (
        <fieldset className="space-y-2">
          <legend className="font-semibold">{t("liturgy.prepare.occurrences")}</legend>
          <ul className="space-y-2">
            {(data.data.occurrences ?? []).map((o) => {
              const taken = !!o.liturgy_id;
              const id = "occ-" + keyOf(o).replace(/[^a-zA-Z0-9]/g, "-");
              return (
                <li key={keyOf(o)} className="flex flex-wrap items-center gap-3 rounded-md border border-border p-3">
                  <input id={id} type="checkbox" className="size-6" disabled={taken} checked={!taken && !unticked.has(keyOf(o))} onChange={() => toggle(o)} />
                  <label htmlFor={id} className="min-h-6 flex-1">
                    <span className="font-medium">{o.service_name}</span>
                    <span className="block text-sm text-muted-foreground">
                      {t(`planning.weekdays.${weekdayOf(o.date)}`)} {longDate(o.date, i18n.language)} · {o.time} · {t(`setup.content_languages.${o.language}`)}
                      {" · "}{o.template_name ? t("liturgy.prepare.template", { name: o.template_name }) : t("liturgy.prepare.no_template")}
                    </span>
                  </label>
                  {taken && o.liturgy_id && (
                    <span className="text-sm">
                      {t("liturgy.prepare.exists")} <Link className="underline" to={liturgyPath(o.liturgy_id)}>{t("liturgy.prepare.open")}</Link>
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        </fieldset>
      )}
      {limits && Number.isFinite(room) && (
        <p className={tooMany ? "text-destructive" : "text-muted-foreground"}>
          {t("liturgy.prepare.slots", { free: Math.max(0, room), max: Math.min(...[limits.max_unpublished_liturgies, limits.max_active_liturgies].filter((l) => !l.unlimited).map((l) => l.max)) })}
          {tooMany ? " " + t("liturgy.prepare.too_many", { n: ticked.length, free: Math.max(0, room) }) : ""}
        </p>
      )}
      {create.error && (
        existing ? (
          <p role="alert" className="text-destructive">
            {t("liturgy.prepare.exists_now")} <Link className="underline" to={liturgyPath(existing)}>{t("liturgy.prepare.open")}</Link>
          </p>
        ) : <ErrorAlert error={create.error} />
      )}
      {free.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <Button disabled={ticked.length === 0 || tooMany || create.isPending} onClick={() => create.mutate([])}>
            {t("liturgy.prepare.create", { count: ticked.length })}
          </Button>
          {archiveAndCreate && (
            <>
              <Button variant="outline" disabled={create.isPending} onClick={() => create.mutate(archivable.map((l) => l.id))}>
                {t("liturgy.prepare.archive_and_create", { count: ticked.length, archive: need })}
              </Button>
              <p className="text-sm text-muted-foreground">{t("liturgy.prepare.archive_hint")}</p>
            </>
          )}
        </div>
      )}
    </div>
  );
}
