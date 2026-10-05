// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect } from "react";
import { Navigate, NavLink, Outlet, useLocation, useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { isCode } from "@/lib/errors";
import i18n, { chooseLanguage } from "@/lib/i18n";
import { meQuery } from "@/lib/queries";
import { showPlanning, showSettings } from "@/lib/scopes";
import { loginWithNext, paths } from "./paths";

const settingsPaths: string[] = [paths.churchSettings, paths.members, paths.roles];

// AppLayout is the frame of every page for logged-in members: it loads
// GET /me, applies the user's language and text size, and shows the menu.
export function AppLayout() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);

  useEffect(() => {
    if (!me.data) return;
    void i18n.changeLanguage(chooseLanguage(me.data));
    document.documentElement.dataset.textSize = me.data.user.preferences.text_size;
  }, [me.data]);

  const logOut = async () => {
    try {
      await call(api.POST("/auth/logout"));
    } finally {
      queryClient.clear();
      delete document.documentElement.dataset.textSize;
      void navigate(paths.login);
    }
  };

  if (isCode(me.error, "unauthenticated")) {
    return <Navigate to={loginWithNext(location.pathname + location.search)} replace />;
  }
  if (me.error) {
    return (
      <main id="main" className="mx-auto max-w-xl p-4">
        <ErrorAlert error={me.error} onRetry={() => void me.refetch()} />
      </main>
    );
  }
  if (!me.data) {
    return <p role="status" className="p-4">{t("app.loading")}</p>;
  }
  if (!me.data.membership) {
    return (
      <main id="main" className="mx-auto max-w-xl space-y-4 p-4">
        <Alert>{t("membership.not_member")}</Alert>
        <Button onClick={() => void logOut()}>{t("nav.log_out")}</Button>
      </main>
    );
  }

  const link = ({ isActive }: { isActive: boolean }) =>
    "inline-flex min-h-12 items-center rounded-md px-3 " + (isActive ? "bg-muted font-semibold" : "hover:bg-muted");
  return (
    <div className="min-h-screen">
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:p-2">{t("app.skip_to_content")}</a>
      <header className="border-b border-border">
        <div className="mx-auto flex max-w-4xl flex-wrap items-center gap-2 px-4 py-2">
          <p className="mr-4 font-semibold">{me.data.church?.name ?? t("app.name")}</p>
          <nav aria-label={t("nav.label")} className="flex flex-1 flex-wrap gap-1">
            <NavLink to={paths.home} end className={link}>{t("nav.assignments")}</NavLink>
            <NavLink to={paths.published} className={link}>{t("nav.published")}</NavLink>
            <NavLink to={paths.library} className={link}>{t("nav.library")}</NavLink>
            {showPlanning(me.data) && <NavLink to={paths.planning} className={link}>{t("nav.liturgies")}</NavLink>}
            <NavLink to={paths.profile} className={link}>{t("nav.profile")}</NavLink>
            {showSettings(me.data) && (
              <NavLink to={paths.churchSettings} className={(s) => link({ isActive: s.isActive || settingsPaths.includes(location.pathname) })}>
                {t("nav.settings")}
              </NavLink>
            )}
          </nav>
          <Button variant="ghost" onClick={() => void logOut()}>{t("nav.log_out")}</Button>
        </div>
      </header>
      <main id="main" className="mx-auto max-w-4xl px-4 py-6">
        <Outlet context={me.data} />
      </main>
      <footer className="mx-auto max-w-4xl px-4 py-6">
        <NavLink className="underline" to={paths.privacy}>{t("nav.privacy")}</NavLink>
      </footer>
    </div>
  );
}
