// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Navigate, NavLink, Outlet, useOutletContext } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { showPlanning } from "@/lib/scopes";
import { paths } from "../paths";

// PlanningLayout is the frame of the "Liturgies" section (09 §5): tabs for the
// templates, services, duties and singing parts. The liturgy list joins the
// tabs with slice 3C.
export function PlanningLayout() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  if (!showPlanning(me)) return <Navigate to={paths.home} replace />;
  const tab = ({ isActive }: { isActive: boolean }) =>
    "inline-flex min-h-12 items-center border-b-2 px-3 " + (isActive ? "border-primary font-semibold" : "border-transparent");
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("planning.title")}</h1>
      <nav aria-label={t("planning.title")} className="flex flex-wrap gap-2 border-b border-border">
        <NavLink to={paths.templates} className={tab}>{t("planning.tabs.templates")}</NavLink>
        <NavLink to={paths.services} className={tab}>{t("planning.tabs.services")}</NavLink>
        <NavLink to={paths.duties} className={tab}>{t("planning.tabs.duties")}</NavLink>
        <NavLink to={paths.singingParts} className={tab}>{t("planning.tabs.parts")}</NavLink>
      </nav>
      <Outlet context={me} />
    </div>
  );
}

// PlanningIndex sends /liturgies to the first tab until the liturgy list exists.
export function PlanningIndex() {
  return <Navigate to={paths.templates} replace />;
}
