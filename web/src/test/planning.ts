// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { ServiceView, TemplateView } from "@liturgist/api-client";
import { meWith } from "./library";

export const planner = meWith(["templates.edit"]);
export const liturgist = meWith(["liturgy.edit"]);

type Entry = { id: string; name: string; position: number; actions: { edit: boolean; delete: boolean } };

// entries is a name list as the API returns it.
export function entries(names: string[], edit = true): { items: Entry[] } {
  return { items: names.map((name, i) => ({ id: "e" + (i + 1), name, position: i, actions: { edit, delete: edit } })) };
}

export function template(changes: Partial<TemplateView> = {}): TemplateView {
  return {
    id: "t1", name: "Ibadah Minggu", language: "id", version: 2,
    items: [
      { title: "Votum dan Salam", item_type: "free_text", default_text: "", default_duty_id: "e1" },
      { title: "Pujian", item_type: "song", default_text: "", default_duty_id: null },
    ],
    actions: { edit: true, delete: true }, ...changes,
  } as TemplateView;
}

export function service(changes: Partial<ServiceView> = {}): ServiceView {
  return {
    id: "s1", name: "Ibadah Umum", language: "id", default_template_id: "t1", default_template_name: "Ibadah Minggu", version: 3,
    times: [{ weekday: 3, time: "19:00" }, { weekday: 7, time: "07:00" }],
    actions: { edit: true, delete: true }, ...changes,
  } as ServiceView;
}
