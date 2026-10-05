// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	in := "\xef\xbb\xbf# comment\n\nLITURGIST_BASE_URL = \"https://x.example\"\r\nA='b c'\nEMPTY=\nQ=\"half\n"
	got, err := parseEnvFile(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"LITURGIST_BASE_URL": "https://x.example", "A": "b c", "EMPTY": "", "Q": `"half`}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseEnvFileRejectsBadLines(t *testing.T) {
	for _, in := range []string{"nokey\n", "=value\n", "ok=1\nbroken\n"} {
		if _, err := parseEnvFile(strings.NewReader(in)); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestApplyEnvKeepsTheEnvironment(t *testing.T) {
	env := map[string]string{"A": "env"}
	set := map[string]string{}
	err := applyEnv(map[string]string{"A": "file", "B": "file"},
		func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		func(k, v string) error { set[k] = v; return nil })
	if err != nil || len(set) != 1 || set["B"] != "file" {
		t.Fatalf("set %v, err %v", set, err)
	}
}
