// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { AxeBuilder } from "@axe-core/playwright";
import { expect, request, type APIRequestContext, type Browser, type Page } from "@playwright/test";
import { adminState, baseURL } from "./env";

export const memberPassword = "kebun anggur subur";

// adminApi is an API client logged in as the church admin.
export async function adminApi(): Promise<APIRequestContext> {
  return request.newContext({ baseURL, storageState: adminState });
}

// tokenOf returns the "#t=" token of an invite or reset link.
export function tokenOf(link: string): string {
  return link.slice(link.indexOf("#t=") + 3);
}

// createInvite creates an invite through the API and returns its link.
export async function createInvite(name: string, email: string, roleIds: string[] = []): Promise<string> {
  const api = await adminApi();
  const res = await api.post("/api/v1/invites", { data: { name, email, role_ids: roleIds } });
  expect(res.status(), await res.text()).toBe(201);
  const { link } = (await res.json()) as { link: string };
  await api.dispose();
  return link;
}

// createMember invites a person and accepts the invite as a new account.
export async function createMember(name: string, email: string): Promise<void> {
  const link = await createInvite(name, email);
  const anon = await request.newContext({ baseURL });
  const res = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name, email, password: memberPassword } });
  expect(res.status(), await res.text()).toBe(201);
  await anon.dispose();
}

// adminPage opens a page logged in as the church admin.
export async function adminPage(browser: Browser): Promise<Page> {
  const context = await browser.newContext({ storageState: adminState });
  return context.newPage();
}

// logIn opens a new page and logs in through the login form.
export async function logIn(browser: Browser, identifier: string, password: string): Promise<Page> {
  const page = await (await browser.newContext()).newPage();
  await page.goto("/login");
  await page.getByLabel("Email or phone number").fill(identifier);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: /^Welcome/ })).toBeVisible();
  return page;
}

// expectAccessible fails on any WCAG 2.2 A or AA violation (E2E-W-005), in
// light and dark mode.
export async function expectAccessible(page: Page): Promise<void> {
  for (const colorScheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme });
    const result = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"])
      .analyze();
    const found = result.violations.map((v) => `${v.id} (${colorScheme}): ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`);
    expect(found, `${page.url()} in ${colorScheme} mode`).toEqual([]);
  }
  await page.emulateMedia({ colorScheme: null });
}

// createSong adds a song through the API as the church admin and returns its ID.
export async function createSong(song: { title: string; language?: string; hymnal_source?: string; hymnal_number?: string; sections: { kind: string; number?: number; text: string }[] }): Promise<string> {
  const api = await adminApi();
  const res = await api.post("/api/v1/songs", {
    data: { language: "id", ...song, sections: song.sections.map((s, i) => ({ key: "k" + i, ...s })) },
  });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  await api.dispose();
  return id;
}

// createReading saves a reading through the API as the church admin and returns its ID.
export async function createReading(reading: { reference: string; translation: string; text: string; attribution?: string }): Promise<string> {
  const api = await adminApi();
  const res = await api.post("/api/v1/readings", { data: reading });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  await api.dispose();
  return id;
}
