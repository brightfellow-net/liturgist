// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { meWith } from "@/test/library";
import { systemStatus } from "@/test/system";
import { SystemPage } from "./SystemPage";

const admin = meWith(["church.settings"]);

function show(body: unknown, status = 200) {
  mockApi({ "GET /system/status": { status, body } });
  renderPage("/settings/system", "/settings/system", <SystemPage />, admin);
}

afterEach(() => vi.unstubAllGlobals());

describe("SystemPage (IT-605)", () => {
  void i18n.changeLanguage("en");

  it("shows the facts in the church's time zone", async () => {
    show(systemStatus());
    expect(await screen.findByText("v0.3.0")).toBeInTheDocument();
    expect(screen.getByText("sqlite, 3 MB, schema 14")).toBeInTheDocument();
    expect(screen.getByText("40 GB of 100 GB")).toBeInTheDocument();
    // 19:00 UTC is 02:00 the next day in Jakarta.
    expect(screen.getByText(/^Oct 6, 2026.*2:00.*\(automatic\)$/)).toBeInTheDocument();
    expect(screen.getByText(/^Oct 1, 2026.*10:00/)).toBeInTheDocument();
    expect(screen.getByText("Not set up")).toBeInTheDocument();
    expect(screen.getByText("HTTPS through a proxy")).toBeInTheDocument();
    expect(screen.getByText("The update check is off.")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers the backup download as a plain link to the API", async () => {
    show(systemStatus());
    const link = await screen.findByRole("link", { name: "Download backup" });
    expect(link).toHaveAttribute("href", "/api/v1/system/backup");
    expect(link).toHaveAttribute("download");
    expect(screen.getByText("Automatic backups are on.")).toBeInTheDocument();
  });

  it("says when there is no backup and none was taken away", async () => {
    show(systemStatus({ backup: { supported: true, scheduled: false, last_at: null, last_kind: "", last_copied_at: null, warning: "" } }));
    expect(await screen.findByText("None yet")).toBeInTheDocument();
    expect(screen.getByText("Never")).toBeInTheDocument();
    expect(screen.getByText("Automatic backups are off.")).toBeInTheDocument();
  });

  it("warns about a full disk, an old backup and plain HTTP", async () => {
    show(systemStatus({
      disk: { free_bytes: 300 * 1024 * 1024, total_bytes: 10 * 1024 ** 3, low: true },
      backup: { supported: true, scheduled: true, last_at: "2026-10-01T03:00:00Z", last_kind: "manual", last_copied_at: null, warning: "stale" },
      https: { mode: "plain_http", plain_http_warning: true, proxy_missing_warning: false },
    }));
    const alerts = await screen.findAllByRole("alert");
    expect(alerts.map((a) => a.textContent)).toEqual([
      "Server storage is almost full: 300 MB left. Free some space soon, or saving and backups may fail.",
      "There has been no backup in the last 2 days.",
      "Liturgist is reachable over plain HTTP: passwords can be read on the network. Use HTTPS.",
    ]);
  });

  it.each([
    ["plain_http", "Plain HTTP"],
    ["behind_proxy", "HTTPS through a proxy"],
    ["built_in", "Built-in HTTPS"],
  ] as const)("names the way HTTPS is provided: %s", async (mode, text) => {
    show(systemStatus({ https: { mode, plain_http_warning: false, proxy_missing_warning: false } }));
    expect(await screen.findByText(text)).toBeInTheDocument();
  });

  it("has no download with PostgreSQL", async () => {
    show(systemStatus({ database: { driver: "postgres", size_bytes: 1, schema_version: 14 }, backup: { supported: false, scheduled: false, last_at: null, last_kind: "", last_copied_at: null, warning: "" } }));
    expect(await screen.findByText("Backups of PostgreSQL are made with pg_dump; see the backup guide.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Download backup" })).toBeNull();
  });

  it("reports the update check", async () => {
    for (const [update, text] of [
      [{ enabled: true, latest: "", available: false }, "Not checked yet."],
      [{ enabled: true, latest: "v0.4.0", available: true }, "Version v0.4.0 is available."],
      [{ enabled: true, latest: "v0.3.0", available: false }, "This is the newest version."],
    ] as const) {
      document.body.innerHTML = "";
      show(systemStatus({ update }));
      expect(await screen.findByText(text)).toBeInTheDocument();
    }
  });

  it("says the page is not available on a server without the system routes", async () => {
    show({ code: "not_found" }, 404);
    expect(await screen.findByText("This page is not available in this edition.")).toBeInTheDocument();
  });

  it("is shown in Indonesian", async () => {
    await i18n.changeLanguage("id");
    try {
      show(systemStatus());
      expect(await screen.findByText("sqlite, 3 MB, skema 14")).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Unduh cadangan" })).toBeInTheDocument();
    } finally {
      await i18n.changeLanguage("en");
    }
  });
});
