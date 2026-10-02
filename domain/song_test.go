// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"errors"
	"strings"
	"testing"
)

func validSong() Song {
	return Song{Language: "id", Title: "Besar Setia-Mu", LicenceStatus: LicenceUnknown}
}

func field(err error) string {
	var in *InvalidInputError
	if errors.As(err, &in) {
		return in.Field
	}
	return ""
}

// TC-S-001.
func TestValidateSong(t *testing.T) {
	ok := func(mut func(*Song)) {
		t.Helper()
		s := validSong()
		mut(&s)
		if err := ValidateSong(&s); err != nil {
			t.Errorf("want valid, got %v", err)
		}
	}
	bad := func(want string, mut func(*Song)) {
		t.Helper()
		s := validSong()
		mut(&s)
		if got := field(ValidateSong(&s)); got != want {
			t.Errorf("want invalid %q, got %q", want, got)
		}
	}
	ok(func(*Song) {})
	ok(func(s *Song) { s.Title = strings.Repeat("a", 200) })
	bad("title", func(s *Song) { s.Title = strings.Repeat("a", 201) })
	bad("title", func(s *Song) { s.Title = "   " })
	bad("language", func(s *Song) { s.Language = "fr" })
	for _, k := range []string{"G", "Bb", "F#m", "Am", ""} {
		ok(func(s *Song) { s.DefaultKey = k })
	}
	for _, k := range []string{"H", "f#m", "G#b", "Gmm"} {
		bad("default_key", func(s *Song) { s.DefaultKey = k })
	}
	ok(func(s *Song) { s.HymnalSource, s.HymnalNumber = "KJ", "12a" })
	bad("hymnal_number", func(s *Song) { s.HymnalNumber = "12" })
	bad("hymnal_number", func(s *Song) { s.HymnalSource = "KJ" })
	bad("hymnal_number", func(s *Song) { s.HymnalSource, s.HymnalNumber = "KJ", "--" })
	bad("hymnal_number", func(s *Song) { s.HymnalSource, s.HymnalNumber = "KJ", "12345678901" })
	ok(func(s *Song) { s.CCLISongNumber = "123456789012" })
	bad("ccli_song_number", func(s *Song) { s.CCLISongNumber = "1234567890123" })
	bad("ccli_song_number", func(s *Song) { s.CCLISongNumber = "12a" })
	bad("licence_status", func(s *Song) { s.LicenceStatus = "free" })
	bad("alt_titles", func(s *Song) { s.AltTitles = make([]string, 11) })
	bad("alt_titles.0", func(s *Song) { s.AltTitles = []string{""} })
	bad("alt_titles.1", func(s *Song) { s.AltTitles = []string{"Other", "OTHER"} })
	bad("alt_titles.0", func(s *Song) { s.AltTitles = []string{"besar setia mu"} }) // same as the title after folding
	bad("copyright_line", func(s *Song) { s.CopyrightLine = strings.Repeat("a", 301) })
	bad("licence_notes", func(s *Song) { s.LicenceNotes = strings.Repeat("a", 2001) })

	s := validSong()
	s.Title, s.LicenceStatus = "  Trimmed  ", ""
	if err := ValidateSong(&s); err != nil || s.Title != "Trimmed" || s.LicenceStatus != LicenceUnknown {
		t.Errorf("trim and default: %+v %v", s, err)
	}
	if got := (Song{Title: "Besar Setia-Mu", HymnalSource: "KJ", HymnalNumber: "12"}); got.TitleKey() != "besar setia mu" || got.HymnalKey() != "kj:12" {
		t.Errorf("keys: %q %q", got.TitleKey(), got.HymnalKey())
	}
}

// TC-S-002.
func TestValidateSections(t *testing.T) {
	verse := func(n int, text string) Section { return Section{Kind: SectionVerse, Number: n, Text: text} }
	cases := []struct {
		name string
		secs []Section
		want string // invalid field, "" = valid
	}{
		{"valid", []Section{verse(1, "a"), {Kind: SectionChorus, Text: "b"}, verse(3, "c")}, ""},
		{"no sections", nil, ""},
		{"verse without number", []Section{{Kind: SectionVerse, Text: "a"}}, "sections.0.number"},
		{"verse number 100", []Section{verse(100, "a")}, "sections.0.number"},
		{"chorus with number", []Section{{Kind: SectionChorus, Number: 1, Text: "a"}}, "sections.0.number"},
		{"two verse 1", []Section{verse(1, "a"), verse(1, "b")}, "sections.1.number"},
		{"unknown kind", []Section{{Kind: "refrain", Text: "a"}}, "sections.0.kind"},
		{"empty text", []Section{verse(1, " \n ")}, "sections.0.text"},
		{"5000 characters", []Section{verse(1, strings.Repeat("a", 5000))}, ""},
		{"5001 characters", []Section{verse(1, strings.Repeat("a", 5001))}, "sections.0.text"},
		{"long label", []Section{{Kind: SectionOther, Label: strings.Repeat("a", 61), Text: "a"}}, "sections.0.label"},
		{"61 sections", func() []Section {
			var s []Section
			for i := range 61 {
				s = append(s, Section{Kind: SectionOther, Text: "a" + strings.Repeat("b", i)})
			}
			return s
		}(), "sections"},
	}
	for _, c := range cases {
		if got := field(ValidateSections(c.secs)); got != c.want {
			t.Errorf("%s: invalid field %q, want %q", c.name, got, c.want)
		}
	}
	secs := []Section{{Kind: SectionOther, Label: "  Intro  ", Text: "\r\n  line one  \r\nline two\t\r\n\r\n"}}
	if err := ValidateSections(secs); err != nil || secs[0].Text != "  line one\nline two" || secs[0].Label != "Intro" {
		t.Errorf("normalisation: %q %q %v", secs[0].Text, secs[0].Label, err)
	}
	if got := NormalizeLyrics("a\rb\r\nc"); got != "a\nb\nc" {
		t.Errorf("line endings: %q", got)
	}
}

func TestValidateArrangement(t *testing.T) {
	secs := []Section{{ID: "S1"}, {ID: "S2"}}
	if err := ValidateArrangement([]SectionID{"S1", "S2", "S1"}, secs); err != nil {
		t.Errorf("repeats are allowed: %v", err)
	}
	if got := field(ValidateArrangement([]SectionID{"S1", "S9"}, secs)); got != "default_arrangement.1" {
		t.Errorf("unknown section: %q", got)
	}
	if got := field(ValidateArrangement(make([]SectionID, 101), secs)); got != "default_arrangement" {
		t.Errorf("101 entries: %q", got)
	}
	if !ValidSectionRequestKey("a1") || ValidSectionRequestKey("") || ValidSectionRequestKey(strings.Repeat("k", 41)) || ValidSectionRequestKey("a\nb") {
		t.Error("request keys")
	}
}
