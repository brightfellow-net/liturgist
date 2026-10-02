// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { jsonBody } from "./api";

type OnRequest = (o: { request: Request }) => Request | undefined;
const onRequest = jsonBody.onRequest as unknown as OnRequest;

describe("jsonBody", () => {
  it("sends bodyless unsafe requests as JSON with {}", async () => {
    const out = onRequest({ request: new Request("http://x/api/v1/invites/1", { method: "DELETE" }) });
    expect(out?.headers.get("Content-Type")).toBe("application/json");
    expect(await out?.text()).toBe("{}");
  });

  it("leaves GET and requests that already have a body alone", () => {
    expect(onRequest({ request: new Request("http://x/api/v1/me") })).toBeUndefined();
    const post = new Request("http://x/api/v1/auth/login", {
      method: "POST", body: "{}", headers: { "Content-Type": "application/json" },
    });
    expect(onRequest({ request: post })).toBeUndefined();
  });
});
