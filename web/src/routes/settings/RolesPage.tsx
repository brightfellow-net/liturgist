// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { RoleView, ScopeInfo } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { meQuery, membersQuery, rolesQuery, scopesQuery } from "@/lib/queries";

// RolesPage lists the church's roles with how many members hold each, and
// creates, edits and deletes them; buttons follow each role's "actions"
// (05 §2, 03 §8).
export function RolesPage() {
  const { t } = useTranslation();
  const roles = useQuery(rolesQuery);
  const scopes = useQuery(scopesQuery);
  const [creating, setCreating] = useState(false);
  const error = roles.error ?? scopes.error;
  if (error) return <ErrorAlert error={error} onRetry={() => void Promise.all([roles.refetch(), scopes.refetch()])} />;
  if (roles.data === undefined || scopes.data === undefined) return <p role="status">{t("app.loading")}</p>;
  const scopeList = scopes.data ?? [];
  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">{t("roles.title")}</h2>
      <p>{t("roles.intro")}</p>
      {creating ? <RoleForm scopes={scopeList} onClose={() => setCreating(false)} /> : (
        <Button onClick={() => setCreating(true)}>{t("roles.new")}</Button>
      )}
      <ul className="space-y-4">
        {(roles.data ?? []).map((r) => <RoleItem key={r.id} role={r} scopes={scopeList} />)}
      </ul>
    </section>
  );
}

function RoleItem({ role, scopes }: { role: RoleView; scopes: ScopeInfo[] }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/roles/{id}", { params: { path: { id: role.id } } })),
    onSuccess: () => refreshAfterRoleChange(queryClient),
  });
  if (editing) return <li><RoleForm role={role} scopes={scopes} onClose={() => setEditing(false)} /></li>;
  const held = new Set(role.scopes ?? []);
  return (
    <li className="space-y-3 rounded-md border border-border p-4">
      <div>
        <p className="font-semibold">{role.name}</p>
        {role.description && <p>{role.description}</p>}
        <p className="text-sm text-muted-foreground">{t("roles.member_count", { count: role.member_count })}</p>
      </div>
      <ul className="list-disc space-y-1 pl-6 text-sm">
        {scopes.filter((s) => held.has(s.scope)).map((s) => <li key={s.scope}>{s.description}</li>)}
        {held.size === 0 && <li>{t("roles.no_scopes")}</li>}
      </ul>
      <ErrorAlert error={remove.error} />
      {(role.actions.edit || role.actions.delete) && (
        <div className="flex flex-wrap gap-2">
          {role.actions.edit && <Button variant="outline" onClick={() => setEditing(true)}>{t("roles.edit")}</Button>}
          {role.actions.delete && (
            <ConfirmButton
              label={t("roles.delete")}
              question={t("roles.delete_question", { name: role.name, count: role.member_count })}
              confirmLabel={t("roles.delete_confirm", { name: role.name })}
              pending={remove.isPending}
              onConfirm={() => remove.mutate()}
            />
          )}
        </div>
      )}
    </li>
  );
}

// A role change can change the member list, the roles held and the
// current member's own scopes (menu).
async function refreshAfterRoleChange(queryClient: ReturnType<typeof useQueryClient>) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: rolesQuery.queryKey }),
    queryClient.invalidateQueries({ queryKey: membersQuery.queryKey }),
    queryClient.invalidateQueries({ queryKey: meQuery.queryKey }),
  ]);
}

const schema = z.object({
  name: z.string().trim().min(1, "common.required").max(60),
  description: z.string().trim().max(200),
  scopes: z.array(z.string()),
});
type Values = z.infer<typeof schema>;

function RoleForm({ role, scopes, onClose }: { role?: RoleView; scopes: ScopeInfo[]; onClose: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: role?.name ?? "", description: role?.description ?? "", scopes: role?.scopes ?? [] },
  });
  const save = useMutation({
    mutationFn: (v: Values) =>
      role
        ? call(api.PATCH("/roles/{id}", { params: { path: { id: role.id } }, body: v }))
        : call(api.POST("/roles", { body: v })),
    onSuccess: async () => {
      await refreshAfterRoleChange(queryClient);
      onClose();
    },
    onError: (err) => {
      for (const f of fieldErrors(err)) {
        if (f === "name" || f === "description") form.setError(f, { message: "common.field_invalid" });
      }
    },
  });
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  return (
    <form noValidate className="max-w-xl space-y-4 rounded-md border border-border p-4" onSubmit={form.handleSubmit((v) => save.mutate(v))}>
      <h3 className="text-lg font-semibold">{role ? t("roles.edit_title", { name: role.name }) : t("roles.new")}</h3>
      <ErrorAlert error={save.error} />
      <Field label={t("roles.name")} error={msg(e.name?.message)}>
        <Input autoComplete="off" {...form.register("name")} />
      </Field>
      <Field label={t("roles.description")} error={msg(e.description?.message)}>
        <Input autoComplete="off" {...form.register("description")} />
      </Field>
      <fieldset className="space-y-1">
        <legend className="font-medium">{t("roles.scopes")}</legend>
        {scopes.map((s) => (
          <label key={s.scope} className="flex min-h-12 items-center gap-3">
            <input type="checkbox" value={s.scope} className="size-5 shrink-0" {...form.register("scopes")} />
            {s.description}
          </label>
        ))}
      </fieldset>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>{t("profile.save")}</Button>
        <Button variant="outline" onClick={onClose}>{t("common.cancel")}</Button>
      </div>
    </form>
  );
}
