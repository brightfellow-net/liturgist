// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { meQuery } from "@/lib/queries";
import { PublicLayout } from "./PublicLayout";

// PrivacyPage is public; logged-in members also see their church's contact.
export function PrivacyPage() {
  const { t } = useTranslation();
  const me = useQuery({ ...meQuery, retry: false, meta: { public: true } });
  const contact = me.data?.church?.privacy_contact;
  const sections = ["what", "ip", "why", "who"] as const;
  return (
    <PublicLayout title={t("privacy.title")}>
      {sections.map((s) => (
        <section key={s} className="space-y-1">
          <h2 className="text-lg font-semibold">{t(`privacy.${s}_title`)}</h2>
          <p>{t(`privacy.${s}`)}</p>
        </section>
      ))}
      <section className="space-y-1">
        <h2 className="text-lg font-semibold">{t("privacy.contact_title")}</h2>
        <p>{contact ? t("privacy.contact_church", { contact }) : t("privacy.contact_default")}</p>
      </section>
    </PublicLayout>
  );
}
