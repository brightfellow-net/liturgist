// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import { api, call } from "./api";
import { summaryOf } from "./messages";

// The page size of the published list (13 §5).
export const publishedPageSize = 50;

// The newest format of a published copy this app can read (13 §3).
export const knownPublishedFormat = 1;

// Query keys are fixed (05 §4).
export const publishedListQuery = (archived: boolean, page: number) =>
  queryOptions({
    queryKey: ["published", { archived, page }],
    queryFn: () =>
      call(api.GET("/published", {
        params: { query: { ...(archived ? { archived: "all" as const } : {}), limit: publishedPageSize, offset: page * publishedPageSize } },
      })),
  });

export const publishedQuery = (id: string) =>
  queryOptions({
    queryKey: ["published", id],
    queryFn: () => call(api.GET("/liturgies/{id}/published", { params: { path: { id } } })),
  });

export const assignmentsQuery = queryOptions({
  queryKey: ["assignments"],
  queryFn: () => call(api.GET("/me/assignments")),
});

// isReadingMode tells whether an address is the reading mode of a published
// view (13 §7): "/published/{id}?read=1". AppLayout then drops its menus.
export function isReadingMode(pathname: string, search: string): boolean {
  return /^\/published\/[^/]+$/.test(pathname) && new URLSearchParams(search).get("read") === "1";
}

// The data of the WhatsApp texts (13 §8); it carries phone numbers, so it is
// asked for only on the messages page.
export const summaryQuery = (id: string) =>
  queryOptions({
    queryKey: ["published", id, "summary"],
    queryFn: async () => summaryOf(await call(api.GET("/liturgies/{id}/published/summary", { params: { path: { id } } }))),
    gcTime: 0, // not kept once the page is left
  });
