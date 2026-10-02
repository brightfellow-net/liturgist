// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"errors"
	"strings"
	"testing"
)

// TC-A-001, TC-A-002
func TestParseIdentifier(t *testing.T) {
	valid := map[string]Identifier{
		"0812-3456-7890":       {Phone, "+6281234567890"},
		"+62 812 3456 7890":    {Phone, "+6281234567890"},
		"62812 3456 7890":      {Phone, "+6281234567890"},
		"(0812) 3456.7890":     {Phone, "+6281234567890"},
		" Budi@Example.ORG ":   {Email, "budi@example.org"},
		"a.b+c@sub.example.id": {Email, "a.b+c@sub.example.id"},
	}
	for in, want := range valid {
		got, err := ParseIdentifier(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v; want %+v", in, got, err, want)
		}
	}
	invalid := []string{
		"", "0812", "0812abc4567", "+62 812 ABCD 7890", "0812-3456-789x", "+", "+1 555", "budi@@example.org", "budi@example", "budi@.example.org",
		"budi@example.org.", "Budi <budi@example.org>", strings.Repeat("a", 250) + "@example.org", "budi",
	}
	for _, in := range invalid {
		if got, err := ParseIdentifier(in); !errors.Is(err, ErrInvalidIdentifier) {
			t.Errorf("%q: expected ErrInvalidIdentifier, got %+v, %v", in, got, err)
		}
	}
}
