// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link, Navigate, useNavigate, useOutletContext, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useFieldArray, useForm, type UseFormReturn } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { Me, TemplateView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { contentLanguages } from "@/lib/church";
import { api, call } from "@/lib/api";
import { fieldErrors, isCode } from "@/lib/errors";
import { dutiesQuery, itemTypes, planningLimits, templateQuery, textItemTypes, type ItemType } from "@/lib/planning";
import { hasScope } from "@/lib/scopes";
import { paths } from "../paths";

const itemSchema = z.object({
  title: z.string().trim().min(1, "planning.templates.title_required").max(200, "common.field_invalid"),
  item_type: z.enum(itemTypes),
  default_duty_id: z.string(),
  default_text: z.string().max(5000, "common.field_invalid"),
});
const schema = z.object({
  name: z.string().trim().min(1, "planning.templates.name_required").max(100, "common.field_invalid"),
  language: z.string().min(1),
  items: z.array(itemSchema).max(planningLimits.templateItems),
});
type Values = z.infer<typeof schema>;

const textarea = "block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive";

// focusSoon puts the keyboard focus back on a button after React has moved its row.
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

function toValues(tpl: TemplateView | undefined, language: string): Values {
  return {
    name: tpl?.name ?? "",
    language: tpl?.language ?? language,
    items: (tpl?.items ?? []).map((i) => ({
      title: i.title, item_type: i.item_type, default_duty_id: i.default_duty_id ?? "", default_text: i.default_text,
    })),
  };
}

// TemplateFormPage adds or edits a template: its name, its language and its
// items, in order. Members without templates.edit see it read-only (09 §5).
export function TemplateFormPage() {
  const { id } = useParams();
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const template = useQuery({ ...templateQuery(id ?? ""), enabled: id !== undefined });
  const [generation, setGeneration] = useState(0); // a reload after a conflict remounts the form
  if (id === undefined) {
    if (!hasScope(me, "templates.edit")) return <Navigate to={paths.templates} replace />;
    return <TemplateForm key="new" language={me.church?.default_language ?? "id"} />;
  }
  if (template.error) {
    return isCode(template.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("planning.not_found")}</Alert>
        <Link className="underline" to={paths.templates}>{t("planning.templates.back")}</Link>
      </div>
    ) : <ErrorAlert error={template.error} onRetry={() => void template.refetch()} />;
  }
  if (!template.data) return <p role="status">{t("app.loading")}</p>;
  const reload = async () => {
    const result = await template.refetch();
    if (result.data) setGeneration((g) => g + 1);
  };
  return <TemplateForm key={generation + ":" + template.data.version} template={template.data} language="id" onReload={reload} />;
}

function TemplateForm({ template, language, onReload }: { template?: TemplateView; language: string; onReload?: () => Promise<void> }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const duties = useQuery(dutiesQuery);
  const [conflict, setConflict] = useState(false);
  const readOnly = template !== undefined && !template.actions.edit;
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: toValues(template, language) });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);

  const done = async (saved?: TemplateView) => {
    if (saved) queryClient.setQueryData(templateQuery(saved.id).queryKey, saved);
    else queryClient.removeQueries({ queryKey: ["template", template?.id] });
    await queryClient.invalidateQueries({ queryKey: ["templates"] });
    void navigate(paths.templates);
  };
  const body = (v: Values) => ({
    name: v.name, language: v.language as (typeof contentLanguages)[number],
    items: v.items.map((i) => ({
      title: i.title, item_type: i.item_type,
      default_text: textItemTypes.includes(i.item_type) ? i.default_text : "",
      default_duty_id: i.default_duty_id,
    })),
  });
  const save = useMutation({
    mutationFn: (v: Values) =>
      template
        ? call(api.PATCH("/templates/{id}", { params: { path: { id: template.id } }, body: { ...body(v), version: template.version } }))
        : call(api.POST("/templates", { body: body(v) })),
    onSuccess: (saved) => done(saved),
    onError: (err) => {
      setConflict(isCode(err, "version_conflict"));
      for (const f of fieldErrors(err)) {
        if (f === "name") form.setError("name", { message: "common.field_invalid" });
      }
    },
  });
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/templates/{id}", { params: { path: { id: template?.id ?? "" } } })),
    onSuccess: () => done(),
  });

  // The duty list must be there before the form: a select that gets its options
  // later would show "None" and save the default duty away.
  if (duties.error) return <ErrorAlert error={duties.error} onRetry={() => void duties.refetch()} />;
  if (!duties.data) return <p role="status">{t("app.loading")}</p>;
  if (readOnly && template) return <TemplateReadOnly template={template} dutyNames={dutyNames(duties.data?.items)} />;

  return (
    <form noValidate className="max-w-3xl space-y-8" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <p><Link className="underline" to={paths.templates}>{t("planning.templates.back")}</Link></p>
      <h2 className="text-xl font-semibold">{template ? template.name : t("planning.templates.new_title")}</h2>
      {conflict ? (
        <Alert variant="error" className="space-y-2">
          <p>{t("planning.conflict")}</p>
          <Button variant="outline" onClick={() => void onReload?.()}>{t("planning.reload")}</Button>
        </Alert>
      ) : <ErrorAlert error={save.error} />}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("planning.name")} hint={t("planning.templates.name_hint")} error={msg(e.name?.message)}>
          <Input autoComplete="off" {...form.register("name")} />
        </Field>
        <Field label={t("planning.language")}>
          <Select {...form.register("language")}>
            {contentLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
      </div>

      <ItemsEditor form={form} duties={duties.data?.items ?? []} />

      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>{template ? t("planning.save_changes") : t("planning.templates.save")}</Button>
        <Button variant="outline" onClick={() => void navigate(paths.templates)}>{t("planning.cancel")}</Button>
      </div>
      {template?.actions.delete && (
        <div className="space-y-2 border-t border-border pt-4">
          <ErrorAlert error={remove.error} />
          <ConfirmButton
            label={t("planning.delete")}
            question={t("planning.templates.delete_question", { name: template.name })}
            confirmLabel={t("planning.templates.delete_confirm", { name: template.name })}
            pending={remove.isPending}
            onConfirm={() => remove.mutate()}
          />
        </div>
      )}
    </form>
  );
}

type DutyRow = { id: string; name: string };
const dutyNames = (rows: DutyRow[] | null | undefined): Record<string, string> => Object.fromEntries((rows ?? []).map((d) => [d.id, d.name]));

function TemplateReadOnly({ template, dutyNames }: { template: TemplateView; dutyNames: Record<string, string> }) {
  const { t } = useTranslation();
  return (
    <article className="max-w-3xl space-y-4">
      <p><Link className="underline" to={paths.templates}>{t("planning.templates.back")}</Link></p>
      <h2 className="text-xl font-semibold">{template.name}</h2>
      <p className="text-muted-foreground">{t(`setup.content_languages.${template.language}`)}</p>
      <p className="text-sm text-muted-foreground">{t("planning.read_only")}</p>
      {(template.items ?? []).length === 0 ? <p>{t("planning.templates.no_items")}</p> : (
        <ol className="space-y-2">
          {(template.items ?? []).map((i, n) => (
            <li key={n} className="rounded-md border border-border p-3">
              <p className="font-medium">{i.title}</p>
              <p className="text-sm text-muted-foreground">
                {t(`planning.item_types.${i.item_type}`)}{i.default_duty_id ? ` · ${dutyNames[i.default_duty_id] ?? ""}` : ""}
              </p>
              {i.default_text && <p className="whitespace-pre-line pt-1">{i.default_text}</p>}
            </li>
          ))}
        </ol>
      )}
    </article>
  );
}

// ItemsEditor edits the items of a template: one card each, reordered with
// buttons that have text, not by dragging (06 §4, P-52).
function ItemsEditor({ form, duties }: { form: UseFormReturn<Values>; duties: DutyRow[] | null }) {
  const { t } = useTranslation();
  const { fields, append, remove, move } = useFieldArray({ control: form.control, name: "items" });
  const items = form.watch("items");
  const [announcement, setAnnouncement] = useState("");
  const errors = form.formState.errors;
  const label = (i: number) => items[i]?.title?.trim() || t("planning.templates.item_n", { n: i + 1 });

  const shift = (i: number, to: number) => {
    const moved = label(i);
    move(i, to);
    setAnnouncement(t("planning.moved", { name: moved, n: to + 1, total: fields.length }));
    focusSoon([`${fields[i].id}-${to < i ? "up" : "down"}`, `${fields[i].id}-${to < i ? "down" : "up"}`]);
  };
  const drop = (i: number) => {
    setAnnouncement(t("planning.removed", { name: label(i) }));
    remove(i);
    focusSoon(["add-item"]);
  };

  return (
    <section aria-labelledby="items-title" className="space-y-4">
      <div>
        <h3 id="items-title" className="text-lg font-semibold">{t("planning.templates.items_title")}</h3>
        <p className="text-muted-foreground">{t("planning.templates.items_intro")}</p>
      </div>
      <p role="status" className="sr-only">{announcement}</p>
      {fields.length === 0 && <p>{t("planning.templates.no_items")}</p>}
      <ol className="space-y-4">
        {fields.map((field, i) => {
          const err = errors.items?.[i];
          const num = t("planning.templates.item_n", { n: i + 1 });
          const takesText = textItemTypes.includes((items[i]?.item_type ?? "other") as ItemType);
          return (
            <li key={field.id}>
              <fieldset className="space-y-3 rounded-md border border-border p-4">
                <legend className="px-1 font-semibold">{num}: {items[i]?.title?.trim()}</legend>
                <div className="grid gap-4 sm:grid-cols-3">
                  <div className="sm:col-span-3">
                    <Field label={t("planning.templates.item_title")} error={err?.title?.message ? t(err.title.message) : undefined}>
                      <Input autoComplete="off" {...form.register(`items.${i}.title`)} />
                    </Field>
                  </div>
                  <Field label={t("planning.templates.item_type")}>
                    <Select {...form.register(`items.${i}.item_type`)}>
                      {itemTypes.map((k) => <option key={k} value={k}>{t(`planning.item_types.${k}`)}</option>)}
                    </Select>
                  </Field>
                  <Field label={t("planning.templates.item_duty")}>
                    <Select {...form.register(`items.${i}.default_duty_id`)}>
                      <option value="">{t("planning.none")}</option>
                      {(duties ?? []).map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </Select>
                  </Field>
                </div>
                {takesText && (
                  <Field label={t("planning.templates.item_text")} hint={t("planning.templates.item_text_hint")} error={err?.default_text?.message ? t(err.default_text.message) : undefined}>
                    <textarea rows={4} className={textarea} {...form.register(`items.${i}.default_text`)} />
                  </Field>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button id={`${field.id}-up`} aria-label={`${t("planning.move_up")} ${num}`} variant="outline" disabled={i === 0} onClick={() => shift(i, i - 1)}>
                    {t("planning.move_up")}
                  </Button>
                  <Button id={`${field.id}-down`} aria-label={`${t("planning.move_down")} ${num}`} variant="outline" disabled={i === fields.length - 1} onClick={() => shift(i, i + 1)}>
                    {t("planning.move_down")}
                  </Button>
                  <Button aria-label={`${t("planning.remove")} ${num}`} variant="outline" onClick={() => drop(i)}>
                    {t("planning.remove")}
                  </Button>
                </div>
              </fieldset>
            </li>
          );
        })}
      </ol>
      <Button id="add-item" variant="outline" disabled={fields.length >= planningLimits.templateItems}
        onClick={() => append({ title: "", item_type: "other", default_duty_id: "", default_text: "" })}>
        {t("planning.templates.add_item")}
      </Button>
    </section>
  );
}
