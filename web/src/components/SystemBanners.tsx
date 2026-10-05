// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { NavLink } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me, SystemStatusView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { formatBytes } from "@/lib/format";
import { systemStatusQuery } from "@/lib/queries";
import { hasScope } from "@/lib/scopes";
import { paths } from "@/routes/paths";

type Warning = { key: string; values?: Record<string, string> };

// systemWarnings lists what the church admin should act on (14 §5): a nearly
// full disk, a stale or never-copied backup, and plain HTTP.
export function systemWarnings(s: SystemStatusView, language: string): Warning[] {
  const out: Warning[] = [];
  if (s.disk.low) out.push({ key: "system.warn_disk_low", values: { free: formatBytes(s.disk.free_bytes, language) } });
  if (s.backup.warning === "stale") out.push({ key: "system.warn_backup_stale" });
  if (s.backup.warning === "not_copied") out.push({ key: "system.warn_backup_not_copied" });
  if (s.https.plain_http_warning) out.push({ key: "system.warn_plain_http" });
  if (s.https.proxy_missing_warning) out.push({ key: "system.warn_proxy_missing" });
  return out;
}

// SystemBanners shows those warnings above every page to members who can
// change church settings. A dismissed banner stays away until the page is
// loaded again. Nothing is shown while loading, on an error, or on a server
// without the system routes.
export function SystemBanners({ me }: { me: Me }) {
  const { t, i18n } = useTranslation();
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const allowed = hasScope(me, "church.settings");
  const status = useQuery({ ...systemStatusQuery, enabled: allowed, retry: false });
  if (!allowed || !status.data) return null;
  const shown = systemWarnings(status.data, i18n.language).filter((w) => !dismissed.has(w.key));
  if (shown.length === 0) return null;
  return (
    <div className="mx-auto max-w-4xl space-y-2 px-4 pt-4 print:hidden">
      {shown.map((w) => (
        <Alert key={w.key} variant="error" className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span className="flex-1">{t(w.key, w.values)}</span>
          <NavLink className="underline" to={paths.system}>{t("system.open_page")}</NavLink>
          <Button variant="ghost" onClick={() => setDismissed(new Set([...dismissed, w.key]))}>{t("system.dismiss")}</Button>
        </Alert>
      ))}
    </div>
  );
}
