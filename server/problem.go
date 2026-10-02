// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Problem is an RFC 9457 problem with a stable `code` (01 §10). Clients
// translate by code, never by detail.
type Problem struct {
	Status int                 `json:"status"`
	Title  string              `json:"title"`
	Code   string              `json:"code"`
	Detail string              `json:"detail,omitempty"`
	Errors []*huma.ErrorDetail `json:"errors,omitempty"`
}

func (p *Problem) Error() string             { return p.Detail }
func (p *Problem) GetStatus() int            { return p.Status }
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
	huma.NewError = newProblem
}

func newProblem(status int, msg string, errs ...error) huma.StatusError {
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

func writeProblem(w http.ResponseWriter, p *Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
