// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import type { ImportCandidateView } from "@liturgist/api-client";
import { api, call } from "./api";

// Query keys are fixed (05 §4): the open batches under ["imports"], one batch
// under ["import", id].
export const importsQuery = queryOptions({
  queryKey: ["imports"],
  queryFn: () => call(api.GET("/imports")),
});

export const importQuery = (id: string) =>
  queryOptions({
    queryKey: ["import", id],
    queryFn: () => call(api.GET("/imports/{id}", { params: { path: { id } } })),
  });

// Limits of 08 §5 that the browser can check before sending.
export const importLimits = { files: 200, fileBytes: 1024 * 1024, totalBytes: 5 * 1024 * 1024, pasteChars: 200_000 } as const;

// The codes a candidate can carry (08 §3, §4); each has an
// "import.warnings.<code>" message. Unknown codes get a generic message.
export const warningCodes = [
  "blocks_numbered", "chorus_guessed", "verse_renumbered", "number_dropped", "ccli_ignored", "key_ignored",
  "arrangement_ignored", "comment_ignored", "block_ignored", "directive_ignored", "duplicate_in_batch", "title_from_file",
] as const;
export const failureCodes = ["validation_failed", "section_in_use", "not_found", "target_changed"] as const;
export const fileReasons = ["not_utf8", "not_xml", "no_song", "too_many_sections", "file_too_large", "too_complex"] as const;

export function warningText(t: TFunction, code: string): string {
  return (warningCodes as readonly string[]).includes(code) ? t(`import.warnings.${code}`) : t("import.warnings.unknown", { code });
}

export function failureText(t: TFunction, code: string): string {
  return (failureCodes as readonly string[]).includes(code) ? t(`import.failures.${code}`) : t("import.failures.unknown");
}

export function reasonText(t: TFunction, reason: string): string {
  return (fileReasons as readonly string[]).includes(reason) ? t(`import.reasons.${reason}`) : t("import.reasons.unknown");
}

// A file read in the browser: its name and decoded text, or the reason it
// cannot be sent.
export type PickedFile = { name: string; text: string } | { name: string; reason: "not_utf8" | "file_too_large" };

// readFiles reads the chosen files as strict UTF-8 (08 §5: the browser reads
// files with File.text()). A file that is not UTF-8 or is over 1 MiB is
// reported rather than sent.
export async function readFiles(files: File[]): Promise<PickedFile[]> {
  const out: PickedFile[] = [];
  for (const f of files) {
    if (f.size > importLimits.fileBytes) {
      out.push({ name: f.name, reason: "file_too_large" });
      continue;
    }
    try {
      out.push({ name: f.name, text: new TextDecoder("utf-8", { fatal: true }).decode(await f.arrayBuffer()) });
    } catch {
      out.push({ name: f.name, reason: "not_utf8" });
    }
  }
  return out;
}

// A candidate's state in the review.
export const chosen = (c: ImportCandidateView) => c.decision === "accept" || c.decision === "merge";
export const isDone = (c: ImportCandidateView) => c.outcome === "applied";
