// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func bridged(level slog.Level) (*zap.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return zap.New(ZapCore(New(&buf, "json", level), "certmagic")), &buf
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

// TC-613: the certificate library's log lines come out of the app logger with
// their message, level, logger name and fields.
func TestZapBridge(t *testing.T) {
	zl, buf := bridged(slog.LevelDebug)
	zl.Named("tls.obtain").With(zap.String("identifier", "liturgi.example.org")).
		Warn("will retry", zap.Error(errors.New("boom")), zap.Duration("retrying_in", 2*time.Second), zap.Strings("names", []string{"a", "b"}))
	got := lines(t, buf)
	if len(got) != 1 {
		t.Fatalf("%v", got)
	}
	m := got[0]
	if m["msg"] != "will retry" || m["level"] != "WARN" || m["source"] != "certmagic" || m["logger"] != "tls.obtain" ||
		m["error"] != "boom" || m["domain"] != "liturgi.example.org" || m["retrying_in"] != float64(2*time.Second) {
		t.Errorf("%v", m)
	}
	if names, _ := m["names"].([]any); len(names) != 2 {
		t.Errorf("names: %v", m["names"])
	}
}

func TestZapBridgeLevels(t *testing.T) {
	zl, buf := bridged(slog.LevelWarn)
	zl.Debug("d")
	zl.Info("i")
	zl.Warn("w")
	zl.Error("e")
	got := lines(t, buf)
	if len(got) != 2 || got[0]["level"] != "WARN" || got[1]["level"] != "ERROR" {
		t.Errorf("only warnings and errors pass at warn level: %v", got)
	}
}

// The app's redaction applies to the library's fields too: the ACME account
// e-mail address is logged under "email".
func TestZapBridgeRedacts(t *testing.T) {
	zl, buf := bridged(slog.LevelInfo)
	zl.Info("using ACME account", zap.String("email", "ops@example.org"), zap.String("account_id", "https://ca/acct/1"))
	out := buf.String()
	if strings.Contains(out, "ops@example.org") || !strings.Contains(out, "https://ca/acct/1") {
		t.Errorf("%s", out)
	}
}
