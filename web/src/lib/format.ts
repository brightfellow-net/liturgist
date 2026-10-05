// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// formatDateTime shows an API timestamp in the church's time zone (05 §6).
export function formatDateTime(iso: string, timeZone: string, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short", timeZone }).format(new Date(iso));
}

// formatBytes writes a size such as "1.4 GB" (units of 1024).
export function formatBytes(bytes: number, language: string): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = Math.max(0, bytes);
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return new Intl.NumberFormat(language, { maximumFractionDigits: unit === 0 ? 0 : 1 }).format(value) + " " + units[unit];
}
