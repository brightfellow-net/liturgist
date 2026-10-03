// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import type { LookupView } from "@liturgist/api-client";
import { api, call } from "./api";

// ReadingFilters are the readings list's search terms, kept in the address bar.
export type ReadingFilters = { q?: string; translation?: string; offset?: number; limit?: number };

export const readingPageSize = 50;

// Query keys are fixed (05 §4): ["readings", filters], ["reading", id],
// ["reading-parse", input] and ["reading-lookup", reference, translation].
export const readingsQuery = (filters: ReadingFilters) =>
  queryOptions({
    queryKey: ["readings", filters],
    queryFn: () => {
      const query = Object.fromEntries(Object.entries(filters).filter(([, v]) => v !== undefined && v !== ""));
      return call(api.GET("/readings", { params: { query } }));
    },
  });

export const readingQuery = (id: string) =>
  queryOptions({
    queryKey: ["reading", id],
    queryFn: () => call(api.GET("/readings/{id}", { params: { path: { id } } })),
  });

// parseQuery asks the server how a typed reference is understood: the only
// parser there is (07 §2.3).
export const parseQuery = (input: string) =>
  queryOptions({
    queryKey: ["reading-parse", input],
    queryFn: () => call(api.GET("/readings/parse", { params: { query: { input } } })),
    retry: false,
    staleTime: 60_000,
  });

export const lookupQuery = (reference: string, translation: string) =>
  queryOptions({
    queryKey: ["reading-lookup", reference, translation],
    queryFn: async () => (await call(api.GET("/readings/lookup", { params: { query: { reference, translation } } }))) as unknown as LookupView,
    retry: false,
  });
