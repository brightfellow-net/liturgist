// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editor, summary, viewer } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { LibraryPage } from "./LibraryPage";

afterEach(() => vi.unstubAllGlobals());

const open = (url = "/library", me = editor) => renderPage("/library", url, <LibraryPage />, me);

describe("LibraryPage", () => {
  void i18n.changeLanguage("en");

  it("explains an empty library to an editor with an action", async () => {
    mockApi({ "GET /songs": { status: 200, body: { items: [], total: 0 } } });
    open();
    expect(await screen.findByRole("heading", { name: "Your library is empty" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a song" })).toHaveAttribute("href", "/library/songs/new");
    expect(screen.getByRole("link", { name: "Paste lyrics" })).toHaveAttribute("href", "/library/import");
    expect(screen.getByRole("link", { name: "Import a file" })).toHaveAttribute("href", "/library/import");
    expect(screen.queryByRole("search")).toBeNull();
  });

  it("points to an unfinished import", async () => {
    mockApi({
      "GET /songs": { status: 200, body: { items: [summary("s1", "A")], total: 1 } },
      "GET /imports": { status: 200, body: { items: [{ id: "b1", source_format: "paste", created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z" }] } },
    });
    open();
    expect(await screen.findByText("You have an unfinished import.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Continue the import" })).toHaveAttribute("href", "/library/import/b1");
    expect(screen.getByRole("link", { name: "Import songs" })).toHaveAttribute("href", "/library/import");
  });

  it("does not ask about imports for a member who cannot edit", async () => {
    const calls = mockApi({ "GET /songs": { status: 200, body: { items: [summary("s1", "A")], total: 1 } } });
    open("/library", viewer);
    await screen.findByRole("link", { name: /A/ });
    expect(calls.some((c) => c.route === "GET /imports")).toBe(false);
  });

  it("tells a member who cannot edit to ask for help, without actions", async () => {
    mockApi({ "GET /songs": { status: 200, body: { items: [], total: 0 } } });
    open("/library", viewer);
    expect(await screen.findByText(/A member who may edit the library can add them/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add a song" })).toBeNull();
  });

  it("lists songs with hymnal number, language and licence", async () => {
    mockApi({
      "GET /songs": {
        status: 200,
        body: {
          items: [
            summary("s1", "Besar Setia-Mu", { hymnal_source: "KJ", hymnal_number: "12", licence_status: "public_domain", has_group: true, alt_titles: ["Great Is Thy Faithfulness"] }),
            summary("s2", "Amazing Grace", { language: "en" }),
          ],
          total: 2,
        },
      },
    });
    open();
    const first = (await screen.findByRole("link", { name: /Besar Setia-Mu/ })).closest("li")!;
    expect(within(first).getByText(/KJ 12 · Indonesian · Public domain · Also in other languages/)).toBeInTheDocument();
    expect(within(first).getByText("Great Is Thy Faithfulness")).toBeInTheDocument();
    expect(first.querySelector("a")).toHaveAttribute("href", "/library/songs/s1");
    expect(screen.getByText("2 songs found.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a song" })).toBeInTheDocument();
  });

  it("searches by the words typed and keeps them in the address", async () => {
    const calls = mockApi({
      "GET /songs": (): { status: number; body: unknown } => ({ status: 200, body: { items: [summary("s1", "Besar Setia-Mu")], total: 1 } }),
    });
    const router = open();
    await userEvent.type(await screen.findByLabelText("Search songs"), "setia{Enter}");
    await vi.waitFor(() => expect(router.state.location.search).toBe("?q=setia"));
    await vi.waitFor(() => expect(calls.filter((c) => c.route === "GET /songs").length).toBeGreaterThan(1));
    expect(screen.getByLabelText("Search songs")).toHaveValue("setia");
  });

  it("says when nothing matches and offers to clear the search", async () => {
    mockApi({ "GET /songs": { status: 200, body: { items: [], total: 0 } } });
    const router = open("/library?q=zzz&language=en");
    expect(await screen.findByText(/No songs match/)).toBeInTheDocument();
    expect(screen.getByLabelText("Language")).toHaveValue("en");
    await userEvent.click(screen.getByRole("button", { name: "Clear search and filters" }));
    await vi.waitFor(() => expect(router.state.location.search).toBe(""));
  });

  it("filters by language and goes back to the first page", async () => {
    mockApi({ "GET /songs": { status: 200, body: { items: [summary("s1", "A")], total: 60 } } });
    const router = open("/library?offset=50");
    await userEvent.selectOptions(await screen.findByLabelText("Language"), "en");
    await vi.waitFor(() => expect(router.state.location.search).toBe("?language=en"));
  });

  it("pages through long results", async () => {
    const calls = mockApi({
      "GET /songs": () => ({ status: 200, body: { items: [summary("s1", "Satu")], total: 120 } }),
    });
    const router = open();
    expect(await screen.findByText("Showing 1 to 1 of 120.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    await vi.waitFor(() => expect(router.state.location.search).toBe("?offset=1"));
    expect(calls.length).toBeGreaterThan(1);
  });

  it("shows a retry button when the search fails", async () => {
    mockApi({ "GET /songs": { status: 503, body: { code: "unavailable" } } });
    open();
    expect(await screen.findByText("The server is busy. Please try again.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});
