// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, request, test, type Page } from "@playwright/test";
import { baseURL } from "./env";
import { adminApi, adminPage, createInvite, createReading, createSong, logIn, memberPassword, tokenOf } from "./helpers";

const card = (page: Page, name: RegExp) => page.getByRole("form", { name });

// dateAhead is a date a number of days from today, as YYYY-MM-DD.
function dateAhead(days: number): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

async function addItem(page: Page, title: string, kind: string) {
  const form = page.getByRole("heading", { name: "Add an item" }).locator("xpath=ancestor::form");
  await form.getByLabel("Title").fill(title);
  await form.getByLabel("Kind").selectOption(kind);
  await form.getByRole("button", { name: "Add item" }).click();
  await expect(page.getByRole("form", { name: new RegExp(`^Item \\d+: ${title}`) })).toBeVisible();
}

// E2E-W-013: create a liturgy, edit it, and find everything after a reload.
test("E2E-W-013 create a liturgy and edit its items, songs, reading and team", async ({ browser }) => {
  const songId = await createSong({
    title: "Lagu Liturgi Uji", hymnal_source: "KJ", hymnal_number: "77",
    sections: [{ kind: "verse", number: 1, text: "Bait satu" }, { kind: "chorus", text: "Ini reff" }],
  });
  await createReading({ reference: "Mzm 100", translation: "TB", text: "Bersoraklah bagi TUHAN, hai seluruh bumi!", attribution: "© LAI" });
  const page = await adminPage(browser);
  await page.goto("/liturgies");
  await page.getByRole("link", { name: "New liturgy" }).first().click();
  await expect(page.getByRole("heading", { name: "New liturgy", level: 2 })).toBeVisible();

  // A one-off service from the seeded template (its items are copied).
  await page.getByLabel("Kind of service").selectOption("oneoff");
  await page.getByLabel("Name of the service").fill("Ibadah Uji 13");
  await page.getByLabel("Date").fill(dateAhead(30));
  await page.getByLabel("Time").fill("09:30");
  await page.getByLabel("Template").selectOption({ label: "Ibadah Minggu" });
  await page.getByRole("button", { name: "Create liturgy" }).click();
  await expect(page.getByRole("heading", { name: "Ibadah Uji 13", level: 2 })).toBeVisible();
  await expect(page.getByRole("form", { name: /^Item 1:/ })).toBeVisible();

  // Our own items, after the copied ones.
  await addItem(page, "Doa Uji", "prayer");
  await addItem(page, "Bacaan Uji", "reading");
  await addItem(page, "Pujian Uji", "song");

  // A prayer: text and a duty, saved.
  const doa = card(page, /^Item \d+: Doa Uji/);
  await doa.getByLabel("Text").fill("Ya Tuhan, kami bersyukur.");
  await doa.getByLabel("Duty").selectOption({ label: "Liturgis" });
  await doa.getByRole("button", { name: "Save item" }).click();
  await expect(doa.getByText("Saved", { exact: true })).toBeVisible();

  // A reading: found among the saved readings.
  const bacaan = card(page, /^Item \d+: Bacaan Uji/);
  await bacaan.getByLabel("Search the saved readings").fill("Mazmur");
  await bacaan.getByRole("button", { name: /^Choose Mazmur 100/ }).click();
  await expect(bacaan.getByText("Mazmur 100 (TB)")).toBeVisible();
  await bacaan.getByRole("button", { name: "Save item" }).click();
  await expect(bacaan.getByText("Saved", { exact: true })).toBeVisible();

  // A song: its sequence is filled, then reordered, with a part and a key change.
  const pujian = card(page, /^Item \d+: Pujian Uji/);
  await pujian.getByLabel("Search the library").fill("Lagu Liturgi");
  await pujian.getByRole("button", { name: "Add Lagu Liturgi Uji" }).click();
  await expect(pujian.getByText(/^Entry 1: Verse 1/)).toBeVisible();
  await expect(pujian.getByText(/^Entry 2: Chorus/)).toBeVisible();
  await pujian.getByRole("button", { name: "Move down Entry 1" }).click();
  await expect(pujian.getByText(/^Entry 1: Chorus/)).toBeVisible();
  await pujian.getByRole("group", { name: /^Entry 1:/ }).getByLabel("Sung by").selectOption({ index: 1 });
  await pujian.getByRole("group", { name: /^Entry 1:/ }).getByLabel("Key change").fill("A");
  await pujian.getByLabel("Key", { exact: true }).fill("G");
  await pujian.getByRole("button", { name: "Show lyrics" }).first().click();
  await expect(pujian.getByText("Ini reff")).toBeVisible();
  await pujian.getByRole("button", { name: "Save item" }).click();
  await expect(pujian.getByText("Saved", { exact: true })).toBeVisible();

  // The team: a member and someone without an account.
  await page.getByLabel("Person (Liturgis)", { exact: true }).selectOption({ label: "Ruth Admin" });
  await page.getByRole("button", { name: "Add person (Liturgis)" }).click();
  await expect(page.getByRole("button", { name: "Remove Ruth Admin (Liturgis)" })).toBeVisible();
  await page.getByLabel("Person (Liturgis)", { exact: true }).selectOption({ label: "Someone without an account" });
  await page.getByLabel("Name (Liturgis)", { exact: true }).fill("Pak Daniel");
  await page.getByRole("button", { name: "Add person (Liturgis)" }).click();
  await expect(page.getByRole("button", { name: "Remove Pak Daniel (Liturgis)" })).toBeVisible();

  // Everything is there after a reload, and the history names the changes.
  await page.reload();
  await expect(card(page, /^Item \d+: Doa Uji/).getByLabel("Text")).toHaveValue("Ya Tuhan, kami bersyukur.");
  await expect(card(page, /^Item \d+: Bacaan Uji/).getByText("Bersoraklah bagi TUHAN, hai seluruh bumi!")).toBeVisible();
  const again = card(page, /^Item \d+: Pujian Uji/);
  await expect(again.getByText(/^Entry 1: Chorus/)).toBeVisible();
  await expect(again.getByRole("group", { name: /^Entry 1:/ }).getByLabel("Key change")).toHaveValue("A");
  await expect(again.getByRole("group", { name: /^Entry 1:/ }).getByLabel("Sung by")).not.toHaveValue("");
  await expect(page.getByRole("button", { name: "Remove Pak Daniel (Liturgis)" })).toBeVisible();
  await expect(page.getByText(/Ruth Admin added Doa Uji/)).toBeVisible();

  // The library refuses to delete a song a liturgy uses.
  await page.goto(`/library/songs/${songId}`);
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: /^Delete Lagu Liturgi Uji/ }).click();
  await expect(page.getByText("This song is used in a liturgy that isn't published yet, so it can't be deleted.")).toBeVisible();

  // The liturgy appears in the list, and deleting it frees the song.
  await page.goto("/liturgies");
  await page.getByRole("link", { name: /Ibadah Uji 13/ }).click();
  await page.getByRole("button", { name: "Delete this liturgy" }).click();
  await page.getByRole("button", { name: "Yes, delete it" }).click();
  await expect(page.getByRole("heading", { name: "Liturgies", level: 1 })).toBeVisible();
  await expect(page.getByRole("link", { name: /Ibadah Uji 13/ })).toHaveCount(0);
  const api = await adminApi();
  expect((await api.delete(`/api/v1/songs/${songId}`, { data: {} })).status()).toBe(204);
  await api.dispose();
});

// E2E-W-014: "Prepare next week" with one slot that already has a liturgy.
test("E2E-W-014 prepare next week", async ({ browser }) => {
  const api = await adminApi();
  const sunday = 7;
  const made: string[] = [];
  for (const name of ["Ibadah Satu", "Ibadah Dua"]) {
    const res = await api.post("/api/v1/services", { data: { name, language: "id", default_template_id: "", times: [{ weekday: sunday, time: name === "Ibadah Satu" ? "08:00" : "10:00" }] } });
    expect(res.status(), await res.text()).toBe(201);
    made.push(((await res.json()) as { id: string }).id);
  }
  const page = await adminPage(browser);
  await page.goto("/liturgies/prepare");
  await expect(page.getByText(/^Week of /)).toBeVisible();
  await expect(page.getByRole("checkbox", { name: /Ibadah Satu/ })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: /Ibadah Dua/ })).toBeChecked();

  // Make one of them first, by hand, so its slot is taken.
  const week = await api.get("/api/v1/liturgies/prepare");
  const occurrences = ((await week.json()) as { occurrences: { service_id: string; service_name: string; date: string; time: string }[] }).occurrences;
  const one = occurrences.find((o) => o.service_name === "Ibadah Satu")!;
  const first = await api.post("/api/v1/liturgies", { data: { service_id: one.service_id, date: one.date, time: one.time } });
  expect(first.status(), await first.text()).toBe(201);
  await page.reload();
  await expect(page.getByRole("checkbox", { name: /Ibadah Satu/ })).toBeDisabled();
  await expect(page.getByRole("checkbox", { name: /Ibadah Dua/ })).toBeChecked();
  await page.getByRole("button", { name: "Create 1 liturgy" }).click();

  // Both are in the list, in date order.
  await expect(page.getByRole("heading", { name: "Liturgies", level: 1 })).toBeVisible();
  const rows = page.getByRole("link", { name: /Ibadah (Satu|Dua)/ });
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText("Ibadah Satu");
  await expect(rows.nth(1)).toContainText("Ibadah Dua");

  // Clean up: the liturgies, then the services.
  const list = (await (await api.get("/api/v1/liturgies?limit=100")).json()) as { items: { id: string; service_name: string }[] };
  for (const l of list.items.filter((x) => /^Ibadah (Satu|Dua)$/.test(x.service_name))) {
    expect((await api.delete(`/api/v1/liturgies/${l.id}`, { data: {} })).status()).toBe(204);
  }
  for (const id of made) expect((await api.delete(`/api/v1/services/${id}`, { data: {} })).status()).toBe(204);
  await api.dispose();
});

// The editor and the pages around it fit a phone, and a second session that
// saved first is reported without losing what was typed (11 §4).
test("E2E-W-013 a conflict on an item keeps the typed text", async ({ browser }) => {
  const api = await adminApi();
  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Konflik", date: dateAhead(40), time: "09:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const l = (await created.json()) as { id: string; version: number };
  const added = await api.post(`/api/v1/liturgies/${l.id}/items`, { data: { liturgy_version: l.version, title: "Votum", item_type: "free_text" } });
  expect(added.status(), await added.text()).toBe(201);

  const one = await adminPage(browser);
  const two = await adminPage(browser);
  await one.goto(`/liturgies/${l.id}`);
  await two.goto(`/liturgies/${l.id}`);
  const first = card(one, /^Item 1: Votum/);
  const second = card(two, /^Item 1: Votum/);
  await second.getByLabel("Text").fill("Teks dari sesi dua");
  await second.getByRole("button", { name: "Save item" }).click();
  await expect(second.getByText("Saved", { exact: true })).toBeVisible();

  await first.getByLabel("Text").fill("Teks dari sesi satu");
  await first.getByRole("button", { name: "Save item" }).click();
  await expect(first.getByText(/changed by someone else/)).toBeVisible();
  await expect(first.getByLabel("Text")).toHaveValue("Teks dari sesi satu");
  await first.getByRole("button", { name: "Show their version" }).click();
  await expect(first.getByText("Teks dari sesi dua")).toBeVisible();
  await first.getByRole("button", { name: "Keep mine and save again" }).click();
  await expect(first.getByText("Saved", { exact: true })).toBeVisible();
  await one.reload();
  await expect(card(one, /^Item 1: Votum/).getByLabel("Text")).toHaveValue("Teks dari sesi satu");

  await api.delete(`/api/v1/liturgies/${l.id}`, { data: {} });
  await api.dispose();
});

// E2E-W-015: undo and redo with two people on one liturgy (11 §7.2).
test("E2E-W-015 undo and redo, refused after a colleague's change, and with the keyboard", async ({ browser }) => {
  const api = await adminApi();
  const roles = (await (await api.get("/api/v1/roles")).json()) as { id: string; origin: string | null }[];
  const editor = roles.find((r) => r.origin === "editor")!;
  const link = await createInvite("Budi Editor", "budi.undo@example.org", [editor.id]);
  const anon = await request.newContext({ baseURL });
  const accepted = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name: "Budi Editor", email: "budi.undo@example.org", password: memberPassword } });
  expect(accepted.status(), await accepted.text()).toBe(201);
  await anon.dispose();

  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Batal", date: dateAhead(45), time: "09:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  let { id, version } = (await created.json()) as { id: string; version: number };
  for (const title of ["Doa Satu", "Doa Dua"]) {
    const res = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title, item_type: "free_text" } });
    expect(res.status(), await res.text()).toBe(201);
    version = ((await res.json()) as { liturgy_version: number }).liturgy_version;
  }

  const ruth = await adminPage(browser);
  const budi = await logIn(browser, "budi.undo@example.org", memberPassword);
  await ruth.goto(`/liturgies/${id}`);
  const one = (page: Page) => card(page, /^Item \d+: Doa Satu/);
  const two = (page: Page) => card(page, /^Item \d+: Doa Dua/);
  const undo = ruth.getByRole("button", { name: "Undo", exact: true });
  const redo = ruth.getByRole("button", { name: "Redo", exact: true });
  let undoRequests = 0;
  ruth.on("request", (r) => { if (r.method() === "POST" && r.url().endsWith("/undo")) undoRequests++; });
  const save = async (c: ReturnType<typeof card>, text: string) => {
    await c.getByLabel("Text").fill(text);
    await c.getByRole("button", { name: "Save item" }).click();
    await expect(c.getByText("Saved", { exact: true })).toBeVisible();
  };

  // Undo and redo with the buttons.
  await save(one(ruth), "satu");
  await undo.click();
  await expect(ruth.getByText(/^Undid: the change to Doa Satu/)).toBeVisible();
  await expect(one(ruth).getByLabel("Text")).toHaveValue("");
  await expect(redo).toBeEnabled();
  await redo.click();
  await expect(ruth.getByText(/^Redid: the change to Doa Satu/)).toBeVisible();
  await expect(one(ruth).getByLabel("Text")).toHaveValue("satu");

  // A colleague changes the same item: Undo says why it cannot, and keeps their text.
  await budi.goto(`/liturgies/${id}`);
  await save(one(budi), "dari Budi");
  await undo.click();
  await expect(ruth.getByText("Someone has changed this since, so it can't be undone.")).toBeVisible();
  await ruth.reload();
  await expect(one(ruth).getByLabel("Text")).toHaveValue("dari Budi");

  // A move is undone although the colleague edited another item meanwhile.
  await ruth.getByRole("button", { name: /^Move up Item \d+: Doa Dua/ }).click();
  await expect(ruth.getByRole("form", { name: /^Item 1: Doa Dua/ })).toBeVisible();
  await budi.reload();
  await save(one(budi), "Budi lagi");
  await undo.click();
  await expect(ruth.getByText(/^Undid: the reordering of the items/)).toBeVisible();
  await expect(ruth.getByRole("form", { name: /^Item 1: Doa Satu/ })).toBeVisible();

  // The keyboard works outside a text field and leaves the text field alone.
  await ruth.reload();
  await save(two(ruth), "dua");
  await ruth.getByRole("heading", { name: "Ibadah Batal", level: 2 }).click();
  await ruth.keyboard.press("Control+z");
  await expect(ruth.getByText(/^Undid: the change to Doa Dua/)).toBeVisible();
  await expect(two(ruth).getByLabel("Text")).toHaveValue("");
  await ruth.keyboard.press("Control+Shift+z");
  await expect(ruth.getByText(/^Redid: the change to Doa Dua/)).toBeVisible();
  await expect(two(ruth).getByLabel("Text")).toHaveValue("dua");
  await two(ruth).getByLabel("Text").focus();
  const before = undoRequests;
  await ruth.keyboard.press("Control+z"); // the browser's own undo, not the server's
  await ruth.waitForTimeout(300);
  expect(undoRequests).toBe(before);

  await api.delete(`/api/v1/liturgies/${id}`, { data: {} });
  await api.dispose();
});
