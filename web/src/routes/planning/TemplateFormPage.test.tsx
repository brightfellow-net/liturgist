// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { entries, liturgist, planner, template } from "@/test/planning";
import { mockApi, renderPage } from "@/test/render";
import { TemplateFormPage } from "./TemplateFormPage";

afterEach(() => vi.unstubAllGlobals());

const newPage = (me = planner) => renderPage("/liturgies/templates/new", "/liturgies/templates/new", <TemplateFormPage />, me);
const editPage = (me = planner) => renderPage("/liturgies/templates/:id", "/liturgies/templates/t1", <TemplateFormPage />, me);
const card = (n: number) => screen.getByRole("group", { name: new RegExp(`^Item ${n}:`) });

describe("TemplateFormPage", () => {
  void i18n.changeLanguage("en");

  it("adds a template with items and sends them in order", async () => {
    const calls = mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis", "Pemusik"]) },
      "POST /templates": { status: 201, body: template({ version: 1 }) },
      "GET /templates": { status: 200, body: { items: [] } },
    });
    newPage();
    await userEvent.type(await screen.findByLabelText("Name"), "Doa Malam");
    await userEvent.click(screen.getByRole("button", { name: "Add an item" }));
    await userEvent.type(within(card(1)).getByLabelText("Title"), "Lagu");
    await userEvent.selectOptions(within(card(1)).getByLabelText("Kind"), "song");
    expect(within(card(1)).queryByLabelText("Default text")).toBeNull(); // songs take no text
    await userEvent.click(screen.getByRole("button", { name: "Add an item" }));
    await userEvent.type(within(card(2)).getByLabelText("Title"), "Doa");
    await userEvent.selectOptions(within(card(2)).getByLabelText("Kind"), "prayer");
    await userEvent.type(within(card(2)).getByLabelText("Default text"), "Bapa kami");
    await userEvent.selectOptions(within(card(2)).getByLabelText("Default duty"), "e1");
    await userEvent.click(screen.getByRole("button", { name: "Move up Item 2" }));
    await userEvent.click(screen.getByRole("button", { name: "Save template" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /templates")).toBe(true));
    expect(calls.find((c) => c.route === "POST /templates")!.body).toEqual({
      name: "Doa Malam", language: "id",
      items: [
        { title: "Doa", item_type: "prayer", default_text: "Bapa kami", default_duty_id: "e1" },
        { title: "Lagu", item_type: "song", default_text: "", default_duty_id: "" },
      ],
    });
  });

  it("asks for a name and a title before sending", async () => {
    const calls = mockApi({ "GET /duties": { status: 200, body: entries([]) } });
    newPage();
    await userEvent.click(await screen.findByRole("button", { name: "Add an item" }));
    await userEvent.click(screen.getByRole("button", { name: "Save template" }));
    expect(await screen.findByText("Enter a name.")).toBeInTheDocument();
    expect(screen.getByText("Enter a title.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /templates")).toBe(false);
  });

  it("edits with the loaded version, and keeps the input after a conflict until Reload", async () => {
    let patched = false;
    const calls = mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis"]) },
      "GET /templates/t1": () => ({ status: 200, body: patched ? template({ name: "Nama orang lain", version: 3 }) : template() }),
      "PATCH /templates/t1": () => { patched = true; return { status: 409, body: { code: "version_conflict" } }; },
    });
    editPage();
    const name = await screen.findByLabelText("Name");
    expect(name).toHaveValue("Ibadah Minggu");
    expect(within(card(1)).getByLabelText("Default duty")).toHaveValue("e1");
    await userEvent.clear(name);
    await userEvent.type(name, "Nama saya");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(await screen.findByText(/changed by someone else while you were editing/)).toBeInTheDocument();
    expect(calls.find((c) => c.route === "PATCH /templates/t1")!.body).toMatchObject({ version: 2, name: "Nama saya" });
    expect(screen.getByLabelText("Name")).toHaveValue("Nama saya"); // kept
    await userEvent.click(screen.getByRole("button", { name: "Reload" }));
    await vi.waitFor(() => expect(screen.getByLabelText("Name")).toHaveValue("Nama orang lain"));
  });

  it("is read-only for a member who may not edit templates", async () => {
    mockApi({
      "GET /duties": { status: 200, body: entries(["Liturgis"]) },
      "GET /templates/t1": { status: 200, body: template({ actions: { edit: false, delete: false } }) },
    });
    editPage(liturgist);
    expect(await screen.findByText("Votum dan Salam")).toBeInTheDocument();
    expect(screen.getByText(/Free text · Liturgis/)).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByLabelText("Name")).toBeNull();
  });

  it("explains a template that a service still uses", async () => {
    mockApi({
      "GET /duties": { status: 200, body: entries([]) },
      "GET /templates/t1": { status: 200, body: template() },
      "DELETE /templates/t1": { status: 409, body: { code: "template_in_use", service_ids: ["s1"] } },
    });
    editPage();
    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await userEvent.click(screen.getByRole("button", { name: "Delete Ibadah Minggu" }));
    expect(await screen.findByText("A service uses this template as its default. Change that service first.")).toBeInTheDocument();
  });

  it("says when the template does not exist", async () => {
    mockApi({ "GET /duties": { status: 200, body: entries([]) }, "GET /templates/t1": { status: 404, body: { code: "not_found" } } });
    editPage();
    expect(await screen.findByText("This does not exist or was deleted.")).toBeInTheDocument();
  });
});
