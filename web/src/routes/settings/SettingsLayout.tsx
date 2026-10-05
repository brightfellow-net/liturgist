// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { NavLink, Outlet, useOutletContext } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { hasScope } from "@/lib/scopes";
import { paths } from "../paths";

// SettingsLayout shows the settings sections this member can open: the
// church (every member may view it), members, and roles (05 §2).
export function SettingsLayout() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const tab = ({ isActive }: { isActive: boolean }) =>
    "inline-flex min-h-12 items-center border-b-2 px-3 " + (isActive ? "border-primary font-semibold" : "border-transparent");
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("settings.title")}</h1>
      <nav aria-label={t("settings.title")} className="flex flex-wrap gap-2 border-b border-border">
        <NavLink to={paths.churchSettings} className={tab}>{t("settings.church")}</NavLink>
        {(hasScope(me, "members.view") || hasScope(me, "members.manage")) && (
          <NavLink to={paths.members} className={tab}>{t("settings.members")}</NavLink>
        )}
        {hasScope(me, "roles.manage") && <NavLink to={paths.roles} className={tab}>{t("settings.roles")}</NavLink>}
        {hasScope(me, "church.settings") && <NavLink to={paths.system} className={tab}>{t("settings.system")}</NavLink>}
      </nav>
      <Outlet context={me} />
    </div>
  );
}
