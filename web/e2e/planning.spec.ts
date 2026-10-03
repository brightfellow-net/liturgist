// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test, type Page } from "@playwright/test";
import { adminPage } from "./helpers";

const row = (page: Page, name: string) => page.getByRole("listitem").filter({ hasText: name });

// E2E-W-012: the planning setup. The church was seeded at setup in Indonesian.
test("E2E-W-012 duties, singing parts, a template and a service", async ({ browser }) => {
  const page = await adminPage(browser);
  await page.goto("/");
  await page.getByRole("navigation", { name: "Main menu" }).getByRole("link", { name: "Liturgies" }).click();
  await expect(page.getByRole("heading", { name: "Liturgies", level: 1 })).toBeVisible();

  // The seeded defaults are there and editable.
  await expect(page.getByRole("link", { name: "Ibadah Minggu" })).toBeVisible();
  await page.getByRole("link", { name: "Duties" }).click();
  await expect(page.getByRole("listitem").first()).toContainText("Liturgis");
  await expect(page.getByRole("listitem")).toHaveCount(7);

  // A duty: add, see the name taken, rename, move to the top.
  await page.getByLabel("New duty").fill("Penerima Tamu");
  await page.getByRole("button", { name: "Add a duty" }).click();
  await expect(row(page, "Penerima Tamu")).toBeVisible();
  await page.getByLabel("New duty").fill("penerima  TAMU");
  await page.getByRole("button", { name: "Add a duty" }).click();
  await expect(page.getByText("There is already a duty with this name.")).toBeVisible();
  await page.getByRole("button", { name: "Rename Penerima Tamu" }).click();
  await page.getByLabel("Name", { exact: true }).fill("Penyambut Tamu");
  await page.getByRole("button", { name: "Save name" }).click();
  await expect(row(page, "Penyambut Tamu")).toBeVisible();
  for (let i = 0; i < 7; i++) await page.getByRole("button", { name: "Move up Penyambut Tamu" }).click();
  await expect(page.getByRole("listitem").first()).toContainText("Penyambut Tamu");
  await page.reload();
  await expect(page.getByRole("listitem").first()).toContainText("Penyambut Tamu"); // the order is stored

  // A singing part.
  await page.getByRole("link", { name: "Singing parts" }).click();
  await page.getByLabel("New singing part").fill("Solo");
  await page.getByRole("button", { name: "Add a singing part" }).click();
  await expect(row(page, "Solo")).toBeVisible();

  // A template with three items, in this order.
  await page.getByRole("link", { name: "Templates" }).click();
  await page.getByRole("link", { name: "Add a template" }).click();
  await page.getByLabel("Name", { exact: true }).fill("Ibadah Pemuda");
  const add = async (n: number, title: string, kind: string, duty?: string, text?: string) => {
    await page.getByRole("button", { name: "Add an item" }).click();
    const item = page.getByRole("group", { name: new RegExp(`^Item ${n}:?`) });
    await item.getByLabel("Title").fill(title);
    await item.getByLabel("Kind").selectOption(kind);
    if (duty) await item.getByLabel("Default duty").selectOption({ label: duty });
    if (text) await item.getByLabel("Default text").fill(text);
  };
  await add(1, "Lagu pembuka", "song");
  await add(2, "Sambutan", "free_text", "Penyambut Tamu", "Selamat datang");
  await add(3, "Khotbah", "sermon");
  await page.getByRole("button", { name: "Move up Item 2" }).click();
  await page.getByRole("button", { name: "Save template" }).click();
  await expect(page.getByRole("link", { name: "Ibadah Pemuda" })).toBeVisible();
  await expect(row(page, "Ibadah Pemuda")).toContainText("3 items");

  // It was stored in the order of the cards.
  await page.getByRole("link", { name: "Ibadah Pemuda" }).click();
  await expect(page.getByRole("group", { name: /^Item 1:/ }).getByLabel("Title")).toHaveValue("Sambutan");
  await expect(page.getByRole("group", { name: /^Item 1:/ }).getByLabel("Default duty")).toHaveValue(/.+/);
  await expect(page.getByRole("group", { name: /^Item 2:/ }).getByLabel("Title")).toHaveValue("Lagu pembuka");
  await expect(page.getByRole("group", { name: /^Item 2:/ }).getByLabel("Default text")).toHaveCount(0); // songs take no text

  // A service with two times and this template.
  await page.getByRole("link", { name: "Back to the templates" }).click();
  await page.getByRole("link", { name: "Services" }).click();
  await page.getByRole("link", { name: "Add a service" }).click();
  await page.getByLabel("Name", { exact: true }).fill("Ibadah Pemuda Sabtu");
  await page.getByLabel("Default template").selectOption({ label: "Ibadah Pemuda" });
  const t1 = page.getByRole("group", { name: "Time 1" });
  await t1.getByLabel("Day").selectOption({ label: "Saturday" });
  await t1.getByLabel("Time", { exact: true }).fill("17:00");
  await page.getByRole("button", { name: "Add a time" }).click();
  const t2 = page.getByRole("group", { name: "Time 2" });
  await t2.getByLabel("Day").selectOption({ label: "Saturday" });
  await t2.getByLabel("Time", { exact: true }).fill("19:00");
  await page.getByRole("button", { name: "Save service" }).click();
  await expect(row(page, "Ibadah Pemuda Sabtu")).toContainText("Saturday 17:00; Saturday 19:00");
  await expect(row(page, "Ibadah Pemuda Sabtu")).toContainText("Template: Ibadah Pemuda");

  // A template that a service uses can't be deleted.
  await page.getByRole("link", { name: "Templates" }).click();
  await page.getByRole("link", { name: "Ibadah Pemuda" }).click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete Ibadah Pemuda" }).click();
  await expect(page.getByText("A service uses this template as its default. Change that service first.")).toBeVisible();

  // A second session saves the template first: this one is told, and keeps what it typed.
  const other = await adminPage(browser);
  await other.goto("/liturgies/templates");
  await other.getByRole("link", { name: "Ibadah Pemuda" }).click();
  await other.getByLabel("Name", { exact: true }).fill("Ibadah Pemuda Baru");
  await other.getByRole("button", { name: "Save changes" }).click();
  await expect(other.getByRole("link", { name: "Ibadah Pemuda Baru" })).toBeVisible();

  await page.getByLabel("Name", { exact: true }).fill("Judul saya");
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText(/changed by someone else while you were editing/)).toBeVisible();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Judul saya");
  await page.getByRole("button", { name: "Reload" }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Ibadah Pemuda Baru");

  // Clean up what this test made, so the others see the seeded church: service, template, duty, part.
  await page.goto("/liturgies/services");
  await page.getByRole("link", { name: "Ibadah Pemuda Sabtu" }).click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete Ibadah Pemuda Sabtu" }).click();
  await expect(page.getByRole("heading", { name: "No services yet" })).toBeVisible();
  await page.goto("/liturgies/templates");
  await page.getByRole("link", { name: "Ibadah Pemuda Baru" }).click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete Ibadah Pemuda Baru" }).click();
  await expect(page.getByRole("link", { name: "Ibadah Pemuda Baru" })).toHaveCount(0);
  await page.goto("/liturgies/duties");
  await row(page, "Penyambut Tamu").getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete Penyambut Tamu" }).click();
  await expect(row(page, "Penyambut Tamu")).toHaveCount(0);
  await page.goto("/liturgies/singing-parts");
  await row(page, "Solo").getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete Solo" }).click();
  await expect(row(page, "Solo")).toHaveCount(0);
});
