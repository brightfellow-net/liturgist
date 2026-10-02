// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { useFragmentToken } from "@/lib/fragmentToken";
import { LinkProblem } from "./LinkProblem";
import { PublicLayout } from "./PublicLayout";
import { paths } from "./paths";

const schema = z.object({
  new_password: z.string().min(10, "errors.weak_password.too_short").max(128, "errors.weak_password.too_long"),
});
type Values = z.infer<typeof schema>;

// ResetPage sets a new password from a reset link, then logs the user in
// (03 §9, 05 §2).
export function ResetPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const token = useFragmentToken();
  // Set on success, so clearing the cache does not ask about the used link again.
  const [finished, setFinished] = useState(false);
  const info = useQuery({
    queryKey: ["reset-inspect"],
    queryFn: () => call(api.POST("/auth/reset/inspect", { body: { token: token ?? "" } })),
    enabled: token !== null && !finished,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    meta: { public: true },
  });
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { new_password: "" } });
  const reset = useMutation({
    mutationFn: (v: Values) => call(api.POST("/auth/reset", { body: { token: token ?? "", ...v } })),
    onSuccess: async () => {
      setFinished(true);
      queryClient.clear();
      await navigate(paths.home, { replace: true });
    },
    onError: (err) => {
      if (fieldErrors(err).includes("new_password")) form.setError("new_password", { message: "common.field_invalid" });
    },
    meta: { public: true },
  });

  const title = t("reset.title");
  if (token === null) return <PublicLayout title={title}><LinkProblem /></PublicLayout>;
  if (info.error) return <PublicLayout title={title}><LinkProblem error={info.error} /></PublicLayout>;
  if (!info.data) return <PublicLayout title={title}><p role="status">{t("app.loading")}</p></PublicLayout>;

  const by = info.data.created_by_name;
  const err = form.formState.errors.new_password?.message;
  return (
    <PublicLayout title={title}>
      <p>{t("reset.for", { name: info.data.user_name })}</p>
      <p>{by ? t("reset.created_by", { name: by }) : t("reset.created_by_server")}</p>
      <form noValidate className="space-y-4" onSubmit={form.handleSubmit((v) => reset.mutate(v))}>
        <ErrorAlert error={reset.error} />
        <Field label={t("reset.new_password")} hint={t("common.password_hint")} error={err && t(err)}>
          <PasswordInput autoComplete="new-password" {...form.register("new_password")} />
        </Field>
        <Button type="submit" disabled={reset.isPending} className="w-full">{t("reset.submit")}</Button>
      </form>
    </PublicLayout>
  );
}
