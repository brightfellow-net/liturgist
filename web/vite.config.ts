// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
/// <reference types="vitest/config" />
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  build: { outDir: "dist", assetsDir: "assets" },
  server: { port: 5173, proxy: { "/api": "http://localhost:8080" } },
  // Page tests type into forms with user-event, which is slow in jsdom when
  // many test files run at once; the default 5 s is too tight.
  test: { environment: "jsdom", setupFiles: ["./src/test/setup.ts"], include: ["src/**/*.test.{ts,tsx}"], testTimeout: 20000 },
});
