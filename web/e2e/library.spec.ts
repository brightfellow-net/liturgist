// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { adminPage, createSong } from "./helpers";

test("E2E-W-007 add a song and find it", async ({ browser }) => {
  // A song that matches none of the searches below: it must disappear from
  // the results, which proves the search was applied.
  await createSong({
    title: "Lagu Pembanding", hymnal_source: "PKJ", hymnal_number: "9",
    sections: [{ kind: "verse", number: 1, text: "Kata lain sama sekali" }],
  });
  const page = await adminPage(browser);
  await page.goto("/library");
  await page.getByRole("link", { name: "Add a song" }).first().click();
  await expect(page.getByRole("heading", { name: "Add a song", level: 1 })).toBeVisible();

  await page.getByLabel("Title", { exact: true }).fill("Tuhan Gembalaku");
  await page.getByLabel("Language").selectOption("id");
  await page.getByLabel("Hymnal", { exact: true }).fill("KJ");
  await page.getByLabel("Number in the hymnal").fill("47");

  await page.getByRole("button", { name: "Add a section" }).click();
  await page.getByRole("group", { name: /^Section 1:/ }).getByLabel("Lyrics").fill("Tuhan gembalaku\ntakkan kekurangan aku");
  await page.getByRole("button", { name: "Add a section" }).click();
  const second = page.getByRole("group", { name: /^Section 2:/ });
  await second.getByLabel("Kind").selectOption("chorus");
  await second.getByLabel("Lyrics").fill("Terpujilah Yehova Rafa");
  await page.getByRole("button", { name: "Save song" }).click();
  await expect(page.getByRole("heading", { name: "Tuhan Gembalaku", level: 1 })).toBeVisible();

  // After a reload, so that the page shows what the server stored and not the
  // copy the form put in the browser's cache: sections, their texts and metadata.
  await page.reload();
  await expect(page.getByRole("heading", { name: "Tuhan Gembalaku", level: 1 })).toBeVisible();
  await expect(page.getByRole("heading", { level: 3 })).toHaveText(["Verse 1", "Chorus"]);
  await expect(page.getByText("takkan kekurangan aku")).toBeVisible();
  await expect(page.getByText("Terpujilah Yehova Rafa")).toBeVisible();
  await expect(page.getByText("KJ 47")).toBeVisible();
  await expect(page.getByText("Indonesian")).toBeVisible();

  // Found by a word from each section, and by the hymnal number. Each search
  // waits for the answer to its own query and checks what the server returned,
  // so a search that ignored its query could not pass.
  await page.getByRole("link", { name: "Back to the library" }).click();
  const search = async (q: string, titles: string[]) => {
    const answer = page.waitForResponse((r) => r.url().includes("/api/v1/songs?") && new URL(r.url()).searchParams.get("q") === q);
    await page.getByLabel("Search songs").fill(q);
    await page.getByRole("button", { name: "Search", exact: true }).click();
    const body = (await (await answer).json()) as { items: { title: string }[] };
    expect(body.items.map((i) => i.title)).toEqual(titles);
    // The form is rebuilt for the new query; wait for that before the next one.
    await expect(page.getByLabel("Search songs")).toHaveValue(q);
    await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe(q);
  };
  await search("kekurangan", ["Tuhan Gembalaku"]);
  await expect(page.getByRole("link", { name: /Tuhan Gembalaku/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /Lagu Pembanding/ })).toHaveCount(0);
  await search("yehova", ["Tuhan Gembalaku"]);
  await expect(page.getByRole("link", { name: /Tuhan Gembalaku/ })).toBeVisible();
  await search("kj 47", ["Tuhan Gembalaku"]);
  await expect(page.getByRole("link", { name: /Tuhan Gembalaku/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /Lagu Pembanding/ })).toHaveCount(0);
  await search("tidak ada lagu ini", []);
  await expect(page.getByText("No songs match.", { exact: false })).toBeVisible();
});

test("E2E-W-008 edit a song and reorder its sections", async ({ browser }) => {
  const id = await createSong({
    title: "Bagi Dia yang Duduk",
    sections: [
      { kind: "verse", number: 1, text: "Satu pertama" },
      { kind: "chorus", text: "Reff tengah" },
      { kind: "verse", number: 2, text: "Dua kedua" },
    ],
  });
  const page = await adminPage(browser);
  await page.goto(`/library/songs/${id}`);
  await page.getByRole("link", { name: "Edit" }).click();
  await expect(page.getByRole("heading", { name: /^Edit Bagi Dia/, level: 1 })).toBeVisible();

  // Move the chorus to the top, then change a title.
  await page.getByRole("button", { name: "Move up Section 2" }).click();
  await expect(page.getByRole("group", { name: /^Section 1:/ }).getByLabel("Lyrics")).toHaveValue("Reff tengah");
  await page.getByLabel("Title", { exact: true }).fill("Bagi Dia yang Duduk di Takhta");
  await page.getByRole("button", { name: "Save song" }).click();

  await expect(page.getByRole("heading", { name: "Bagi Dia yang Duduk di Takhta", level: 1 })).toBeVisible();
  await expect(page.getByRole("heading", { level: 3 })).toHaveText(["Chorus", "Verse 1", "Verse 2"]);

  // The order survived a reload; the sections kept their texts.
  await page.reload();
  await expect(page.getByRole("heading", { level: 3 })).toHaveText(["Chorus", "Verse 1", "Verse 2"]);
  await expect(page.getByText("Reff tengah")).toBeVisible();

  // A second editor saving first makes this save fail with a clear message.
  await page.getByRole("link", { name: "Edit" }).click();
  const other = await adminPage(browser);
  await other.goto(`/library/songs/${id}/edit`);
  await other.getByLabel("Title", { exact: true }).fill("Judul orang lain");
  await other.getByRole("button", { name: "Save song" }).click();
  await expect(other.getByRole("heading", { name: "Judul orang lain", level: 1 })).toBeVisible();

  await page.getByLabel("Title", { exact: true }).fill("Judul saya");
  await page.getByRole("button", { name: "Save song" }).click();
  await expect(page.getByText(/changed by someone else/)).toBeVisible();
  await page.getByRole("button", { name: "Reload" }).click();
  await expect(page.getByLabel("Title", { exact: true })).toHaveValue("Judul orang lain");
});

test("E2E-W-009 add a reading and be told it exists", async ({ browser }) => {
  const page = await adminPage(browser);
  await page.goto("/library");
  await page.getByRole("navigation", { name: "Library" }).getByRole("link", { name: "Readings" }).click();
  await page.getByRole("link", { name: "Add a reading" }).click();
  await expect(page.getByRole("heading", { name: "Add a reading", level: 1 })).toBeVisible();

  // The reference is understood by the server's parser, in the Indonesian book name.
  await page.getByLabel("Bible reference").fill("Yoh 3:16-21");
  await expect(page.getByText("Understood as: Yohanes 3:16-21")).toBeVisible();
  await page.getByLabel("Translation").selectOption("TB");
  await page.getByLabel("Text of the reading").fill("Karena begitu besar kasih Allah akan dunia ini,\nsehingga Ia telah mengaruniakan Anak-Nya yang tunggal.");
  await page.getByLabel("Credit line (optional)").fill("Terjemahan Baru © LAI");
  await page.getByRole("button", { name: "Save reading" }).click();

  await expect(page.getByRole("heading", { name: "Yohanes 3:16-21 (TB)", level: 1 })).toBeVisible();
  await expect(page.getByText("Terjemahan Baru © LAI")).toBeVisible();

  // Found by a word from the text.
  await page.getByRole("link", { name: "Back to the readings" }).click();
  await page.getByLabel("Search readings").fill("mengaruniakan");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByRole("link", { name: /Yohanes 3:16-21 \(TB\)/ })).toBeVisible();

  // The same passage typed another way: the page says it exists and links to it.
  await page.getByRole("link", { name: "Add a reading" }).click();
  await page.getByLabel("Bible reference").fill("Yohanes 3 : 16 – 21");
  await page.getByLabel("Translation").selectOption("TB");
  await expect(page.getByText("You already saved this reading.")).toBeVisible();
  await page.getByRole("link", { name: "Open the saved reading" }).click();
  await expect(page.getByRole("heading", { name: "Yohanes 3:16-21 (TB)", level: 1 })).toBeVisible();

  // Nonsense is explained, not saved.
  await page.goto("/library/readings/new");
  await page.getByLabel("Bible reference").fill("Foo 1");
  await expect(page.getByText("I don't know this book. Try the usual short name, e.g. Yoh or Mzm.")).toBeVisible();
});
