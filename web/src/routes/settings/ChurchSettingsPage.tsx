// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { ChurchView } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { contentLanguages, timeZones } from "@/lib/church";
import { fieldErrors } from "@/lib/errors";
import { languages } from "@/lib/i18n";
import { churchQuery, meQuery, translationsQuery } from "@/lib/queries";
import { ChurchLogoSection } from "./ChurchLogoSection";

// Instant feedback only; the server's rules decide (05 §4).
const schema = z.object({
  name: z.string().trim().min(1, "common.required").max(120),
  default_ui_language: z.enum(languages),
  default_language: z.enum(contentLanguages),
  default_translation_code: z.string().min(1, "common.required"),
  time_zone: z.string().min(1, "common.required"),
  key_display: z.enum(["do", "letter"]),
  feedback_url: z.string().trim().refine((v) => v === "" || v.startsWith("https://"), "church.feedback_url_invalid"),
  privacy_contact: z.string().trim().max(500),
  show_credits: z.boolean(),
  licence_footer: z.string().trim().max(200, "church.licence_footer_long"),
  print_paper: z.enum(["a4", "f4"]),
  print_lyrics: z.enum(["full", "first_lines"]),
  print_readings: z.boolean(),
  print_assignments: z.boolean(),
  print_keys: z.boolean(),
  print_notes: z.boolean(),
  print_size: z.enum(["normal", "large"]),
});
type Values = z.infer<typeof schema>;
type FieldName = keyof Values;

function valuesOf(c: ChurchView): Values {
  return {
    name: c.name, default_ui_language: c.default_ui_language, default_language: c.default_language,
    default_translation_code: c.default_translation_code, time_zone: c.time_zone, key_display: c.key_display,
    feedback_url: c.feedback_url ?? "", privacy_contact: c.privacy_contact ?? "",
    show_credits: c.show_credits, licence_footer: c.licence_footer,
    print_paper: c.print.paper, print_lyrics: c.print.lyrics, print_readings: c.print.readings,
    print_assignments: c.print.assignments, print_keys: c.print.keys, print_notes: c.print.notes, print_size: c.print.size,
  };
}

// The print settings go out as one complete object that replaces the old one (13 §6).
function bodyOf(v: Values) {
  const { print_paper, print_lyrics, print_readings, print_assignments, print_keys, print_notes, print_size, ...rest } = v;
  return {
    ...rest,
    print: { paper: print_paper, lyrics: print_lyrics, readings: print_readings, assignments: print_assignments, keys: print_keys, notes: print_notes, size: print_size },
  };
}

// ChurchSettingsPage shows the church's settings to every member; members
// whose church "actions.edit" is true can change them (05 §2).
export function ChurchSettingsPage() {
  const { t } = useTranslation();
  const church = useQuery(churchQuery);
  if (church.error) return <ErrorAlert error={church.error} onRetry={() => void church.refetch()} />;
  if (!church.data) return <p role="status">{t("app.loading")}</p>;
  return (
    <div className="space-y-8">
      {church.data.actions.edit && <ChurchLogoSection church={church.data} />}
      <ChurchForm church={church.data} />
    </div>
  );
}

function ChurchForm({ church }: { church: ChurchView }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const translations = useQuery(translationsQuery);
  const form = useForm<Values>({ resolver: zodResolver(schema), values: valuesOf(church) });
  const save = useMutation({
    mutationFn: (v: Values) => call(api.PATCH("/church", { body: bodyOf(v) })),
    onSuccess: async (updated) => {
      queryClient.setQueryData(churchQuery.queryKey, updated);
      await queryClient.invalidateQueries({ queryKey: meQuery.queryKey }); // name and default language
    },
    onError: (err) => {
      for (const f of fieldErrors(err)) form.setError(f as FieldName, { message: "common.field_invalid" });
    },
  });
  const canEdit = church.actions.edit;
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  const zones = timeZones.includes(church.time_zone as (typeof timeZones)[number]) ? timeZones : [...timeZones, church.time_zone];
  return (
    <form noValidate className="max-w-xl space-y-4" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      {!canEdit && <Alert>{t("church.read_only")}</Alert>}
      <ErrorAlert error={save.error} />
      {save.isSuccess && !form.formState.isDirty && <Alert>{t("profile.saved")}</Alert>}
      <fieldset disabled={!canEdit} className="space-y-4">
        <legend className="sr-only">{t("settings.church")}</legend>
        <Field label={t("setup.church_name")} error={msg(e.name?.message)}>
          <Input {...form.register("name")} />
        </Field>
        <Field label={t("setup.ui_language")}>
          <Select {...form.register("default_ui_language")}>
            {languages.map((l) => <option key={l} value={l}>{t(`language.${l}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("setup.content_language")}>
          <Select {...form.register("default_language")}>
            {contentLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("setup.translation")} error={msg(e.default_translation_code?.message)}>
          <Select {...form.register("default_translation_code")}>
            {(translations.data ?? [{ code: church.default_translation_code, name: church.default_translation_code, language: "" }]).map((tr) => (
              <option key={tr.code} value={tr.code}>{tr.code === tr.name ? tr.code : `${tr.code} — ${tr.name}`}</option>
            ))}
          </Select>
        </Field>
        <Field label={t("setup.time_zone")} error={msg(e.time_zone?.message)}>
          <Select {...form.register("time_zone")}>
            {zones.map((z) => <option key={z} value={z}>{t(`setup.time_zones.${z}`, { defaultValue: z })}</option>)}
          </Select>
        </Field>
        <fieldset className="space-y-2">
          <legend className="font-medium">{t("setup.key_display")}</legend>
          {(["do", "letter"] as const).map((k) => (
            <label key={k} className="flex min-h-12 items-center gap-3">
              <input type="radio" value={k} className="size-5" {...form.register("key_display")} />
              {t(`setup.key_${k}`)}
            </label>
          ))}
        </fieldset>
        <Field label={t("church.feedback_url")} hint={t("church.feedback_url_hint")} error={msg(e.feedback_url?.message)}>
          <Input type="url" inputMode="url" {...form.register("feedback_url")} />
        </Field>
        <Field label={t("church.privacy_contact")} hint={t("church.privacy_contact_hint")} error={msg(e.privacy_contact?.message)}>
          <Input {...form.register("privacy_contact")} />
        </Field>
        <fieldset className="space-y-4">
          <legend className="text-lg font-semibold">{t("church.printing")}</legend>
          <label className="flex min-h-12 items-center gap-3">
            <input type="checkbox" className="size-5" {...form.register("show_credits")} />
            {t("church.show_credits")}
          </label>
          <Field label={t("church.licence_footer")} hint={t("church.licence_footer_hint")} error={msg(e.licence_footer?.message)}>
            <Input {...form.register("licence_footer")} />
          </Field>
          <Field label={t("print.paper")}>
            <Select {...form.register("print_paper")}>
              {(["a4", "f4"] as const).map((v) => <option key={v} value={v}>{t(`print.paper_${v}`)}</option>)}
            </Select>
          </Field>
          <Field label={t("print.lyrics")}>
            <Select {...form.register("print_lyrics")}>
              {(["full", "first_lines"] as const).map((v) => <option key={v} value={v}>{t(`print.lyrics_${v}`)}</option>)}
            </Select>
          </Field>
          {(["readings", "assignments", "keys", "notes"] as const).map((k) => (
            <label key={k} className="flex min-h-12 items-center gap-3">
              <input type="checkbox" className="size-5" {...form.register(`print_${k}`)} />
              {t(`print.${k}`)}
            </label>
          ))}
          <Field label={t("print.size")}>
            <Select {...form.register("print_size")}>
              {(["normal", "large"] as const).map((v) => <option key={v} value={v}>{t(`print.size_${v}`)}</option>)}
            </Select>
          </Field>
        </fieldset>
        {canEdit && <Button type="submit" disabled={save.isPending}>{t("profile.save")}</Button>}
      </fieldset>
    </form>
  );
}
