// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// SectionKind is the kind of a song section (06 §2.2).
type SectionKind string

// Section kinds.
const (
	SectionVerse     SectionKind = "verse"
	SectionPreChorus SectionKind = "pre_chorus"
	SectionChorus    SectionKind = "chorus"
	SectionBridge    SectionKind = "bridge"
	SectionTag       SectionKind = "tag"
	SectionIntro     SectionKind = "intro"
	SectionEnding    SectionKind = "ending"
	SectionOther     SectionKind = "other"
)

// SectionKinds lists every kind.
var SectionKinds = []SectionKind{SectionVerse, SectionPreChorus, SectionChorus, SectionBridge,
	SectionTag, SectionIntro, SectionEnding, SectionOther}

// LicenceStatus records what a church knows about the right to use a song (06 §2.1).
type LicenceStatus string

// Licence statuses.
const (
	LicenceUnknown            LicenceStatus = "unknown"
	LicencePublicDomain       LicenceStatus = "public_domain"
	LicenceChurch             LicenceStatus = "church_licence"
	LicencePermissionObtained LicenceStatus = "permission_obtained"
)

// LicenceStatuses lists every status.
var LicenceStatuses = []LicenceStatus{LicenceUnknown, LicencePublicDomain, LicenceChurch, LicencePermissionObtained}

// Song limits (06 §2).
const (
	MaxSections          = 60
	MaxSectionText       = 5000
	MaxArrangement       = 100
	MaxAltTitles         = 10
	MaxSongTitle         = 200
	MaxSongTerms         = 10
	MaxSongQuery         = 200
	maxLicenceNotes      = 2000
	maxCopyrightLine     = 300
	maxPersonField       = 200
	maxSectionLabel      = 60
	maxHymnalSource      = 40
	maxHymnalNumber      = 10
	maxCCLINumber        = 12
	maxSectionRequestKey = 40
)

// Section is one lyrics section; its position is its index in Song.Sections.
type Section struct {
	ID     SectionID
	Kind   SectionKind
	Number int    // verses only (1–99); 0 otherwise
	Label  string // "" = derive from kind and number
	Text   string
}

// Song is a church's song with its sections (06 §2).
type Song struct {
	ID                 SongID
	GroupID            SongGroupID // "" = not in a group
	Language           string
	Title              string
	AltTitles          []string
	HymnalSource       string
	HymnalNumber       string
	Lyricist           string
	Composer           string
	Translator         string
	DefaultKey         string
	CopyrightHolder    string
	CopyrightLine      string
	CCLISongNumber     string
	LicenceStatus      LicenceStatus
	LicenceNotes       string
	Sections           []Section
	DefaultArrangement []SectionID // entries name sections of this song; repeats allowed
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// TitleKey is the folded title, used for ordering and duplicate detection.
func (s Song) TitleKey() string { return Fold(s.Title) }

// HymnalKey is the song's canonical hymnal key ("" without a hymnal number).
func (s Song) HymnalKey() string { return HymnalKey(s.HymnalSource, s.HymnalNumber) }

var (
	keyRe  = regexp.MustCompile(`^[A-G][#b]?m?$`)
	ccliRe = regexp.MustCompile(`^[0-9]{1,12}$`)
)

func tooLong(field string, limit int) error {
	return &InvalidInputError{Field: field, Message: "At most " + strconv.Itoa(limit) + " characters."}
}

// NormalizeLyrics normalises section text (06 §2.2): line endings become "\n",
// trailing spaces on each line and leading and trailing blank lines are removed.
func NormalizeLyrics(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// ValidateSong trims and checks every field of the song except its sections
// and arrangement (06 §2.1). Failures are InvalidInputError values.
func ValidateSong(s *Song) error {
	s.Title = strings.TrimSpace(s.Title)
	if n := utf8.RuneCountInString(s.Title); n < 1 || n > MaxSongTitle {
		return &InvalidInputError{Field: "title", Message: "Title must be 1 to 200 characters."}
	}
	if !slices.Contains(ContentLanguages, s.Language) {
		return &InvalidInputError{Field: "language", Message: "Choose one of the supported languages."}
	}
	if len(s.AltTitles) > MaxAltTitles {
		return &InvalidInputError{Field: "alt_titles", Message: "At most 10 alternative titles."}
	}
	seen := map[string]bool{Fold(s.Title): true}
	for i, t := range s.AltTitles {
		t = strings.TrimSpace(t)
		s.AltTitles[i] = t
		field := "alt_titles." + strconv.Itoa(i)
		if n := utf8.RuneCountInString(t); n < 1 || n > MaxSongTitle {
			return &InvalidInputError{Field: field, Message: "Title must be 1 to 200 characters."}
		}
		k := Fold(t)
		if seen[k] && k != "" {
			return &InvalidInputError{Field: field, Message: "Alternative titles must be different."}
		}
		seen[k] = true
	}
	for _, f := range []struct {
		field string
		v     *string
		max   int
	}{
		{"hymnal_source", &s.HymnalSource, maxHymnalSource}, {"hymnal_number", &s.HymnalNumber, maxHymnalNumber},
		{"lyricist", &s.Lyricist, maxPersonField}, {"composer", &s.Composer, maxPersonField},
		{"translator", &s.Translator, maxPersonField}, {"copyright_holder", &s.CopyrightHolder, maxPersonField},
		{"copyright_line", &s.CopyrightLine, maxCopyrightLine}, {"licence_notes", &s.LicenceNotes, maxLicenceNotes},
	} {
		*f.v = strings.TrimSpace(*f.v)
		if utf8.RuneCountInString(*f.v) > f.max {
			return tooLong(f.field, f.max)
		}
	}
	if (s.HymnalSource == "") != (s.HymnalNumber == "") {
		return &InvalidInputError{Field: "hymnal_number", Message: "Give both the hymnal and the number, or neither."}
	}
	if s.HymnalSource != "" && (Fold(s.HymnalSource) == "" || Fold(s.HymnalNumber) == "") {
		return &InvalidInputError{Field: "hymnal_number", Message: "The hymnal and the number need letters or digits."}
	}
	s.DefaultKey = strings.TrimSpace(s.DefaultKey)
	if s.DefaultKey != "" && !keyRe.MatchString(s.DefaultKey) {
		return &InvalidInputError{Field: "default_key", Message: "Use a key such as G, Bb or F#m."}
	}
	s.CCLISongNumber = strings.TrimSpace(s.CCLISongNumber)
	if s.CCLISongNumber != "" && !ccliRe.MatchString(s.CCLISongNumber) {
		return &InvalidInputError{Field: "ccli_song_number", Message: "Use 1 to 12 digits."}
	}
	if s.LicenceStatus == "" {
		s.LicenceStatus = LicenceUnknown
	}
	if !slices.Contains(LicenceStatuses, s.LicenceStatus) {
		return &InvalidInputError{Field: "licence_status", Message: "Unknown licence status."}
	}
	return nil
}

// ValidateSections normalises and checks the sections of a song (06 §2.2).
func ValidateSections(secs []Section) error {
	if len(secs) > MaxSections {
		return &InvalidInputError{Field: "sections", Message: "At most 60 sections."}
	}
	verses := map[int]bool{}
	for i := range secs {
		sec := &secs[i]
		field := "sections." + strconv.Itoa(i)
		if !slices.Contains(SectionKinds, sec.Kind) {
			return &InvalidInputError{Field: field + ".kind", Message: "Unknown section kind."}
		}
		if sec.Kind == SectionVerse {
			if sec.Number < 1 || sec.Number > 99 {
				return &InvalidInputError{Field: field + ".number", Message: "A verse needs a number from 1 to 99."}
			}
			if verses[sec.Number] {
				return &InvalidInputError{Field: field + ".number", Message: "Two verses have this number."}
			}
			verses[sec.Number] = true
		} else if sec.Number != 0 {
			return &InvalidInputError{Field: field + ".number", Message: "Only verses have a number."}
		}
		sec.Label = strings.TrimSpace(sec.Label)
		if utf8.RuneCountInString(sec.Label) > maxSectionLabel {
			return tooLong(field+".label", maxSectionLabel)
		}
		sec.Text = NormalizeLyrics(sec.Text)
		if n := utf8.RuneCountInString(sec.Text); n < 1 || n > MaxSectionText {
			return &InvalidInputError{Field: field + ".text", Message: "The text must be 1 to 5000 characters."}
		}
	}
	return nil
}

// ValidateArrangement checks that every entry names a section of secs.
func ValidateArrangement(arr []SectionID, secs []Section) error {
	if len(arr) > MaxArrangement {
		return &InvalidInputError{Field: "default_arrangement", Message: "At most 100 entries."}
	}
	for i, id := range arr {
		if !slices.ContainsFunc(secs, func(s Section) bool { return s.ID == id }) {
			return &InvalidInputError{Field: "default_arrangement." + strconv.Itoa(i), Message: "This is not a section of the song."}
		}
	}
	return nil
}

// ValidSectionRequestKey reports whether k is usable as a request-local key
// for a new section (1–40 characters, no control characters).
func ValidSectionRequestKey(k string) bool {
	if n := utf8.RuneCountInString(k); n < 1 || n > maxSectionRequestKey {
		return false
	}
	return !strings.ContainsFunc(k, unicode.IsControl)
}
