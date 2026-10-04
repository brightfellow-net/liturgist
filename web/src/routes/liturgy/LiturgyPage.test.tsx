// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { duties, editorMe, liturgy, noEdits, noParts, songItem, textItem } from "@/test/liturgy";
import { mockApi, renderPage } from "@/test/render";
import { LiturgyPage } from "./LiturgyPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const base = (l = liturgy()) => ({
  "GET /liturgies/l1": { status: 200, body: l },
  "GET /duties": duties,
  "GET /singing-parts": noParts,
  "GET /liturgies/l1/edits": noEdits,
  "GET /liturgies/assignable": { status: 200, body: { items: [{ user_id: "u2", name: "Budi" }] } },
});
const page = () => renderPage("/liturgies/:id", "/liturgies/l1", <LiturgyPage />, editorMe);
const card = (name: RegExp | string) => screen.findByRole("form", { name });
const conflict = { status: 409, body: { code: "version_conflict", scope: "item", item_id: "i1" } };

describe("TC-E-002 item form state", () => {
  it("saves with the version it loaded and shows Saved", async () => {
    const calls = mockApi({
      ...base(),
      "PATCH /liturgies/l1/items/i1": { status: 200, body: { item: textItem({ text: "Selamat siang", version: 3 }), liturgy_version: 5 } },
    });
    page();
    const votum = await card(/^Item 1/);
    await userEvent.clear(within(votum).getByLabelText("Text"));
    await userEvent.type(within(votum).getByLabelText("Text"), "Selamat siang");
    expect(within(votum).getByText("Unsaved changes")).toBeInTheDocument();
    await userEvent.click(within(votum).getByRole("button", { name: "Save item" }));
    expect(await within(votum).findByText("Saved")).toBeInTheDocument();
    expect(calls.find((c) => c.route.startsWith("PATCH"))!.body).toEqual({ version: 2, title: "Votum", duty_id: "", text: "Selamat siang" });
  });

  it("keeps what was typed on a conflict and sends it again with their version", async () => {
    let attempt = 0;
    const theirs = textItem({ text: "Teks mereka", version: 4 });
    const calls = mockApi({
      ...base(),
      "PATCH /liturgies/l1/items/i1": (body) => {
        attempt++;
        return attempt === 1 ? conflict : { status: 200, body: { item: textItem({ text: (body as { text: string }).text, version: 5 }), liturgy_version: 5 } };
      },
    });
    page();
    const votum = await card(/^Item 1/);
    await userEvent.clear(within(votum).getByLabelText("Text"));
    await userEvent.type(within(votum).getByLabelText("Text"), "Teks saya");
    await userEvent.click(within(votum).getByRole("button", { name: "Save item" }));
    expect(await within(votum).findByText(/changed by someone else/)).toBeInTheDocument();
    expect(within(votum).getByLabelText("Text")).toHaveValue("Teks saya");

    // Their version is read from the server and shown beside the typed text.
    mockApi({ ...base(liturgy({ items: [theirs, songItem()] })), "PATCH /liturgies/l1/items/i1": (body) => ({ status: 200, body: { item: textItem({ text: (body as { text: string }).text, version: 5 }), liturgy_version: 5 } }) });
    await userEvent.click(within(votum).getByRole("button", { name: "Show their version" }));
    expect(await within(votum).findByText("Teks mereka")).toBeInTheDocument();
    expect(within(votum).getByLabelText("Text")).toHaveValue("Teks saya");

    const again = mockApi({ ...base(liturgy({ items: [theirs, songItem()] })), "PATCH /liturgies/l1/items/i1": (body) => ({ status: 200, body: { item: textItem({ text: (body as { text: string }).text, version: 5 }), liturgy_version: 5 } }) });
    await userEvent.click(within(votum).getByRole("button", { name: "Keep mine and save again" }));
    expect(await within(votum).findByText("Saved")).toBeInTheDocument();
    expect(again.find((c) => c.route.startsWith("PATCH"))!.body).toMatchObject({ version: 4, text: "Teks saya" });
    expect(calls.filter((c) => c.route.startsWith("PATCH"))).toHaveLength(1);
  });

  it("does not replace unsaved input when the liturgy is reloaded", async () => {
    mockApi(base());
    page();
    const votum = await card(/^Item 1/);
    await userEvent.type(within(votum).getByLabelText("Text"), "!");
    expect(within(votum).getByLabelText("Text")).toHaveValue("Selamat pagi!");
  });
});

describe("TC-E-003 sequence editor", () => {
  const open = async () => {
    const calls = mockApi({
      ...base(),
      "PUT /liturgies/l1/items/i2/songs": (body) => ({ status: 200, body: { item: songItem({ version: 4 }), liturgy_version: 5, echo: body } }),
      "GET /songs/s1": { status: 200, body: { id: "s1", title: "Besar Setia-Mu", sections: [{ id: "a1", kind: "verse", number: 1, label: null, text: "Besar setia-Mu" }, { id: "a2", kind: "chorus", number: null, label: null, text: "Setiap pagi" }], default_arrangement: ["a1", "a2", "a1"] } },
    });
    page();
    return { calls, pujian: await card(/^Item 2/) };
  };

  it("moves and removes entries and sends the whole song list", async () => {
    const { calls, pujian } = await open();
    await userEvent.click(within(pujian).getByRole("button", { name: "Move down Entry 1" }));
    expect(within(pujian).getByText(/^Entry 1: Chorus/)).toBeInTheDocument();
    await userEvent.click(within(pujian).getByRole("button", { name: "Remove Entry 2" }));
    await userEvent.type(within(pujian).getByLabelText("Key"), "A");
    await userEvent.click(within(pujian).getByRole("button", { name: "Save item" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route.startsWith("PUT"))).toBe(true));
    expect(calls.find((c) => c.route.startsWith("PUT"))!.body).toEqual({
      version: 3,
      songs: [{ song_id: "s1", key: "GA", note: "", entries: [{ section_id: "a2", key_change: "", note: "" }] }],
    });
  });

  it("offers only the sections of the song and fills from the default order", async () => {
    const { pujian } = await open();
    const select = within(pujian).getAllByLabelText("Section")[0];
    expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Verse 1", "Chorus", "Verse 2"]);
    await userEvent.click(within(pujian).getByRole("button", { name: "Fill from the default order" }));
    expect(await within(pujian).findByText(/^Entry 3:/)).toBeInTheDocument();
  });

  it("shows the lyrics of a section on demand", async () => {
    const { pujian } = await open();
    await userEvent.click(within(pujian).getAllByRole("button", { name: "Show lyrics" })[0]);
    expect(await within(pujian).findByText("Besar setia-Mu")).toBeInTheDocument();
  });

  it("shows the key in Do words next to the input", async () => {
    mockApi(base());
    renderPage("/liturgies/:id", "/liturgies/l1", <LiturgyPage />, { ...editorMe, church: { ...editorMe.church!, key_display: "do" } });
    const pujian = await card(/^Item 2/);
    expect(within(pujian).getByText("Do = G")).toBeInTheDocument();
  });
});

describe("structure", () => {
  it("moves an item with the liturgy version and announces it", async () => {
    const calls = mockApi({ ...base(), "PUT /liturgies/l1/items/order": { status: 200, body: { liturgy_version: 6, item_ids: ["i2", "i1"] } } });
    page();
    await userEvent.click(await screen.findByRole("button", { name: /^Move down Item 1/ }));
    await vi.waitFor(() => expect(screen.getAllByText("Votum moved to place 2 of 2.").length).toBeGreaterThan(0));
    expect(calls.find((c) => c.route.startsWith("PUT"))!.body).toEqual({ liturgy_version: 5, item_ids: ["i2", "i1"] });
    expect(screen.getAllByRole("form")[0]).toHaveAccessibleName(/^Item 1: Pujian/);
  });

  it("reloads the list and says so when the order was changed by someone else", async () => {
    mockApi({ ...base(), "PUT /liturgies/l1/items/order": { status: 409, body: { code: "version_conflict", scope: "liturgy" } } });
    page();
    await userEvent.click(await screen.findByRole("button", { name: /^Move down Item 1/ }));
    expect((await screen.findAllByText(/changed by someone else. The list was reloaded/)).length).toBeGreaterThan(0);
  });

  it("adds an item at the end", async () => {
    const calls = mockApi({ ...base(), "POST /liturgies/l1/items": { status: 201, body: { item: textItem({ id: "i3", title: "Doa", item_type: "prayer", position: 2, version: 1 }), liturgy_version: 6 } } });
    page();
    const form = (await screen.findByRole("heading", { name: "Add an item" })).closest("form")!;
    await userEvent.type(within(form).getByLabelText("Title"), "Doa");
    await userEvent.selectOptions(within(form).getByLabelText("Kind"), "prayer");
    await userEvent.click(screen.getByRole("button", { name: "Add item" }));
    expect(await screen.findByRole("form", { name: /^Item 3: Doa/ })).toBeInTheDocument();
    expect(calls.find((c) => c.route === "POST /liturgies/l1/items")!.body).toEqual({ liturgy_version: 5, title: "Doa", item_type: "prayer" });
  });
});

describe("read-only and problems", () => {
  it("shows a locked liturgy without forms", async () => {
    mockApi(base(liturgy({ state: "in_review", actions: { edit: false, delete: false, submit: false, approve: false, request_changes: false, reopen: false } })));
    page();
    expect(await screen.findByText(/This liturgy is being reviewed/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save item" })).toBeNull();
    expect(screen.getByRole("article", { name: /^Item 1: Votum/ })).toBeInTheDocument();
  });

  it("lists problems in words with a link to the item", async () => {
    mockApi(base(liturgy({ problems: [{ code: "song_missing", item_id: "i2" }] })));
    page();
    const link = await screen.findByRole("link", { name: "Item 2 (Pujian) has no song yet." });
    expect(link).toHaveAttribute("href", "#item-i2");
  });

  it("explains a missing liturgy", async () => {
    mockApi({ ...base(), "GET /liturgies/l1": { status: 404, body: { code: "not_found" } } });
    page();
    expect(await screen.findByText("This liturgy does not exist or was deleted.")).toBeInTheDocument();
  });
});

describe("team", () => {
  it("assigns a member and a free-text name and removes one", async () => {
    const calls = mockApi({
      ...base(liturgy({ items: [textItem({ duty_id: "d1" })] })),
      "POST /liturgies/l1/assignments": (body) => ({ status: 201, body: { id: "as1", duty_id: "d1", user_id: (body as { user_id?: string }).user_id ?? null, name: (body as { name?: string }).name ?? "Budi", former_member: false } }),
      "DELETE /liturgies/l1/assignments/as1": { status: 204 },
    });
    page();
    await screen.findByRole("option", { name: "Budi" });
    await userEvent.selectOptions(await screen.findByLabelText("Person (Liturgis)"), "u2");
    await userEvent.click(screen.getByRole("button", { name: "Add person (Liturgis)" }));
    const removeBudi = await screen.findByRole("button", { name: "Remove Budi (Liturgis)" });
    expect(calls.find((c) => c.route.startsWith("POST /liturgies/l1/assign"))!.body).toEqual({ duty_id: "d1", user_id: "u2" });
    await userEvent.click(removeBudi);
    await vi.waitFor(() => expect(screen.queryByRole("button", { name: "Remove Budi (Liturgis)" })).toBeNull());
  });
});
