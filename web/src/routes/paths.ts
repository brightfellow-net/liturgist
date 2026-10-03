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
