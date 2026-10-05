// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect } from "react";
import type { Me } from "@liturgist/api-client";
import { Navigate, NavLink, Outlet, useLocation, useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { isCode, NetworkError } from "@/lib/errors";
import { rememberUser, clearOffline, userMarker } from "@/lib/offline";
import { isReadingMode } from "@/lib/published";
import i18n, { chooseLanguage } from "@/lib/i18n";
import { meQuery } from "@/lib/queries";
import { showPlanning, showSettings } from "@/lib/scopes";
import { SystemBanners } from "@/components/SystemBanners";
import { loginWithNext, paths } from "./paths";

const settingsPaths: string[] = [paths.churchSettings, paths.members, paths.roles, paths.system];

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
    void rememberUser(me.data.user.id);
  }, [me.data]);

  const logOut = async () => {
    // The saved copies go first, whatever the answer of the server: logging
    // out with no network must clear them too (13 §7).
    await clearOffline();
    try {
      await call(api.POST("/auth/logout"));
    } catch {
      // Offline: the session on the server stays until it expires, but this
      // phone no longer shows anything of it.
    } finally {
      queryClient.clear();
      delete document.documentElement.dataset.textSize;
      void navigate(paths.login);
    }
  };

  if (isCode(me.error, "unauthenticated")) {
    return <Navigate to={loginWithNext(location.pathname + location.search)} replace />;
  }
  // No network, but this phone holds a user's saved copies: show them (13 §7).
  const marker = userMarker();
  if (me.error instanceof NetworkError && marker) {
    return <OfflineFrame userID={marker} logOut={() => void logOut()} />;
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

  if (isReadingMode(location.pathname, location.search)) {
    return (
      <main id="main" className="mx-auto max-w-3xl px-4 py-4">
        <Outlet context={me.data} />
      </main>
    );
  }
  const link = ({ isActive }: { isActive: boolean }) =>
    "inline-flex min-h-12 items-center rounded-md px-3 " + (isActive ? "bg-muted font-semibold" : "hover:bg-muted");
  return (
    <div className="min-h-screen">
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:p-2 print:hidden">{t("app.skip_to_content")}</a>
      <header className="border-b border-border print:hidden">
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
      <SystemBanners me={me.data} />
      <main id="main" className="mx-auto max-w-4xl px-4 py-6 print:max-w-none print:p-0">
        <Outlet context={me.data} />
      </main>
      <footer className="mx-auto max-w-4xl px-4 py-6 print:hidden">
        <NavLink className="underline" to={paths.privacy}>{t("nav.privacy")}</NavLink>
      </footer>
    </div>
  );
}

// OfflineFrame is the app when the server cannot be reached: GET /me is not
// saved, so there is no menu, only "My assignments" and the saved views they
// link to. The sign-in state is not re-checked; it is re-checked when the
// network is back.
function OfflineFrame({ userID, logOut }: { userID: string; logOut: () => void }) {
  const { t } = useTranslation();
  const me = {
    user: { id: userID, name: "", preferences: { text_size: "normal", ui_language: null } },
    church: null,
    membership: null,
  } as unknown as Me;
  return (
    <div className="min-h-screen">
      <header className="border-b border-border">
        <div className="mx-auto flex max-w-4xl flex-wrap items-center gap-2 px-4 py-2">
          <p className="mr-4 font-semibold">{t("app.name")}</p>
          <nav aria-label={t("nav.label")} className="flex flex-1 flex-wrap gap-1">
            <NavLink to={paths.home} end className="inline-flex min-h-12 items-center rounded-md bg-muted px-3 font-semibold">{t("nav.assignments")}</NavLink>
          </nav>
          <Button variant="ghost" onClick={logOut}>{t("nav.log_out")}</Button>
        </div>
      </header>
      <main id="main" className="mx-auto max-w-4xl px-4 py-6">
        <Outlet context={me} />
      </main>
    </div>
  );
}
