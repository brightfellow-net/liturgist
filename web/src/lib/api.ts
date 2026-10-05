// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import createClient, { type Middleware } from "openapi-fetch";
import type { paths } from "@liturgist/api-client";
import { ApiError, NetworkError } from "./errors";
import { trackSavedCopies } from "./offline";

// jsonBody sends every unsafe request as JSON, with "{}" when it has no body,
// as the CSRF rules require (03 §6, 05 §4).
export const jsonBody: Middleware = {
  onRequest({ request }) {
    if (request.method === "GET" || request.method === "HEAD" || request.headers.has("Content-Type")) {
      return undefined;
    }
    const headers = new Headers(request.headers);
    headers.set("Content-Type", "application/json");
    return new Request(request, { headers, body: "{}" });
  },
};

// fetch is looked up on each call, so tests can replace globalThis.fetch.
export const api = createClient<paths>({
  baseUrl: "/api/v1",
  credentials: "same-origin",
  fetch: (request) => globalThis.fetch(request),
});
api.use(jsonBody);
api.use(trackSavedCopies);

type Result<T> = { data?: T; error?: unknown; response: Response };

// call awaits an openapi-fetch call and returns its data, or throws ApiError
// (non-2xx) or NetworkError (no response).
export async function call<T>(request: Promise<Result<T>>): Promise<T> {
  let result: Result<T>;
  try {
    result = await request;
  } catch {
    throw new NetworkError("network");
  }
  if (!result.response.ok) throw ApiError.from(result.response, result.error);
  return result.data as T;
}
