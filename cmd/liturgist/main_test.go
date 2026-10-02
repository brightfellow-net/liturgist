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

// The setup commands of 01 §6, with their exit codes.
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
}
