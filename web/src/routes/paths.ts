// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Every internal route; links use these, never hand-built strings (05 §5).
export const paths = {
  home: "/",
  login: "/login",
  setup: "/setup",
  invite: "/invite",
  reset: "/reset",
  profile: "/profile",
  churchSettings: "/settings/church",
  members: "/settings/members",
  roles: "/settings/roles",
  privacy: "/privacy",
  library: "/library",
  songNew: "/library/songs/new",
  song: "/library/songs/:id",
  songEdit: "/library/songs/:id/edit",
  readings: "/library/readings",
  readingNew: "/library/readings/new",
  reading: "/library/readings/:id",
  import: "/library/import",
  planning: "/liturgies",
  liturgyPrepare: "/liturgies/prepare",
  liturgyNew: "/liturgies/new",
  liturgy: "/liturgies/:id",
  templates: "/liturgies/templates",
  templateNew: "/liturgies/templates/new",
  template: "/liturgies/templates/:id",
  services: "/liturgies/services",
  serviceNew: "/liturgies/services/new",
  service: "/liturgies/services/:id",
  duties: "/liturgies/duties",
  singingParts: "/liturgies/singing-parts",
  importReview: "/library/import/:id",
  published: "/published",
  publishedView: "/published/:id",
  publishedPrint: "/published/:id/print",
} as const;

// songPath and songEditPath fill in a song's ID.
export const songPath = (id: string) => paths.song.replace(":id", encodeURIComponent(id));
export const songEditPath = (id: string) => paths.songEdit.replace(":id", encodeURIComponent(id));

// loginWithNext is the login page returning to next afterwards.
export function loginWithNext(next: string): string {
  return next === paths.home ? paths.login : paths.login + "?next=" + encodeURIComponent(next);
}

// safeNext accepts only paths inside this app (no "//host" or "https:" links).
export function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/\\") ? next : paths.home;
}

// readingPath fills in a reading's ID.
export const readingPath = (id: string) => paths.reading.replace(":id", encodeURIComponent(id));

// templatePath and servicePath fill in an ID.
export const templatePath = (id: string) => paths.template.replace(":id", encodeURIComponent(id));
export const servicePath = (id: string) => paths.service.replace(":id", encodeURIComponent(id));

// importPath fills in an import batch's ID.
export const importPath = (id: string) => paths.importReview.replace(":id", encodeURIComponent(id));

// liturgyPath fills in a liturgy's ID.
export const liturgyPath = (id: string) => paths.liturgy.replace(":id", encodeURIComponent(id));

// publishedPath fills in a liturgy's ID.
export const publishedPrintPath = (id: string) => paths.publishedPrint.replace(":id", encodeURIComponent(id));
export const publishedPath = (id: string) => paths.publishedView.replace(":id", encodeURIComponent(id));
