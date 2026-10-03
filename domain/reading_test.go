// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-R-005: the rules of a reading.
func TestValidateReading(t *testing.T) {
	ok := func() domain.Reading {
		return domain.Reading{ReferenceDisplay: " Yoh  3:16 ", Text: "Karena begitu besar kasih Allah \r\n", Attribution: " TB © LAI "}
	}
	r := ok()
	if err := domain.ValidateReading(&r); err != nil {
		t.Fatal(err)
	}
	if r.ReferenceDisplay != "Yoh 3:16" || r.Text != "Karena begitu besar kasih Allah" || r.Attribution != "TB © LAI" {
		t.Errorf("not normalised: %+v", r)
	}

	field := func(r domain.Reading) string {
		var in *domain.InvalidInputError
		if err := domain.ValidateReading(&r); errors.As(err, &in) {
			return in.Field
		}
		return ""
	}
	bad := ok()
	bad.Text = strings.Repeat("a", 20001)
	if field(bad) != "text" {
		t.Error("20001 characters of text accepted")
	}
	bad.Text = strings.Repeat("a", 20000)
	if field(bad) != "" {
		t.Error("20000 characters of text rejected")
	}
	bad.Text = " \n\n "
	if field(bad) != "text" {
		t.Error("empty text accepted")
	}
	bad = ok()
	bad.Attribution = strings.Repeat("a", 301)
	if field(bad) != "attribution" {
		t.Error("301 characters of attribution accepted")
	}
	bad.Attribution = strings.Repeat("a", 300)
	if field(bad) != "" {
		t.Error("300 characters of attribution rejected")
	}
	bad = ok()
	bad.ReferenceDisplay = "  "
	if field(bad) != "reference_display" {
		t.Error("empty display accepted")
	}
}

func TestReadingSearchFold(t *testing.T) {
	r := domain.Reading{Reference: "JHN 3:16", ReferenceDisplay: "Yoh 3:16", Text: "Karena begitu besar", Translation: domain.Translation{Language: "id"}}
	if got, want := r.SearchFold(), "yoh 3 16 yohanes 3 16 karena begitu besar"; got != want {
		t.Errorf("SearchFold = %q, want %q", got, want)
	}
	zh := domain.Reading{Reference: "JHN 3:16", ReferenceDisplay: "约 3:16", Text: "神爱世人", Translation: domain.Translation{Language: "zh-Hans"}}
	if got, want := zh.SearchFold(), "约316yohanes316神爱世人"; got != want {
		t.Errorf("zh SearchFold = %q, want %q", got, want)
	}
	if got := domain.Snippet("a  b\nc", 10); got != "a b c" {
		t.Errorf("Snippet = %q", got)
	}
	if got := domain.Snippet(strings.Repeat("x", 130), 120); len([]rune(got)) != 121 {
		t.Errorf("long Snippet has %d characters", len([]rune(got)))
	}
}
