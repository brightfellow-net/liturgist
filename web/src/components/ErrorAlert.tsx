// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from "react-i18next";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/errors";

// ErrorAlert shows a translated error, with a retry button when given one.
export function ErrorAlert({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation();
  if (!error) return null;
  return (
    <Alert variant="error" className="space-y-2">
      <p>{errorText(t, error)}</p>
      {onRetry && <Button variant="outline" onClick={onRetry}>{t("common.retry")}</Button>}
    </Alert>
  );
}
