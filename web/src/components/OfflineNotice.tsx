// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from "react-i18next";
import { Alert } from "@/components/ui/alert";
import { fromSavedCopy } from "@/lib/offline";

// OfflineNotice says so when the page shows a saved copy (13 §7). The service
// worker marks such an answer with X-From-Cache, which lib/offline.ts records
// by path. savedAt is the copy's published_at, already formatted.
export function OfflineNotice({ path, savedAt }: { path: string; savedAt?: string }) {
  const { t } = useTranslation();
  if (!fromSavedCopy(path)) return null;
  return <Alert>{savedAt ? t("offline.saved_copy_at", { date: savedAt }) : t("offline.saved_copy")}</Alert>;
}
