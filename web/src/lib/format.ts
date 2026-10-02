// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// formatDateTime shows an API timestamp in the church's time zone (05 §6).
export function formatDateTime(iso: string, timeZone: string, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short", timeZone }).format(new Date(iso));
}
