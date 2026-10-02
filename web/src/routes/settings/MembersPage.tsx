// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useOutletContext } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { InviteView, Me, MemberView, RoleView } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { ShareLink } from "@/components/ShareLink";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { formatDateTime } from "@/lib/format";
import { invitesQuery, membersQuery, rolesQuery, scopesQuery } from "@/lib/queries";
import { hasScope } from "@/lib/scopes";

// MembersPage lists the church's members and open invites. Every button
// follows the item's "actions"; the invite section is shown to members
// holding members.manage (05 §2).
export function MembersPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const members = useQuery({ ...membersQuery, enabled: hasScope(me, "members.view") });
  // For readable "permissions you don't have" errors (ErrorAlert).
  useQuery({ ...scopesQuery, enabled: hasScope(me, "members.manage") || hasScope(me, "roles.manage") });
  const usage = members.data?.usage.team_members;
  return (
    <div className="space-y-10">
      {hasScope(me, "members.view") && (
        <section className="space-y-4">
          <h2 className="text-xl font-semibold">{t("members.title")}</h2>
          {usage && usage.max !== null && <p>{t("members.usage", { used: usage.used, max: usage.max })}</p>}
          {members.error && <ErrorAlert error={members.error} onRetry={() => void members.refetch()} />}
          {!members.data && !members.error && <p role="status">{t("app.loading")}</p>}
          <ul className="space-y-4">
            {(members.data?.members ?? []).map((m) => <MemberItem key={m.id} me={me} member={m} />)}
          </ul>
        </section>
      )}
      {hasScope(me, "members.manage") && <Invites me={me} />}
    </div>
  );
}

function when(me: Me, iso: string, language: string) {
  return formatDateTime(iso, me.church?.time_zone ?? "UTC", language);
}

function MemberItem({ me, member }: { me: Me; member: MemberView }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [link, setLink] = useState<{ link: string; expires_at: string } | null>(null);
  const a = member.actions;
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/members/{membershipId}", { params: { path: { membershipId: member.id } } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: membersQuery.queryKey }),
  });
  const reset = useMutation({
    mutationFn: () => call(api.POST("/members/{membershipId}/password-reset", { params: { path: { membershipId: member.id } } })),
    onSuccess: async (l) => {
      setLink(l);
      await queryClient.invalidateQueries({ queryKey: membersQuery.queryKey });
    },
  });
  const roles = (member.roles ?? []).map((r) => r.name);
  const last = member.last_reset;
  return (
    <li className="space-y-3 rounded-md border border-border p-4">
      <div>
        <p className="font-semibold">{member.name}</p>
        <p className="text-sm text-muted-foreground">{[member.email, member.phone].filter(Boolean).join(" · ")}</p>
        <p className="text-sm">{roles.length > 0 ? roles.join(", ") : t("members.team_member")}</p>
        {last && (
          <p className="text-sm text-muted-foreground">
            {last.used_at
              ? t("members.reset_used", { when: when(me, last.used_at, i18n.language) })
              : t("members.reset_open", { name: last.created_by_name ?? t("members.server_admin"), when: when(me, last.expires_at, i18n.language) })}
          </p>
        )}
      </div>
      <ErrorAlert error={remove.error ?? reset.error} />
      {link && (
        <ShareLink
          label={t("members.reset_link", { name: member.name })}
          link={link.link}
          message={t("members.reset_message", { name: member.name, link: link.link })}
          expires={t("share.expires", { when: when(me, link.expires_at, i18n.language) })}
        />
      )}
      {editing && <RolesEditor member={member} onDone={() => setEditing(false)} />}
      {(a.edit_roles || a.create_reset_link || a.remove) && !editing && (
        <div className="flex flex-wrap gap-2">
          {a.edit_roles && <Button variant="outline" onClick={() => setEditing(true)}>{t("members.edit_roles")}</Button>}
          {a.create_reset_link && (
            <Button variant="outline" disabled={reset.isPending} onClick={() => reset.mutate()}>{t("members.create_reset_link")}</Button>
          )}
          {a.remove && (
            <ConfirmButton
              label={t("members.remove")}
              question={t("members.remove_question", { name: member.name })}
              confirmLabel={t("members.remove_confirm", { name: member.name })}
              pending={remove.isPending}
              onConfirm={() => remove.mutate()}
            />
          )}
        </div>
      )}
    </li>
  );
}

// RoleChoices are checkboxes for the church's roles.
function RoleChoices({ roles, chosen, onChange }: { roles: RoleView[]; chosen: string[]; onChange: (ids: string[]) => void }) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-1">
      <legend className="font-medium">{t("members.roles")}</legend>
      <p className="text-sm text-muted-foreground">{t("members.roles_hint")}</p>
      {roles.map((r) => (
        <label key={r.id} className="flex min-h-12 items-center gap-3">
          <input
            type="checkbox"
            className="size-5"
            checked={chosen.includes(r.id)}
            onChange={(e) => onChange(e.target.checked ? [...chosen, r.id] : chosen.filter((id) => id !== r.id))}
          />
          {r.name}
        </label>
      ))}
    </fieldset>
  );
}

function RolesEditor({ member, onDone }: { member: MemberView; onDone: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const roles = useQuery(rolesQuery);
  const [chosen, setChosen] = useState((member.roles ?? []).map((r) => r.id));
  const save = useMutation({
    mutationFn: () =>
      call(api.PATCH("/members/{membershipId}", { params: { path: { membershipId: member.id } }, body: { role_ids: chosen } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: membersQuery.queryKey });
      await queryClient.invalidateQueries({ queryKey: rolesQuery.queryKey }); // member counts
      onDone();
    },
  });
  if (roles.error) return <ErrorAlert error={roles.error} onRetry={() => void roles.refetch()} />;
  if (roles.data === undefined) return <p role="status">{t("app.loading")}</p>;
  return (
    <div className="space-y-3">
      <ErrorAlert error={save.error} />
      <RoleChoices roles={roles.data ?? []} chosen={chosen} onChange={setChosen} />
      <div className="flex flex-wrap gap-2">
        <Button disabled={save.isPending} onClick={() => save.mutate()}>{t("profile.save")}</Button>
        <Button variant="outline" onClick={onDone}>{t("common.cancel")}</Button>
      </div>
    </div>
  );
}

function Invites({ me }: { me: Me }) {
  const { t } = useTranslation();
  const invites = useQuery(invitesQuery);
  const [creating, setCreating] = useState(false);
  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">{t("invites.title")}</h2>
      {creating ? <InviteForm me={me} onClose={() => setCreating(false)} /> : (
        <Button onClick={() => setCreating(true)}>{t("invites.new")}</Button>
      )}
      {invites.error && <ErrorAlert error={invites.error} onRetry={() => void invites.refetch()} />}
      {invites.data !== undefined && (invites.data ?? []).length === 0 && <p className="text-muted-foreground">{t("invites.none")}</p>}
      <ul className="space-y-4">
        {(invites.data ?? []).map((inv) => <InviteItem key={inv.id} me={me} invite={inv} />)}
      </ul>
    </section>
  );
}

function inviteMessage(t: ReturnType<typeof useTranslation>["t"], me: Me, name: string, link: string) {
  return t("invites.message", { name, church: me.church?.name ?? "", link });
}

function InviteItem({ me, invite }: { me: Me; invite: InviteView }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [link, setLink] = useState<{ link: string; expires_at: string } | null>(null);
  const refresh = () => queryClient.invalidateQueries({ queryKey: invitesQuery.queryKey });
  const regenerate = useMutation({
    mutationFn: () => call(api.POST("/invites/{id}/regenerate", { params: { path: { id: invite.id } } })),
    onSuccess: async (l) => {
      setLink(l);
      await refresh();
    },
  });
  const cancel = useMutation({
    mutationFn: () => call(api.DELETE("/invites/{id}", { params: { path: { id: invite.id } } })),
    onSuccess: async () => {
      await refresh();
      await queryClient.invalidateQueries({ queryKey: membersQuery.queryKey }); // usage
    },
  });
  const roles = (invite.roles ?? []).map((r) => r.name);
  const expired = invite.status === "expired";
  return (
    <li className="space-y-3 rounded-md border border-border p-4">
      <div>
        <p className="font-semibold">{invite.name}</p>
        <p className="text-sm text-muted-foreground">{[invite.email, invite.phone].filter(Boolean).join(" · ")}</p>
        <p className="text-sm">{roles.length > 0 ? roles.join(", ") : t("members.team_member")}</p>
        <p className="text-sm text-muted-foreground">
          {expired
            ? t("invites.expired", { when: when(me, invite.expires_at, i18n.language) })
            : t("invites.pending", { name: invite.created_by_name ?? t("members.server_admin"), when: when(me, invite.expires_at, i18n.language) })}
        </p>
      </div>
      <ErrorAlert error={regenerate.error ?? cancel.error} />
      {link && (
        <ShareLink
          label={t("invites.link", { name: invite.name })}
          link={link.link}
          message={inviteMessage(t, me, invite.name, link.link)}
          expires={t("share.expires", { when: when(me, link.expires_at, i18n.language) })}
        />
      )}
      {(invite.actions.regenerate || invite.actions.cancel) && (
        <div className="flex flex-wrap gap-2">
          {invite.actions.regenerate && (
            <Button variant="outline" disabled={regenerate.isPending} onClick={() => regenerate.mutate()}>{t("invites.regenerate")}</Button>
          )}
          {invite.actions.cancel && (
            <ConfirmButton
              label={t("invites.cancel")}
              question={t("invites.cancel_question", { name: invite.name })}
              confirmLabel={t("invites.cancel_confirm")}
              pending={cancel.isPending}
              onConfirm={() => cancel.mutate()}
            />
          )}
        </div>
      )}
    </li>
  );
}

const inviteSchema = z
  .object({
    name: z.string().trim().min(1, "common.required").max(120),
    email: z.string().trim().max(254),
    phone: z.string().trim().max(32),
  })
  .refine((v) => v.email !== "" || v.phone !== "", { path: ["email"], message: "invite.need_identifier" });
type InviteValues = z.infer<typeof inviteSchema>;

function InviteForm({ me, onClose }: { me: Me; onClose: () => void }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const roles = useQuery(rolesQuery);
  const [roleIds, setRoleIds] = useState<string[]>([]);
  const form = useForm<InviteValues>({ resolver: zodResolver(inviteSchema), defaultValues: { name: "", email: "", phone: "" } });
  const create = useMutation({
    mutationFn: (v: InviteValues) =>
      call(api.POST("/invites", { body: { name: v.name, email: v.email || undefined, phone: v.phone || undefined, role_ids: roleIds } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: invitesQuery.queryKey });
      await queryClient.invalidateQueries({ queryKey: membersQuery.queryKey }); // usage
    },
    onError: (err) => {
      for (const f of fieldErrors(err)) {
        if (f === "name" || f === "email" || f === "phone") form.setError(f, { message: "common.field_invalid" });
      }
    },
  });

  if (create.data) {
    const c = create.data;
    return (
      <div className="space-y-3">
        <Alert>{t("invites.created", { name: c.invite.name })}</Alert>
        <ShareLink
          label={t("invites.link", { name: c.invite.name })}
          link={c.link}
          message={inviteMessage(t, me, c.invite.name, c.link)}
          expires={t("share.expires", { when: when(me, c.expires_at, i18n.language) })}
        />
        <Button variant="outline" onClick={onClose}>{t("common.done")}</Button>
      </div>
    );
  }
  const e = form.formState.errors;
  const msg = (m?: string) => (m ? t(m) : undefined);
  return (
    <form noValidate className="max-w-xl space-y-4 rounded-md border border-border p-4" onSubmit={form.handleSubmit((v) => create.mutate(v))}>
      <h3 className="text-lg font-semibold">{t("invites.new")}</h3>
      <ErrorAlert error={create.error ?? roles.error} />
      <Field label={t("invites.name")} error={msg(e.name?.message)}>
        <Input autoComplete="off" {...form.register("name")} />
      </Field>
      <Field label={t("invite.email")} hint={t("invites.contact_hint")} error={msg(e.email?.message)}>
        <Input type="email" autoComplete="off" {...form.register("email")} />
      </Field>
      <Field label={t("invite.phone")} error={msg(e.phone?.message)}>
        <Input type="tel" autoComplete="off" {...form.register("phone")} />
      </Field>
      {roles.data !== undefined && <RoleChoices roles={roles.data ?? []} chosen={roleIds} onChange={setRoleIds} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={create.isPending}>{t("invites.create")}</Button>
        <Button variant="outline" onClick={onClose}>{t("common.cancel")}</Button>
      </div>
    </form>
  );
}
