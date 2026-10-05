// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { expect, request, test, type Page } from "@playwright/test";
import { baseURL } from "./env";
import { adminApi, adminPage, createInvite, createReading, createSong, expectAccessible, logIn, memberPassword, tokenOf } from "./helpers";

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

// E2E-W-016: the review round (12 §2): an editor submits, the liturgist sends it
// back with a note, the editor fixes and resubmits, the liturgist approves and reopens.
test("E2E-W-016 submit, request changes, resubmit, approve and reopen", async ({ browser }) => {
  const api = await adminApi();
  const roles = (await (await api.get("/api/v1/roles")).json()) as { id: string; origin: string | null }[];
  const editor = roles.find((r) => r.origin === "editor")!;
  const link = await createInvite("Sari Editor", "sari.review@example.org", [editor.id]);
  const anon = await request.newContext({ baseURL });
  const accepted = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name: "Sari Editor", email: "sari.review@example.org", password: memberPassword } });
  expect(accepted.status(), await accepted.text()).toBe(201);
  await anon.dispose();

  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Tinjau", date: dateAhead(50), time: "10:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const { id, version } = (await created.json()) as { id: string; version: number };
  const item = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title: "Doa Pembuka", item_type: "free_text" } });
  expect(item.status(), await item.text()).toBe(201);

  const sari = await logIn(browser, "sari.review@example.org", memberPassword);
  const lead = await adminPage(browser);
  await sari.goto(`/liturgies/${id}`);
  await lead.goto(`/liturgies/${id}`);
  const saveButton = (p: Page) => p.getByRole("button", { name: "Save item" });

  // The editor submits; the form turns read-only and the history says so.
  await expect(saveButton(sari)).toBeVisible();
  await expect(sari.getByRole("button", { name: "Approve" })).toHaveCount(0);
  await sari.getByRole("button", { name: "Submit for review" }).click();
  await expect(sari.getByText(/This liturgy is being reviewed/)).toBeVisible();
  await expect(saveButton(sari)).toHaveCount(0);
  await expect(sari.getByText("Sari Editor: Draft → In review")).toBeVisible();
  await expectAccessible(sari);

  // The liturgist comments on the item while the liturgy is read-only, then sends it back with a note.
  await lead.reload();
  await lead.getByRole("button", { name: "Comment on Doa Pembuka" }).click();
  const commentForm = lead.getByRole("group", { name: "Comment on Doa Pembuka" });
  await commentForm.getByLabel(/^Comment/).fill("Doanya terlalu pendek");
  await commentForm.getByRole("button", { name: "Add comment" }).click();
  await expect(lead.getByText("Comment added.").first()).toBeVisible();
  await expect(lead.getByText("1 open comment").first()).toBeVisible();
  await expectAccessible(lead);
  await lead.getByRole("button", { name: "Request changes" }).click();
  await lead.getByLabel(/Note/).fill("Tambahkan doa syafaat");
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(lead.getByText("Sent back for changes.").first()).toBeVisible();

  // The editor sees the note, works again, and resubmits.
  await sari.reload();
  await expect(sari.getByText("Admin wrote: Tambahkan doa syafaat")).toBeVisible();
  await expect(saveButton(sari)).toBeVisible();
  const itemComments = sari.getByRole("group", { name: "Comments on Doa Pembuka" });
  await expect(itemComments.getByText("Doanya terlalu pendek")).toBeVisible();
  await itemComments.getByRole("button", { name: "Resolve" }).click();
  await expect(sari.getByText("Comment resolved.").first()).toBeVisible();
  await expect(sari.getByText("1 resolved comment").first()).toBeVisible();
  await sari.getByRole("button", { name: "Submit for review" }).click();
  await expect(sari.getByText(/This liturgy is being reviewed/)).toBeVisible();

  // The liturgist's page from before the resubmit is not stale in content (nothing was edited), so approve works; then reopen.
  await lead.reload();
  await lead.getByRole("button", { name: "Approve", exact: true }).click();
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(lead.getByText(/Approved\. Reopen it to make changes/)).toBeVisible();
  await expectAccessible(lead);
  await lead.getByRole("button", { name: "Reopen as draft" }).click();
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(saveButton(lead)).toBeVisible();
  await expect(lead.getByText("Admin: Approved → Draft")).toBeVisible();

  await api.delete(`/api/v1/liturgies/${id}`, { data: {} });
  await api.dispose();
});

// E2E-W-017 (slice 5A): the liturgist approves and publishes, archives and unarchives,
// reopens; a team member with no scope cannot open the editable liturgy (13 §2, §4, §5).
test("E2E-W-017 publish, archive, unarchive and reopen", async ({ browser }) => {
  const api = await adminApi();
  const link = await createInvite("Tia Team", "tia.team@example.org", []);
  const anon = await request.newContext({ baseURL });
  const accepted = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name: "Tia Team", email: "tia.team@example.org", password: memberPassword } });
  expect(accepted.status(), await accepted.text()).toBe(201);
  await anon.dispose();

  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Terbit", date: dateAhead(60), time: "10:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const { id, version } = (await created.json()) as { id: string; version: number };
  const duties = (await (await api.get("/api/v1/duties")).json()) as { items: { id: string }[] };
  const item = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title: "Doa Pembuka", item_type: "prayer", duty_id: duties.items[0].id } });
  expect(item.status(), await item.text()).toBe(201);
  const assignable = (await (await api.get("/api/v1/liturgies/assignable")).json()) as { items: { user_id: string; name: string }[] };
  const tiaID = assignable.items.find((m) => m.name === "Tia Team")!.user_id;
  const assigned = await api.post(`/api/v1/liturgies/${id}/assignments`, { data: { duty_id: duties.items[0].id, user_id: tiaID } });
  expect(assigned.status(), await assigned.text()).toBe(201);

  const lead = await adminPage(browser);
  await lead.goto(`/liturgies/${id}`);
  await lead.getByRole("button", { name: "Submit for review" }).click();
  await expect(lead.getByText(/This liturgy is being reviewed/)).toBeVisible();
  await lead.getByRole("button", { name: "Approve", exact: true }).click();
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(lead.getByText(/Approved\. Reopen it to make changes/)).toBeVisible();

  await lead.getByRole("button", { name: "Publish" }).click();
  await lead.getByLabel(/Note/).fill("Shalom");
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(lead.getByText(/Published as version 1 on/).first()).toBeVisible();
  await expect(lead.getByRole("button", { name: "Delete this liturgy" })).toHaveCount(0);
  await expectAccessible(lead);

  // A team member with no role cannot open the editable liturgy: 404 in every state.
  const tia = await logIn(browser, "tia.team@example.org", memberPassword);
  const asTia = await tia.request.get(`/api/v1/liturgies/${id}`);
  expect(asTia.status()).toBe(404);

  // She reads the published copy instead: her card on the home page, then the view.
  await tia.goto("/");
  const card = tia.getByRole("listitem").filter({ hasText: "Ibadah Terbit" });
  await expect(card.getByText("Doa Pembuka")).toBeVisible();
  await expectAccessible(tia);
  await card.getByRole("link", { name: "Open the liturgy" }).click();
  await expect(tia.getByRole("heading", { name: "Ibadah Terbit", level: 1 })).toBeVisible();
  await expect(tia.getByRole("heading", { name: "Doa Pembuka", level: 2 })).toBeVisible();
  await expectAccessible(tia);
  await tia.getByRole("button", { name: "Large", exact: true }).click();
  await expect(tia.locator("html")).toHaveAttribute("data-text-size", "large");
  await tia.goto("/published");
  await expect(tia.getByRole("link", { name: /Ibadah Terbit/ })).toBeVisible();
  await expectAccessible(tia);

  await lead.getByRole("button", { name: "Archive", exact: true }).click();
  await expect(lead.getByText("Archived.").first()).toBeVisible();
  await expect(lead.getByText(/Archived\. This liturgy stays readable/)).toBeVisible();
  await lead.getByRole("button", { name: "Unarchive" }).click();
  await expect(lead.getByRole("button", { name: "Archive", exact: true })).toBeVisible();

  await lead.getByRole("button", { name: "Reopen as draft" }).click();
  await lead.getByRole("button", { name: "Send", exact: true }).click();
  await expect(lead.getByText(/Being revised\. The team still sees version 1/)).toBeVisible();
  await expect(lead.getByText("Admin: Published → Draft")).toBeVisible();

  // The team still reads version 1, marked as being revised.
  await tia.goto("/published");
  await expect(tia.getByText("Being revised")).toBeVisible();
  const stillThere = await tia.request.get(`/api/v1/liturgies/${id}/published`);
  expect(stillThere.status()).toBe(200);
  expect(((await stillThere.json()) as { revising: boolean; number: number }).revising).toBe(true);

  // It was published once, so it can never be deleted: the API says so and the button is gone.
  await expect(lead.getByRole("button", { name: "Delete this liturgy" })).toHaveCount(0);
  const del = await api.delete(`/api/v1/liturgies/${id}`, { data: {} });
  expect(del.status()).toBe(409);
  await api.dispose();
});

// E2E-W-018: the church's print settings, then the print view of a published liturgy.
test("E2E-W-018 print settings and the print view", async ({ browser }) => {
  const admin = await adminPage(browser);
  await admin.goto("/settings/church");
  await admin.getByLabel("Licence line").fill("CCLI License #7654321");
  await admin.getByLabel("Paper").selectOption("f4");
  await admin.getByLabel("Show notes").uncheck();
  await expectAccessible(admin);
  await admin.getByRole("button", { name: "Save" }).click();
  await expect(admin.getByText("Saved.")).toBeVisible();
  await admin.reload();
  await expect(admin.getByLabel("Licence line")).toHaveValue("CCLI License #7654321");
  await expect(admin.getByLabel("Paper")).toHaveValue("f4");
  await expect(admin.getByLabel("Show notes")).not.toBeChecked();

  const songId = await createSong({ title: "Besar Setia-Mu", hymnal_source: "KJ", hymnal_number: "12", sections: [{ kind: "verse", number: 1, text: "Besar setia-Mu\nTak berkesudahan" }] });
  const api = await adminApi();
  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Cetak", date: dateAhead(61), time: "08:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const { id, version } = (await created.json()) as { id: string; version: number };
  const duties = (await (await api.get("/api/v1/duties")).json()) as { items: { id: string }[] };
  const item = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title: "Pujian", item_type: "song", duty_id: duties.items[0].id } });
  expect(item.status(), await item.text()).toBe(201);
  const made = (await item.json()) as { item: { id: string; version: number } };
  const added = await api.post(`/api/v1/liturgies/${id}/items/${made.item.id}/songs`, { data: { version: made.item.version, song_id: songId } });
  expect(added.status(), await added.text()).toBe(201);
  let step = await (await api.post(`/api/v1/liturgies/${id}/submit`, { data: {} })).json();
  step = await (await api.post(`/api/v1/liturgies/${id}/approve`, { data: { edit_seq: step.edit_seq } })).json();
  const published = await api.post(`/api/v1/liturgies/${id}/publish`, { data: { edit_seq: step.edit_seq } });
  expect(published.status(), await published.text()).toBe(200);

  // From the published view to the print view; the church defaults are the starting point.
  await admin.goto(`/published/${id}`);
  await admin.getByRole("link", { name: "Print" }).click();
  await expect(admin.getByRole("heading", { name: "Ibadah Cetak", level: 1 })).toBeVisible();
  await expect(admin.getByLabel("Paper")).toHaveValue("f4");
  await expect(admin.getByText("CCLI License #7654321")).toBeVisible();
  await expect(admin.getByText("Tak berkesudahan")).toBeVisible();
  await expectAccessible(admin);

  // Choosing changes this page and its address, and keeps after a reload.
  await admin.getByLabel("Lyrics").selectOption("first_lines");
  await expect(admin.getByText("Tak berkesudahan")).toHaveCount(0);
  await admin.reload();
  await expect(admin.getByLabel("Lyrics")).toHaveValue("first_lines");
  expect(admin.url()).toContain("lyrics=first_lines");
  await admin.getByLabel("Sheet").selectOption("musician");
  await expect(admin.getByRole("heading", { name: /Besar Setia-Mu/, level: 3 })).toBeVisible();

  // On paper: no menu, no controls, and the paper size of the page.
  await admin.emulateMedia({ media: "print" });
  await expect(admin.getByRole("navigation")).toBeHidden();
  await expect(admin.getByRole("button", { name: "Print / Save as PDF" })).toBeHidden();
  await expect(admin.getByRole("heading", { name: "Ibadah Cetak", level: 1 })).toBeVisible();
  await admin.emulateMedia({ media: "screen" });

  // The church defaults did not change by choosing here.
  const church = (await (await api.get("/api/v1/church")).json()) as { print: { lyrics: string; paper: string } };
  expect(church.print).toMatchObject({ lyrics: "full", paper: "f4" });
  await api.dispose();
});

// E2E-W-019: reading mode, then the saved copy when the network is gone, and logging out clears it.
test("E2E-W-019 reading mode and offline use", async ({ browser }) => {
  const api = await adminApi();
  const link = await createInvite("Ola Offline", "ola.offline@example.org", []);
  const anon = await request.newContext({ baseURL });
  const accepted = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name: "Ola Offline", email: "ola.offline@example.org", password: memberPassword } });
  expect(accepted.status(), await accepted.text()).toBe(201);
  await anon.dispose();

  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Luring", date: dateAhead(62), time: "09:00", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const { id, version } = (await created.json()) as { id: string; version: number };
  const duties = (await (await api.get("/api/v1/duties")).json()) as { items: { id: string }[] };
  const item = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title: "Doa Syafaat", item_type: "prayer", duty_id: duties.items[0].id } });
  expect(item.status(), await item.text()).toBe(201);
  const assignable = (await (await api.get("/api/v1/liturgies/assignable")).json()) as { items: { user_id: string; name: string }[] };
  const olaID = assignable.items.find((m) => m.name === "Ola Offline")!.user_id;
  const assigned = await api.post(`/api/v1/liturgies/${id}/assignments`, { data: { duty_id: duties.items[0].id, user_id: olaID } });
  expect(assigned.status(), await assigned.text()).toBe(201);
  let step = await (await api.post(`/api/v1/liturgies/${id}/submit`, { data: {} })).json();
  step = await (await api.post(`/api/v1/liturgies/${id}/approve`, { data: { edit_seq: step.edit_seq } })).json();
  const published = await api.post(`/api/v1/liturgies/${id}/publish`, { data: { edit_seq: step.edit_seq } });
  expect(published.status(), await published.text()).toBe(200);
  await api.dispose();

  const ola = await logIn(browser, "ola.offline@example.org", memberPassword);
  const context = ola.context();
  // The worker takes control of the page once it is active; wait for that.
  await ola.goto("/");
  await ola.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
  await ola.reload();
  await expect.poll(() => ola.evaluate(() => !!navigator.serviceWorker.controller)).toBe(true);
  await expect(ola.getByRole("listitem").filter({ hasText: "Ibadah Luring" })).toBeVisible();

  // Reading mode: no menus, her part marked in words, "Go to my part" moves focus.
  await ola.goto(`/published/${id}`);
  await expect(ola.getByRole("heading", { name: "Ibadah Luring", level: 1 })).toBeVisible();
  await ola.getByRole("link", { name: "Reading mode" }).click();
  await expect(ola.getByText("Your part")).toBeVisible();
  await expect(ola.getByRole("navigation")).toHaveCount(0);
  await ola.getByRole("button", { name: "Go to my part" }).click();
  await expect(ola.getByRole("heading", { name: "Doa Syafaat" })).toBeFocused();
  await expectAccessible(ola);

  // The network goes: the saved view still opens, and says what it is.
  await context.setOffline(true);
  await ola.reload();
  await expect(ola.getByRole("heading", { name: "Ibadah Luring", level: 1 })).toBeVisible();
  await expect(ola.getByText(/Offline — showing the last saved copy, published/)).toBeVisible();
  // The app opens offline too: "My assignments" from the saved copy, nothing else.
  await ola.goto("/");
  await expect(ola.getByText("Offline — showing the last saved copy.")).toBeVisible();
  await expect(ola.getByRole("listitem").filter({ hasText: "Ibadah Luring" })).toBeVisible();
  await expect(ola.getByRole("link", { name: "Library" })).toHaveCount(0);
  await expectAccessible(ola);

  // Logging out offline clears the saved copies first.
  await ola.getByRole("button", { name: "Log out" }).click();
  await expect(ola.getByRole("heading", { name: /Log in/i })).toBeVisible();
  expect(await ola.evaluate(async () => (await caches.keys()).filter((n) => n.startsWith("pub-")))).toEqual([]);
  expect(await ola.evaluate(() => localStorage.getItem("liturgist.user"))).toBeNull();
});

// E2E-W-020: the WhatsApp texts after publishing, and after publishing again.
test("E2E-W-020 messages for the team", async ({ browser }) => {
  const api = await adminApi();
  const created = await api.post("/api/v1/liturgies", { data: { service_name: "Ibadah Pesan", date: dateAhead(63), time: "08:30", template_id: "", language: "id" } });
  expect(created.status(), await created.text()).toBe(201);
  const { id, version } = (await created.json()) as { id: string; version: number };
  const duties = (await (await api.get("/api/v1/duties")).json()) as { items: { id: string }[] };
  const item = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: version, title: "Doa Pembuka", item_type: "prayer", duty_id: duties.items[0].id } });
  expect(item.status(), await item.text()).toBe(201);
  const assigned = await api.post(`/api/v1/liturgies/${id}/assignments`, { data: { duty_id: duties.items[0].id, name: "Pak Joko" } });
  expect(assigned.status(), await assigned.text()).toBe(201);
  const publish = async () => {
    let step = await (await api.post(`/api/v1/liturgies/${id}/submit`, { data: {} })).json();
    step = await (await api.post(`/api/v1/liturgies/${id}/approve`, { data: { edit_seq: step.edit_seq } })).json();
    const res = await api.post(`/api/v1/liturgies/${id}/publish`, { data: { edit_seq: step.edit_seq } });
    expect(res.status(), await res.text()).toBe(200);
  };
  await publish();

  const admin = await adminPage(browser);
  await admin.goto(`/liturgies/${id}`);
  await admin.getByRole("link", { name: "Send to the team" }).click();
  await expect(admin.getByRole("heading", { name: "Send to the team", level: 1 })).toBeVisible();
  const team = admin.getByRole("textbox", { name: "Text for Team summary" });
  await expect(team).toHaveValue(/\*Liturgi Ibadah Pesan\*\n.*· 08:30/);
  await expect(team).toHaveValue(/Liturgis: Pak Joko/);
  await expect(admin.getByRole("link", { name: "Share to WhatsApp" })).toHaveAttribute("href", /^https:\/\/wa\.me\/\?text=\*Liturgi%20Ibadah%20Pesan/);
  await expect(admin.getByText(/first version, so there is nothing to compare/)).toBeVisible();
  await expect(admin.getByText("Name only, no account")).toBeVisible();
  await expectAccessible(admin);

  // Published again with one more item: the change summary names it.
  const reopened = await api.post(`/api/v1/liturgies/${id}/reopen`, { data: {} });
  expect(reopened.status(), await reopened.text()).toBe(200);
  const now = (await (await api.get(`/api/v1/liturgies/${id}`)).json()) as { version: number };
  const more = await api.post(`/api/v1/liturgies/${id}/items`, { data: { liturgy_version: now.version, title: "Penutup", item_type: "prayer" } });
  expect(more.status(), await more.text()).toBe(201);
  await publish();
  await admin.reload();
  await expect(admin.getByRole("textbox", { name: "Text for Change summary" })).toHaveValue(/\+ Penutup ditambahkan/);
  await expectAccessible(admin);

  // A member with no scope is refused, and has no link.
  const link = await createInvite("Mia Member", "mia.member@example.org", []);
  const anon = await request.newContext({ baseURL });
  const accepted = await anon.post("/api/v1/invites/accept", { data: { token: tokenOf(link), name: "Mia Member", email: "mia.member@example.org", password: memberPassword } });
  expect(accepted.status(), await accepted.text()).toBe(201);
  await anon.dispose();
  const mia = await logIn(browser, "mia.member@example.org", memberPassword);
  await mia.goto(`/published/${id}`);
  await expect(mia.getByRole("heading", { name: "Ibadah Pesan", level: 1 })).toBeVisible();
  await expect(mia.getByRole("link", { name: "Send to the team" })).toHaveCount(0);
  await mia.goto(`/published/${id}/messages`);
  await expect(mia.getByRole("alert")).toBeVisible();
  await expect(mia.getByRole("textbox")).toHaveCount(0);
  await api.dispose();
});
