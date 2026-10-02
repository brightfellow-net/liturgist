// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

func cli(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// IT-A-012 and the step-1 commands of 01 §6, with their exit codes.
func TestOperatorCommands(t *testing.T) {
	t.Setenv("LITURGIST_DATA_DIR", t.TempDir())
	t.Setenv("LITURGIST_BASE_URL", "https://liturgi.example.org")
	if code, _, errOut := cli("", "migrate"); code != exitOK {
		t.Fatalf("migrate: %d %s", code, errOut)
	}

	code, out, errOut := cli("", "setup-link")
	if code != exitOK || !strings.Contains(out, "https://liturgi.example.org/setup#t=") || !strings.Contains(out, "=====") {
		t.Errorf("setup-link before setup: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("", "user", "list"); code != exitOK {
		t.Errorf("user list before setup: %d", code)
	}

	setup := []string{"setup", "--church-name", "GKY Citragarden", "--admin-name", "Admin",
		"--admin-identifier", "admin@example.org", "--password-stdin"}
	if code, _, errOut := cli("short\n", setup...); code != exitError || !strings.Contains(errOut, "weak password") {
		t.Errorf("weak password: %d %q", code, errOut)
	}
	if code, out, errOut := cli("kopi susu pagi hari\n", setup...); code != exitOK || !strings.Contains(out, "set up") {
		t.Fatalf("setup: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("kopi susu pagi hari\n", setup...); code != exitAlreadySetUp {
		t.Errorf("second setup: %d", code)
	}
	if code, _, _ := cli("", "setup-link"); code != exitAlreadySetUp {
		t.Errorf("setup-link after setup: %d", code)
	}
	if code, _, _ := cli("", "setup", "--church-name", "X"); code != exitConfig {
		t.Errorf("setup without required flags: %d", code)
	}
	if code, _, _ := cli("", "setup", "--church-name", "X", "--admin-name", "A", "--admin-identifier", "a@example.org"); code != exitError {
		t.Errorf("setup without a terminal or --password-stdin: %d", code)
	}

	code, out, _ = cli("", "user", "list")
	if code != exitOK || !strings.Contains(out, "admin@example.org") || !strings.Contains(out, "Church admin") {
		t.Errorf("user list: %d %q", code, out)
	}
	code, out, errOut = cli("", "user", "reset-password", "ADMIN@example.org")
	if code != exitOK || !strings.Contains(out, "https://liturgi.example.org/reset#t=") || !strings.Contains(out, "WIB") ||
		strings.Contains(errOut, "LITURGIST_BASE_URL is not set") {
		t.Errorf("reset-password: %d %q %q", code, out, errOut)
	}
	if code, _, _ := cli("", "user", "reset-password", "nobody@example.org"); code != exitNoSuchUser {
		t.Errorf("reset-password for an unknown user: %d", code)
	}
	if code, _, _ := cli("", "member", "grant-admin", "admin@example.org"); code != exitOK {
		t.Errorf("grant-admin: %d", code)
	}
	if code, _, _ := cli("", "member", "grant-admin", "nobody@example.org"); code != exitNoSuchUser {
		t.Errorf("grant-admin for an unknown user: %d", code)
	}
	for _, args := range [][]string{{"--all"}, {"--identifier", "admin@example.org"}, {"--ip", "203.0.113.5"}, {"--ip", "2001:db8::1"}} {
		if code, _, errOut := cli("", append([]string{"auth", "clear-throttle"}, args...)...); code != exitOK {
			t.Errorf("clear-throttle %v: %d %s", args, code, errOut)
		}
	}
	for _, args := range [][]string{{}, {"--all", "--ip", "203.0.113.5"}} {
		if code, _, _ := cli("", append([]string{"auth", "clear-throttle"}, args...)...); code != exitConfig {
			t.Errorf("clear-throttle %v: %d", args, code)
		}
	}
	if code, _, errOut := cli("", "auth", "clear-throttle", "--ip", "nonsense"); code != exitError || !strings.Contains(errOut, "not an IP") {
		t.Errorf("bad IP: %d %q", code, errOut)
	}

	t.Setenv("LITURGIST_BASE_URL", "")
	if _, _, errOut := cli("", "user", "reset-password", "admin@example.org"); !strings.Contains(errOut, "LITURGIST_BASE_URL is not set") {
		t.Errorf("missing base URL warning: %q", errOut)
	}
}
