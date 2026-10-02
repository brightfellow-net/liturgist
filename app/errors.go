// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
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

// Errors of the setup, church, role and member use cases (01 §10).
var (
	ErrNotSetUp        = errors.New("not set up")                              // 409 not_set_up
	ErrAlreadySetUp    = errors.New("already set up")                          // 409 already_set_up
	ErrForbidden       = errors.New("forbidden")                               // 403 forbidden
	ErrLockout         = errors.New("change would leave nobody to administer") // 409 lockout_prevented
	ErrRoleNameTaken   = errors.New("role name taken")                         // 409 role_name_taken
	ErrIdentifierTaken = errors.New("identifier taken")                        // 409 identifier_taken
	ErrTooManyChurches = errors.New("more than one church")                    // serve exit 7
)

// NotFoundError is ErrNotFound with the reason that is logged, never shown
// (04 §5): missing, not_member or not_visible.
type NotFoundError struct{ Reason string }

func (e *NotFoundError) Error() string { return "not found (" + e.Reason + ")" }

// Is makes errors.Is(err, ErrNotFound) true.
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// Not-found reasons.
const (
	ReasonMissing    = "missing"
	ReasonNotMember  = "not_member"
	ReasonNotVisible = "not_visible"
)

func notFound(reason string) error { return &NotFoundError{Reason: reason} }

// ScopeNotHeldError maps to 403 scope_not_held with the missing scopes.
type ScopeNotHeldError struct{ Scopes []domain.Scope }

func (e *ScopeNotHeldError) Error() string { return fmt.Sprintf("scopes not held: %v", e.Scopes) }

// InvalidTokenError maps to 400 invalid_token with a reason.
type InvalidTokenError struct{ Reason domain.TokenReason }

func (e *InvalidTokenError) Error() string { return "invalid token: " + string(e.Reason) }
