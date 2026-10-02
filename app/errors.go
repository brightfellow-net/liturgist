// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"fmt"
	"time"
)

// Errors returned by persistence adapters (02 §8).
var (
	ErrNotFound    = errors.New("not found")
	ErrReferenced  = errors.New("referenced row missing or still in use")
	ErrInvalid     = errors.New("database rejected the row (check constraint)")
	ErrUnavailable = errors.New("database unavailable")

	ErrInvalidCredentials = errors.New("invalid credentials") // 401 invalid_credentials
	ErrUnauthenticated    = errors.New("not logged in")       // 401 unauthenticated
)

// TooManyAttemptsError maps to 429 too_many_attempts with Retry-After.
type TooManyAttemptsError struct{ RetryAfter time.Duration }

func (e *TooManyAttemptsError) Error() string { return "too many attempts" }

// UniqueError reports a unique-constraint violation; Constraint is the
// constraint name, identical in both dialects (02 §8).
type UniqueError struct{ Constraint string }

func (e *UniqueError) Error() string {
	return fmt.Sprintf("unique constraint %q violated", e.Constraint)
}
