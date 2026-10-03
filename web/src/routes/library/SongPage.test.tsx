// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editor, song, summary } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { SongPage } from "./SongPage";

afterEach(() => vi.unstubAllGlobals());

const open = () => renderPage("/library/songs/:id", "/library/songs/s1", <SongPage />, editor);

describe("SongPage", () => {
  void i18n.changeLanguage("en");

  it("shows the lyrics section by section with derived names, and the default order", async () => {
    mockApi({ "GET /songs/s1": { status: 200, body: song() } });
    open();
    expect(await screen.findByRole("heading", { name: "Besar Setia-Mu", level: 1 })).toBeInTheDocument();
    expect(screen.getByText("Public domain", { selector: "p" })).toBeInTheDocument(); // credit line
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual(["Verse 1", "Chorus", "Verse 2"]);
    const order = screen.getByRole("region", { name: "Default order" });
    expect(within(order).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Verse 1", "Chorus", "Verse 2", "Chorus"]);
    expect(screen.getByText("KJ 12")).toBeInTheDocument();
    expect(screen.getByText("Great Is Thy Faithfulness", { exact: false })).toBeInTheDocument();
  });

  it("uses the custom label, and says all sections are used when no order is defined", async () => {
    mockApi({
      "GET /songs/s1": {
        status: 200,
        body: song({
          sections: [{ id: "a1", kind: "chorus", number: null, label: "Reff akhir", text: "Haleluya" }],
          default_arrangement: [],
        }),
      },
    });
    open();
    expect(await screen.findByRole("heading", { name: "Reff akhir", level: 3 })).toBeInTheDocument();
    expect(screen.getByText("All sections, in the order shown above.")).toBeInTheDocument();
  });

  it("shows no edit or delete buttons when the actions are false", async () => {
    mockApi({ "GET /songs/s1": { status: 200, body: song({ actions: { edit: false, delete: false } }) } });
    open();
    await screen.findByRole("heading", { name: "Besar Setia-Mu", level: 1 });
    expect(screen.queryByRole("link", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Link another version…" })).toBeNull();
  });

  it("asks before deleting and explains a song that is in use", async () => {
    const calls = mockApi({
      "GET /songs/s1": { status: 200, body: song() },
      "DELETE /songs/s1": { status: 409, body: { code: "song_in_use" } },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    expect(screen.getByText("Delete “Besar Setia-Mu”? Its lyrics are removed for good.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "DELETE /songs/s1")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Delete Besar Setia-Mu" }));
    expect(await screen.findByText("This song is used in a liturgy that isn't published yet, so it can't be deleted.")).toBeInTheDocument();
  });

  it("links other versions as links and links another song, explaining a conflict", async () => {
    const calls = mockApi({
      "GET /songs/s1": { status: 200, body: song({ versions: [{ id: "s2", title: "Great Is Thy Faithfulness", language: "en" }] }) },
      "GET /songs": { status: 200, body: { items: [summary("s1", "Besar Setia-Mu"), summary("s2", "Great Is Thy Faithfulness", { language: "en" }), summary("s3", "Setia-Mu Tuhan", { language: "zh-Hans" })], total: 3 } },
      "POST /songs/s1/link": { status: 409, body: { code: "group_conflict", reason: "language_taken" } },
    });
    open();
    const versions = (await screen.findByRole("heading", { name: "Other languages" })).closest("section")!;
    expect(within(versions).getByRole("link", { name: "Great Is Thy Faithfulness" })).toHaveAttribute("href", "/library/songs/s2");
    await userEvent.click(within(versions).getByRole("button", { name: "Link another version…" }));
    await userEvent.type(within(versions).getByLabelText("Find the song to link"), "setia{Enter}");
    // The song itself and the version already linked are not offered.
    const choice = await within(versions).findByRole("button", { name: /Link “Setia-Mu Tuhan”/ });
    expect(within(versions).queryByRole("button", { name: /Link “Besar Setia-Mu”/ })).toBeNull();
    expect(within(versions).queryByRole("button", { name: /Link “Great Is Thy Faithfulness”/ })).toBeNull();
    await userEvent.click(choice);
    expect(await within(versions).findByText("There is already a version in this language.")).toBeInTheDocument();
    expect(calls.find((c) => c.route === "POST /songs/s1/link")!.body).toEqual({ other_song_id: "s3" });
  });

  it("unlinks after asking", async () => {
    const calls = mockApi({
      "GET /songs/s1": { status: 200, body: song({ versions: [{ id: "s2", title: "Great Is Thy Faithfulness", language: "en" }] }) },
      "GET /songs": { status: 200, body: { items: [], total: 0 } },
      "DELETE /songs/s1/link": { status: 204 },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Unlink" }));
    expect(calls.some((c) => c.route === "DELETE /songs/s1/link")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Unlink" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "DELETE /songs/s1/link")).toBe(true));
  });

  it("says when the song does not exist", async () => {
    mockApi({ "GET /songs/s1": { status: 404, body: { code: "not_found" } } });
    open();
    expect(await screen.findByText("This song does not exist or was deleted.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the library" })).toHaveAttribute("href", "/library");
  });
});
