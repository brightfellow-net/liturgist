// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package unlimited is the community Entitlements: everything allowed, no limits (04 §8).
package unlimited

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// Entitlements allows every feature with no limits.
type Entitlements struct{}

// Has returns true.
func (Entitlements) Has(context.Context, domain.ChurchID, app.Feature) (bool, error) {
	return true, nil
}

// Limit returns unlimited.
func (Entitlements) Limit(context.Context, domain.ChurchID, app.LimitName) (app.Limit, error) {
	return app.Limit{Unlimited: true}, nil
}
