// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useOutletContext } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";

export function HomePage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">{t("home.title", { name: me.user.name })}</h1>
      {me.church && <p>{t("home.church", { church: me.church.name })}</p>}
      <p className="text-muted-foreground">{t("home.assignments")}</p>
    </div>
  );
}
