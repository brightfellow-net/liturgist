// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { meWith } from "@/test/library";
import { systemStatus } from "@/test/system";
import { SystemBanners } from "./SystemBanners";

const lowDisk = { free_bytes: 300 * 1024 * 1024, total_bytes: 10 * 1024 ** 3, low: true };
const stale = { supported: true, scheduled: true, last_at: null, last_kind: "", last_copied_at: null, warning: "stale" };

afterEach(() => vi.unstubAllGlobals());

function page(me: ReturnType<typeof meWith>) {
  renderPage("/", "/", <SystemBanners me={me} />);
}

describe("SystemBanners (IT-605)", () => {
  void i18n.changeLanguage("en");

  it("shows the warnings to a church admin, each with a link to the system page", async () => {
    mockApi({ "GET /system/status": { status: 200, body: systemStatus({ disk: lowDisk, backup: stale }) } });
    page(meWith(["church.settings"]));
    expect(await screen.findByText(/Server storage is almost full: 300 MB left/)).toBeInTheDocument();
    expect(screen.getByText("There has been no backup in the last 2 days.")).toBeInTheDocument();
    const links = screen.getAllByRole("link", { name: "Open the system page" });
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute("href", "/settings/system");
  });

  it("can be dismissed one at a time", async () => {
    mockApi({ "GET /system/status": { status: 200, body: systemStatus({ disk: lowDisk, backup: stale }) } });
    page(meWith(["church.settings"]));
    await screen.findByText(/Server storage is almost full/);
    await userEvent.click(screen.getAllByRole("button", { name: "Dismiss" })[0]);
    expect(screen.queryByText(/Server storage is almost full/)).toBeNull();
    expect(screen.getByText("There has been no backup in the last 2 days.")).toBeInTheDocument();
  });

  it("shows nothing when all is well", async () => {
    const calls = mockApi({ "GET /system/status": { status: 200, body: systemStatus() } });
    page(meWith(["church.settings"]));
    await vi.waitFor(() => expect(calls).toHaveLength(1));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("does not even ask a member without church.settings", async () => {
    const calls = mockApi({ "GET /system/status": { status: 200, body: systemStatus({ disk: lowDisk }) } });
    page(meWith(["liturgy.edit", "members.manage"]));
    await new Promise((r) => setTimeout(r, 50));
    expect(calls).toHaveLength(0);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("stays silent on a server without the system routes and on errors", async () => {
    for (const [status, body] of [[404, { code: "not_found" }], [500, { code: "internal" }]] as const) {
      document.body.innerHTML = "";
      const calls = mockApi({ "GET /system/status": { status, body } });
      page(meWith(["church.settings"]));
      await vi.waitFor(() => expect(calls.length).toBeGreaterThan(0));
      await new Promise((r) => setTimeout(r, 20));
      expect(screen.queryByRole("alert")).toBeNull();
    }
  });
});
