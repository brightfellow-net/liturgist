// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-R-001: typed references and their standard form.
var referenceCases = []struct{ in, want string }{
	{"Yoh 3:16-21", "JHN 3:16-21"},
	{"Kej. 1:1–2:3", "GEN 1:1-2:3"},
	{"Mzm 23", "PSA 23"},
	{"mazmur 23", "PSA 23"},
	{"Mzm 23–25", "PSA 23-25"},
	{"Mat. 5:3, 5-7", "MAT 5:3,5-7"},
	{"1 Kor 13", "1CO 13"},
	{"I Kor 13", "1CO 13"},
	{"1Kor13", "1CO 13"},
	{"1Kor. 13", "1CO 13"},
	{"II Korintus 5:17", "2CO 5:17"},
	{"iii yoh 4", "3JN 1:4"},
	{"Yud 3", "JUD 1:3"},
	{"Yud 3-5", "JUD 1:3-5"},
	{"Yud 1:3", "JUD 1:3"},
	{"JHN 3:16", "JHN 3:16"},
	{"  yoh\t3:16 — 21 ", "JHN 3:16-21"},
	{"YOH 3:16", "JHN 3:16"},
	{"Yoh 3:16", "JHN 3:16"},
	{"Hakim-hakim 6", "JDG 6"},
	{"Kisah Para Rasul 2:1-4", "ACT 2:1-4"},
	{"Kis 2:1 - 4", "ACT 2:1-4"},
	{"Yoh 3:16-16", "JHN 3:16"},
	{"Yoh 3:16-3:18", "JHN 3:16-18"},
	{"Mzm 23-23", "PSA 23"},
	{"Why 22:20", "REV 22:20"},
	{"Im 3", "LEV 3"},
	{"Ibr 11:1", "HEB 11:1"},
	{"Yoh 3:16,17", "JHN 3:16,17"},
	{"Yoh 151:1", ""},
}

func TestParseReference(t *testing.T) {
	for _, c := range referenceCases {
		got, err := domain.ParseReference(c.in)
		if c.want == "" {
			if err == nil {
				t.Errorf("%q parsed as %s, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("%q = %s, want %s", c.in, got, c.want)
		}
	}
}

// TC-R-004: the standard form parses back to itself.
func TestReferenceRoundTrip(t *testing.T) {
	for _, c := range referenceCases {
		if c.want == "" {
			continue
		}
		again, err := domain.ParseReference(c.want)
		if err != nil || again.String() != c.want {
			t.Errorf("%q does not round trip: %v, %v", c.want, again, err)
		}
	}
}

// TC-R-003: errors and their reasons.
func TestParseReferenceErrors(t *testing.T) {
	cases := []struct {
		in     string
		reason domain.ReferenceReason
	}{
		{"", domain.RefEmpty},
		{"   ", domain.RefEmpty},
		{"Foo 1", domain.RefUnknownBook},
		{"3:16", domain.RefUnknownBook},
		{"Yo 3:16", domain.RefUnknownBook},
		{"Yoh", domain.RefMissingChapter},
		{"Yoh.", domain.RefMissingChapter},
		{"Yoh 3:0", domain.RefBadNumber},
		{"Yoh 0", domain.RefBadNumber},
		{"Yoh 151", domain.RefBadNumber},
		{"Yoh 3:177", domain.RefBadNumber},
		{"Yoh 3 16", domain.RefBadNumber},
		{"Yoh 3.16", domain.RefBadNumber},
		{"Yoh 3:", domain.RefBadNumber},
		{"Yoh 3:21-16", domain.RefBadRange},
		{"Yoh 3:16,16", domain.RefBadRange},
		{"Yoh 3:16-18,17", domain.RefBadRange},
		{"Mzm 25-23", domain.RefBadRange},
		{"Kej 2:1-1:3", domain.RefBadRange},
		{"Yoh 3:16a", domain.RefUnsupported},
		{"Yoh 3;4", domain.RefUnsupported},
		{"Yoh 3,4", domain.RefUnsupported},
		{"Yoh 3:16-4:2,5", domain.RefUnsupported},
		{"Yoh 3:1,4:2", domain.RefUnsupported},
		{"Yud 2:3", domain.RefBadNumber},
		{strings.Repeat("a", 101), domain.RefTooLong},
		{"Yoh 3:16" + strings.Repeat(" ", 93), domain.RefTooLong}, // 101 characters, checked before trimming
	}
	for _, c := range cases {
		_, err := domain.ParseReference(c.in)
		var re *domain.ReferenceError
		if !errors.As(err, &re) || re.Reason != c.reason {
			t.Errorf("%q: got %v, want reason %s", c.in, err, c.reason)
		}
	}
	// Exactly 100 characters are accepted.
	if _, err := domain.ParseReference("Yoh 3:16" + strings.Repeat(" ", 92)); err != nil {
		t.Errorf("100 characters: %v", err)
	}
}

func TestReferenceFields(t *testing.T) {
	r, err := domain.ParseReference("Mat. 5:3, 5-7")
	if err != nil {
		t.Fatal(err)
	}
	if r.Canonical("id") != "Matius 5:3,5-7" || r.BookOrdinal() != 40 || r.Chapter != 5 || r.Verse != 3 {
		t.Errorf("got %+v, canonical %q", r, r.Canonical("id"))
	}
	r, _ = domain.ParseReference("Mzm 23")
	if r.Canonical("id") != "Mazmur 23" || r.Chapter != 23 || r.Verse != 0 {
		t.Errorf("got %+v", r)
	}
	if got := domain.CollapseSpaces("  Yoh \t 3:16  "); got != "Yoh 3:16" {
		t.Errorf("CollapseSpaces = %q", got)
	}
}
