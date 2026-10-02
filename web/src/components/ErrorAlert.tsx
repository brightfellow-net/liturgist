// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/errors";
import { scopesQuery } from "@/lib/queries";

// ErrorAlert shows a translated error, with a retry button when given one.
export function ErrorAlert({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation();
  // Scope descriptions, if a page has loaded them (members and roles pages).
  const scopes = useQuery({ ...scopesQuery, enabled: false }).data;
  if (!error) return null;
  const scopeName = (s: string) => scopes?.find((x) => x.scope === s)?.description ?? s;
  return (
    <Alert variant="error" className="space-y-2">
      <p>{errorText(t, error, scopeName)}</p>
      {onRetry && <Button variant="outline" onClick={onRetry}>{t("common.retry")}</Button>}
    </Alert>
  );
}
