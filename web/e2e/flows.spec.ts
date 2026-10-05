// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, request, test } from "@playwright/test";
import { baseURL, churchName } from "./env";
import { adminPage, createMember, logIn, memberPassword } from "./helpers";

test("E2E-W-002 invite and accept", async ({ browser }) => {
  const admin = await adminPage(browser);
  await admin.goto("/settings/members");
  await admin.getByRole("button", { name: "Invite a person" }).click();
  await admin.getByLabel("Name", { exact: true }).fill("Yohanes");
  await admin.getByLabel("Email").fill("yohanes@example.org");
  await admin.getByRole("button", { name: "Create invite" }).click();
  const link = await admin.getByLabel("Invite link for Yohanes").inputValue();

  const page = await (await browser.newContext()).newPage();
  await page.goto(link);
  await expect(page.getByLabel("Your name")).toHaveValue("Yohanes");
  await page.getByLabel("Choose a password").fill(memberPassword);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText(`You are joining ${churchName} as Yohanes.`)).toBeVisible();
  await page.getByRole("button", { name: "Join" }).click();

  await expect(page.getByRole("heading", { name: "Welcome, Yohanes" })).toBeVisible();
  const nav = page.getByRole("navigation", { name: "Main menu" });
  await expect(nav.getByRole("link")).toHaveText(["My assignments", "Published", "Library", "Profile"]); // team member: no Settings
});

test("E2E-W-003 login throttle message", async ({ browser }) => {
  await createMember("Sari", "sari@example.org");
  const page = await (await browser.newContext()).newPage();
  await page.goto("/login");
  await page.getByLabel("Email or phone number").fill("sari@example.org");
  const message = page.getByText(/Too many attempts. Try again in \d+ minutes?\./);
  for (let i = 0; i < 6 && !(await message.isVisible()); i++) {
    await page.getByLabel("Password", { exact: true }).fill("salah sekali " + i);
    await page.getByRole("button", { name: "Log in" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
  }
  await expect(message).toBeVisible();
});

test("E2E-W-004 reset link", async ({ browser }) => {
  await createMember("Paulus", "paulus@example.org");
  const admin = await adminPage(browser);
  await admin.goto("/settings/members");
  const item = admin.getByRole("listitem").filter({ hasText: "Paulus" });
  await item.getByRole("button", { name: "Create reset link" }).click();
  const link = await item.getByLabel("Reset link for Paulus").inputValue();

  const page = await (await browser.newContext()).newPage();
  await page.goto(link);
  await expect(page.getByText(`This link was created by Ruth Admin.`)).toBeVisible();
  await page.getByLabel("New password").fill("domba hilang ditemukan");
  await page.getByRole("button", { name: "Save password and log in" }).click();
  await expect(page.getByRole("heading", { name: "Welcome, Paulus" })).toBeVisible();

  const api = await request.newContext({ baseURL });
  const old = await api.post("/api/v1/auth/login", { data: { identifier: "paulus@example.org", password: memberPassword } });
  expect(old.status()).toBe(401);
  await api.dispose();
});

test("E2E-W-006 role editor", async ({ browser }) => {
  await createMember("Lukas", "lukas@example.org");
  const admin = await adminPage(browser);
  await admin.goto("/settings/roles");
  await admin.getByRole("button", { name: "New role" }).click();
  await admin.getByLabel("Role name").fill("Multimedia");
  await admin.getByLabel("See the member list with contact details").check();
  await admin.getByRole("button", { name: "Save" }).click();
  await expect(admin.getByRole("listitem").filter({ hasText: "Multimedia" }).first()).toBeVisible();

  await admin.goto("/settings/members");
  const item = admin.getByRole("listitem").filter({ hasText: "Lukas" });
  await item.getByRole("button", { name: "Change roles" }).click();
  await item.getByLabel("Multimedia").check();
  await item.getByRole("button", { name: "Save" }).click();
  await expect(item.getByRole("checkbox")).toHaveCount(0); // editor closed
  await expect(item.getByText("Multimedia", { exact: true })).toBeVisible();

  const page = await logIn(browser, "lukas@example.org", memberPassword);
  await page.getByRole("link", { name: "Settings" }).click();
  const tabs = page.getByRole("navigation", { name: "Settings" });
  await expect(tabs.getByRole("link")).toHaveText(["Church", "Members"]); // no Roles
  await tabs.getByRole("link", { name: "Members" }).click();
  await expect(page.getByText("Lukas", { exact: true })).toBeVisible();
});
