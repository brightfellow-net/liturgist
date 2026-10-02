// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { defineConfig, devices } from "@playwright/test";
import { baseURL } from "./e2e/env";

// End-to-end tests run against the real binary (05 §9): global-setup builds
// it with the web app already built into web/dist, starts "liturgist serve"
// with a temporary data folder, and stops it afterwards. The "setup" project
// completes first-time setup; the others depend on it.
export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  workers: 1, // one server, one database
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    ...devices["Desktop Chrome"],
    baseURL,
    // PLAYWRIGHT_CHANNEL=chrome uses an installed Google Chrome instead of
    // Playwright's own Chromium ("pnpm exec playwright install chromium").
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
    trace: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /setup\.spec\.ts/ },
    { name: "app", testIgnore: /setup\.spec\.ts/, dependencies: ["setup"] },
  ],
});
