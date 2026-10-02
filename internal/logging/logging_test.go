// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TC-F-010
func TestRedaction(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "json", slog.LevelInfo)
	log.Info("x",
		"email", "a@b.c",
		"Password", "secret1",
		"reset_token", "tok123",
		SetupLinkKey, "http://localhost:8080/setup#t=abc",
		"user_id", "01J0")
	out := buf.String()
	for _, leaked := range []string{"a@b.c", "secret1", "tok123"} {
		if strings.Contains(out, leaked) {
			t.Errorf("secret %q leaked: %s", leaked, out)
		}
	}
	for _, kept := range []string{"setup#t=abc", "01J0"} {
		if !strings.Contains(out, kept) {
			t.Errorf("value %q missing: %s", kept, out)
		}
	}
}

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, "text", slog.LevelWarn).Info("hidden")
	if buf.Len() != 0 {
		t.Errorf("info logged at warn level: %s", buf.String())
	}
}
