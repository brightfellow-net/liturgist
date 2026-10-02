// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import en from "@liturgist/i18n/en.json";
import id from "@liturgist/i18n/id.json";
import { browserLanguage, chooseLanguage } from "./i18n";
import { errorCodes } from "./errors";

const pluralSuffix = /_(zero|one|two|few|many|other)$/;

// keys returns the dotted keys of nested messages, plural forms folded into
// their base key (Indonesian has no singular form).
function keys(messages: object, prefix = ""): Set<string> {
  const out = new Set<string>();
  for (const [k, v] of Object.entries(messages)) {
    const key = prefix + k.replace(pluralSuffix, "");
    if (typeof v === "object" && v !== null) {
      for (const sub of keys(v, key + ".")) out.add(sub);
    } else {
      out.add(key);
    }
  }
  return out;
}

describe("TC-W-003 translations", () => {
  it("has the same keys in en and id", () => {
    const e = keys(en), i = keys(id);
    expect([...e].filter((k) => !i.has(k))).toEqual([]);
    expect([...i].filter((k) => !e.has(k))).toEqual([]);
  });

  it("has a message for every API error code", () => {
    const e = keys(en);
    const missing = errorCodes.filter((c) => !e.has("errors." + c) && ![...e].some((k) => k.startsWith("errors." + c + ".")));
    expect(missing).toEqual([]);
  });
});

describe("TC-W-005 language choice", () => {
  const me = (pref: "en" | "id" | undefined, church: "en" | "id" | null) => ({
    user: { id: "u", name: "Sari", email: null, phone: null, preferences: { text_size: "normal", ui_language: pref } },
    church: church === null ? null : ({ default_ui_language: church } as never),
  });

  it("follows the church default when the user has no preference", () => {
    expect(chooseLanguage(me(undefined, "id"))).toBe("id");
  });
  it("prefers the user's choice", () => {
    expect(chooseLanguage(me("en", "id"))).toBe("en");
  });
  it("falls back to English", () => {
    expect(chooseLanguage(me(undefined, null))).toBe("en");
  });
  it("uses the browser language before login", () => {
    expect(browserLanguage("id-ID")).toBe("id");
    expect(browserLanguage("en-GB")).toBe("en");
    expect(browserLanguage("nl")).toBe("en");
  });
});
