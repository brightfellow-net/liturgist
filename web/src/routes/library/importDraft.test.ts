// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { candidate } from "@/test/library";
import { draftToValues, valuesToDraft } from "./importDraft";

describe("draft and form values", () => {
  it("turns arrangement positions into section names and back", () => {
    const d = candidate("c1", "Besar Setia-Mu").draft;
    const v = draftToValues(d);
    expect(v.sections.map((s) => s.ref)).toEqual(["d0", "d1"]);
    expect(v.arrangement).toEqual(["d0", "d1", "d0"]);
    expect(valuesToDraft(v).default_arrangement).toEqual([0, 1, 0]);
  });

  it("follows a section that moved, and drops one that was removed", () => {
    const v = draftToValues(candidate("c1", "X").draft);
    const swapped = { ...v, sections: [v.sections[1], v.sections[0]] };
    expect(valuesToDraft(swapped).default_arrangement).toEqual([1, 0, 1]);
    const removed = { ...v, sections: [v.sections[1]] };
    expect(valuesToDraft(removed).default_arrangement).toEqual([0]);
  });

  it("sends the verse number of verses only and keeps fields it does not show", () => {
    const d = { ...candidate("c1", "X").draft, lyricist: "Thomas Chisholm", licence_status: "public_domain" as const };
    const out = valuesToDraft(draftToValues(d));
    expect(out.sections).toEqual([
      { kind: "verse", number: 1, label: "", text: "Besar setia-Mu" },
      { kind: "chorus", label: "", text: "Setiap pagi" },
    ]);
    expect(out.lyricist).toBe("Thomas Chisholm");
    expect(out.licence_status).toBe("public_domain");
  });
});
