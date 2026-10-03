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

// Errors of the church, role, invite and reset use cases (01 §10).
var (
	ErrNotSetUp          = errors.New("not set up")                              // 409 not_set_up
	ErrAlreadySetUp      = errors.New("already set up")                          // 409 already_set_up
	ErrForbidden         = errors.New("forbidden")                               // 403 forbidden
	ErrLockout           = errors.New("change would leave nobody to administer") // 409 lockout_prevented
	ErrRoleNameTaken     = errors.New("role name taken")                         // 409 role_name_taken
	ErrAlreadyMember     = errors.New("already a member")                        // 409 already_member
	ErrInviteExists      = errors.New("open invite exists")                      // 409 invite_exists
	ErrIdentifierTaken   = errors.New("identifier taken")                        // 409 identifier_taken
	ErrInviteMismatch    = errors.New("invite is for another account")           // 403 invite_identifier_mismatch
	ErrResetNotAllowed   = errors.New("user belongs to another church")          // 409 reset_not_allowed
	ErrTooManyChurches   = errors.New("more than one church")                    // serve exit 7
	ErrNotMember         = errors.New("not a member of the church")              // CLI exit 6
	errNoReadyMadeOrigin = errors.New("unknown ready-made role")
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

// LimitReachedError maps to 403 limit_reached.
type LimitReachedError struct {
	Limit     LimitName
	Used, Max int
}

func (e *LimitReachedError) Error() string {
	return fmt.Sprintf("limit %s reached (%d of %d)", e.Limit, e.Used, e.Max)
}

// Errors of the song library (01 §10, 06).
var (
	ErrVersionConflict = errors.New("changed by someone else") // 409 version_conflict
	ErrSongInUse       = errors.New("song in use")             // 409 song_in_use
)

// SectionInUseError maps to 409 section_in_use with the section IDs.
type SectionInUseError struct{ IDs []domain.SectionID }

func (e *SectionInUseError) Error() string { return "sections in use" }

// Reasons of GroupConflictError.
const (
	ReasonAlreadyGrouped = "already_grouped"
	ReasonLanguageTaken  = "language_taken"
)

// GroupConflictError maps to 409 group_conflict with a reason.
type GroupConflictError struct{ Reason string }

func (e *GroupConflictError) Error() string { return "group conflict: " + e.Reason }

// ErrReadingInUse means unpublished liturgies still use the reading (07);
// it maps to 409 reading_in_use.
var ErrReadingInUse = errors.New("reading in use")

// ReadingExistsError maps to 409 reading_exists with the existing reading.
type ReadingExistsError struct{ ID domain.ReadingID }

func (e *ReadingExistsError) Error() string { return "reading exists: " + string(e.ID) }

// Reasons of ImportUnreadableError (08 §4.4).
const (
	ImportNotUTF8         = "not_utf8"
	ImportNotXML          = "not_xml"
	ImportNoSong          = "no_song"
	ImportTooManySections = "too_many_sections"
	ImportFileTooLarge    = "file_too_large"
	ImportTooComplex      = "too_complex"
	ImportAlreadyApplied  = "already_applied"
	ImportTargetChanged   = "target_changed"
)

// ImportUnreadableError maps to 422 import_unreadable with a reason.
type ImportUnreadableError struct{ Reason string }

func (e *ImportUnreadableError) Error() string { return "import unreadable: " + e.Reason }

// ImportConflictError maps to 409 import_conflict with a reason.
type ImportConflictError struct{ Reason string }

func (e *ImportConflictError) Error() string { return "import conflict: " + e.Reason }

// ErrImportTooLarge maps to 413 validation_failed (08 §5).
var ErrImportTooLarge = errors.New("import too large")

// Errors of the planning setup (01 §10, 09).
var (
	ErrDutyInUse        = errors.New("duty in use")         // 409 duty_in_use
	ErrSingingPartInUse = errors.New("singing part in use") // 409 singing_part_in_use
)

// NameTakenError maps to 409 name_taken; Reason is duty, singing_part,
// template or service.
type NameTakenError struct{ Reason string }

func (e *NameTakenError) Error() string { return "name taken: " + e.Reason }

// TemplateInUseError maps to 409 template_in_use with the services that have
// the template as their default.
type TemplateInUseError struct{ ServiceIDs []domain.ServiceID }

func (e *TemplateInUseError) Error() string { return "template in use" }
