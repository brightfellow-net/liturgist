// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { contentLanguages, timeZones } from "@/lib/church";
import { languages } from "@/lib/i18n";
import { useFragmentToken } from "@/lib/fragmentToken";
import { setupStatusQuery, translationsQuery } from "@/lib/queries";
import { PublicLayout } from "./PublicLayout";
import { paths } from "./paths";

// Instant feedback only; the server's rules decide (05 §4).
const schema = z.object({
  church: z.object({
    name: z.string().trim().min(1, "common.required").max(120),
    default_ui_language: z.enum(["en", "id"]),
    default_language: z.enum(contentLanguages),
    default_translation_code: z.string().min(1, "common.required"),
    time_zone: z.string().min(1),
    key_display: z.enum(["do", "letter"]),
  }),
  admin: z.object({
    name: z.string().trim().min(1, "common.required").max(120),
    identifier: z.string().trim().min(1, "common.required").max(300),
    password: z.string().min(10, "errors.weak_password.too_short").max(128, "errors.weak_password.too_long"),
  }),
});
type Values = z.infer<typeof schema>;
type FieldName = "church.name" | "church.default_translation_code" | "church.time_zone" | "admin.name" | "admin.identifier" | "admin.password";

export function SetupPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const token = useFragmentToken();
  const status = useQuery({ ...setupStatusQuery, enabled: token !== null });
  const translations = useQuery({ ...translationsQuery, enabled: token !== null });
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      church: { name: "", default_ui_language: i18n.language === "id" ? "id" : "en", default_language: "id",
        default_translation_code: "TB", time_zone: "Asia/Jakarta", key_display: "do" },
      admin: { name: "", identifier: "", password: "" },
    },
  });
  const setup = useMutation({
    mutationFn: (v: Values) => call(api.POST("/setup", { body: { token: token ?? "", ...v } })),
    onSuccess: async () => {
      queryClient.clear();
      await navigate(paths.home, { replace: true });
    },
    onError: (err) => {
      for (const f of fieldErrors(err)) {
        form.setError(f as FieldName, { message: "common.field_invalid" });
      }
    },
  });

  if (token === null) {
    return <PublicLayout title={t("setup.title")}><Alert>{t("setup.incomplete")}</Alert></PublicLayout>;
  }
  if (status.data?.set_up) {
    return (
      <PublicLayout title={t("setup.title")}>
        <Alert>{t("setup.already")}</Alert>
        <Link className="underline" to={paths.login}>{t("common.go_to_login")}</Link>
      </PublicLayout>
    );
  }

  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  return (
    <PublicLayout title={t("setup.title")}>
      <p>{t("setup.intro")}</p>
      <form noValidate className="space-y-6" onSubmit={form.handleSubmit((v) => setup.mutate(v))}>
        <ErrorAlert error={setup.error ?? status.error ?? translations.error} />
        <fieldset className="space-y-4">
          <legend className="text-lg font-semibold">{t("setup.church_section")}</legend>
          <Field label={t("setup.church_name")} error={msg(e.church?.name?.message)}>
            <Input {...form.register("church.name")} />
          </Field>
          <Field label={t("setup.ui_language")}>
            <Select {...form.register("church.default_ui_language")}>
              {languages.map((l) => <option key={l} value={l}>{t(`language.${l}`)}</option>)}
            </Select>
          </Field>
          <Field label={t("setup.content_language")}>
            <Select {...form.register("church.default_language")}>
              {contentLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
            </Select>
          </Field>
          <Field label={t("setup.translation")} error={msg(e.church?.default_translation_code?.message)}>
            <Select {...form.register("church.default_translation_code")}>
              {(translations.data ?? [{ code: "TB", name: "TB", language: "id" }]).map((tr) => (
                <option key={tr.code} value={tr.code}>{tr.code === tr.name ? tr.code : `${tr.code} — ${tr.name}`}</option>
              ))}
            </Select>
          </Field>
          <Field label={t("setup.time_zone")} error={msg(e.church?.time_zone?.message)}>
            <Select {...form.register("church.time_zone")}>
              {timeZones.map((z) => <option key={z} value={z}>{t(`setup.time_zones.${z}`)}</option>)}
            </Select>
          </Field>
          <fieldset className="space-y-2">
            <legend className="font-medium">{t("setup.key_display")}</legend>
            {(["do", "letter"] as const).map((k) => (
              <label key={k} className="flex min-h-12 items-center gap-3">
                <input type="radio" value={k} className="size-5" {...form.register("church.key_display")} />
                {t(`setup.key_${k}`)}
              </label>
            ))}
          </fieldset>
        </fieldset>
        <fieldset className="space-y-4">
          <legend className="text-lg font-semibold">{t("setup.admin_section")}</legend>
          <Field label={t("setup.admin_name")} error={msg(e.admin?.name?.message)}>
            <Input autoComplete="name" {...form.register("admin.name")} />
          </Field>
          <Field label={t("setup.admin_identifier")} error={msg(e.admin?.identifier?.message)}>
            <Input autoComplete="username" {...form.register("admin.identifier")} />
          </Field>
          <Field label={t("setup.admin_password")} hint={t("common.password_hint")} error={msg(e.admin?.password?.message)}>
            <PasswordInput autoComplete="new-password" {...form.register("admin.password")} />
          </Field>
        </fieldset>
        <Button type="submit" disabled={setup.isPending} className="w-full">{t("setup.submit")}</Button>
      </form>
    </PublicLayout>
  );
}
