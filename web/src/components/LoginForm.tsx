// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";

const schema = z.object({
  identifier: z.string().trim().min(1, "common.required").max(300),
  password: z.string().min(1, "common.required").max(1024),
});
type Values = z.infer<typeof schema>;

// LoginForm logs in with email or phone and password. It is used by the
// login page and by the invite page ("log in to accept").
export function LoginForm({ identifier = "", submitLabel, onSuccess }: {
  identifier?: string;
  submitLabel: string;
  onSuccess: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { identifier, password: "" } });
  const login = useMutation({
    mutationFn: (v: Values) => call(api.POST("/auth/login", { body: v })),
    onSuccess,
    meta: { public: true },
  });
  const err = form.formState.errors;
  return (
    <form noValidate className="space-y-4" onSubmit={form.handleSubmit((v) => login.mutate(v))}>
      <ErrorAlert error={login.error} />
      <Field label={t("login.identifier")} error={err.identifier && t(err.identifier.message ?? "common.field_invalid")}>
        <Input autoComplete="username" {...form.register("identifier")} />
      </Field>
      <Field label={t("login.password")} error={err.password && t(err.password.message ?? "common.field_invalid")}>
        <PasswordInput autoComplete="current-password" {...form.register("password")} />
      </Field>
      <Button type="submit" disabled={login.isPending} className="w-full">{submitLabel}</Button>
    </form>
  );
}
