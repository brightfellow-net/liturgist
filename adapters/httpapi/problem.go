// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package httpapi holds the HTTP adapter: middleware for sessions, client IP
// and CSRF, the Huma operations, and the mapping of errors to problems (01 §10).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// Problem is an RFC 9457 problem with a stable code (01 §10). Clients
// translate by code, never by detail.
type Problem struct {
	Status int      `json:"status"`
	Title  string   `json:"title"`
	Code   string   `json:"code"`
	Detail string   `json:"detail,omitempty"`
	Reason string   `json:"reason,omitempty"`
	Scopes []string `json:"scopes,omitempty"` // scope_not_held
	Limit  string   `json:"limit,omitempty"`  // limit_reached
	Used   *int     `json:"used,omitempty"`
	Max    *int     `json:"max,omitempty"`
	// SectionIDs lists the sections in use (section_in_use).
	SectionIDs []string            `json:"section_ids,omitempty"`
	Errors     []*huma.ErrorDetail `json:"errors,omitempty"`
}

// Error implements error.
func (p *Problem) Error() string { return p.Detail }

// GetStatus implements huma.StatusError.
func (p *Problem) GetStatus() int { return p.Status }

// ContentType implements huma.ContentTypeFilter.
func (p *Problem) ContentType(string) string { return "application/problem+json" }

// defaultCodes maps statuses Huma produces on its own (validation, routing) to codes.
var defaultCodes = map[int]string{
	http.StatusBadRequest:            "validation_failed",
	http.StatusNotFound:              "not_found",
	http.StatusRequestEntityTooLarge: "validation_failed",
	http.StatusUnprocessableEntity:   "validation_failed",
	http.StatusServiceUnavailable:    "unavailable",
}

func init() {
	huma.NewError = NewProblem
}

// NewProblem builds problems for Huma's own errors.
func NewProblem(status int, msg string, errs ...error) huma.StatusError {
	p := &Problem{Status: status, Title: http.StatusText(status), Detail: msg, Code: defaultCodes[status]}
	if p.Code == "" {
		p.Code = "internal"
	}
	if status >= http.StatusInternalServerError {
		p.Detail = "" // details only in the log
	}
	for _, e := range errs {
		var d *huma.ErrorDetail
		if errors.As(e, &d) {
			p.Errors = append(p.Errors, d)
		}
	}
	return p
}

func problem(status int, code, detail string) *Problem {
	return &Problem{Status: status, Title: http.StatusText(status), Code: code, Detail: detail}
}

// WriteProblem writes p as the response (for middleware outside Huma).
func WriteProblem(w http.ResponseWriter, p *Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// MapError turns use-case errors into problems (01 §10). Unknown errors are
// logged and become 500 internal without details. Every 404 has the same
// body; its reason is only logged (04 §5).
func MapError(ctx context.Context, err error, log *slog.Logger) error {
	var (
		weak     *domain.WeakPasswordError
		invalid  *domain.InvalidInputError
		many     *app.TooManyAttemptsError
		notHeld  *app.ScopeNotHeldError
		badToken *app.InvalidTokenError
		limit    *app.LimitReachedError
		nf       *app.NotFoundError
		inUse    *app.SectionInUseError
		group    *app.GroupConflictError
	)
	info := RequestInfoFrom(ctx)
	switch {
	case errors.Is(err, app.ErrInvalidCredentials):
		return problem(http.StatusUnauthorized, "invalid_credentials", "The email/phone or password is incorrect.")
	case errors.Is(err, app.ErrUnauthenticated):
		return problem(http.StatusUnauthorized, "unauthenticated", "Please log in.")
	case errors.As(err, &many):
		secs := int(math.Ceil(many.RetryAfter.Seconds()))
		return huma.ErrorWithHeaders(problem(http.StatusTooManyRequests, "too_many_attempts", "Too many attempts."),
			http.Header{"Retry-After": {strconv.Itoa(secs)}})
	case errors.As(err, &weak):
		p := problem(http.StatusUnprocessableEntity, "weak_password", "The password is too weak.")
		p.Reason = string(weak.Reason)
		return p
	case errors.As(err, &invalid):
		p := problem(http.StatusUnprocessableEntity, "validation_failed", invalid.Message)
		p.Errors = []*huma.ErrorDetail{{Location: "body." + invalid.Field, Message: invalid.Message}}
		return p
	case errors.Is(err, domain.ErrInvalidIdentifier):
		return problem(http.StatusUnprocessableEntity, "invalid_identifier", "Enter a valid email address or phone number.")
	case errors.As(err, &notHeld):
		p := problem(http.StatusForbidden, "scope_not_held", "You can only grant permissions you hold yourself.")
		for _, sc := range notHeld.Scopes {
			p.Scopes = append(p.Scopes, string(sc))
		}
		log.Info("scope_not_held", "user_id", string(info.UserID), "request_id", info.RequestID)
		return p
	case errors.As(err, &badToken):
		p := problem(http.StatusBadRequest, "invalid_token", "This link can no longer be used.")
		p.Reason = string(badToken.Reason)
		log.Info("invalid_token", "reason", p.Reason, "request_id", info.RequestID)
		return p
	case errors.As(err, &limit):
		p := problem(http.StatusForbidden, "limit_reached", "The plan's limit has been reached.")
		p.Limit, p.Used, p.Max = string(limit.Limit), &limit.Used, &limit.Max
		return p
	case errors.As(err, &inUse):
		p := problem(http.StatusConflict, "section_in_use", "A section is used in a liturgy that isn't published yet.")
		for _, id := range inUse.IDs {
			p.SectionIDs = append(p.SectionIDs, string(id))
		}
		return p
	case errors.As(err, &group):
		p := problem(http.StatusConflict, "group_conflict", "These songs can't be linked.")
		p.Reason = group.Reason
		return p
	case errors.Is(err, app.ErrForbidden):
		log.Info("forbidden", "user_id", string(info.UserID), "request_id", info.RequestID)
		return problem(http.StatusForbidden, "forbidden", "You don't have permission to do this.")
	case errors.Is(err, app.ErrNotFound):
		reason := app.ReasonMissing
		if errors.As(err, &nf) {
			reason = nf.Reason
		}
		log.Info("not_found", "not_found_reason", reason, "user_id", string(info.UserID), "request_id", info.RequestID)
		return problem(http.StatusNotFound, "not_found", "")
	case errors.Is(err, app.ErrUnavailable):
		return problem(http.StatusServiceUnavailable, "unavailable", "")
	}
	if p, ok := conflicts[errorKey(err)]; ok {
		log.Info(p.Code, "user_id", string(info.UserID), "request_id", info.RequestID)
		c := *p
		return &c
	}
	log.Error("unexpected error", "error", err, "request_id", info.RequestID)
	return problem(http.StatusInternalServerError, "internal", "")
}

// conflicts are the plain sentinel errors with their problems.
var conflicts = map[error]*Problem{
	app.ErrNotSetUp:        problem(http.StatusConflict, "not_set_up", "Liturgist is not set up yet."),
	app.ErrAlreadySetUp:    problem(http.StatusConflict, "already_set_up", "Liturgist is already set up."),
	app.ErrLockout:         problem(http.StatusConflict, "lockout_prevented", "Someone must keep the permissions to manage roles and members."),
	app.ErrRoleNameTaken:   problem(http.StatusConflict, "role_name_taken", "Another role already has this name."),
	app.ErrAlreadyMember:   problem(http.StatusConflict, "already_member", "This person is already a member of the church."),
	app.ErrInviteExists:    problem(http.StatusConflict, "invite_exists", "This person already has an open invite."),
	app.ErrIdentifierTaken: problem(http.StatusConflict, "identifier_taken", "This email or phone number belongs to another account."),
	app.ErrResetNotAllowed: problem(http.StatusConflict, "reset_not_allowed", "This person also belongs to another church."),
	app.ErrVersionConflict: problem(http.StatusConflict, "version_conflict", "This was changed by someone else. Reload to see their version."),
	app.ErrSongInUse:       problem(http.StatusConflict, "song_in_use", "This song is used in a liturgy that isn't published yet."),
	app.ErrInviteMismatch: problem(http.StatusForbidden, "invite_identifier_mismatch",
		"This invite is for another account. Log out and log in as that person."),
}

// errorKey returns the sentinel in conflicts that err wraps, if any.
func errorKey(err error) error {
	for k := range conflicts {
		if errors.Is(err, k) {
			return k
		}
	}
	return nil
}
