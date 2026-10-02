// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { readFragmentToken } from "./fragmentToken";

describe("TC-W-001 fragmentToken", () => {
  it("returns the token and removes it from the address bar", () => {
    window.history.replaceState(null, "", "/invite?x=1#t=abc");
    expect(readFragmentToken()).toBe("abc");
    expect(window.location.hash).toBe("");
    expect(window.location.pathname + window.location.search).toBe("/invite?x=1");
  });

  it("returns null without a fragment", () => {
    window.history.replaceState(null, "", "/invite");
    expect(readFragmentToken()).toBeNull();
  });

  it("returns null for an empty or different fragment, and still removes it", () => {
    window.history.replaceState(null, "", "/reset#t=");
    expect(readFragmentToken()).toBeNull();
    window.history.replaceState(null, "", "/reset#other=1");
    expect(readFragmentToken()).toBeNull();
    expect(window.location.hash).toBe("");
  });
});
