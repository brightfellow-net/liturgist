// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { batch, candidate, editor, viewer } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { ImportPage } from "./ImportPage";

afterEach(() => vi.unstubAllGlobals());

const open = (me = editor) => renderPage("/library/import", "/library/import", <ImportPage />, me);

describe("ImportPage", () => {
  void i18n.changeLanguage("en");

  it("offers three ways to import and says nothing is saved yet", () => {
    open();
    expect(screen.getByRole("heading", { name: "Paste lyrics", level: 2 })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "OpenLyrics files", level: 2 })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "ChordPro files", level: 2 })).toBeInTheDocument();
    expect(screen.getByText(/Nothing is saved to the library yet/)).toBeInTheDocument();
  });

  it("sends the pasted lyrics with the title and goes to the review", async () => {
    const calls = mockApi({ "POST /imports": { status: 201, body: batch([candidate("c1", "Besar Setia-Mu")]) } });
    const router = open();
    await userEvent.type(screen.getByLabelText("Title", { exact: true }), "Besar Setia-Mu");
    await userEvent.type(screen.getByLabelText("Lyrics", { exact: true }), "Besar setia-Mu");
    await userEvent.click(screen.getByRole("button", { name: "Read the lyrics" }));
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/import/b1"));
    expect(calls[0].body).toMatchObject({ format: "paste", language: "en", files: [{ name: "Besar Setia-Mu", text: "Besar setia-Mu" }] });
  });

  it("asks for the title and the lyrics before sending", async () => {
    const calls = mockApi({});
    open();
    await userEvent.click(screen.getByRole("button", { name: "Read the lyrics" }));
    expect(screen.getAllByText("This field is required.")).toHaveLength(2);
    expect(calls).toHaveLength(0);
  });

  it("sends the chosen files and keeps the ones the server rejected for the review", async () => {
    const calls = mockApi({
      "POST /imports": { status: 201, body: batch([candidate("c1", "A")], { rejected: [{ name: "b.xml", song_index: 0, reason: "not_xml" }] }) },
    });
    const router = open();
    const input = screen.getByLabelText("Choose OpenLyrics files");
    await userEvent.upload(input, [new File(["<song/>"], "a.xml", { type: "text/xml" }), new File(["x"], "b.xml", { type: "text/xml" })]);
    await userEvent.click(screen.getByRole("button", { name: "Read the OpenLyrics files" }));
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/import/b1"));
    expect(calls[0].body).toMatchObject({ format: "openlyrics", files: [{ name: "a.xml", text: "<song/>" }, { name: "b.xml", text: "x" }] });
    expect(router.state.location.state).toEqual({ rejected: [{ name: "b.xml", song_index: 0, reason: "not_xml" }] });
  });

  it("does not send a file that is not UTF-8", async () => {
    const calls = mockApi({});
    open();
    const bad = new File([new Uint8Array([0xff, 0xfe, 0x41])], "old.cho");
    await userEvent.upload(screen.getByLabelText("Choose ChordPro files"), bad);
    await userEvent.click(screen.getByRole("button", { name: "Read the ChordPro files" }));
    expect(await screen.findByText("None of these files can be read.")).toBeInTheDocument();
    expect(screen.getByText(/old\.cho: This file isn't plain text in UTF-8/)).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it("explains a file the server could not read", async () => {
    mockApi({ "POST /imports": { status: 422, body: { code: "import_unreadable", reason: "no_song" } } });
    open();
    await userEvent.upload(screen.getByLabelText("Choose OpenLyrics files"), new File(["x"], "a.xml"));
    await userEvent.click(screen.getByRole("button", { name: "Read the OpenLyrics files" }));
    expect(await screen.findByText("No song was found.")).toBeInTheDocument();
  });

  it("sends a member who may not edit back to the library", () => {
    const router = open(viewer);
    expect(router.state.location.pathname).toBe("/library");
  });
});
