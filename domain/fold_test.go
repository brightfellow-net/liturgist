// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"reflect"
	"testing"
)

// TC-S-003.
func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Besar Setia-Mu!":      "besar setia mu",
		"Cafè  Ünï":            "cafe uni",
		"Café":                "cafe", // e + combining acute
		"Café":                 "cafe", // precomposed é
		"主，我愿意":                "主 我愿意",
		"ＡＢＣ １２":               "abc 12", // full-width
		"  --  ":               "",
		"":                     "",
		"Allah's":              "allah s",
		"İstanbul":             "istanbul",
		"Tuhan, Engkau   Baik": "tuhan engkau baik",
		"1.  Bait":             "1 bait",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	if got := FoldZh("主，我愿意 ABC"); got != "主我愿意abc" {
		t.Errorf("FoldZh = %q", got)
	}
	if FoldFor("zh-Hans", "a b") != "ab" || FoldFor("id", "a b") != "a b" {
		t.Error("FoldFor")
	}
}

// TC-S-004.
func TestHymnalKeyAndQuery(t *testing.T) {
	for _, c := range []struct{ src, num, want string }{
		{"KJ", "12", "kj:12"},
		{" pkj ", "12A", "pkj:12a"},
		{"NKB 2", "5", "nkb2:5"},
		{"KJ", "", ""},
	} {
		if got := HymnalKey(c.src, c.num); got != c.want {
			t.Errorf("HymnalKey(%q, %q) = %q, want %q", c.src, c.num, got, c.want)
		}
	}
	for q, want := range map[string]string{"kj 12": "kj:12", "PKJ12a": "pkj:12a", "KJ. 12": "kj:12"} {
		if got, ok := ParseHymnalQuery(q); !ok || got != want {
			t.Errorf("ParseHymnalQuery(%q) = %q %v, want %q", q, got, ok, want)
		}
	}
	for _, q := range []string{"besar", "kj", "12", "kj 12 13", "主 12", ""} {
		if got, ok := ParseHymnalQuery(q); ok {
			t.Errorf("ParseHymnalQuery(%q) = %q, want no hymnal query", q, got)
		}
	}
	if HymnalSourceKey(" PKJ ") != "pkj" {
		t.Error("source key")
	}
}

func TestSearchTerms(t *testing.T) {
	if got := SearchTerms("  Besar, Setia-Mu "); !reflect.DeepEqual(got, []string{"besar", "setia", "mu"}) {
		t.Errorf("terms %v", got)
	}
	if len(SearchTerms("")) != 0 {
		t.Error("empty query has no terms")
	}
}
