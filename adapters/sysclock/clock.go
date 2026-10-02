// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package sysclock is the production app.Clock.
package sysclock

import "time"

// Clock reads the system clock.
type Clock struct{}

// Now returns UTC truncated to microseconds, the canonical precision (02 §4).
func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
