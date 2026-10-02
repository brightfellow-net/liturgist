// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { PublicLayout } from "./PublicLayout";
import { paths } from "./paths";

export function NotFoundPage() {
  const { t } = useTranslation();
  return (
    <PublicLayout title={t("not_found.title")}>
      <p>{t("not_found.text")}</p>
      <Link className="underline" to={paths.home}>{t("not_found.home")}</Link>
    </PublicLayout>
  );
}
