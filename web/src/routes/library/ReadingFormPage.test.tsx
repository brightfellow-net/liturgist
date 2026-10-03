// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editorTB, lookup, reading, translations, viewer } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { ReadingFormPage } from "./ReadingFormPage";

afterEach(() => vi.unstubAllGlobals());


const ok = (reference: string, canonical: string) => ({ status: 200, body: { reference, canonical, display: canonical } });
const routes = (extra: Parameters<typeof mockApi>[0] = {}) =>
  mockApi({
    "GET /translations": { status: 200, body: translations },
    "GET /readings/parse": ok("JHN 3:16-21", "Yohanes 3:16-21"),
    "GET /readings/lookup": { status: 200, body: lookup() },
    "POST /readings": { status: 201, body: reading() },
    "GET /readings/r1": { status: 200, body: reading() },
    "GET /readings": { status: 200, body: { items: [], total: 0 } },
    ...extra,
  });
const open = (me = editorTB) => renderPage("/library/readings/new", "/library/readings/new", <ReadingFormPage />, me);

describe("ReadingFormPage (07 §5)", () => {
  void i18n.changeLanguage("en");

  it("shows how the typed reference was understood after a pause", async () => {
    const calls = routes();
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16-21");
    expect(calls.filter((c) => c.route === "GET /readings/parse")).toHaveLength(0); // still typing
    expect(await screen.findByText("Understood as: Yohanes 3:16-21", {}, { timeout: 3000 })).toBeInTheDocument();
    expect(calls.filter((c) => c.route === "GET /readings/parse")).toHaveLength(1);
  });

  it("checks at once when the field loses focus", async () => {
    routes();
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16-21");
    await userEvent.tab();
    expect(await screen.findByText("Understood as: Yohanes 3:16-21", {}, { timeout: 300 })).toBeInTheDocument();
  });

  it("explains why a reference was not understood", async () => {
    routes({ "GET /readings/parse": { status: 422, body: { code: "invalid_reference", reason: "unknown_book" } } });
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Foo 1");
    await userEvent.tab();
    expect(await screen.findByText("I don't know this book. Try the usual short name, e.g. Yoh or Mzm.")).toBeInTheDocument();
  });

  it("pre-selects the church's translation and fills the credit line once", async () => {
    routes({ "GET /readings/lookup": { status: 200, body: lookup({ suggested_attribution: "© LAI" }) } });
    open();
    expect(await screen.findByLabelText("Translation")).toHaveValue("TB");
    await userEvent.type(screen.getByLabelText("Bible reference"), "Yoh 3:16");
    await userEvent.tab();
    await vi.waitFor(() => expect(screen.getByLabelText("Credit line (optional)")).toHaveValue("© LAI"));
  });

  it("saves the typed text and goes to the reading", async () => {
    const calls = routes();
    const router = open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16-21");
    await userEvent.selectOptions(screen.getByLabelText("Translation"), "BIS");
    await userEvent.type(screen.getByLabelText("Text of the reading"), "Karena begitu besar");
    await userEvent.type(screen.getByLabelText("Credit line (optional)"), "LAI");
    await userEvent.click(screen.getByRole("button", { name: "Save reading" }));
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/readings/r1"));
    expect(calls.find((c) => c.route === "POST /readings")!.body).toEqual({
      reference: "Yoh 3:16-21", translation: "BIS", text: "Karena begitu besar", attribution: "LAI",
    });
  });

  it("needs the text before sending anything", async () => {
    const calls = routes();
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16");
    await userEvent.click(screen.getByRole("button", { name: "Save reading" }));
    expect(await screen.findByText("Enter the text of the reading.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /readings")).toBe(false);
  });

  it("says the reading exists, with a link, instead of the form", async () => {
    routes({ "GET /readings/lookup": { status: 200, body: lookup({ reading: reading() }) } });
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yohanes 3:16-21");
    await userEvent.tab();
    expect(await screen.findByText("You already saved this reading.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the saved reading" })).toHaveAttribute("href", "/library/readings/r1");
    expect(screen.queryByLabelText("Text of the reading")).toBeNull();
  });

  it("links to the existing reading when saving is refused with reading_exists", async () => {
    routes({ "POST /readings": { status: 409, body: { code: "reading_exists", reading_id: "r9" } } });
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16");
    await userEvent.type(screen.getByLabelText("Text of the reading"), "teks");
    await userEvent.click(screen.getByRole("button", { name: "Save reading" }));
    expect(await screen.findByRole("link", { name: "Open the saved reading" })).toHaveAttribute("href", "/library/readings/r9");
  });

  it("offers text from a provider and saves it through the server", async () => {
    const calls = routes({
      "GET /readings/lookup": { status: 200, body: lookup({ provider: { text: "Text from licensed", attribution: "© licensed", source: "licensed", may_store: true } }) },
      "POST /readings/from-provider": { status: 201, body: reading({ source_provider: "licensed" }) },
    });
    const router = open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16-21");
    await userEvent.tab();
    expect(await screen.findByText("Text from licensed")).toBeInTheDocument();
    expect(screen.getByText("© licensed")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save this text" }));
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/readings/r1"));
    // The browser never sends the text: only the reference, the translation and the provider.
    expect(calls.find((c) => c.route === "POST /readings/from-provider")!.body).toEqual({ reference: "Yoh 3:16-21", translation: "TB", provider: "licensed" });
  });

  it("does not offer saving text that the provider forbids storing", async () => {
    routes({ "GET /readings/lookup": { status: 200, body: lookup({ provider: { text: "Read-only text", attribution: "", source: "readonly", may_store: false } }) } });
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16");
    await userEvent.tab();
    expect(await screen.findByText("Read-only text")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save this text" })).toBeNull();
    expect(screen.getByText(/does not allow saving its text/)).toBeInTheDocument();
    expect(screen.getByLabelText("Text of the reading")).toBeInTheDocument(); // can still be pasted
  });

  it("says when the Bible text source could not be reached", async () => {
    routes({ "GET /readings/lookup": { status: 200, body: lookup({ provider_error: true }) } });
    open();
    await userEvent.type(await screen.findByLabelText("Bible reference"), "Yoh 3:16");
    await userEvent.tab();
    expect(await screen.findByText("The Bible text source could not be reached. You can still paste the text.")).toBeInTheDocument();
  });

  it("sends a member who cannot edit back to the list", async () => {
    routes();
    const router = open(viewer);
    await vi.waitFor(() => expect(router.state.location.pathname).toBe("/library/readings"));
  });
});
