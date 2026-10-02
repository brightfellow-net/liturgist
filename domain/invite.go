// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "time"

// SetupLifetime is how long a setup link stays valid (03 §10).
const SetupLifetime = 24 * time.Hour

// TokenReason says why a link token can't be used (400 invalid_token).
type TokenReason string

// Token reasons (01 §10).
const (
	TokenUnknown TokenReason = "unknown"
)
