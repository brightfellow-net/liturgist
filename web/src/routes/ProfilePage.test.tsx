// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Me } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { ProfilePage } from "./ProfilePage";

const me = {
  user: { id: "u1", name: "Ruth", email: "ruth@example.org", phone: null, preferences: { text_size: "normal", ui_language: "id" } },
  church: { default_ui_language: "id" },
  membership: { id: "m1", roles: [], scopes: [], actions: {} },
} as unknown as Me;

afterEach(() => vi.unstubAllGlobals());

describe("ProfilePage", () => {
  void i18n.changeLanguage("en");

  it("saves text size and goes back to the church's language", async () => {
    const calls = mockApi({ "PATCH /me": { status: 200, body: { ...me.user, preferences: { text_size: "larger" } } } });
    renderPage("/profile", "/profile", <ProfilePage />, me);
    await userEvent.click(await screen.findByLabelText("Larger"));
    await userEvent.selectOptions(screen.getByLabelText("Language of the app"), "Church default (Bahasa Indonesia)");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    expect(calls[0]).toEqual({
      route: "PATCH /me",
      body: { name: "Ruth", preferences: { text_size: "larger", ui_language: null } },
    });
  });

  it("shows a wrong current password", async () => {
    mockApi({ "POST /me/password": { status: 401, body: { code: "invalid_credentials" } } });
    renderPage("/profile", "/profile", <ProfilePage />, me);
    await userEvent.type(screen.getByLabelText("Current password", { exact: false }), "salah sekali");
    await userEvent.type(screen.getByLabelText("New password", { exact: false }), "kidung jemaat baru");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByText("The email/phone or password is incorrect.")).toBeInTheDocument();
  });

  it("logs out on other devices", async () => {
    const calls = mockApi({ "POST /me/sessions/end-others": { status: 204 } });
    renderPage("/profile", "/profile", <ProfilePage />, me);
    await userEvent.click(screen.getByRole("button", { name: "Log out on all other devices" }));
    expect(await screen.findByText("You have been logged out on all other devices.")).toBeInTheDocument();
    expect(calls[0]).toEqual({ route: "POST /me/sessions/end-others", body: {} });
  });
});
