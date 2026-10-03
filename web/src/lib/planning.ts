// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { queryOptions } from "@tanstack/react-query";
import { api, call } from "./api";

// The values the planning API accepts (09 §2).
export const itemTypes = ["song", "reading", "prayer", "sermon", "free_text", "other"] as const;
export type ItemType = (typeof itemTypes)[number];
export const textItemTypes: readonly ItemType[] = ["prayer", "sermon", "free_text", "other"];
export const weekdays = [1, 2, 3, 4, 5, 6, 7] as const;

// Limits the forms check before sending; the server checks them again (09 §2).
export const planningLimits = { duties: 50, singingParts: 30, templateItems: 60, serviceTimes: 14 } as const;

// Query keys are fixed (05 §4).
export const dutiesQuery = queryOptions({ queryKey: ["duties"], queryFn: () => call(api.GET("/duties")) });
export const singingPartsQuery = queryOptions({ queryKey: ["singing-parts"], queryFn: () => call(api.GET("/singing-parts")) });
export const templatesQuery = queryOptions({ queryKey: ["templates"], queryFn: () => call(api.GET("/templates")) });
export const servicesQuery = queryOptions({ queryKey: ["services"], queryFn: () => call(api.GET("/services")) });

export const templateQuery = (id: string) =>
  queryOptions({
    queryKey: ["template", id],
    queryFn: () => call(api.GET("/templates/{id}", { params: { path: { id } } })),
  });

export const serviceQuery = (id: string) =>
  queryOptions({
    queryKey: ["service", id],
    queryFn: () => call(api.GET("/services/{id}", { params: { path: { id } } })),
  });

// The two name lists have the same operations on different paths.
export type NameList = {
  key: "duty" | "part";
  query: typeof dutiesQuery | typeof singingPartsQuery;
  max: number;
  create: (name: string) => Promise<unknown>;
  rename: (id: string, name: string) => Promise<unknown>;
  reorder: (ids: string[]) => Promise<{ items: NameEntry[] | null }>;
  remove: (id: string) => Promise<unknown>;
};
export type NameEntry = { id: string; name: string; position: number; actions: { edit: boolean; delete: boolean } };

export const dutyList: NameList = {
  key: "duty",
  query: dutiesQuery,
  max: planningLimits.duties,
  create: (name) => call(api.POST("/duties", { body: { name } })),
  rename: (id, name) => call(api.PATCH("/duties/{id}", { params: { path: { id } }, body: { name } })),
  reorder: (ids) => call(api.PUT("/duties/order", { body: { ids } })),
  remove: (id) => call(api.DELETE("/duties/{id}", { params: { path: { id } } })),
};

export const partList: NameList = {
  key: "part",
  query: singingPartsQuery,
  max: planningLimits.singingParts,
  create: (name) => call(api.POST("/singing-parts", { body: { name } })),
  rename: (id, name) => call(api.PATCH("/singing-parts/{id}", { params: { path: { id } }, body: { name } })),
  reorder: (ids) => call(api.PUT("/singing-parts/order", { body: { ids } })),
  remove: (id) => call(api.DELETE("/singing-parts/{id}", { params: { path: { id } } })),
};
