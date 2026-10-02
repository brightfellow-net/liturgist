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
	Status int                 `json:"status"`
	Title  string              `json:"title"`
	Code   string              `json:"code"`
	Detail string              `json:"detail,omitempty"`
	Reason string              `json:"reason,omitempty"`
	Scopes []string            `json:"scopes,omitempty"` // scope_not_held
	Errors []*huma.ErrorDetail `json:"errors,omitempty"`
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
		nf       *app.NotFoundError
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
	app.ErrIdentifierTaken: problem(http.StatusConflict, "identifier_taken", "This email or phone number belongs to another account."),
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
