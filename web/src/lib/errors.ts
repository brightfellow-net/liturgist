// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { TFunction } from "i18next";

// Every API error code (01 §10); each has an "errors.<code>" message.
export const errorCodes = [
  "validation_failed", "weak_password", "invalid_identifier", "unauthenticated", "invalid_credentials",
  "forbidden", "scope_not_held", "csrf_rejected", "invite_identifier_mismatch", "limit_reached",
  "not_found", "invalid_token", "already_set_up", "not_set_up", "already_member", "invite_exists",
  "identifier_taken", "lockout_prevented", "role_name_taken", "reset_not_allowed", "too_many_attempts",
  "version_conflict", "song_in_use", "section_in_use", "group_conflict",
  "name_taken", "duty_in_use", "singing_part_in_use", "template_in_use",
  "invalid_reference", "reading_exists", "reading_in_use", "import_unreadable", "import_conflict",
  "liturgy_exists", "liturgy_locked", "liturgy_not_deletable", "assignment_exists",
  "internal", "unavailable",
] as const;

// Problem is the error body (RFC 9457 with "code", 01 §10).
export type Problem = {
  status?: number;
  code?: string;
  detail?: string;
  reason?: string;
  reading_id?: string; // reading_exists: the reading that is already saved
  scope?: string; // version_conflict: "item" or "liturgy" (10 §5)
  item_id?: string;
  liturgy_id?: string; // liturgy_exists: the liturgy that holds the slot
  scopes?: string[];
  limit?: string;
  used?: number;
  max?: number;
  errors?: { location?: string; message?: string }[];
};

// ApiError is a non-2xx response.
export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem;
  readonly retryAfter?: number; // seconds, from Retry-After

  constructor(status: number, problem: Problem, retryAfter?: number) {
    super(problem.code ?? "http_" + status);
    this.status = status;
    this.problem = problem;
    this.retryAfter = retryAfter;
  }

  get code(): string | undefined {
    return this.problem.code;
  }

  static from(response: Response, body: unknown): ApiError {
    const problem = typeof body === "object" && body !== null ? (body as Problem) : {};
    const header = Number(response.headers.get("Retry-After"));
    return new ApiError(response.status, problem, Number.isFinite(header) && header > 0 ? header : undefined);
  }
}

// NetworkError means the server could not be reached.
export class NetworkError extends Error {}

export function isCode(err: unknown, code: string): boolean {
  return err instanceof ApiError && err.code === code;
}

// errorText translates an error by its code, never by the server's detail
// text (05 §8). Unknown codes get the generic message. scopeName turns a
// scope such as "liturgy.edit" into its description, when known.
export function errorText(t: TFunction, err: unknown, scopeName: (scope: string) => string = (s) => s): string {
  if (err instanceof NetworkError) return t("errors.network");
  if (!(err instanceof ApiError)) return t("errors.generic");
  const p = err.problem;
  // 413 has no code of its own: the body was too large (08 §5).
  if (err.status === 413) return t("errors.too_large");
  switch (p.code) {
    case "too_many_attempts": {
      const minutes = Math.max(1, Math.ceil((err.retryAfter ?? 60) / 60));
      return t("errors.too_many_attempts", { count: minutes });
    }
    case "invalid_token":
    case "weak_password": {
      const text = t(`errors.${p.code}.${p.reason ?? ""}`, { defaultValue: "" });
      return text || t(`errors.${p.code}.unknown`);
    }
    case "group_conflict": {
      const text = t(`errors.group_conflict.${p.reason ?? ""}`, { defaultValue: "" });
      return text || t("errors.group_conflict.unknown");
    }
    case "invalid_reference":
    case "import_unreadable":
    case "import_conflict": {
      const text = t(`errors.${p.code}.${p.reason ?? ""}`, { defaultValue: "" });
      return text || t(`errors.${p.code}.unknown`);
    }
    case "validation_failed":
      // A fixed count such as 50 duties (09 §2.1): say how many are allowed.
      if (p.reason === "limit") return t("errors.limit", { max: p.max ?? 0 });
      return p.reason === "language_mismatch" ? t("errors.language_mismatch") : t("errors.validation_failed");
    case "name_taken": {
      const text = t(`errors.name_taken.${p.reason ?? ""}`, { defaultValue: "" });
      return text || t("errors.name_taken.unknown");
    }
    case "limit_reached":
      // The liturgy limits name which count was reached (10 §8).
      return p.limit?.includes("liturgies") ? t("errors.liturgy_limit", { max: p.max ?? 0 }) : t("errors.limit_reached", { max: p.max ?? 0 });
    case "scope_not_held":
      return t("errors.scope_not_held", { scopes: (p.scopes ?? []).map(scopeName).join("; ") });
  }
  if (p.code) {
    const text = t(`errors.${p.code}`, { defaultValue: "" });
    if (text) return text;
  }
  return t("errors.generic");
}

// fieldErrors returns the fields a 422 response marks as invalid, keyed by
// their path in the request body ("church.name"). The server's English
// messages are not shown; forms show a translated message per field.
export function fieldErrors(err: unknown): string[] {
  if (!(err instanceof ApiError) || err.status !== 422) return [];
  return (err.problem.errors ?? [])
    .map((e) => (e.location ?? "").replace(/^body\./, ""))
    .filter((f) => f !== "");
}
