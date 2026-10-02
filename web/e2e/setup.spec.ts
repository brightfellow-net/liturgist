// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import fs from "node:fs";
import { expect, test } from "@playwright/test";
import { admin, adminState, churchName, setupLinkFile } from "./env";
import { expectAccessible } from "./helpers";

// E2E-W-001: first-time setup through the link printed in the server log.
test("E2E-W-001 first-time setup", async ({ page }) => {
  await page.goto(fs.readFileSync(setupLinkFile, "utf8"));
  await expect(page.getByRole("heading", { name: "Set up Liturgist" })).toBeVisible();
  await expect(page).toHaveURL(/\/setup$/); // the token left the address bar
  await expectAccessible(page); // E2E-W-005 for the setup page

  await page.getByLabel("Church name").fill(churchName);
  await page.getByLabel("Your name").fill(admin.name);
  await page.getByLabel("Your email or phone number").fill(admin.email);
  await page.getByLabel("Password", { exact: true }).fill(admin.password);
  await page.getByRole("button", { name: "Create church" }).click();

  await expect(page.getByRole("heading", { name: `Welcome, ${admin.name}` })).toBeVisible();
  await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
  await page.context().storageState({ path: adminState });
});
