// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useQuery } from "@tanstack/react-query";
import { useOutletContext } from "react-router";
import { useTranslation } from "react-i18next";
import type { Me, SystemStatusView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { formatBytes, formatDateTime } from "@/lib/format";
import { systemStatusQuery } from "@/lib/queries";
import { systemWarnings } from "@/components/SystemBanners";

// SystemPage is the church admin's view of the server: version, database,
// disk, backups, HTTPS and updates, with the backup download (14 §5).
export function SystemPage() {
  const { t, i18n } = useTranslation();
  const me = useOutletContext<Me>();
  const status = useQuery(systemStatusQuery);
  if (status.error) return <ErrorAlert error={status.error} onRetry={() => void status.refetch()} />;
  if (status.data === undefined) return <p role="status">{t("app.loading")}</p>;
  if (status.data === null) return <Alert>{t("system.not_available")}</Alert>;
  const s = status.data;
  const lang = i18n.language;
  const zone = me.church?.time_zone ?? "UTC";
  const when = (iso: string | null | undefined) => (iso ? formatDateTime(iso, zone, lang) : null);
  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">{t("system.title")}</h2>
      <p>{t("system.intro")}</p>
      {systemWarnings(s, lang).map((w) => <Alert key={w.key} variant="error">{t(w.key, w.values)}</Alert>)}
      <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[max-content_1fr]">
        <Row label={t("system.version")}>{s.version}</Row>
        <Row label={t("system.database")}>
          {t("system.database_value", { driver: s.database.driver, size: formatBytes(s.database.size_bytes, lang), schema: s.database.schema_version })}
        </Row>
        <Row label={t("system.disk")}>
          {t("system.disk_value", { free: formatBytes(s.disk.free_bytes, lang), total: formatBytes(s.disk.total_bytes, lang) })}
        </Row>
        <Row label={t("system.last_backup")}>{lastBackup(s, when, t)}</Row>
        <Row label={t("system.last_copy")}>{when(s.backup.last_copied_at) ?? t("system.last_copy_none")}</Row>
        <Row label={t("system.email")}>{s.email_configured ? t("system.email_on") : t("system.email_off")}</Row>
        <Row label={t("system.https")}>{t("system.https_" + s.https.mode)}</Row>
        <Row label={t("system.update")}>{updateText(s, t)}</Row>
      </dl>
      {s.backup.supported ? (
        <div className="space-y-2">
          <p>{s.backup.scheduled ? t("system.backups_scheduled") : t("system.backups_unscheduled")}</p>
          <a href="/api/v1/system/backup" download className={buttonVariants()}>{t("system.download")}</a>
          <p className="text-sm text-muted-foreground">{t("system.download_hint")}</p>
        </div>
      ) : (
        <p>{t("system.download_unsupported")}</p>
      )}
    </section>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="font-semibold">{label}</dt>
      <dd>{children}</dd>
    </>
  );
}

function lastBackup(s: SystemStatusView, when: (iso: string | null | undefined) => string | null, t: (k: string) => string): string {
  const at = when(s.backup.last_at);
  if (!at) return t("system.last_backup_none");
  return `${at} (${t("system.kind_" + (s.backup.last_kind === "auto" ? "auto" : "manual"))})`;
}

function updateText(s: SystemStatusView, t: (k: string, o?: Record<string, string>) => string): string {
  if (!s.update.enabled) return t("system.update_off");
  if (!s.update.latest) return t("system.update_unknown");
  return s.update.available ? t("system.update_available", { latest: s.update.latest }) : t("system.update_current");
}
