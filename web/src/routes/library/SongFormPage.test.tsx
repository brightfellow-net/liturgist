// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editor, song, summary, viewer } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { SongFormPage } from "./SongFormPage";

afterEach(() => vi.unstubAllGlobals());

const group = (n: number) => screen.getByRole("group", { name: new RegExp(`^Section ${n}:`) });

describe("SongFormPage: sections editor (TC-S web)", () => {
  void i18n.changeLanguage("en");

  async function openNew(extra: Parameters<typeof mockApi>[0] = {}) {
    const calls = mockApi({
      "GET /songs": { status: 200, body: { items: [], total: 0 } },
      "POST /songs": { status: 201, body: song({ id: "new" }) },
      "GET /songs/new": { status: 200, body: song({ id: "new" }) },
      ...extra,
    });
    renderPage("/library/songs/new", "/library/songs/new", <SongFormPage />, editor);
    await screen.findByRole("heading", { name: "Add a song", level: 1 });
    return calls;
  }

  it("reorders and removes sections with buttons, and saves them in that order", async () => {
    const calls = await openNew();
    await userEvent.type(screen.getByLabelText("Title"), "Lagu baru");
    const add = screen.getByRole("button", { name: "Add a section" });
    await userEvent.click(add);
    await userEvent.type(within(group(1)).getByLabelText("Lyrics"), "Satu");
    await userEvent.click(add);
    await userEvent.type(within(group(2)).getByLabelText("Lyrics"), "Dua");
    await userEvent.click(add);
    await userEvent.type(within(group(3)).getByLabelText("Lyrics"), "Tiga");
    // Verse numbers are proposed one after another.
    expect(within(group(3)).getByLabelText("Verse number")).toHaveValue("3");

    await userEvent.click(within(group(3)).getByRole("button", { name: "Move up Section 3" }));
    expect(within(group(2)).getByLabelText("Lyrics")).toHaveValue("Tiga");
    expect(screen.getByText("Verse 3 moved to position 2 of 3.")).toBeInTheDocument();
    expect(within(group(1)).getByRole("button", { name: "Move up Section 1" })).toBeDisabled();
    expect(within(group(3)).getByRole("button", { name: "Move down Section 3" })).toBeDisabled();

    await userEvent.click(within(group(1)).getByRole("button", { name: "Remove Section 1" }));
    expect(screen.getByText("Verse 1 removed.")).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: /^Section 3:/ })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Save song" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /songs")).toBe(true));
    const body = calls.find((c) => c.route === "POST /songs")!.body as { sections: { key: string; number: number; text: string }[] };
    expect(body.sections.map((s) => [s.text, s.number])).toEqual([["Tiga", 3], ["Dua", 2]]);
    expect(body.sections.every((s) => s.key !== "")).toBe(true);
  });

  it("checks the verse numbers and lyrics before sending anything", async () => {
    const calls = await openNew();
    await userEvent.type(screen.getByLabelText("Title"), "Lagu baru");
    const add = screen.getByRole("button", { name: "Add a section" });
    await userEvent.click(add);
    await userEvent.click(add);
    await userEvent.clear(within(group(2)).getByLabelText("Verse number"));
    await userEvent.type(within(group(2)).getByLabelText("Verse number"), "1");
    await userEvent.click(screen.getByRole("button", { name: "Save song" }));
    expect((await screen.findAllByText("Enter the lyrics of this section."))).toHaveLength(2);
    expect(screen.getByText("Another verse already has this number.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /songs")).toBe(false);
  });

  it("builds the default order from the sections and drops it when a section is removed", async () => {
    const calls = await openNew();
    await userEvent.type(screen.getByLabelText("Title"), "Lagu baru");
    const add = screen.getByRole("button", { name: "Add a section" });
    await userEvent.click(add);
    await userEvent.type(within(group(1)).getByLabelText("Lyrics"), "Satu");
    await userEvent.click(add);
    await userEvent.selectOptions(within(group(2)).getByLabelText("Kind"), "chorus");
    await userEvent.type(within(group(2)).getByLabelText("Lyrics"), "Reff");
    expect(within(group(2)).queryByLabelText("Verse number")).toBeNull();

    const order = screen.getByRole("region", { name: "Default order" });
    const addToOrder = within(order).getByRole("button", { name: "Add to the order" });
    await userEvent.selectOptions(within(order).getByLabelText("Section to add"), "1. Verse 1");
    await userEvent.click(addToOrder);
    await userEvent.selectOptions(within(order).getByLabelText("Section to add"), "2. Chorus");
    await userEvent.click(addToOrder);
    await userEvent.click(addToOrder);
    expect(within(order).getAllByRole("listitem").map((li) => li.textContent?.split("Move")[0])).toEqual(["Verse 1", "Chorus", "Chorus"]);

    await userEvent.click(within(order).getByRole("button", { name: /^Move up Chorus 2/ }));
    expect(within(order).getAllByRole("listitem").map((li) => li.textContent?.split("Move")[0])).toEqual(["Chorus", "Verse 1", "Chorus"]);

    await userEvent.click(within(group(1)).getByRole("button", { name: "Remove Section 1" }));
    expect(within(order).getAllByRole("listitem").map((li) => li.textContent?.split("Move")[0])).toEqual(["Chorus", "Chorus"]);

    await userEvent.click(within(order).getByRole("button", { name: "Use all sections in order" }));
    expect(within(order).getByText("No order defined: all sections are used in order.")).toBeInTheDocument();
    await userEvent.click(within(order).getByRole("button", { name: "Add to the order" }));
    await userEvent.click(within(order).getByRole("button", { name: "Add to the order" }));

    await userEvent.click(screen.getByRole("button", { name: "Save song" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /songs")).toBe(true));
    const body = calls.find((c) => c.route === "POST /songs")!.body as { sections: { key: string }[]; default_arrangement: string[] };
    expect(body.default_arrangement).toEqual([body.sections[0].key, body.sections[0].key]);
  });

  it("warns about a hymnal number that is already in the library but still saves", async () => {
    const calls = await openNew({
      "GET /songs": { status: 200, body: { items: [summary("s9", "Besar Setia-Mu", { hymnal_source: "KJ", hymnal_number: "12" })], total: 1 } },
    });
    await userEvent.type(screen.getByLabelText("Title"), "Lagu baru");
    await userEvent.type(screen.getByLabelText("Hymnal", { selector: "input" }), "KJ");
    await userEvent.type(screen.getByLabelText("Number in the hymnal"), "12");
    const link = await screen.findByRole("link", { name: "Besar Setia-Mu" }, { timeout: 3000 });
    expect(link).toHaveAttribute("href", "/library/songs/s9");
    await userEvent.click(screen.getByRole("button", { name: "Save song" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /songs")).toBe(true));
    expect((calls.find((c) => c.route === "POST /songs")!.body as { hymnal_number: string }).hymnal_number).toBe("12");
  });

  it("sends the visitor without edit permission back to the library", async () => {
    mockApi({});
    const router = renderPage("/library/songs/new", "/library/songs/new", <SongFormPage />, viewer);
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library"));
  });
});

describe("SongFormPage: editing and version conflicts", () => {
  void i18n.changeLanguage("en");

  it("sends the loaded version, keeps section IDs, and keeps the input after a conflict until Reload", async () => {
    let patches = 0;
    const calls = mockApi({
      "GET /songs/s1": () => (patches === 0
        ? { status: 200, body: song() }
        : { status: 200, body: song({ title: "Judul dari orang lain", version: 4 }) }),
      "GET /songs": { status: 200, body: { items: [], total: 0 } },
      "PATCH /songs/s1": () => {
        patches += 1;
        return { status: 409, body: { code: "version_conflict" } };
      },
    });
    renderPage("/library/songs/:id/edit", "/library/songs/s1/edit", <SongFormPage />, editor);
    const title = await screen.findByLabelText("Title");
    expect(title).toHaveValue("Besar Setia-Mu");
    await userEvent.clear(title);
    await userEvent.type(title, "Judul saya");
    await userEvent.click(screen.getByRole("button", { name: "Save song" }));

    const alert = await screen.findByText(/This song was changed by someone else while you were editing/);
    expect(alert).toBeInTheDocument();
    const sent = calls.find((c) => c.route === "PATCH /songs/s1")!.body as { version: number; sections: { id: string }[]; title: string };
    expect(sent.version).toBe(3);
    expect(sent.title).toBe("Judul saya");
    expect(sent.sections.map((s) => s.id)).toEqual(["a1", "a2", "a3"]);
    expect(screen.getByLabelText("Title")).toHaveValue("Judul saya"); // input kept

    await userEvent.click(screen.getByRole("button", { name: "Reload" }));
    await vi.waitFor(() => expect(screen.getByLabelText("Title")).toHaveValue("Judul dari orang lain"));
    expect(screen.queryByText(/changed by someone else while/)).toBeNull();
  });

  it("does not offer the form when the song's actions say it cannot be edited", async () => {
    mockApi({ "GET /songs/s1": { status: 200, body: song({ actions: { edit: false, delete: false } }) } });
    const router = renderPage("/library/songs/:id/edit", "/library/songs/s1/edit", <SongFormPage />, editor);
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/songs/s1"));
  });
});
