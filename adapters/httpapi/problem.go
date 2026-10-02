// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package httpapi holds the HTTP adapter: middleware for sessions, client IP
// and CSRF, the Huma operations, and the mapping of errors to problems (01 §10).
package httpapi

import (
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
// logged and become 500 internal without details.
func MapError(err error, log *slog.Logger) error {
	var (
		weak    *domain.WeakPasswordError
		invalid *domain.InvalidInputError
		many    *app.TooManyAttemptsError
	)
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
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "not_found", "")
	case errors.Is(err, app.ErrUnavailable):
		return problem(http.StatusServiceUnavailable, "unavailable", "")
	}
	log.Error("unexpected error", "error", err)
	return problem(http.StatusInternalServerError, "internal", "")
}
