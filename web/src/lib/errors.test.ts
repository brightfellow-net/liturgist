// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import i18n from "./i18n";
import { ApiError, NetworkError, errorText, fieldErrors } from "./errors";

const t = i18n.getFixedT("en");

function apiError(status: number, body: object, headers: Record<string, string> = {}) {
  return ApiError.from(new Response(null, { status, headers }), body);
}

describe("TC-W-002 error messages", () => {
  it("says how long to wait after too many attempts", () => {
    const err = apiError(429, { code: "too_many_attempts" }, { "Retry-After": "900" });
    expect(errorText(t, err)).toBe("Too many attempts. Try again in 15 minutes.");
  });

  it("translates by code and reason, never by detail", () => {
    expect(errorText(t, apiError(400, { code: "invalid_token", reason: "expired", detail: "x" }))).toBe(
      "This link has expired. Ask your church admin for a new one.",
    );
    expect(errorText(t, apiError(422, { code: "weak_password", reason: "common" }))).toBe(
      "This password is too common. Choose another.",
    );
    expect(errorText(t, apiError(403, { code: "limit_reached", max: 12 }))).toContain("12 team members");
  });

  it("falls back to generic text", () => {
    expect(errorText(t, apiError(418, { code: "teapot", detail: "I'm a teapot" }))).toBe("Something went wrong. Please try again.");
    expect(errorText(t, apiError(400, { code: "invalid_token", reason: "new_reason" }))).toBe(
      "This link can't be used. Ask your church admin for a new one.",
    );
    expect(errorText(t, new Error("boom"))).toBe("Something went wrong. Please try again.");
  });

  it("names missing permissions by their descriptions when known", () => {
    const err = apiError(403, { code: "scope_not_held", scopes: ["liturgy.edit", "x.new"] });
    const names: Record<string, string> = { "liturgy.edit": "Create and edit liturgies" };
    expect(errorText(t, err, (s) => names[s] ?? s)).toBe(
      "You can only give permissions you have yourself: Create and edit liturgies; x.new.",
    );
  });

  it("explains why songs cannot be linked, by reason", () => {
    expect(errorText(t, apiError(409, { code: "group_conflict", reason: "language_taken" }))).toBe("There is already a version in this language.");
    expect(errorText(t, apiError(409, { code: "group_conflict", reason: "already_grouped" }))).toBe(
      "One of these songs is already linked to other versions. Unlink it first.",
    );
    expect(errorText(t, apiError(409, { code: "group_conflict" }))).toBe("These songs can't be linked.");
    expect(errorText(t, apiError(409, { code: "version_conflict" }))).toBe("This song was changed by someone else. Reload to see their version.");
  });

  it("explains network failures", () => {
    expect(errorText(t, new NetworkError("x"))).toBe("Can't reach the server. Check your connection.");
  });

  it("is translated", () => {
    const err = apiError(429, { code: "too_many_attempts" }, { "Retry-After": "60" });
    expect(errorText(i18n.getFixedT("id"), err)).toBe("Terlalu banyak percobaan. Coba lagi dalam 1 menit.");
  });
});

describe("fieldErrors", () => {
  it("returns body field paths of a 422", () => {
    const err = apiError(422, { code: "validation_failed", errors: [{ location: "body.church.name", message: "x" }] });
    expect(fieldErrors(err)).toEqual(["church.name"]);
    expect(fieldErrors(apiError(409, { code: "already_set_up" }))).toEqual([]);
  });
});
