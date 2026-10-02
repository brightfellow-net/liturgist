// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { LanguageSwitch } from "@/components/LanguageSwitch";
import { paths } from "./paths";

// PublicLayout frames the pages used before login.
export function PublicLayout({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="mx-auto flex min-h-screen max-w-xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <p className="text-xl font-semibold">{t("app.name")}</p>
        <LanguageSwitch />
      </header>
      <main id="main" className="space-y-6">
        <h1 className="text-2xl font-semibold">{title}</h1>
        {children}
      </main>
      <footer className="mt-auto">
        <Link className="underline" to={paths.privacy}>{t("nav.privacy")}</Link>
      </footer>
    </div>
  );
}
