// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { ChurchSettingsPage } from "./ChurchSettingsPage";

const church = {
  id: "c1", name: "GKY Uji", default_ui_language: "id", default_language: "id", default_translation_code: "TB",
  time_zone: "Asia/Jakarta", key_display: "do", feedback_url: null, privacy_contact: null, actions: { edit: true },
  show_credits: true, licence_footer: "",
  print: { paper: "a4", lyrics: "full", readings: true, assignments: true, keys: true, notes: true, size: "normal" },
};
const translations = [{ code: "TB", name: "Terjemahan Baru", language: "id" }];

afterEach(() => vi.unstubAllGlobals());

describe("ChurchSettingsPage", () => {
  void i18n.changeLanguage("en");

  it("is read-only without actions.edit", async () => {
    mockApi({ "GET /church": { status: 200, body: { ...church, actions: { edit: false } } }, "GET /translations": { status: 200, body: translations } });
    renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);
    expect(await screen.findByLabelText("Church name")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("saves changes, clearing empty optional fields", async () => {
    const calls = mockApi({
      "GET /church": { status: 200, body: church },
      "GET /translations": { status: 200, body: translations },
      "PATCH /church": { status: 200, body: { ...church, privacy_contact: "Ruth, 0812" } },
    });
    renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);
    await userEvent.type(await screen.findByLabelText("Privacy contact"), "Ruth, 0812");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    expect(calls.find((c) => c.route === "PATCH /church")?.body).toEqual({
      name: "GKY Uji", default_ui_language: "id", default_language: "id", default_translation_code: "TB",
      time_zone: "Asia/Jakarta", key_display: "do", feedback_url: "", privacy_contact: "Ruth, 0812",
      show_credits: true, licence_footer: "",
      print: { paper: "a4", lyrics: "full", readings: true, assignments: true, keys: true, notes: true, size: "normal" },
    });
  });

  // WT-P-005
  it("saves the print settings as one complete object", async () => {
    const calls = mockApi({
      "GET /church": { status: 200, body: church },
      "GET /translations": { status: 200, body: translations },
      "PATCH /church": { status: 200, body: church },
    });
    renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);
    await userEvent.type(await screen.findByLabelText("Licence line"), "CCLI License #1234567");
    await userEvent.click(screen.getByLabelText(/Show song credits/));
    await userEvent.selectOptions(screen.getByLabelText("Paper"), "f4");
    await userEvent.selectOptions(screen.getByLabelText("Lyrics"), "first_lines");
    await userEvent.click(screen.getByLabelText("Show notes"));
    await userEvent.selectOptions(screen.getByLabelText("Text size"), "large");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    const body = calls.find((c) => c.route === "PATCH /church")?.body as Record<string, unknown>;
    expect(body.show_credits).toBe(false);
    expect(body.licence_footer).toBe("CCLI License #1234567");
    expect(body.print).toEqual({ paper: "f4", lyrics: "first_lines", readings: true, assignments: true, keys: true, notes: false, size: "large" });
  });

  it("refuses a licence line over 200 characters before sending", async () => {
    const calls = mockApi({ "GET /church": { status: 200, body: church }, "GET /translations": { status: 200, body: translations } });
    renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);
    await userEvent.click(await screen.findByLabelText("Licence line"));
    await userEvent.paste("x".repeat(201));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("At most 200 characters.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "PATCH /church")).toBe(false);
  });

  it("rejects a feedback link without https", async () => {
    const calls = mockApi({ "GET /church": { status: 200, body: church }, "GET /translations": { status: 200, body: translations } });
    renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);
    await userEvent.type(await screen.findByLabelText("Feedback link", { exact: false }), "http://example.org");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("The link must start with https://.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "PATCH /church")).toBe(false);
  });
});
