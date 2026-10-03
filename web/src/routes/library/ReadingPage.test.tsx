// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { reading } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { ReadingPage } from "./ReadingPage";

afterEach(() => vi.unstubAllGlobals());

const open = () => renderPage("/library/readings/:id", "/library/readings/r1", <ReadingPage />);

describe("ReadingPage", () => {
  void i18n.changeLanguage("en");

  it("shows the reading with its attribution", async () => {
    mockApi({ "GET /readings/r1": { status: 200, body: reading() } });
    open();
    expect(await screen.findByRole("heading", { name: "Yohanes 3:16-21 (TB)", level: 1 })).toBeInTheDocument();
    expect(screen.getByText(/Karena begitu besar kasih Allah/)).toBeInTheDocument();
    expect(screen.getByText("© LAI")).toBeInTheDocument();
    expect(screen.queryByText(/Text saved from the source/)).toBeNull();
  });

  it("says where provider text came from", async () => {
    mockApi({ "GET /readings/r1": { status: 200, body: reading({ source_provider: "licensed" }) } });
    open();
    expect(await screen.findByText("Text saved from the source “licensed”.")).toBeInTheDocument();
  });

  it("shows no buttons when the actions are false", async () => {
    mockApi({ "GET /readings/r1": { status: 200, body: reading({ actions: { edit: false, delete: false } }) } });
    open();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
  });

  it("edits the text and sends the loaded version", async () => {
    const calls = mockApi({
      "GET /readings/r1": { status: 200, body: reading() },
      "GET /readings": { status: 200, body: { items: [], total: 0 } },
      "PATCH /readings/r1": { status: 200, body: reading({ text: "Teks baru", version: 3 }) },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const box = screen.getByLabelText("Text of the reading");
    await userEvent.clear(box);
    await userEvent.type(box, "Teks baru");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(await screen.findByText("Teks baru")).toBeInTheDocument();
    expect(calls.find((c) => c.route === "PATCH /readings/r1")!.body).toEqual({ text: "Teks baru", attribution: "© LAI", version: 2 });
  });

  it("keeps the input after a conflict until Reload", async () => {
    let patched = false;
    mockApi({
      "GET /readings/r1": () => ({ status: 200, body: patched ? reading({ text: "Teks orang lain", version: 3 }) : reading() }),
      "PATCH /readings/r1": () => {
        patched = true;
        return { status: 409, body: { code: "version_conflict" } };
      },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const box = screen.getByLabelText("Text of the reading");
    await userEvent.clear(box);
    await userEvent.type(box, "Teks saya");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(await screen.findByText(/changed by someone else while you were editing/)).toBeInTheDocument();
    expect(screen.getByLabelText("Text of the reading")).toHaveValue("Teks saya");
    await userEvent.click(screen.getByRole("button", { name: "Reload" }));
    expect(await screen.findByText("Teks orang lain")).toBeInTheDocument();
  });

  it("asks before deleting and explains a reading in use", async () => {
    const calls = mockApi({
      "GET /readings/r1": { status: 200, body: reading() },
      "DELETE /readings/r1": { status: 409, body: { code: "reading_in_use" } },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    expect(screen.getByText("Delete Yohanes 3:16-21 (TB)? Its text is removed for good.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "DELETE /readings/r1")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Delete Yohanes 3:16-21" }));
    expect(await screen.findByText("This reading is used in a liturgy that isn't published yet, so it can't be deleted.")).toBeInTheDocument();
  });

  it("says when the reading does not exist", async () => {
    mockApi({ "GET /readings/r1": { status: 404, body: { code: "not_found" } } });
    open();
    expect(await screen.findByText("This reading does not exist or was deleted.")).toBeInTheDocument();
  });
});
