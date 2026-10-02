// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Me, MemberView } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { MembersPage } from "./MembersPage";

function meWith(scopes: string[]): Me {
  return {
    user: { id: "u1", name: "Ruth", email: "ruth@example.org", phone: null, preferences: {} },
    church: { name: "GKY Uji", time_zone: "Asia/Jakarta" },
    membership: { id: "m1", roles: [], scopes, actions: {} },
  } as unknown as Me;
}

function member(id: string, name: string, actions: MemberView["actions"]): MemberView {
  return { id, user_id: "u" + id, name, email: name.toLowerCase() + "@example.org", phone: null,
    joined_at: "2026-10-01T00:00:00Z", roles: [], actions };
}

const none = { remove: false, edit_roles: false, create_reset_link: false };
const all = { remove: true, edit_roles: true, create_reset_link: true };
const usage = { team_members: { used: 3, max: null } };

afterEach(() => vi.unstubAllGlobals());

describe("MembersPage", () => {
  void i18n.changeLanguage("en");

  it("shows no remove button when actions.remove is false (TC-W-004)", async () => {
    mockApi({ "GET /members": { status: 200, body: { members: [member("1", "Maria", { ...all, remove: false })], usage } } });
    renderPage("/settings/members", "/settings/members", <MembersPage />, meWith(["members.view"]));
    const item = (await screen.findByText("Maria")).closest("li")!;
    expect(within(item).queryByRole("button", { name: "Remove from church" })).toBeNull();
    expect(within(item).getByRole("button", { name: "Change roles" })).toBeInTheDocument();
  });

  it("is a read-only list when every action is false (TC-W-004)", async () => {
    const calls = mockApi({ "GET /members": { status: 200, body: { members: [member("1", "Maria", none), member("2", "Yohanes", none)], usage } } });
    renderPage("/settings/members", "/settings/members", <MembersPage />, meWith(["members.view"]));
    await screen.findByText("Yohanes");
    expect(screen.queryAllByRole("button")).toEqual([]);
    expect(screen.queryByText("Invites")).toBeNull(); // no members.manage
    expect(calls.map((c) => c.route)).toEqual(["GET /members"]);
  });

  it("shows usage when the church has a limit", async () => {
    mockApi({ "GET /members": { status: 200, body: { members: [], usage: { team_members: { used: 9, max: 12 } } } } });
    renderPage("/settings/members", "/settings/members", <MembersPage />, meWith(["members.view"]));
    expect(await screen.findByText(/9 of 12 team members/)).toBeInTheDocument();
  });

  it("asks before removing a member", async () => {
    const calls = mockApi({
      "GET /members": { status: 200, body: { members: [member("7", "Maria", all)], usage } },
      "DELETE /members/7": { status: 204 },
    });
    renderPage("/settings/members", "/settings/members", <MembersPage />, meWith(["members.view"]));
    await userEvent.click(await screen.findByRole("button", { name: "Remove from church" }));
    expect(calls.some((c) => c.route === "DELETE /members/7")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Remove Maria" }));
    await vi.waitFor(() => expect(calls.some((c) => c.route === "DELETE /members/7")).toBe(true));
  });

  it("creates an invite with roles and shows its link once", async () => {
    const calls = mockApi({
      "GET /members": { status: 200, body: { members: [], usage } },
      "GET /invites": { status: 200, body: [] },
      "GET /roles": { status: 200, body: [{ id: "r1", name: "Liturgist", description: "", origin: "liturgist", scopes: [], member_count: 0, actions: { edit: true, delete: true } }] },
      "POST /invites": {
        status: 201,
        body: { link: "http://localhost/invite#t=abc", expires_at: "2026-10-09T03:00:00Z",
          invite: { id: "i1", name: "Yohanes", email: null, phone: "+6281234567890", roles: [], status: "pending",
            created_at: "2026-10-02T03:00:00Z", created_by_name: "Ruth", expires_at: "2026-10-09T03:00:00Z", actions: { cancel: true, regenerate: true } } },
      },
    });
    renderPage("/settings/members", "/settings/members", <MembersPage />, meWith(["members.view", "members.manage"]));
    await userEvent.click(await screen.findByRole("button", { name: "Invite a person" }));
    await userEvent.type(screen.getByLabelText("Name"), "Yohanes");
    await userEvent.type(screen.getByLabelText("Phone number"), "081234567890");
    await userEvent.click(await screen.findByLabelText("Liturgist"));
    await userEvent.click(screen.getByRole("button", { name: "Create invite" }));
    expect(await screen.findByLabelText("Invite link for Yohanes")).toHaveValue("http://localhost/invite#t=abc");
    expect(screen.getByRole("link", { name: "Share to WhatsApp" }).getAttribute("href")).toContain(encodeURIComponent("http://localhost/invite#t=abc"));
    expect(calls.find((c) => c.route === "POST /invites")?.body).toEqual({ name: "Yohanes", phone: "081234567890", role_ids: ["r1"] });
  });
});
