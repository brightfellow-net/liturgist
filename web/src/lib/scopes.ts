// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { Me } from "@liturgist/api-client";

// Scopes decide only which menu items and settings sections are shown;
// buttons follow the "actions" in API responses (05 §4, §8).
export function hasScope(me: Me, scope: string): boolean {
  return me.membership?.scopes?.includes(scope) ?? false;
}

// showPlanning is true for members who work with liturgies or templates; the
// "Liturgies" menu item and its tabs are for them (09 §5).
export function showPlanning(me: Me): boolean {
  return ["templates.edit", "liturgy.edit", "liturgy.comment", "liturgy.approve", "liturgy.manage"].some((s) => hasScope(me, s));
}

// showSettings is true for members holding any settings scope (05 §2).
export function showSettings(me: Me): boolean {
  return ["church.settings", "members.view", "members.manage", "roles.manage"].some((s) => hasScope(me, s));
}
