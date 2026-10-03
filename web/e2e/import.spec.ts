// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { adminPage } from "./helpers";

test("E2E-W-010 paste lyrics, review, import, find the song", async ({ browser }) => {
  const page = await adminPage(browser);
  await page.goto("/library");
  await page.getByRole("link", { name: "Import songs" }).or(page.getByRole("link", { name: "Paste lyrics" })).first().click();
  await expect(page.getByRole("heading", { name: "Import songs", level: 1 })).toBeVisible();

  await page.getByLabel("Title", { exact: true }).fill("Bagi Tuhan Kuberlutut");
  await page.getByLabel("Language").selectOption("id");
  await page.getByLabel("Lyrics", { exact: true }).fill(
    "Bait 1\nBagi Tuhan kuberlutut\nmenyembah Rajaku\n\nReff:\nHosana di tempat maha tinggi\n\nBait 2\nKuserahkan hidupku\n\nReff",
  );
  await page.getByRole("button", { name: "Read the lyrics" }).click();

  // Nothing is in the library yet; the review shows what was understood.
  await expect(page.getByRole("heading", { name: "Check the songs", level: 1 })).toBeVisible();
  await expect(page.getByRole("button", { name: "Bagi Tuhan Kuberlutut" })).toBeVisible();
  await expect(page.getByText("Nothing has been saved yet.")).toBeVisible();
  await expect(page.getByRole("heading", { level: 4 })).toHaveText(["Verse 1", "Chorus", "Verse 2"]);
  await expect(page.getByLabel("Add as new song")).not.toBeChecked();
  await expect(page.getByRole("button", { name: "Import chosen songs" })).toBeDisabled();

  await page.getByLabel("Add as new song").check();
  await expect(page.getByText("1 song chosen")).toBeVisible();
  await page.getByRole("button", { name: "Import chosen songs" }).click();
  await expect(page.getByText("1 added, 0 merged, 0 skipped.")).toBeVisible();

  // The song is in the library, found by a word from its lyrics.
  await page.getByRole("link", { name: "Bagi Tuhan Kuberlutut" }).first().click();
  await expect(page.getByRole("heading", { name: "Bagi Tuhan Kuberlutut", level: 1 })).toBeVisible();
  await expect(page.getByRole("heading", { level: 3 })).toHaveText(["Verse 1", "Chorus", "Verse 2"]);
  await page.getByRole("link", { name: "Back to the library" }).click();
  await page.getByLabel("Search songs").fill("kuserahkan");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByRole("link", { name: /Bagi Tuhan Kuberlutut/ })).toBeVisible();
  // The batch is finished, so the library no longer mentions it.
  await expect(page.getByText("You have an unfinished import.")).toHaveCount(0);
});

const openLyrics = `<?xml version="1.0" encoding="UTF-8"?>
<song xmlns="http://openlyrics.info/namespace/2009/song" version="0.8">
  <properties><titles><title>Kudapat Damai</title></titles><authors><author>Penulis Contoh</author></authors></properties>
  <lyrics>
    <verse name="v1"><lines>Kudapat damai<br/>dalam hatiku</lines></verse>
    <verse name="c"><lines>Haleluya</lines></verse>
  </lyrics>
</song>`;

const chordPro = `{title: Kudapat Damai}
{start_of_verse}
[G]Kudapat damai dalam [D]hatiku
{end_of_verse}
{start_of_chorus}
Haleluya, Haleluya
{end_of_chorus}
{new_song}
{title: Satu Lagu Lagi}
[C]Hanya satu bait saja
`;

test("E2E-W-011 import an OpenLyrics file and a ChordPro file, and merge", async ({ browser }) => {
  const page = await adminPage(browser);
  await page.goto("/library/import");

  // An OpenLyrics file becomes a new song.
  await page.getByLabel("Choose OpenLyrics files").setInputFiles({ name: "damai.xml", mimeType: "text/xml", buffer: Buffer.from(openLyrics) });
  await page.getByRole("button", { name: "Read the OpenLyrics files" }).click();
  await expect(page.getByRole("heading", { name: "Check the songs", level: 1 })).toBeVisible();
  await page.getByLabel("Add as new song").check();
  await page.getByRole("button", { name: "Import chosen songs" }).click();
  await expect(page.getByText("1 added, 0 merged, 0 skipped.")).toBeVisible();

  // A ChordPro file with two songs: one matches the song just added.
  await page.goto("/library/import");
  await page.getByLabel("Choose ChordPro files").setInputFiles({ name: "lagu.cho", mimeType: "text/plain", buffer: Buffer.from(chordPro) });
  await page.getByRole("button", { name: "Read the ChordPro files" }).click();
  await expect(page.getByText("2 songs found")).toBeVisible();
  const damai = page.getByRole("listitem").filter({ has: page.getByRole("button", { name: "Kudapat Damai" }) });
  await expect(damai.getByRole("link", { name: "Kudapat Damai" })).toBeVisible(); // the duplicate notice

  // "Add all without duplicates" leaves the duplicate for the member.
  await page.getByRole("button", { name: "Add all without duplicates" }).click();
  await expect(page.getByText("1 song chosen")).toBeVisible();
  await damai.getByRole("button", { name: "Kudapat Damai" }).click();
  await expect(page.getByLabel("Merge into “Kudapat Damai”")).not.toBeChecked();

  // The merge preview shows the change before anything happens.
  await page.getByLabel("Merge into “Kudapat Damai”").check();
  const preview = page.getByRole("region", { name: "Merge preview" });
  await expect(preview).toBeVisible();
  await expect(preview.getByText("Haleluya, Haleluya")).toBeVisible();
  await page.getByRole("button", { name: "Use this merge" }).click();
  await expect(page.getByText("2 songs chosen")).toBeVisible();
  await page.getByRole("button", { name: "Import chosen songs" }).click();
  await expect(page.getByText("1 added, 1 merged, 0 skipped.")).toBeVisible();

  // The merged song has the new chorus; the other song was added.
  await page.getByRole("link", { name: "Kudapat Damai" }).first().click();
  await expect(page.getByText("Haleluya, Haleluya")).toBeVisible();
  await expect(page.getByText("Penulis Contoh")).toBeVisible();
  await page.goto("/library?q=Satu+Lagu+Lagi");
  await expect(page.getByRole("link", { name: /Satu Lagu Lagi/ })).toBeVisible();
});

test("E2E-W-010 a file that is not UTF-8 is not sent", async ({ browser }) => {
  const page = await adminPage(browser);
  await page.goto("/library/import");
  await page.getByLabel("Choose ChordPro files").setInputFiles({ name: "latin1.cho", mimeType: "text/plain", buffer: Buffer.from([0x43, 0xe9, 0xff]) });
  await page.getByRole("button", { name: "Read the ChordPro files" }).click();
  await expect(page.getByText("None of these files can be read.")).toBeVisible();
  await expect(page.getByText(/latin1\.cho: This file isn't plain text in UTF-8/)).toBeVisible();
});

// The import pages need no sideways scrolling on a phone (WCAG 1.4.10).
test("E2E-W-010 the import pages fit a 320 px wide screen", async ({ browser }) => {
  const context = await browser.newContext({ storageState: (await import("./env")).adminState, viewport: { width: 320, height: 640 } });
  const page = await context.newPage();
  const fits = () => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);

  await page.goto("/library/import");
  await expect(page.getByRole("heading", { name: "Import songs", level: 1 })).toBeVisible();
  expect(await fits()).toBe(true);

  await page.getByLabel("Title", { exact: true }).fill("Lagu untuk layar sempit dengan judul yang cukup panjang sekali");
  await page.getByLabel("Lyrics", { exact: true }).fill("Bait 1\nBaris pertama yang cukup panjang untuk melihat apakah baris ini patah\n\nReff\nHaleluya");
  await page.getByRole("button", { name: "Read the lyrics" }).click();
  await expect(page.getByRole("heading", { name: "Check the songs", level: 1 })).toBeVisible();
  expect(await fits()).toBe(true);
  await page.getByRole("button", { name: "Edit this song" }).click();
  await expect(page.getByRole("heading", { name: "Edit before importing" })).toBeVisible();
  expect(await fits()).toBe(true);

  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Cancel this import" }).click();
  await page.getByRole("button", { name: "Cancel the import" }).click();
  await context.close();
});
