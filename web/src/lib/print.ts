// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// What the print page can change (13 §6). The church's settings are the
// defaults; the choice lives in the page's query string, so a link keeps it.
export type PrintDefaults = {
  paper: "a4" | "f4";
  lyrics: "full" | "first_lines";
  readings: boolean;
  assignments: boolean;
  keys: boolean;
  notes: boolean;
  size: "normal" | "large";
};
export type PrintOptions = PrintDefaults & { variant: "team" | "musician" };

export const printVariants = ["team", "musician"] as const;
export const printFlags = ["readings", "assignments", "keys", "notes"] as const;

// The paper sizes of @page (13 §6): F4 is 215 x 330 mm.
export const pageSize = { a4: "A4", f4: "215mm 330mm" } as const;

function oneOf<T extends string>(value: string | null, allowed: readonly T[], fallback: T): T {
  return allowed.find((a) => a === value) ?? fallback;
}

// parsePrint reads the options from the query string; a missing or invalid
// value falls back to the church default.
export function parsePrint(params: URLSearchParams, d: PrintDefaults): PrintOptions {
  const flag = (name: (typeof printFlags)[number]) => {
    const v = params.get(name);
    return v === "1" ? true : v === "0" ? false : d[name];
  };
  return {
    variant: oneOf(params.get("variant"), printVariants, "team"),
    paper: oneOf(params.get("paper"), ["a4", "f4"], d.paper),
    lyrics: oneOf(params.get("lyrics"), ["full", "first_lines"], d.lyrics),
    readings: flag("readings"), assignments: flag("assignments"), keys: flag("keys"), notes: flag("notes"),
    size: oneOf(params.get("size"), ["normal", "large"], d.size),
  };
}

// printParams writes every option, so a copied link prints the same page.
export function printParams(o: PrintOptions): URLSearchParams {
  const p = new URLSearchParams({ variant: o.variant, paper: o.paper, lyrics: o.lyrics, size: o.size });
  for (const f of printFlags) p.set(f, o[f] ? "1" : "0");
  return p;
}

// firstLine is the first line of a section's text that is not blank.
export function firstLine(text: string): string {
  return text.split("\n").map((l) => l.trim()).find((l) => l !== "") ?? "";
}
