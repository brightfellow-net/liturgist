// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"errors"
	"strings"
	"testing"
)

// TC-A-003
func TestCheckPassword(t *testing.T) {
	id := PasswordIdentity{Name: "Budi Santoso", Email: "budi.s@example.org", Phone: "+6281234567890", ChurchName: "GKY Citragarden"}
	cases := map[string]PasswordProblem{
		"password123":                   PasswordCommon,
		"Password123":                   PasswordCommon,
		"Haleluya2026":                  PasswordCommon,
		"TuhanYesus1":                   PasswordCommon,
		"puji tuhan2026":                PasswordCommon,
		"short4567":                     PasswordTooShort,
		strings.Repeat("x", 129):        PasswordTooLong,
		"budi.s@example.org":            PasswordMatchesIdentity,
		"BUDI.S@example.org":            PasswordMatchesIdentity,
		"budi santoso":                  PasswordMatchesIdentity,
		"BudiSantoso":                   PasswordMatchesIdentity,
		"gky citragarden":               PasswordMatchesIdentity,
		"081234567890":                  PasswordMatchesIdentity,
		"6281234567890":                 PasswordMatchesIdentity,
		"+6281234567890":                PasswordMatchesIdentity,
		"budi.s1234":                    "",
		"kopi susu pagi hari":           "",
		strings.Repeat("😀", 10):         "",
		"correct horse battery staple ": "",
		strings.Repeat("y", 128):        "",
	}
	for pw, want := range cases {
		err := CheckPassword(pw, id)
		var weak *WeakPasswordError
		switch {
		case want == "" && err != nil:
			t.Errorf("%q: unexpected %v", pw, err)
		case want != "" && (!errors.As(err, &weak) || weak.Reason != want):
			t.Errorf("%q: got %v, want %s", pw, err, want)
		}
	}
	// Local part alone ("budi.s") is too short to matter; with a longer local part:
	long := PasswordIdentity{Email: "budisantoso@example.org"}
	if err := CheckPassword("BudiSantoso", long); err == nil {
		t.Error("email local part must be rejected")
	}
}

func TestNormalizeKeepsSpaces(t *testing.T) {
	if NormalizePassword(" pass word ") != " pass word " {
		t.Error("spaces must be kept")
	}
	if NormalizePassword("é") != "é" {
		t.Error("NFKC must compose é")
	}
}
