// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
/// <reference types="vitest/config" />
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// serviceWorker writes dist/sw.js from web/sw/sw.js: it names the caches after
// this build and lists the files to precache (13 §7). The build id is a hash of
// those files' names, which carry their content hashes, so it changes exactly
// when the app does.
function serviceWorker(): Plugin {
  return {
    name: "liturgist-service-worker",
    apply: "build",
    generateBundle(_options, bundle) {
      const files = Object.keys(bundle).filter((f) => !f.endsWith(".map") && f !== "index.html").sort();
      const index = bundle["index.html"];
      const html = index && index.type === "asset" ? String(index.source) : "";
      const build = createHash("sha256").update(files.join("\n")).update(html).digest("hex").slice(0, 12);
      const shell = ["/", "/manifest.webmanifest", "/icon-192.png", ...files.map((f) => "/" + f)];
      const source = readFileSync(fileURLToPath(new URL("./sw/sw.js", import.meta.url)), "utf8")
        .replace("__BUILD__", build)
        .replace("__SHELL__", JSON.stringify(shell));
      this.emitFile({ type: "asset", fileName: "sw.js", source });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), serviceWorker()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  build: { outDir: "dist", assetsDir: "assets" },
  server: { port: 5173, proxy: { "/api": "http://localhost:8080" } },
  // Page tests type into forms with user-event, which is slow in jsdom when
  // many test files run at once; the default 5 s is too tight.
  test: { environment: "jsdom", setupFiles: ["./src/test/setup.ts"], include: ["src/**/*.test.{ts,tsx}"], testTimeout: 20000 },
});
