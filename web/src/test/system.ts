// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { SystemStatusView } from "@liturgist/api-client";

const GB = 1024 ** 3;

// systemStatus is a healthy community install; tests change what they need.
export function systemStatus(changes: Record<string, unknown> = {}): SystemStatusView {
  const base = {
    version: "v0.3.0", commit: "abc1234",
    database: { driver: "sqlite", size_bytes: 3 * 1024 * 1024, schema_version: 14 },
    disk: { free_bytes: 40 * GB, total_bytes: 100 * GB, low: false },
    backup: { supported: true, scheduled: true, last_at: "2026-10-05T19:00:00Z", last_kind: "auto", last_copied_at: "2026-10-01T03:00:00Z", warning: "" },
    email_configured: false,
    https: { mode: "behind_proxy", plain_http_warning: false, proxy_missing_warning: false },
    update: { enabled: false, latest: "", available: false },
  };
  return { ...base, ...changes } as unknown as SystemStatusView;
}
