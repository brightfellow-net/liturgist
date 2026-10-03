// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { liturgist, planner, service } from "@/test/planning";
import { mockApi, renderPage } from "@/test/render";
import { ServiceFormPage } from "./ServiceFormPage";

afterEach(() => vi.unstubAllGlobals());

const templates = { status: 200, body: { items: [{ id: "t1", name: "Ibadah Minggu", language: "id", item_count: 7, version: 1, actions: { edit: true, delete: true } }] } };
const newPage = (me = planner) => renderPage("/liturgies/services/new", "/liturgies/services/new", <ServiceFormPage />, me);
const editPage = (me = planner) => renderPage("/liturgies/services/:id", "/liturgies/services/s1", <ServiceFormPage />, me);
const timeBox = (n: number) => screen.getByRole("group", { name: `Time ${n}` });

describe("ServiceFormPage", () => {
  void i18n.changeLanguage("en");

  it("adds a service with times and a default template", async () => {
    const calls = mockApi({
      "GET /templates": templates,
      "POST /services": { status: 201, body: service({ version: 1 }) },
      "GET /services": { status: 200, body: { items: [] } },
    });
    newPage();
    await userEvent.type(await screen.findByLabelText("Name"), "Ibadah Umum");
    await userEvent.selectOptions(screen.getByLabelText("Default template"), "t1");
    await userEvent.type(within(timeBox(1)).getByLabelText("Time"), "07:00");
    await userEvent.click(screen.getByRole("button", { name: "Add a time" }));
    await userEvent.selectOptions(within(timeBox(2)).getByLabelText("Day"), "3");
    await userEvent.type(within(timeBox(2)).getByLabelText("Time"), "19:00");
    await userEvent.click(screen.getByRole("button", { name: "Save service" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /services")).toBe(true));
    expect(calls.find((c) => c.route === "POST /services")!.body).toEqual({
      name: "Ibadah Umum", language: "id", default_template_id: "t1",
      times: [{ weekday: 7, time: "07:00" }, { weekday: 3, time: "19:00" }],
    });
  });

  it("needs a name and a time, and at least one time", async () => {
    const calls = mockApi({ "GET /templates": templates });
    newPage();
    await userEvent.click(await screen.findByRole("button", { name: "Save service" }));
    expect(await screen.findByText("Enter a name.")).toBeInTheDocument();
    expect(screen.getByText("Enter a time.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Remove Time 1" }));
    await userEvent.click(screen.getByRole("button", { name: "Save service" }));
    expect(await screen.findByText("Add at least one time.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /services")).toBe(false);
  });

  it("edits with the loaded version and clears the template", async () => {
    const calls = mockApi({
      "GET /templates": templates,
      "GET /services/s1": { status: 200, body: service() },
      "PATCH /services/s1": { status: 200, body: service({ version: 4 }) },
      "GET /services": { status: 200, body: { items: [] } },
    });
    editPage();
    expect(await screen.findByLabelText("Name")).toHaveValue("Ibadah Umum");
    expect(within(timeBox(1)).getByLabelText("Day")).toHaveValue("3");
    await vi.waitFor(() => expect(screen.getByLabelText("Default template")).toHaveValue("t1"));
    await userEvent.selectOptions(screen.getByLabelText("Default template"), "");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "PATCH /services/s1")).toBe(true));
    expect(calls.find((c) => c.route === "PATCH /services/s1")!.body).toEqual({
      version: 3, name: "Ibadah Umum", language: "id", default_template_id: "",
      times: [{ weekday: 3, time: "19:00" }, { weekday: 7, time: "07:00" }],
    });
  });

  it("is read-only for a member who may not edit", async () => {
    mockApi({ "GET /templates": templates, "GET /services/s1": { status: 200, body: service({ actions: { edit: false, delete: false } }) } });
    editPage(liturgist);
    expect(await screen.findByText(/Wednesday 19:00; Sunday 07:00/)).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
