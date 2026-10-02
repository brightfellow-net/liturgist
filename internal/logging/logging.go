// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package logging builds the slog logger with redaction of secrets (01 §9).
package logging

import (
	"io"
	"log/slog"
	"strings"
)

var secretKeys = map[string]bool{
	"token": true, "password": true, "email": true, "phone": true,
	"identifier": true, "cookie": true, "authorization": true,
}

// SetupLinkKey is the one attribute deliberately not redacted (03 §10, accepted risk).
const SetupLinkKey = "setup_link"

func redact(_ []string, a slog.Attr) slog.Attr {
	k := strings.ToLower(a.Key)
	if secretKeys[k] || strings.HasSuffix(k, "_token") {
		return slog.String(a.Key, "[redacted]")
	}
	return a
}

// New returns a logger writing text or JSON to w, with redaction.
func New(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: redact}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
