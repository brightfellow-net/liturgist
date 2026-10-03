// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import i18n from "@/lib/i18n";
import { liturgist, planner } from "@/test/planning";
import { mockApi, renderPage } from "@/test/render";
import { ServicesPage } from "./ServicesPage";
import { TemplatesPage } from "./TemplatesPage";

afterEach(() => vi.unstubAllGlobals());

const row = (id: string, name: string) => ({ id, name, language: "id", item_count: 7, version: 1, actions: { edit: true, delete: true } });

describe("TemplatesPage", () => {
  void i18n.changeLanguage("en");
  const open = (me = planner) => renderPage("/liturgies/templates", "/liturgies/templates", <TemplatesPage />, me);

  it("lists the templates with language and item count", async () => {
    mockApi({ "GET /templates": { status: 200, body: { items: [row("t1", "Ibadah Minggu")] } } });
    open();
    const link = await screen.findByRole("link", { name: "Ibadah Minggu" });
    expect(link).toHaveAttribute("href", "/liturgies/templates/t1");
    expect(screen.getByText(/Bahasa Indonesia|Indonesian/)).toBeInTheDocument();
    expect(screen.getByText(/7 items/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a template" })).toHaveAttribute("href", "/liturgies/templates/new");
  });

  it("explains the empty state and offers adding to editors only", async () => {
    mockApi({ "GET /templates": { status: 200, body: { items: [] } } });
    open();
    expect(await screen.findByText("No templates yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a template" })).toBeInTheDocument();
  });

  it("shows readers an empty state without a button", async () => {
    mockApi({ "GET /templates": { status: 200, body: { items: [] } } });
    open(liturgist);
    expect(await screen.findByText(/A member who may edit templates can add them/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add a template" })).toBeNull();
  });
});

describe("ServicesPage", () => {
  void i18n.changeLanguage("en");

  it("writes the times in words", async () => {
    mockApi({
      "GET /services": {
        status: 200,
        body: { items: [{ id: "s1", name: "Ibadah Umum", language: "id", default_template_id: "t1", default_template_name: "Ibadah Minggu", version: 1,
          times: [{ weekday: 7, time: "07:00" }, { weekday: 3, time: "19:00" }], actions: { edit: true, delete: true } }] },
      },
    });
    renderPage("/liturgies/services", "/liturgies/services", <ServicesPage />, planner);
    expect(await screen.findByRole("link", { name: "Ibadah Umum" })).toHaveAttribute("href", "/liturgies/services/s1");
    expect(screen.getByText(/Sunday 07:00; Wednesday 19:00/)).toBeInTheDocument();
    expect(screen.getByText(/Template: Ibadah Minggu/)).toBeInTheDocument();
  });

  it("explains the empty state", async () => {
    mockApi({ "GET /services": { status: 200, body: { items: [] } } });
    renderPage("/liturgies/services", "/liturgies/services", <ServicesPage />, planner);
    expect(await screen.findByText("No services yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a service" })).toBeInTheDocument();
  });
});
