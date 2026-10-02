// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import type { Me } from "@liturgist/api-client";
import { api, call } from "./api";

// Query keys are fixed (05 §4).
export const meQuery = queryOptions({
  queryKey: ["me"],
  queryFn: (): Promise<Me> => call(api.GET("/me")),
});

export const setupStatusQuery = queryOptions({
  queryKey: ["setup-status"],
  queryFn: () => call(api.GET("/setup/status")),
});

export const translationsQuery = queryOptions({
  queryKey: ["translations"],
  queryFn: () => call(api.GET("/translations")),
  staleTime: Infinity,
});
