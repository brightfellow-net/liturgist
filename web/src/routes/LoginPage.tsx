// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";
import { setupStatusQuery } from "@/lib/queries";
import { PublicLayout } from "./PublicLayout";
import { paths, safeNext } from "./paths";

const schema = z.object({
  identifier: z.string().trim().min(1, "common.required").max(300),
  password: z.string().min(1, "common.required").max(1024),
});
type Values = z.infer<typeof schema>;

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const queryClient = useQueryClient();
  const [forgot, setForgot] = useState(false);
  const status = useQuery(setupStatusQuery);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { identifier: "", password: "" } });
  const login = useMutation({
    mutationFn: (v: Values) => call(api.POST("/auth/login", { body: v })),
    onSuccess: async () => {
      queryClient.clear();
      await navigate(safeNext(params.get("next")), { replace: true });
    },
  });

  if (status.data && !status.data.set_up) return <Navigate to={paths.setup} replace />;
  const err = form.formState.errors;
  return (
    <PublicLayout title={t("login.title")}>
      <form noValidate className="space-y-4" onSubmit={form.handleSubmit((v) => login.mutate(v))}>
        <ErrorAlert error={login.error} />
        <Field label={t("login.identifier")} error={err.identifier && t(err.identifier.message ?? "common.field_invalid")}>
          <Input autoComplete="username" {...form.register("identifier")} />
        </Field>
        <Field label={t("login.password")} error={err.password && t(err.password.message ?? "common.field_invalid")}>
          <PasswordInput autoComplete="current-password" {...form.register("password")} />
        </Field>
        <Button type="submit" disabled={login.isPending} className="w-full">{t("login.submit")}</Button>
      </form>
      <div className="space-y-2">
        <Button variant="link" aria-expanded={forgot} onClick={() => setForgot(!forgot)}>{t("login.forgot")}</Button>
        {forgot && <Alert>{t("login.forgot_text")}</Alert>}
      </div>
    </PublicLayout>
  );
}
