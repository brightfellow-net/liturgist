// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-R-002: the book table.
func TestBooks(t *testing.T) {
	if len(domain.Books) != 66 {
		t.Fatalf("%d books", len(domain.Books))
	}
	codes := map[string]bool{}
	for i, b := range domain.Books {
		if b.Ordinal != i+1 || b.Code == "" || b.Name == "" || b.Abbr == "" {
			t.Errorf("book %d is incomplete: %+v", i+1, b)
		}
		if codes[b.Code] {
			t.Errorf("code %s twice", b.Code)
		}
		codes[b.Code] = true
	}

	// Every name, abbreviation and code parses to its own book. Two
	// spellings of different books would make one overwrite the other.
	for _, b := range domain.Books {
		for _, spelling := range []string{b.Name, b.Abbr, b.Code} {
			ref, err := domain.ParseReference(spelling + " 1")
			if b.Code == "OBA" || b.Code == "PHM" || b.Code == "2JN" || b.Code == "3JN" || b.Code == "JUD" {
				if err != nil || ref.Book != b.Code || ref.String() != b.Code+" 1:1" {
					t.Errorf("%q: %v, %v", spelling, ref, err)
				}
				continue
			}
			if err != nil || ref.Book != b.Code {
				t.Errorf("%q parses as %v (%v), want %s", spelling, ref, err, b.Code)
			}
		}
	}

	// The aliases table lists each spelling once, for exactly one book.
	seen := map[string]string{}
	for _, b := range domain.Books {
		for _, s := range []string{b.Name, b.Abbr, b.Code} {
			key := domain.BookKey(s)
			if other, ok := seen[key]; ok && other != b.Code {
				t.Errorf("spelling %q names both %s and %s", s, other, b.Code)
			}
			seen[key] = b.Code
		}
	}
	for alias, code := range domain.BookAliases() {
		if code2, ok := seen[alias]; ok && code2 != code {
			t.Errorf("alias %q names %s but also %s", alias, code, code2)
		}
	}

	for in, want := range map[string]string{"Hak 1": "JDG 1", "Hag 1": "HAG 1", "Yl 2": "JOL 2", "Am 5": "AMO 5", "Yoh 1": "JHN 1", "1Yoh 1": "1JN 1"} {
		got, err := domain.ParseReference(in)
		if err != nil || got.String() != want {
			t.Errorf("%q = %v (%v), want %s", in, got, err, want)
		}
	}
}
