// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { InvitePage } from "./InvitePage";

const info = {
  church_name: "GKY Citragarden", invitee_name: "Maria", email: "maria@example.org", phone: null,
  status: "pending", owner_exists: false,
};
const loggedOut = { status: 401, body: { code: "unauthenticated" } };

afterEach(() => vi.unstubAllGlobals());

describe("InvitePage", () => {
  void i18n.changeLanguage("en");

  it("shows the incomplete-link message without calling the API (TC-W-001)", async () => {
    const calls = mockApi({});
    renderPage("/invite", "/invite", <InvitePage />);
    expect(await screen.findByText(/This link is incomplete/)).toBeInTheDocument();
    expect(calls).toEqual([]);
  });

  it("creates a new account after the confirmation", async () => {
    const calls = mockApi({
      "POST /invites/inspect": { status: 200, body: info },
      "GET /me": loggedOut,
      "POST /invites/accept": { status: 201 },
    });
    const router = renderPage("/invite", "/invite#t=tok", <InvitePage />);
    expect(window.location.hash).toBe("");
    const name = await screen.findByLabelText("Your name");
    expect(name).toHaveValue("Maria");
    await userEvent.clear(name);
    await userEvent.type(name, "Maria Santoso");
    await userEvent.type(screen.getByLabelText("Choose a password", { exact: false }), "lagu pujian pagi");
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByText("You are joining GKY Citragarden as Maria Santoso.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Join" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(calls.find((c) => c.route === "POST /invites/accept")?.body).toEqual({
      token: "tok", name: "Maria Santoso", password: "lagu pujian pagi", email: "maria@example.org",
    });
  });

  it("switches to logging in when the email already has an account", async () => {
    mockApi({
      "POST /invites/inspect": { status: 200, body: info },
      "GET /me": loggedOut,
      "POST /invites/accept": { status: 409, body: { code: "identifier_taken" } },
    });
    renderPage("/invite", "/invite#t=tok", <InvitePage />);
    await userEvent.type(await screen.findByLabelText("Choose a password", { exact: false }), "lagu pujian pagi");
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));
    await userEvent.click(screen.getByRole("button", { name: "Join" }));
    expect(await screen.findByText(/already has an account. Log in to accept/)).toBeInTheDocument();
    expect(screen.getByLabelText("Email or phone number")).toHaveValue("maria@example.org");
  });

  it("joins with the account that is logged in", async () => {
    const calls = mockApi({
      "POST /invites/inspect": { status: 200, body: { ...info, owner_exists: true } },
      "GET /me": { status: 200, body: { user: { id: "u1", name: "Maria Santoso", email: null, phone: null, preferences: {} }, church: null, membership: null } },
      "POST /invites/accept-existing": { status: 201 },
    });
    const router = renderPage("/invite", "/invite#t=tok", <InvitePage />);
    await userEvent.click(await screen.findByRole("button", { name: "Join with this account" }));
    expect(screen.getByText("You are joining GKY Citragarden as Maria Santoso.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Join" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(calls.find((c) => c.route === "POST /invites/accept-existing")?.body).toEqual({ token: "tok" });
  });

  it("explains an expired link", async () => {
    mockApi({
      "POST /invites/inspect": { status: 400, body: { code: "invalid_token", reason: "expired" } },
      "GET /me": loggedOut,
    });
    renderPage("/invite", "/invite#t=tok", <InvitePage />);
    expect(await screen.findByText(/This link has expired/)).toBeInTheDocument();
  });
});
