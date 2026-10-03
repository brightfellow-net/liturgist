// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Types generated from openapi.json (make gen). The web app builds its
// openapi-fetch client on these; a future mobile app can reuse them.
export type { components, paths } from "./schema";

import type { components } from "./schema";

type Schemas = components["schemas"];
export type ChurchView = Schemas["ChurchView"];
export type MembershipView = Schemas["MembershipView"];
export type MemberView = Schemas["MemberView"];
export type InviteView = Schemas["InviteView"];
export type RoleView = Schemas["RoleView"];
export type ScopeInfo = Schemas["Item1"]; // GET /scopes item
export type SongView = Schemas["SongView"];
export type SongSummaryView = Schemas["SongSummaryView"];
export type SectionView = Schemas["SectionView"];
export type SongSection = Schemas["SongSection"];
export type SongRefView = Schemas["SongRefView"];
export type ReadingView = Schemas["ReadingView"];
export type ReadingSummaryView = Schemas["ReadingSummaryView"];
export type TranslationRefView = Schemas["TranslationRefView"];
export type ProviderTextView = Schemas["ProviderTextView"];
export type TemplateView = Schemas["TemplateView"];
export type TemplateSummaryView = Schemas["TemplateSummaryView"];
export type ServiceView = Schemas["ServiceView"];

// LookupView is GET /readings/lookup. Huma can't mark a nested object as
// nullable in OpenAPI either (see Me): reading and provider are null when
// there is none.
export type LookupView = Omit<Schemas["LookupView"], "reading" | "provider"> & {
  reading: ReadingView | null;
  provider: ProviderTextView | null;
};

// Me is GET /me. Huma can't mark a nested object as nullable in OpenAPI, so
// the generated type misses that church and membership are null before
// setup and for non-members (04 §2); this is the only hand-corrected type.
export type Me = Omit<Schemas["MeOutputBody"], "church" | "membership"> & {
  church: ChurchView | null;
  membership: MembershipView | null;
};

// Import types. Huma can't mark a reference as nullable, so the server leaves
// these fields out when they have no value (08 §5); the generated types
// already show them as optional.
export type ImportBatchView = Schemas["ImportBatchView"];
export type ImportCandidateView = Schemas["ImportCandidateView"];
export type ImportSummaryView = Schemas["ImportSummaryView"];
export type ImportRejectedView = Schemas["ImportRejectedView"];
export type SongDraft = Schemas["SongDraftBody"];
export type DraftSection = Schemas["DraftSectionBody"];
export type MergePreviewView = Schemas["MergePreviewView"];
export type ApplyResultView = Schemas["ApplyResultView"];
