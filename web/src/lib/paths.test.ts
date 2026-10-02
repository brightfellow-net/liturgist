// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { loginWithNext, safeNext } from "@/routes/paths";

describe("paths", () => {
  it("only returns to paths inside the app after login", () => {
    expect(safeNext("/settings/members")).toBe("/settings/members");
    expect(safeNext(null)).toBe("/");
    expect(safeNext("//evil.example")).toBe("/");
    expect(safeNext("/\\evil.example")).toBe("/");
    expect(safeNext("https://evil.example")).toBe("/");
  });

  it("builds the login link", () => {
    expect(loginWithNext("/")).toBe("/login");
    expect(loginWithNext("/profile?x=1")).toBe("/login?next=%2Fprofile%3Fx%3D1");
  });
});
