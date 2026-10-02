// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import "context"

// Extension points (04 §7) used in step 1. They are provisional [P-27]: the
// signatures may change until the step that first uses them.

// URLBuilder builds browser paths and shareable links (04 §4). route starts
// with "/" and has no church prefix.
type URLBuilder interface {
	AppPath(ctx context.Context, route string) string
	AppURL(ctx context.Context, route string) string
}
