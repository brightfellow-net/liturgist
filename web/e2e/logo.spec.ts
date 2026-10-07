// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { adminPage, createMember, expectAccessible, logIn, memberPassword } from "./helpers";

// A 1 x 1 PNG; the server keeps small images as they are.
const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

test("E2E-W-022 church logo: upload, header, reload, other members, remove", async ({ browser }) => {
  const admin = await adminPage(browser);
  await admin.goto("/settings/church");
  await expect(admin.getByText(/No logo yet/)).toBeVisible();
  await expect(admin.locator("header img")).toHaveCount(0);

  await admin.getByLabel("Choose image").setInputFiles({ name: "logo.png", mimeType: "image/png", buffer: png });
  const preview = admin.getByRole("img", { name: "The current logo" });
  await expect(preview).toBeVisible();
  await expect.poll(() => preview.evaluate((e: HTMLImageElement) => e.naturalWidth)).toBeGreaterThan(0);
  // The header shows it left of the church name, without a reload.
  const header = admin.locator("header img");
  await expect(header).toBeVisible();
  await expect.poll(() => header.evaluate((e: HTMLImageElement) => e.naturalWidth)).toBeGreaterThan(0);
  await expectAccessible(admin);

  // Still there after a reload; the address carries the version.
  await admin.reload();
  await expect(admin.locator("header img")).toHaveAttribute("src", /^\/api\/v1\/church\/logo\?v=[0-9a-f]{16}$/);

  // A team member sees it in the header but has no way to change it.
  await createMember("Rina", "rina@example.org");
  const member = await logIn(browser, "rina@example.org", memberPassword);
  await expect(member.locator("header img")).toBeVisible();
  const put = await member.request.put("/api/v1/church/logo", { data: { image: png.toString("base64") } });
  expect(put.status()).toBe(403);

  // Removing it takes the image out of the header too.
  await admin.getByRole("button", { name: "Remove logo" }).click();
  await expect(admin.getByText(/No logo yet/)).toBeVisible();
  await expect(admin.locator("header img")).toHaveCount(0);
});
