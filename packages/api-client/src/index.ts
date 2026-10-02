// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Types generated from openapi.json (make gen). The web app builds its
// openapi-fetch client on these; a future mobile app can reuse them.
export type { components, paths } from "./schema";

import type { components } from "./schema";

type Schemas = components["schemas"];
export type ChurchView = Schemas["ChurchView"];
export type MembershipView = Schemas["MembershipView"];

// Me is GET /me. Huma can't mark a nested object as nullable in OpenAPI, so
// the generated type misses that church and membership are null before
// setup and for non-members (04 §2); this is the only hand-corrected type.
export type Me = Omit<Schemas["MeOutputBody"], "church" | "membership"> & {
  church: ChurchView | null;
  membership: MembershipView | null;
};
