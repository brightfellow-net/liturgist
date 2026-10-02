// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { RolesPage } from "./RolesPage";

const scopes = [
  { scope: "members.view", description: "See the member list" },
  { scope: "roles.manage", description: "Manage roles" },
];
const admin = { id: "r1", name: "Church admin", description: "", origin: "church_admin", scopes: ["members.view", "roles.manage"], member_count: 2, actions: { edit: true, delete: false } };

afterEach(() => vi.unstubAllGlobals());

describe("RolesPage", () => {
  void i18n.changeLanguage("en");

  it("lists roles with member counts and follows actions", async () => {
    mockApi({ "GET /roles": { status: 200, body: [admin] }, "GET /scopes": { status: 200, body: scopes } });
    renderPage("/settings/roles", "/settings/roles", <RolesPage />);
    const item = (await screen.findByText("Church admin")).closest("li")!;
    expect(within(item).getByText("2 members have this role.")).toBeInTheDocument();
    expect(within(item).getByText("Manage roles")).toBeInTheDocument();
    expect(within(item).queryByRole("button", { name: "Delete role" })).toBeNull();
  });

  it("creates a role with the ticked permissions", async () => {
    const calls = mockApi({
      "GET /roles": { status: 200, body: [admin] },
      "GET /scopes": { status: 200, body: scopes },
      "POST /roles": { status: 201, body: { ...admin, id: "r2", name: "Multimedia" } },
    });
    renderPage("/settings/roles", "/settings/roles", <RolesPage />);
    await userEvent.click(await screen.findByRole("button", { name: "New role" }));
    await userEvent.type(screen.getByLabelText("Role name"), "Multimedia");
    await userEvent.click(screen.getByLabelText("See the member list"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await vi.waitFor(() =>
      expect(calls.find((c) => c.route === "POST /roles")?.body).toEqual({ name: "Multimedia", description: "", scopes: ["members.view"] }),
    );
  });

  it("asks before deleting and says how many members hold the role", async () => {
    mockApi({
      "GET /roles": { status: 200, body: [{ ...admin, name: "Editor", actions: { edit: true, delete: true }, member_count: 1 }] },
      "GET /scopes": { status: 200, body: scopes },
    });
    renderPage("/settings/roles", "/settings/roles", <RolesPage />);
    await userEvent.click(await screen.findByRole("button", { name: "Delete role" }));
    expect(screen.getByText("Delete the role Editor? 1 member will lose it.")).toBeInTheDocument();
  });

  it("shows the lock-out safeguard", async () => {
    mockApi({
      "GET /roles": { status: 200, body: [admin] },
      "GET /scopes": { status: 200, body: scopes },
      "PATCH /roles/r1": { status: 409, body: { code: "lockout_prevented" } },
    });
    renderPage("/settings/roles", "/settings/roles", <RolesPage />);
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    await userEvent.click(screen.getByLabelText("Manage roles"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Someone must keep the permissions to manage roles and members.")).toBeInTheDocument();
  });
});
