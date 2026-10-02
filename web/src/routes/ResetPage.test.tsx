// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { ResetPage } from "./ResetPage";

afterEach(() => vi.unstubAllGlobals());

describe("ResetPage", () => {
  void i18n.changeLanguage("en");

  it("shows the incomplete-link message without calling the API", async () => {
    const calls = mockApi({});
    renderPage("/reset", "/reset", <ResetPage />);
    expect(await screen.findByText(/This link is incomplete/)).toBeInTheDocument();
    expect(calls).toEqual([]);
  });

  it("names who created the link, then sets the password", async () => {
    const calls = mockApi({
      "POST /auth/reset/inspect": { status: 200, body: { user_name: "Yohanes", created_by_name: "Ruth", expires_at: "2026-10-03T10:00:00Z" } },
      "POST /auth/reset": { status: 204 },
    });
    const router = renderPage("/reset", "/reset#t=tok", <ResetPage />);
    expect(await screen.findByText("This link was created by Ruth.")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("New password", { exact: false }), "kidung jemaat baru");
    await userEvent.click(screen.getByRole("button", { name: "Save password and log in" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(calls.at(-1)).toEqual({ route: "POST /auth/reset", body: { token: "tok", new_password: "kidung jemaat baru" } });
  });

  it("says when the server administrator created the link", async () => {
    mockApi({ "POST /auth/reset/inspect": { status: 200, body: { user_name: "Yohanes", created_by_name: null, expires_at: "2026-10-03T10:00:00Z" } } });
    renderPage("/reset", "/reset#t=tok", <ResetPage />);
    expect(await screen.findByText("This link was created by the server administrator.")).toBeInTheDocument();
  });

  it("shows why a password is refused", async () => {
    mockApi({
      "POST /auth/reset/inspect": { status: 200, body: { user_name: "Yohanes", created_by_name: "Ruth", expires_at: "2026-10-03T10:00:00Z" } },
      "POST /auth/reset": { status: 422, body: { code: "weak_password", reason: "common" } },
    });
    renderPage("/reset", "/reset#t=tok", <ResetPage />);
    await userEvent.type(await screen.findByLabelText("New password", { exact: false }), "haleluya123");
    await userEvent.click(screen.getByRole("button", { name: "Save password and log in" }));
    expect(await screen.findByText("This password is too common. Choose another.")).toBeInTheDocument();
  });
});
