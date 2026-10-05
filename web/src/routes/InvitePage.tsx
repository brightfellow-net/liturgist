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
import { Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { LoginForm } from "@/components/LoginForm";
import { PasswordInput } from "@/components/PasswordInput";
import { api, call } from "@/lib/api";
import { clearOffline } from "@/lib/offline";
import { fieldErrors, isCode } from "@/lib/errors";
import { useFragmentToken } from "@/lib/fragmentToken";
import { meQuery } from "@/lib/queries";
import { LinkProblem } from "./LinkProblem";
import { PublicLayout } from "./PublicLayout";
import { paths } from "./paths";

// Instant feedback only; the server's rules decide (05 §4).
const schema = z
  .object({
    name: z.string().trim().min(1, "common.required").max(120),
    email: z.string().trim().max(254),
    phone: z.string().trim().max(32),
    password: z.string().min(10, "errors.weak_password.too_short").max(128, "errors.weak_password.too_long"),
  })
  .refine((v) => v.email !== "" || v.phone !== "", { path: ["email"], message: "invite.need_identifier" });
type Values = z.infer<typeof schema>;
type FieldName = "name" | "email" | "phone" | "password";

// Step is where the invitee is: choosing how to join (new account or the
// logged-in account), logging in, or confirming (03 §7: "You are joining
// {church} as {name}" before the final step).
type Step = "choose" | "login" | "confirm-new" | "confirm-existing";

// InvitePage accepts an invite: as a new account, by logging in, or with
// the account already logged in (05 §2).
export function InvitePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const token = useFragmentToken();
  // Set on success, so clearing the cache does not ask about the used link again.
  const [finished, setFinished] = useState(false);
  const [step, setStep] = useState<Step>("choose");
  const [taken, setTaken] = useState(false);

  // Public queries: a 401 here only means "not logged in".
  const invite = useQuery({
    queryKey: ["invite-inspect"],
    queryFn: () => call(api.POST("/invites/inspect", { body: { token: token ?? "" } })),
    enabled: token !== null && !finished,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    meta: { public: true },
  });
  const me = useQuery({ ...meQuery, enabled: token !== null && !finished, retry: false, meta: { public: true } });

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    values: invite.data && {
      name: invite.data.invitee_name, email: invite.data.email ?? "", phone: invite.data.phone ?? "", password: "",
    },
    resetOptions: { keepDirtyValues: true },
  });

  const done = async () => {
    setFinished(true);
    queryClient.clear();
    await navigate(paths.home, { replace: true });
  };
  const accept = useMutation({
    mutationFn: (v: Values) =>
      call(api.POST("/invites/accept", {
        body: { token: token ?? "", name: v.name, password: v.password, email: v.email || undefined, phone: v.phone || undefined },
      })),
    onSuccess: done,
    onError: (err) => {
      if (isCode(err, "identifier_taken")) {
        setTaken(true);
        setStep("login");
        return;
      }
      const fields = fieldErrors(err);
      for (const f of fields) form.setError(f as FieldName, { message: "common.field_invalid" });
      if (fields.length > 0 || isCode(err, "weak_password")) setStep("choose");
    },
    meta: { public: true },
  });
  const acceptExisting = useMutation({
    mutationFn: () => call(api.POST("/invites/accept-existing", { body: { token: token ?? "" } })),
    onSuccess: done,
    meta: { public: true },
  });
  const logOut = useMutation({
    mutationFn: async () => {
      await clearOffline();
      return call(api.POST("/auth/logout"));
    },
    onSettled: () => queryClient.resetQueries({ queryKey: meQuery.queryKey }),
    meta: { public: true },
  });

  const title = t("invite.title");
  if (token === null) return <PublicLayout title={title}><LinkProblem /></PublicLayout>;
  if (invite.error) return <PublicLayout title={title}><LinkProblem error={invite.error} /></PublicLayout>;
  if (!invite.data || me.isPending) {
    return <PublicLayout title={title}><p role="status">{t("app.loading")}</p></PublicLayout>;
  }

  const info = invite.data;
  const user = me.data?.user;
  const intro = <p>{t("invite.invited", { church: info.church_name, name: info.invitee_name })}</p>;

  if (step === "confirm-new" || step === "confirm-existing") {
    const isNew = step === "confirm-new";
    const pending = isNew ? accept.isPending : acceptExisting.isPending;
    return (
      <PublicLayout title={title}>
        <ErrorAlert error={isNew ? accept.error : acceptExisting.error} />
        <p className="text-lg">{t("invite.confirm", { church: info.church_name, name: isNew ? form.getValues("name") : user?.name })}</p>
        <div className="flex flex-wrap gap-2">
          <Button disabled={pending} onClick={() => (isNew ? accept.mutate(form.getValues()) : acceptExisting.mutate())}>
            {t("invite.join")}
          </Button>
          <Button variant="outline" onClick={() => setStep("choose")}>{t("common.back")}</Button>
        </div>
      </PublicLayout>
    );
  }

  if (user) {
    return (
      <PublicLayout title={title}>
        {intro}
        <ErrorAlert error={logOut.error} />
        <p>{t("invite.logged_in_as", { name: user.name })}</p>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => setStep("confirm-existing")}>{t("invite.join_with_account")}</Button>
          <Button variant="outline" disabled={logOut.isPending} onClick={() => logOut.mutate()}>{t("invite.use_other_account")}</Button>
        </div>
      </PublicLayout>
    );
  }

  if (step === "login" || info.owner_exists) {
    return (
      <PublicLayout title={title}>
        {intro}
        <Alert>{taken ? t("invite.identifier_taken") : t("invite.has_account")}</Alert>
        <LoginForm
          identifier={info.email ?? info.phone ?? ""}
          submitLabel={t("invite.log_in")}
          onSuccess={async () => {
            await me.refetch();
            setStep("confirm-existing");
          }}
        />
        {!info.owner_exists && (
          <Button variant="link" onClick={() => setStep("choose")}>{t("common.back")}</Button>
        )}
      </PublicLayout>
    );
  }

  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  return (
    <PublicLayout title={title}>
      {intro}
      <form noValidate className="space-y-4" onSubmit={form.handleSubmit(() => setStep("confirm-new"))}>
        <h2 className="text-lg font-semibold">{t("invite.new_account")}</h2>
        <Field label={t("invite.name")} error={msg(e.name?.message)}>
          <Input autoComplete="name" {...form.register("name")} />
        </Field>
        <Field label={t("invite.email")} hint={t("invite.contact_hint")} error={msg(e.email?.message)}>
          <Input type="email" autoComplete="email" {...form.register("email")} />
        </Field>
        <Field label={t("invite.phone")} error={msg(e.phone?.message)}>
          <Input type="tel" autoComplete="tel" {...form.register("phone")} />
        </Field>
        <Field label={t("invite.password")} hint={t("common.password_hint")} error={msg(e.password?.message)}>
          <PasswordInput autoComplete="new-password" {...form.register("password")} />
        </Field>
        <Button type="submit" className="w-full">{t("invite.continue")}</Button>
      </form>
      <Button variant="link" onClick={() => setStep("login")}>{t("invite.already_have_account")}</Button>
    </PublicLayout>
  );
}
