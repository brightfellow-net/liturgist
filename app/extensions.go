// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/brightfellow-net/liturgist/domain"
)

// Extension points (04 §7) used in step 1. They are provisional [P-27]: the
// signatures may change until the step that first uses them.

// URLBuilder builds browser paths and shareable links (04 §4). route starts
// with "/" and has no church prefix.
type URLBuilder interface {
	AppPath(ctx context.Context, route string) string
	AppURL(ctx context.Context, route string) string
}

// Feature is an entitlement flag; the constants live only here (04 §8).
type Feature string

// Features.
const (
	FeatureAutomaticNotifications Feature = "automatic_notifications"
	FeatureLicensedBibleText      Feature = "licensed_bible_text"
	FeatureObjectStorage          Feature = "object_storage"
	FeatureSSOLogin               Feature = "sso_login"
	FeatureAIImport               Feature = "ai_import"
)

// LimitName names a usage limit.
type LimitName string

// Limits.
const (
	LimitMaxActiveLiturgies      LimitName = "max_active_liturgies"
	LimitMaxUnpublishedLiturgies LimitName = "max_unpublished_liturgies"
	LimitMaxTeamMembers          LimitName = "max_team_members"
)

// Limit is a usage limit; Max is meaningful only when !Unlimited.
type Limit struct {
	Unlimited bool
	Max       int
}

// Entitlements answers what a church may use. An error is treated as
// unavailable, never as allowed.
type Entitlements interface {
	Has(ctx context.Context, church domain.ChurchID, f Feature) (bool, error)
	Limit(ctx context.Context, church domain.ChurchID, l LimitName) (Limit, error)
}
