// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editorMe } from "@/test/liturgy";
import { meWith } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { LiturgiesPage } from "./LiturgiesPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const row = { id: "l1", date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum", language: "id", state: "draft", item_count: 7, version: 1, actions: { edit: true, delete: true } };
const page = (me = editorMe, url = "/liturgies") => renderPage("/liturgies", url, <LiturgiesPage />, me);

describe("LiturgiesPage", () => {
  it("lists upcoming liturgies oldest first from today in the church's zone", async () => {
    const calls: string[] = [];
    vi.stubGlobal("fetch", async (request: Request) => {
      calls.push(new URL(request.url).pathname + new URL(request.url).search);
      return new Response(JSON.stringify({ items: [row], total: 1 }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    page();
    const link = await screen.findByRole("link", { name: /Ibadah Umum/ });
    expect(link).toHaveAttribute("href", "/liturgies/l1");
    expect(screen.getByText(/Draft · 7 items/)).toBeInTheDocument();
    expect(calls.find((c) => c.includes("order=date_asc"))).toMatch(/from=\d{4}-\d{2}-\d{2}/);
  });

  it("the Past tab asks for dates before today, newest first", async () => {
    const calls = mockApi({ "GET /liturgies": { status: 200, body: { items: [], total: 0 } } });
    page(editorMe, "/liturgies?tab=past");
    expect(await screen.findByText("There are no past liturgies.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "GET /liturgies")).toBe(true);
  });

  it("explains an empty church and offers both buttons, with a link when there are no services", async () => {
    mockApi({ "GET /liturgies": { status: 200, body: { items: [], total: 0 } }, "GET /services": { status: 200, body: { items: [] } } });
    page();
    expect(await screen.findByText("No liturgies yet")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Prepare next week" }).length).toBeGreaterThan(0);
    expect(await screen.findByRole("link", { name: "Go to the services" })).toBeInTheDocument();
  });

  it("shows no buttons to a member who may not edit", async () => {
    mockApi({ "GET /liturgies": { status: 200, body: { items: [row], total: 1 } }, "GET /services": { status: 200, body: { items: [] } } });
    page(meWith(["liturgy.comment"]));
    await screen.findByRole("link", { name: /Ibadah Umum/ });
    expect(screen.queryByRole("link", { name: "Prepare next week" })).toBeNull();
    expect(screen.queryByRole("link", { name: "New liturgy" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Past" }));
  });
});
