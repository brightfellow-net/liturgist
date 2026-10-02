// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(cleanup);

// Node's Request (unlike a browser's) rejects relative URLs such as the
// API client's "/api/v1/…"; resolve them against the test page's address.
const NodeRequest = globalThis.Request;
globalThis.Request = class extends NodeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(typeof input === "string" ? new URL(input, window.location.href) : input, init);
  }
};
