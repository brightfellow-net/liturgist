// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { adminApi, adminPage, createInvite, createMember, expectAccessible } from "./helpers";

// E2E-W-005: axe finds no WCAG 2.2 A/AA violation on any step-1 page. The
// setup page is checked in setup.spec.ts, before setup completes.
test.describe("E2E-W-005 accessibility", () => {
  for (const [path, heading] of [
    ["/", /^Welcome/],
    ["/profile", "Profile"],
    ["/settings/church", "Settings"],
    ["/settings/members", "Settings"],
    ["/settings/roles", "Settings"],
  ] as const) {
    test(`member page ${path}`, async ({ browser }) => {
      const page = await adminPage(browser);
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading, level: 1 })).toBeVisible();
      await expect(page.getByRole("status").filter({ hasText: "Loading" })).toHaveCount(0);
      await expectAccessible(page);
    });
  }

  for (const [path, heading] of [
    ["/login", "Log in"],
    ["/privacy", "Privacy"],
    ["/no-such-page", "Page not found"],
    ["/invite", "Join your church team"], // incomplete link
  ] as const) {
    test(`public page ${path}`, async ({ page }) => {
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
      await expectAccessible(page);
    });
  }

  test("invite page with a link", async ({ page }) => {
    await page.goto(await createInvite("Maria", "maria@example.org"));
    await expect(page.getByLabel("Your name")).toHaveValue("Maria");
    await expectAccessible(page);
  });

  test("reset page with a link", async ({ page }) => {
    await createMember("Timotius", "timotius@example.org");
    const api = await adminApi();
    const members = (await (await api.get("/api/v1/members")).json()) as { members: { id: string; name: string }[] };
    const id = members.members.find((m) => m.name === "Timotius")!.id;
    const { link } = (await (await api.post(`/api/v1/members/${id}/password-reset`, { data: {} })).json()) as { link: string };
    await api.dispose();
    await page.goto(link);
    await expect(page.getByLabel("New password")).toBeVisible();
    await expectAccessible(page);
  });
});
