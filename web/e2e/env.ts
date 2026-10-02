// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import os from "node:os";
import path from "node:path";

// The test server's address and working folder. Fixed (not random), because
// the config and every worker must agree on them.
export const port = Number(process.env.E2E_PORT || 8099);
export const baseURL = `http://localhost:${port}`;
export const workDir = path.join(os.tmpdir(), `liturgist-e2e-${port}`);
export const adminState = path.join(workDir, "admin.json");
export const setupLinkFile = path.join(workDir, "setup-link");

export const admin = { name: "Ruth Admin", email: "ruth@example.org", password: "pohon ara berbuah lebat" };
export const churchName = "GKY Uji";
