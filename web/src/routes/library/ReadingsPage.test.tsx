// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editor, translations, viewer } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { ReadingsPage } from "./ReadingsPage";

afterEach(() => vi.unstubAllGlobals());

const summary = (id: string, canonical: string, snippet: string, code = "TB") => ({
  id, reference: "X", canonical, reference_display: canonical, snippet,
  translation: { code, name: "n", language: "id" }, actions: { edit: true, delete: true },
});
const open = (url = "/library/readings", me = editor) => renderPage("/library/readings", url, <ReadingsPage />, me);

describe("ReadingsPage", () => {
  void i18n.changeLanguage("en");

  it("explains the empty state and offers adding to an editor", async () => {
    mockApi({ "GET /readings": { status: 200, body: { items: [], total: 0 } }, "GET /translations": { status: 200, body: translations } });
    open();
    expect(await screen.findByRole("heading", { name: "No readings saved yet" })).toBeInTheDocument();
    expect(screen.getByText(/Type a reference and paste the text once/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a reading" })).toHaveAttribute("href", "/library/readings/new");
  });

  it("does not offer adding to a member who cannot edit", async () => {
    mockApi({ "GET /readings": { status: 200, body: { items: [], total: 0 } }, "GET /translations": { status: 200, body: translations } });
    open("/library/readings", viewer);
    expect(await screen.findByText(/A member who may edit the library can add them/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add a reading" })).toBeNull();
  });

  it("lists readings with their snippet and translation", async () => {
    mockApi({
      "GET /readings": { status: 200, body: { items: [summary("r1", "Yohanes 3:16", "Karena begitu besar"), summary("r2", "Mazmur 23", "TUHAN adalah gembalaku", "BIS")], total: 2 } },
      "GET /translations": { status: 200, body: translations },
    });
    open();
    const first = await screen.findByRole("link", { name: /Yohanes 3:16 \(TB\)/ });
    expect(first).toHaveAttribute("href", "/library/readings/r1");
    expect(first).toHaveTextContent("Karena begitu besar");
    expect(screen.getByRole("link", { name: /Mazmur 23 \(BIS\)/ })).toBeInTheDocument();
    expect(screen.getByText("2 readings found.")).toBeInTheDocument();
  });

  it("searches, filters by translation, and clears", async () => {
    const calls = mockApi({
      "GET /readings": { status: 200, body: { items: [summary("r1", "Yohanes 3:16", "x")], total: 1 } },
      "GET /translations": { status: 200, body: translations },
    });
    const router = open();
    await userEvent.type(await screen.findByLabelText("Search readings"), "gembala{Enter}");
    await vi.waitFor(() => expect(router.state.location.search).toBe("?q=gembala"));
    await userEvent.selectOptions(screen.getByLabelText("Translation"), "BIS");
    await vi.waitFor(() => expect(router.state.location.search).toBe("?q=gembala&translation=BIS"));
    expect(calls.some((c) => c.route === "GET /readings")).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Clear search and filters" }));
    await vi.waitFor(() => expect(router.state.location.search).toBe(""));
  });

  it("says when nothing matches", async () => {
    mockApi({ "GET /readings": { status: 200, body: { items: [], total: 0 } }, "GET /translations": { status: 200, body: translations } });
    open("/library/readings?q=zzz");
    expect(await screen.findByText(/No readings match/)).toBeInTheDocument();
  });
});
