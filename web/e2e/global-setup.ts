// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { baseURL, port, setupLinkFile, workDir } from "./env";

const repo = fileURLToPath(new URL("../..", import.meta.url));

// globalSetup builds and starts the server; the function it returns stops
// it. The data folder is deleted unless E2E_KEEP is set.
export default async function globalSetup() {
  if (!fs.existsSync(path.join(repo, "web/dist/index.html"))) {
    throw new Error("web/dist is empty: run \"pnpm --filter web build\" first");
  }
  fs.rmSync(workDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(workDir, "data"), { recursive: true });
  const bin = path.join(workDir, process.platform === "win32" ? "liturgist.exe" : "liturgist");
  execFileSync("go", ["build", "-o", bin, "./cmd/liturgist"], { cwd: repo, stdio: "inherit" });

  const log = fs.openSync(path.join(workDir, "serve.log"), "w");
  const server = spawn(bin, ["serve"], {
    env: {
      ...process.env,
      LITURGIST_DATA_DIR: path.join(workDir, "data"),
      LITURGIST_LISTEN: `127.0.0.1:${port}`,
      LITURGIST_BASE_URL: baseURL,
    },
    stdio: ["ignore", log, log],
  });
  const exited = new Promise((resolve) => server.once("exit", resolve));

  const link = await waitForSetupLink();
  fs.writeFileSync(setupLinkFile, link);

  return async () => {
    server.kill("SIGTERM");
    await exited;
    if (!process.env.E2E_KEEP) fs.rmSync(workDir, { recursive: true, force: true });
  };
}

// waitForSetupLink waits until the server answers and has printed the
// setup link (01 §8) to its log.
async function waitForSetupLink(): Promise<string> {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(baseURL + "/healthz");
      const found = fs.readFileSync(path.join(workDir, "serve.log"), "utf8").match(/https?:\/\/\S+\/setup#t=[\w-]+/);
      if (res.ok && found) return found[0];
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error("the server did not start; see " + path.join(workDir, "serve.log"));
}
