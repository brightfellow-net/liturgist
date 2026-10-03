// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { NavLink, Outlet, useOutletContext } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { paths } from "../paths";

// LibraryLayout is the frame of the two library lists: the songs and the
// readings are its two tabs (06 §4, 07 §5).
export function LibraryLayout() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const tab = ({ isActive }: { isActive: boolean }) =>
    "inline-flex min-h-12 items-center border-b-2 px-3 " + (isActive ? "border-primary font-semibold" : "border-transparent");
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("library.title")}</h1>
      <nav aria-label={t("library.title")} className="flex flex-wrap gap-2 border-b border-border">
        <NavLink to={paths.library} end className={tab}>{t("library.songs")}</NavLink>
        <NavLink to={paths.readings} className={tab}>{t("readings.title")}</NavLink>
      </nav>
      <Outlet context={me} />
    </div>
  );
}
