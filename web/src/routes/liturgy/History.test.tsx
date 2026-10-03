// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editorMe } from "@/test/liturgy";
import { mockApi, renderPage } from "@/test/render";
import { History } from "./History";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

// An edit of the signed-in person (u1) unless the author is given.
const edit = (id: string, status = "done", user = "u1", command = "item.update") => ({
  id, seq: Number(id.slice(1)), user_id: user, user_name: user === "u1" ? "Ruth" : "Budi", command, item_id: "i1",
  before: { title: "Votum" }, after: { title: "Votum" }, liturgy_version_after: 3, item_version_after: 2, status,
  created_at: "2026-10-03T09:00:00Z",
});
const edits = (...items: ReturnType<typeof edit>[]) => ({ status: 200, body: { items } });
const done = { status: 200, body: { edit: { id: "e2", seq: 2, command: "item.update", item_id: "i1", status: "undone" }, liturgy_version: 3, item_id: "i1", item_version: 3 } };
const refused = (reason: string) => ({ status: 409, body: { code: "undo_refused", reason } });
const page = (canEdit = true) => renderPage("/liturgies/:id", "/liturgies/l1", <History id="l1" canEdit={canEdit} />, editorMe);
const key = (init: KeyboardEventInit, target: Element = document.body) => {
  const e = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(e);
  return e;
};

describe("Undo and redo buttons", () => {
  it("undoes, says what was undone and re-reads the liturgy and the history", async () => {
    const calls = mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": done, "GET /liturgies/l1": { status: 200, body: {} } });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Undid: the change to an item")).toBeInTheDocument();
    expect(calls.map((c) => c.route)).toContain("POST /liturgies/l1/undo");
    await waitFor(() => expect(calls.filter((c) => c.route === "GET /liturgies/l1/edits").length).toBeGreaterThan(1));
  });

  it("explains a refusal in words and keeps Undo available", async () => {
    mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": refused("changed_since") });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Someone has changed this since, so it can't be undone.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
  });

  it("switches Undo off when nothing applies, until a new change appears", async () => {
    mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": refused("nothing_to_undo") });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Nothing to undo.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
  });

  it("offers Redo only when the person has an undone change", async () => {
    mockApi({ "GET /liturgies/l1/edits": edits(edit("e2"), edit("e1", "undone", "u2")) });
    page();
    expect(await screen.findByRole("button", { name: "Redo" })).toBeDisabled();
  });

  it("redoes the newest undone change and marks undone rows with words", async () => {
    const calls = mockApi({
      "GET /liturgies/l1/edits": edits(edit("e2", "undone")),
      "POST /liturgies/l1/redo": { status: 200, body: { ...done.body, edit: { ...done.body.edit, status: "done" } } },
    });
    page();
    expect(await screen.findByText(/· undone/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(await screen.findByText("Redid: the change to an item")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /liturgies/l1/redo")).toBe(true);
  });

  it("shows no buttons when the liturgy cannot be edited", async () => {
    const calls = mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": done });
    page(false);
    expect(await screen.findByText(/Ruth changed Votum/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    key({ key: "z", ctrlKey: true });
    await new Promise((r) => setTimeout(r, 20));
    expect(calls.some((c) => c.route === "POST /liturgies/l1/undo")).toBe(false);
  });
});

// TC-E-006: the keys call the server only outside text fields.
describe("TC-E-006 undo keys", () => {
  it("Ctrl+Z, Cmd+Z, Ctrl+Shift+Z and Ctrl+Y outside a text field", async () => {
    const calls = mockApi({ "GET /liturgies/l1/edits": edits(edit("e2", "undone")), "POST /liturgies/l1/undo": done, "POST /liturgies/l1/redo": done });
    page();
    await screen.findByRole("button", { name: "Redo" });
    const used = (route: string) => calls.filter((c) => c.route === route).length;

    expect(key({ key: "z", ctrlKey: true }).defaultPrevented).toBe(true);
    await waitFor(() => expect(used("POST /liturgies/l1/undo")).toBe(1));
    key({ key: "z", metaKey: true });
    await waitFor(() => expect(used("POST /liturgies/l1/undo")).toBe(2));
    key({ key: "Z", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(used("POST /liturgies/l1/redo")).toBe(1));
    key({ key: "y", ctrlKey: true });
    await waitFor(() => expect(used("POST /liturgies/l1/redo")).toBe(2));
  });

  it("leaves the browser's own undo alone inside a text field", async () => {
    const calls = mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": done });
    page();
    await screen.findByRole("button", { name: "Undo" });
    const area = document.createElement("textarea");
    document.body.append(area);
    area.focus();
    const e = key({ key: "z", ctrlKey: true }, area);
    expect(e.defaultPrevented).toBe(false);
    area.remove();
    await new Promise((r) => setTimeout(r, 20));
    expect(calls.some((c) => c.route === "POST /liturgies/l1/undo")).toBe(false);
  });

  it("ignores plain z and a key with Alt", async () => {
    const calls = mockApi({ "GET /liturgies/l1/edits": edits(edit("e2")), "POST /liturgies/l1/undo": done });
    page();
    await screen.findByRole("button", { name: "Undo" });
    key({ key: "z" });
    key({ key: "z", ctrlKey: true, altKey: true });
    await new Promise((r) => setTimeout(r, 20));
    expect(calls.some((c) => c.route === "POST /liturgies/l1/undo")).toBe(false);
  });
});
