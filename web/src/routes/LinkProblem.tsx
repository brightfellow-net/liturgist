// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Alert } from "@/components/ui/alert";
import { ErrorAlert } from "@/components/ErrorAlert";
import { paths } from "./paths";

// LinkProblem is shown when an invite or reset link can't be used: either
// its "#t=" part is missing (no API call is made) or the server rejected it.
export function LinkProblem({ error }: { error?: unknown }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      {error ? <ErrorAlert error={error} /> : <Alert>{t("common.link_incomplete")}</Alert>}
      <Link className="underline" to={paths.login}>{t("common.go_to_login")}</Link>
    </div>
  );
}
