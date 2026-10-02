// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package unlimited_test

import (
	"context"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/entitlements/unlimited"
	"github.com/brightfellow-net/liturgist/app"
)

// TC-T-005
func TestUnlimited(t *testing.T) {
	e := unlimited.Entitlements{}
	for _, f := range []app.Feature{app.FeatureAutomaticNotifications, app.FeatureLicensedBibleText, app.FeatureObjectStorage, app.FeatureSSOLogin, app.FeatureAIImport} {
		if ok, err := e.Has(context.Background(), "c", f); !ok || err != nil {
			t.Errorf("%s: %v %v", f, ok, err)
		}
	}
	for _, l := range []app.LimitName{app.LimitMaxActiveLiturgies, app.LimitMaxUnpublishedLiturgies, app.LimitMaxTeamMembers} {
		if lim, err := e.Limit(context.Background(), "c", l); !lim.Unlimited || err != nil {
			t.Errorf("%s: %+v %v", l, lim, err)
		}
	}
}
