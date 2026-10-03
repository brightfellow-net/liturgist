// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { dutyList, partList } from "@/lib/planning";
import { entries, liturgist, planner } from "@/test/planning";
import { mockApi, renderPage } from "@/test/render";
import { NameListPage } from "./NameListPage";

afterEach(() => vi.unstubAllGlobals());

const open = (list = dutyList, me = planner) => renderPage("/liturgies/duties", "/liturgies/duties", <NameListPage list={list} />, me);
const rowOf = (name: string) => screen.getByText(name).closest("li") as HTMLElement;

describe("NameListPage", () => {
  void i18n.changeLanguage("en");

  it("lists the duties in order with their buttons", async () => {
    mockApi({ "GET /duties": { status: 200, body: entries(["Liturgis", "Pemusik"]) } });
    open();
    expect(await screen.findByRole("heading", { name: "Duties", level: 2 })).toBeInTheDocument();
    await screen.findByText("Liturgis");
    const items = screen.getAllByRole("listitem");
    expect(items.map((li) => li.textContent)).toEqual([expect.stringContaining("Liturgis"), expect.stringContaining("Pemusik")]);
    expect(screen.getByRole("button", { name: "Move up Liturgis" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Move down Pemusik" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Move down Liturgis" })).toBeEnabled();
    expect(screen.getByText("2 of 50 used.")).toBeInTheDocument();
  });

  it("is read-only without the actions", async () => {
    mockApi({ "GET /duties": { status: 200, body: entries(["Liturgis"], false) } });
    open(dutyList, liturgist);
    await screen.findByText("Liturgis");
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByLabelText("New duty")).toBeNull();
    expect(screen.getByText(/only members who may edit templates can change it/)).toBeInTheDocument();
  });

  it("says so when there are none, and adds the first", async () => {
    let created = false;
    const calls = mockApi({
      "GET /duties": () => ({ status: 200, body: created ? entries(["Liturgis"]) : { items: [] } }),
      "POST /duties": () => { created = true; return { status: 201, body: entries(["Liturgis"]).items[0] }; },
    });
    open();
    expect(await screen.findByText("There are no duties yet.")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("New duty"), "Liturgis");
    await userEvent.click(screen.getByRole("button", { name: "Add a duty" }));
    expect(await screen.findByText("1 of 50 used.")).toBeInTheDocument();
    expect(calls.find((c) => c.route === "POST /duties")!.body).toEqual({ name: "Liturgis" });
    expect(screen.getByLabelText("New duty")).toHaveValue("");
  });

  it("uses the singing part wording and path", async () => {
    mockApi({ "GET /singing-parts": { status: 200, body: entries(["Semua"]) } });
    open(partList);
    expect(await screen.findByRole("heading", { name: "Singing parts", level: 2 })).toBeInTheDocument();
    expect(await screen.findByText("1 of 30 used.")).toBeInTheDocument();
  });

  it("renames in the row", async () => {
    const calls = mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis", "Pemusik"]) },
      "PATCH /duties/e2": { status: 200, body: { id: "e2", name: "Musisi", position: 1, actions: { edit: true, delete: true } } },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Rename Pemusik" }));
    const box = screen.getByLabelText("Name");
    await userEvent.clear(box);
    await userEvent.type(box, "Musisi");
    await userEvent.click(screen.getByRole("button", { name: "Save name" }));
    expect(calls.find((c) => c.route === "PATCH /duties/e2")!.body).toEqual({ name: "Musisi" });
  });

  it("tells a name that is taken", async () => {
    mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis"]) },
      "POST /duties": { status: 409, body: { code: "name_taken", reason: "duty" } },
    });
    open();
    await userEvent.type(await screen.findByLabelText("New duty"), "liturgis");
    await userEvent.click(screen.getByRole("button", { name: "Add a duty" }));
    expect(await screen.findByText("There is already a duty with this name.")).toBeInTheDocument();
  });

  it("sends the whole new order when a row moves, and shows the answer", async () => {
    const calls = mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis", "Pemusik", "Kolektan"]) },
      "PUT /duties/order": { status: 200, body: { items: [entries(["Liturgis", "Pemusik", "Kolektan"]).items[1], entries(["Liturgis", "Pemusik", "Kolektan"]).items[0], entries(["Liturgis", "Pemusik", "Kolektan"]).items[2]] } },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Move up Pemusik" }));
    expect(calls.find((c) => c.route === "PUT /duties/order")!.body).toEqual({ ids: ["e2", "e1", "e3"] });
    expect(await screen.findByText("Pemusik moved to place 1 of 3.")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")[0]).toHaveTextContent("Pemusik");
  });

  it("reloads and says so when the list was changed by someone else", async () => {
    let reordered = false;
    mockApi({
      "GET /duties": () => ({ status: 200, body: entries(reordered ? ["Pemusik", "Liturgis"] : ["Liturgis", "Pemusik"]) }),
      "PUT /duties/order": () => { reordered = true; return { status: 409, body: { code: "version_conflict" } }; },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Move down Liturgis" }));
    expect(await screen.findByText(/The list was changed by someone else/)).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")[0]).toHaveTextContent("Pemusik");
  });

  it("asks before deleting and explains a duty in use", async () => {
    const calls = mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis"]) },
      "DELETE /duties/e1": { status: 409, body: { code: "duty_in_use" } },
    });
    open();
    await screen.findByText("Liturgis");
    await userEvent.click(within(rowOf("Liturgis")).getByRole("button", { name: "Delete" }));
    expect(calls.some((c) => c.route === "DELETE /duties/e1")).toBe(false);
    expect(screen.getByText(/Template items that use it lose their default duty/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Delete Liturgis" }));
    expect(await screen.findByText("This duty is used in a liturgy, so it can't be deleted.")).toBeInTheDocument();
  });

  it("disables adding at the limit and names the limit when the server says so", async () => {
    mockApi({
      "GET /singing-parts": { status: 200, body: entries(Array.from({ length: 29 }, (_, i) => "Part " + i)) },
      "POST /singing-parts": { status: 422, body: { code: "validation_failed", reason: "limit", max: 30, used: 30 } },
    });
    open(partList);
    await userEvent.type(await screen.findByLabelText("New singing part"), "Satu");
    await userEvent.click(screen.getByRole("button", { name: "Add a singing part" }));
    expect(await screen.findByText("You can have at most 30 of these.")).toBeInTheDocument();
  });
});
