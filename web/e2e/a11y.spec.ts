// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { adminApi, adminPage, createInvite, createMember, createReading, createSong, expectAccessible } from "./helpers";

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

// The library pages (06 §4): the empty library first, before any test adds a
// song, then a list, a song, and the form.
test.describe("E2E-W-005 accessibility of the library", () => {
  test("empty library", async ({ browser }) => {
    const page = await adminPage(browser);
    await page.goto("/library");
    await expect(page.getByRole("heading", { name: "Your library is empty" })).toBeVisible();
    await expectAccessible(page);
  });

  test("list, song, and forms", async ({ browser }) => {
    const id = await createSong({
      title: "Lagu untuk uji aksesibilitas",
      hymnal_source: "KJ",
      hymnal_number: "1",
      sections: [{ kind: "verse", number: 1, text: "Satu" }, { kind: "chorus", text: "Reff" }],
    });
    const page = await adminPage(browser);
    await page.goto("/library");
    await expect(page.getByRole("link", { name: /Lagu untuk uji aksesibilitas/ })).toBeVisible();
    await expectAccessible(page);

    await page.goto(`/library/songs/${id}`);
    await expect(page.getByRole("heading", { name: "Lagu untuk uji aksesibilitas", level: 1 })).toBeVisible();
    await expectAccessible(page);

    await page.getByRole("button", { name: "Link another version…" }).click();
    await expect(page.getByLabel("Find the song to link")).toBeVisible();
    await expectAccessible(page);

    await page.goto(`/library/songs/${id}/edit`);
    await expect(page.getByRole("heading", { level: 1 })).toContainText("Lagu untuk uji aksesibilitas");
    await page.getByRole("button", { name: "Add to the order" }).click();
    await expectAccessible(page);

    await page.goto("/library/songs/new");
    await expect(page.getByRole("heading", { name: "Add a song", level: 1 })).toBeVisible();
    await page.getByRole("button", { name: "Save song" }).click(); // shows the errors
    await expect(page.getByText("This field is required.")).toBeVisible();
    await expectAccessible(page);
  });
});

test.describe("E2E-W-005 accessibility of readings", () => {
  test("empty readings", async ({ browser }) => {
    const page = await adminPage(browser);
    await page.goto("/library/readings");
    await expect(page.getByRole("heading", { name: "No readings saved yet" })).toBeVisible();
    await expectAccessible(page);
  });

  test("list, reading, and form", async ({ browser }) => {
    const id = await createReading({ reference: "Mzm 23", translation: "TB", text: "TUHAN adalah gembalaku, takkan kekurangan aku.", attribution: "© LAI" });
    const page = await adminPage(browser);
    await page.goto("/library/readings");
    await expect(page.getByRole("link", { name: /Mazmur 23 \(TB\)/ })).toBeVisible();
    await expectAccessible(page);

    await page.goto(`/library/readings/${id}`);
    await expect(page.getByRole("heading", { name: "Mazmur 23 (TB)", level: 1 })).toBeVisible();
    await expectAccessible(page);

    await page.getByRole("button", { name: "Edit" }).click();
    await expect(page.getByLabel("Text of the reading")).toBeVisible();
    await expectAccessible(page);

    await page.goto("/library/readings/new");
    await page.getByLabel("Bible reference").fill("Foo 1");
    await expect(page.getByText(/I don't know this book/)).toBeVisible();
    await page.getByRole("button", { name: "Save reading" }).click();
    await expect(page.getByText("Enter the text of the reading.")).toBeVisible();
    await expectAccessible(page);
  });
});

test.describe("E2E-W-005 accessibility of the import pages", () => {
  test("import and review", async ({ browser }) => {
    const page = await adminPage(browser);
    await page.goto("/library/import");
    await expect(page.getByRole("heading", { name: "Import songs", level: 1 })).toBeVisible();
    await page.getByRole("button", { name: "Read the lyrics" }).click(); // shows the errors
    await expect(page.getByText("This field is required.").first()).toBeVisible();
    await expectAccessible(page);

    await page.getByLabel("Title", { exact: true }).fill("Lagu impor aksesibel");
    await page.getByLabel("Lyrics", { exact: true }).fill("Satu dua\n\nSatu dua\n\nTiga empat");
    await page.getByRole("button", { name: "Read the lyrics" }).click();
    await expect(page.getByRole("heading", { name: "Check the songs", level: 1 })).toBeVisible();
    await expect(page.getByText("A part that comes back several times was taken to be the chorus.")).toBeVisible();
    await expectAccessible(page);

    await page.getByRole("button", { name: "Edit this song" }).click();
    await expect(page.getByRole("heading", { name: "Edit before importing" })).toBeVisible();
    await expectAccessible(page);
    await page.getByRole("button", { name: "Cancel", exact: true }).click();

    // Leave the batch unfinished: the library then points to it.
    await page.goto("/library");
    await expect(page.getByText("You have an unfinished import.")).toBeVisible();
    await expectAccessible(page);
    await page.getByRole("link", { name: "Continue the import" }).click();
    await page.getByRole("button", { name: "Cancel this import" }).click();
    await page.getByRole("button", { name: "Cancel the import" }).click();
    await expect(page.getByRole("heading", { name: "Songs", level: 1 }).or(page.getByRole("heading", { name: "Library", level: 1 }))).toBeVisible();
  });
});

test.describe("E2E-W-005 accessibility of the planning pages", () => {
  test("templates, services, duties and singing parts", async ({ browser }) => {
    const page = await adminPage(browser);
    await page.goto("/liturgies/templates");
    await expect(page.getByRole("heading", { name: "Templates", level: 2 })).toBeVisible();
    await expect(page.getByRole("link", { name: "Ibadah Minggu" })).toBeVisible();
    await expectAccessible(page);

    await page.getByRole("link", { name: "Ibadah Minggu" }).click();
    await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Ibadah Minggu");
    await expectAccessible(page);
    await page.getByRole("button", { name: "Save changes" }).click(); // nothing changed: valid, back to the list
    await page.getByRole("link", { name: "Add a template" }).click();
    await page.getByRole("button", { name: "Add an item" }).click();
    await page.getByRole("button", { name: "Save template" }).click(); // shows the errors
    await expect(page.getByText("Enter a name.")).toBeVisible();
    await expect(page.getByText("Enter a title.")).toBeVisible();
    await expectAccessible(page);

    await page.goto("/liturgies/services");
    await expect(page.getByRole("heading", { name: "No services yet" })).toBeVisible();
    await expectAccessible(page);
    await page.getByRole("link", { name: "Add a service" }).first().click();
    await page.getByRole("button", { name: "Save service" }).click(); // shows the errors
    await expect(page.getByText("Enter a time.")).toBeVisible();
    await expectAccessible(page);

    for (const [path, heading] of [["/liturgies/duties", "Duties"], ["/liturgies/singing-parts", "Singing parts"]] as const) {
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading, level: 2 })).toBeVisible();
      await expect(page.getByRole("listitem").first()).toBeVisible();
      await expectAccessible(page);
    }
    await page.getByRole("button", { name: "Rename Semua" }).click();
    await expectAccessible(page);
  });
});

test.describe("E2E-W-005 accessibility of the liturgy pages", () => {
  test("list, prepare, new and the editor with a conflict", async ({ browser }) => {
    const page = await adminPage(browser);
    await page.goto("/liturgies");
    await expect(page.getByRole("heading", { name: "No liturgies yet" })).toBeVisible();
    await expectAccessible(page);

    await page.goto("/liturgies/prepare");
    await expect(page.getByText(/^Week of /)).toBeVisible();
    await expectAccessible(page);

    await page.goto("/liturgies/new");
    await expect(page.getByRole("heading", { name: "New liturgy", level: 2 })).toBeVisible();
    await expectAccessible(page);

    // An editor with a text item, a song item with a sequence, and a reading item.
    const songId = await createSong({ title: "Lagu editor aksesibel", sections: [{ kind: "verse", number: 1, text: "Satu" }, { kind: "chorus", text: "Reff" }] });
    const api = await adminApi();
    const date = new Date(Date.now() + 50 * 86_400_000).toISOString().slice(0, 10);
    const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah aksesibel", date, time: "09:00", template_id: "", language: "id" } });
    expect(created.status(), await created.text()).toBe(201);
    const { id, version } = (await created.json()) as { id: string; version: number };
    let v = version;
    const add = async (title: string, item_type: string) => {
      const res = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: v, title, item_type } });
      expect(res.status(), await res.text()).toBe(201);
      const body = (await res.json()) as { item: { id: string; version: number }; liturgy_version: number };
      v = body.liturgy_version;
      return body.item;
    };
    await add("Votum", "free_text");
    const song = await add("Pujian", "song");
    await add("Bacaan", "reading");
    const res = await api.post(`/api/v1/liturgies/${id}/items/${song.id}/songs`, { data: { version: song.version, song_id: songId } });
    expect(res.status(), await res.text()).toBe(201);

    await page.goto(`/liturgies/${id}`);
    await expect(page.getByRole("heading", { name: "Ibadah aksesibel", level: 2 })).toBeVisible();
    await expect(page.getByText(/^Entry 1:/)).toBeVisible();
    await expect(page.getByRole("status").filter({ hasText: "Loading" })).toHaveCount(0);
    await expectAccessible(page);

    // The conflict state: another session saves the text first.
    const other = await adminPage(browser);
    await other.goto(`/liturgies/${id}`);
    await other.getByRole("form", { name: /^Item 1:/ }).getByLabel("Text").fill("Dari sesi lain");
    await other.getByRole("button", { name: "Save item" }).first().click();
    await expect(other.getByText("Saved", { exact: true })).toBeVisible();
    await page.getByRole("form", { name: /^Item 1:/ }).getByLabel("Text").fill("Dari sesi ini");
    await page.getByRole("button", { name: "Save item" }).first().click();
    await page.getByRole("button", { name: "Show their version" }).click();
    await expect(page.getByText("Dari sesi lain")).toBeVisible();
    await expectAccessible(page);

    // It fits a phone without sideways scrolling.
    await page.setViewportSize({ width: 320, height: 800 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    await page.setViewportSize({ width: 1280, height: 720 });

    expect((await api.delete(`/api/v1/liturgies/${id}`, { data: {} })).status()).toBe(204);
    expect((await api.delete(`/api/v1/songs/${songId}`, { data: {} })).status()).toBe(204);
    await api.dispose();
  });
});
