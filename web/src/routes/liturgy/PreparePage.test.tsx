// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { editorMe } from "@/test/liturgy";
import { meWith } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { PreparePage } from "./PreparePage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const occ = (service_id: string, name: string, date: string, time: string, liturgy_id: string | null = null) => ({
  service_id, service_name: name, language: "id", date, time, template_id: "t1", template_name: "Ibadah Minggu", liturgy_id,
});
const unlimited = { unlimited: true, max: 0, used: 0 };
const week = (limits = { max_unpublished_liturgies: unlimited, max_active_liturgies: unlimited }) => ({
  status: 200,
  body: {
    week: "2026-10-12", limits,
    occurrences: [occ("sv1", "Ibadah Umum", "2026-10-14", "19:00"), occ("sv2", "Ibadah Pemuda", "2026-10-17", "17:00", "l9"), occ("sv3", "Ibadah Siang", "2026-10-18", "07:00")],
  },
});
const page = () => renderPage("/liturgies/prepare", "/liturgies/prepare", <PreparePage />, editorMe);

describe("TC-E-004 prepare page", () => {
  it("ticks the free slots, disables the taken one and creates only the ticked", async () => {
    const calls = mockApi({
      "GET /liturgies/prepare": week(),
      "POST /liturgies/prepare": { status: 201, body: { items: [] } },
      "GET /liturgies": { status: 200, body: { items: [], total: 0 } },
    });
    page();
    expect(await screen.findByText("Week of 12 October 2026")).toBeInTheDocument();
    const taken = screen.getByRole("checkbox", { name: /Ibadah Pemuda/ });
    expect(taken).toBeDisabled();
    expect(taken).not.toBeChecked();
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("href", "/liturgies/l9");
    await userEvent.click(screen.getByRole("checkbox", { name: /Ibadah Siang/ }));
    await userEvent.click(screen.getByRole("button", { name: "Create 1 liturgy" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /liturgies/prepare")).toBe(true));
    expect(calls.find((c) => c.route === "POST /liturgies/prepare")!.body).toEqual({ occurrences: [{ service_id: "sv1", date: "2026-10-14", time: "19:00" }] });
  });

  it("disables Create and says why when more are ticked than the limit allows", async () => {
    mockApi({ "GET /liturgies/prepare": week({ max_unpublished_liturgies: { unlimited: false, max: 5, used: 4 }, max_active_liturgies: unlimited }) });
    page();
    expect(await screen.findByText(/1 of 5 liturgy places free/)).toBeInTheDocument();
    expect(screen.getByText(/You ticked 2; only 1 can be created/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create 2 liturgies" })).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox", { name: /Ibadah Siang/ }));
    expect(screen.getByRole("button", { name: "Create 1 liturgy" })).toBeEnabled();
  });

  // WT-P-001: archive the oldest published liturgies to make room under max_active_liturgies.
  it("offers to archive the oldest published liturgies when the active limit is full", async () => {
    const old = (id: string, date: string, archive = true) => ({
      id, date, time: "", service_name: "Lama " + id, language: "id", state: "published", item_count: 1, version: 1, archived: false,
      actions: { edit: false, delete: false, submit: false, approve: false, request_changes: false, reopen: true, comment: false, publish: false, archive, unarchive: false },
    });
    const calls = mockApi({
      "GET /liturgies/prepare": week({ max_unpublished_liturgies: unlimited, max_active_liturgies: { unlimited: false, max: 7, used: 7 } }),
      "GET /liturgies": { status: 200, body: { items: [old("o1", "2026-09-27"), old("o2", "2026-10-04")], total: 2 } },
      "POST /liturgies/prepare": { status: 201, body: { items: [], archived: ["o1", "o2"] } },
    });
    renderPage("/liturgies/prepare", "/liturgies/prepare", <PreparePage />, meWith(["liturgy.edit", "liturgy.manage"]));
    await screen.findByText("Week of 12 October 2026");
    expect(screen.getByRole("button", { name: "Create 2 liturgies" })).toBeDisabled();
    await userEvent.click(await screen.findByRole("button", { name: "Archive the 2 oldest published liturgies and create 2" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "POST /liturgies/prepare")).toBe(true));
    expect(calls.find((c) => c.route === "POST /liturgies/prepare")!.body).toEqual({
      occurrences: [{ service_id: "sv1", date: "2026-10-14", time: "19:00" }, { service_id: "sv3", date: "2026-10-18", time: "07:00" }],
      archive_ids: ["o1", "o2"],
    });
  });

  it("does not offer archiving when the unpublished limit is the one that is full, or without liturgy.manage", async () => {
    mockApi({
      "GET /liturgies/prepare": week({ max_unpublished_liturgies: { unlimited: false, max: 5, used: 5 }, max_active_liturgies: { unlimited: false, max: 7, used: 7 } }),
      "GET /liturgies": { status: 200, body: { items: [], total: 0 } },
    });
    renderPage("/liturgies/prepare", "/liturgies/prepare", <PreparePage />, meWith(["liturgy.edit", "liturgy.manage"]));
    await screen.findByText(/You ticked 2; only 0 can be created/);
    expect(screen.queryByRole("button", { name: /Archive the/ })).toBeNull();
  });

  it("asks for the other week when the week buttons are used", async () => {
    const calls = mockApi({ "GET /liturgies/prepare": week() });
    page();
    await screen.findByText("Week of 12 October 2026");
    await userEvent.click(screen.getByRole("button", { name: "Next week" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "GET /liturgies/prepare" && calls.filter((x) => x.route === c.route).length > 1)).toBe(true));
  });

  it("explains an empty week and links to the services", async () => {
    mockApi({ "GET /liturgies/prepare": { status: 200, body: { week: "2026-10-12", occurrences: [], limits: { max_unpublished_liturgies: unlimited, max_active_liturgies: unlimited } } } });
    page();
    expect(await screen.findByText(/no regular services yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to the services" })).toBeInTheDocument();
  });

  it("is for members who may edit liturgies", () => {
    mockApi({});
    renderPage("/liturgies/prepare", "/liturgies/prepare", <PreparePage />, meWith([]));
    expect(screen.getByText("other page")).toBeInTheDocument();
  });
});
