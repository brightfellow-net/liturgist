// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editorMe, liturgy } from "@/test/liturgy";
import { service } from "@/test/planning";
import { mockApi, renderPage } from "@/test/render";
import { NewLiturgyPage } from "./NewLiturgyPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const templates = { status: 200, body: { items: [
  { id: "t1", name: "Ibadah Minggu", language: "id", item_count: 7, version: 1, actions: { edit: true, delete: true } },
  { id: "t2", name: "Sunday service", language: "en", item_count: 5, version: 1, actions: { edit: true, delete: true } },
] } };
const services = { status: 200, body: { items: [service()] } };
const page = () => renderPage("/liturgies/new", "/liturgies/new", <NewLiturgyPage />, editorMe);

describe("NewLiturgyPage", () => {
  it("creates a liturgy from a regular service with its template and first time preselected", async () => {
    const calls = mockApi({ "GET /services": services, "GET /templates": templates, "POST /liturgies": { status: 201, body: liturgy() }, "GET /liturgies": { status: 200, body: { items: [], total: 0 } } });
    page();
    await screen.findByLabelText("Service");
    expect(screen.getByLabelText("Template")).toHaveValue("t1");
    await userEvent.click(screen.getByRole("button", { name: "Create liturgy" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /liturgies")).toBe(true));
    expect(calls.find((c) => c.route === "POST /liturgies")!.body).toMatchObject({ service_id: "s1", time: "19:00", template_id: "t1", language: "id" });
  });

  it("offers only templates of the chosen language and sends none when none fits", async () => {
    const calls = mockApi({ "GET /services": services, "GET /templates": templates, "POST /liturgies": { status: 201, body: liturgy() }, "GET /liturgies": { status: 200, body: { items: [], total: 0 } } });
    page();
    await userEvent.selectOptions(await screen.findByLabelText("Language"), "en");
    expect(screen.getByLabelText("Template")).toHaveValue("");
    expect(screen.queryByRole("option", { name: "Ibadah Minggu" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Create liturgy" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /liturgies")).toBe(true));
    expect(calls.find((c) => c.route === "POST /liturgies")!.body).toMatchObject({ template_id: "", language: "en" });
  });

  it("creates a one-off service by name and shows a link when the slot is taken", async () => {
    mockApi({ "GET /services": services, "GET /templates": templates, "POST /liturgies": { status: 409, body: { code: "liturgy_exists", liturgy_id: "l7" } } });
    page();
    await userEvent.selectOptions(await screen.findByLabelText("Kind of service"), "oneoff");
    await userEvent.type(screen.getByLabelText("Name of the service"), "Natal");
    await userEvent.type(screen.getByLabelText("Time"), "18:00");
    await userEvent.click(screen.getByRole("button", { name: "Create liturgy" }));
    const open = await screen.findByRole("link", { name: "Open" });
    expect(open).toHaveAttribute("href", "/liturgies/l7");
  });
});
