// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useOutletContext } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { languages } from "@/lib/i18n";
import { meQuery } from "@/lib/queries";

const textSizes = ["normal", "large", "larger"] as const;

const detailsSchema = z.object({
  name: z.string().trim().min(1, "common.required").max(120),
  text_size: z.enum(textSizes),
  ui_language: z.enum(["", ...languages]), // "" = church default
});
type Details = z.infer<typeof detailsSchema>;

const passwordSchema = z.object({
  current_password: z.string().min(1, "common.required").max(1024),
  new_password: z.string().min(10, "errors.weak_password.too_short").max(128, "errors.weak_password.too_long"),
});
type Passwords = z.infer<typeof passwordSchema>;

// ProfilePage changes the user's own name, preferences and password, and
// ends their other sessions (05 §2).
export function ProfilePage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  return (
    <div className="max-w-xl space-y-10">
      <h1 className="text-2xl font-semibold">{t("profile.title")}</h1>
      <DetailsForm me={me} />
      <PasswordForm />
      <section className="space-y-3">
        <h2 className="text-lg font-semibold">{t("profile.devices_title")}</h2>
        <EndOtherSessions />
      </section>
    </div>
  );
}

function DetailsForm({ me }: { me: Me }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const prefs = me.user.preferences;
  const form = useForm<Details>({
    resolver: zodResolver(detailsSchema),
    defaultValues: {
      name: me.user.name,
      text_size: textSizes.find((s) => s === prefs.text_size) ?? "normal",
      ui_language: languages.find((l) => l === prefs.ui_language) ?? "",
    },
  });
  const save = useMutation({
    mutationFn: (v: Details) =>
      call(api.PATCH("/me", {
        body: { name: v.name, preferences: { text_size: v.text_size, ui_language: v.ui_language || null } },
      })),
    onSuccess: (user) => {
      // AppLayout applies the new language and text size from the cache.
      queryClient.setQueryData(meQuery.queryKey, (old) => old && { ...old, user });
    },
    onError: (err) => {
      if (fieldErrors(err).includes("name")) form.setError("name", { message: "common.field_invalid" });
    },
  });
  const nameError = form.formState.errors.name?.message;
  const churchDefault = me.church ? t(`language.${me.church.default_ui_language}`) : t("language.en");
  return (
    <form noValidate className="space-y-4" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <h2 className="text-lg font-semibold">{t("profile.details")}</h2>
      <ErrorAlert error={save.error} />
      {save.isSuccess && !form.formState.isDirty && <Alert>{t("profile.saved")}</Alert>}
      <Field label={t("profile.name")} error={nameError && t(nameError)}>
        <Input autoComplete="name" {...form.register("name")} />
      </Field>
      <fieldset className="space-y-2">
        <legend className="font-medium">{t("profile.text_size")}</legend>
        {textSizes.map((s) => (
          <label key={s} className="flex min-h-12 items-center gap-3">
            <input type="radio" value={s} className="size-5" {...form.register("text_size")} />
            {t(`profile.text_sizes.${s}`)}
          </label>
        ))}
      </fieldset>
      <Field label={t("profile.ui_language")}>
        <Select {...form.register("ui_language")}>
          <option value="">{t("profile.church_default", { language: churchDefault })}</option>
          {languages.map((l) => <option key={l} value={l}>{t(`language.${l}`)}</option>)}
        </Select>
      </Field>
      <Button type="submit" disabled={save.isPending}>{t("profile.save")}</Button>
    </form>
  );
}

function PasswordForm() {
  const { t } = useTranslation();
  const form = useForm<Passwords>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { current_password: "", new_password: "" },
  });
  const change = useMutation({
    mutationFn: (v: Passwords) => call(api.POST("/me/password", { body: v })),
    onSuccess: () => form.reset(),
    onError: (err) => {
      for (const f of fieldErrors(err)) form.setError(f as keyof Passwords, { message: "common.field_invalid" });
    },
  });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  return (
    <form noValidate className="space-y-4" onSubmit={form.handleSubmit((v) => change.mutate(v))}>
      <h2 className="text-lg font-semibold">{t("profile.password_title")}</h2>
      <ErrorAlert error={change.error} />
      {change.isSuccess && <Alert>{t("profile.password_changed")}</Alert>}
      <Field label={t("profile.current_password")} error={msg(e.current_password?.message)}>
        <PasswordInput autoComplete="current-password" {...form.register("current_password")} />
      </Field>
      <Field label={t("profile.new_password")} hint={t("common.password_hint")} error={msg(e.new_password?.message)}>
        <PasswordInput autoComplete="new-password" {...form.register("new_password")} />
      </Field>
      <Button type="submit" disabled={change.isPending}>{t("profile.change_password")}</Button>
    </form>
  );
}

function EndOtherSessions() {
  const { t } = useTranslation();
  const end = useMutation({ mutationFn: () => call(api.POST("/me/sessions/end-others")) });
  return (
    <>
      <p>{t("profile.devices_text")}</p>
      <ErrorAlert error={end.error} />
      {end.isSuccess && <Alert>{t("profile.others_ended")}</Alert>}
      <Button variant="outline" disabled={end.isPending} onClick={() => end.mutate()}>{t("profile.end_others")}</Button>
    </>
  );
}
