// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Limits of a reading (07 §3).
const (
	MaxReadingText        = 20000
	MaxReadingAttribution = 300
	MaxReadingQuery       = 200
	SourceManual          = "manual"
)

// Reading is a Bible passage a church saved: the reference, the translation
// and the text a member typed or a provider supplied (07 §3).
type Reading struct {
	ID               ReadingID
	Reference        string // standard form, e.g. JHN 3:16-21
	ReferenceDisplay string // as typed
	Translation      Translation
	Text             string
	Attribution      string
	SourceProvider   string // "manual" or a provider ID; set by the server only
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ValidateReading normalises and checks the editable fields of a reading
// (07 §3). Failures are InvalidInputError values.
func ValidateReading(r *Reading) error {
	r.ReferenceDisplay = CollapseSpaces(r.ReferenceDisplay)
	if n := utf8.RuneCountInString(r.ReferenceDisplay); n < 1 || n > MaxReferenceInput {
		return &InvalidInputError{Field: "reference_display", Message: "1 to 100 characters."}
	}
	r.Text = NormalizeLyrics(r.Text)
	if n := utf8.RuneCountInString(r.Text); n < 1 || n > MaxReadingText {
		return &InvalidInputError{Field: "text", Message: "1 to 20000 characters."}
	}
	r.Attribution = strings.TrimSpace(r.Attribution)
	if utf8.RuneCountInString(r.Attribution) > MaxReadingAttribution {
		return tooLong("attribution", MaxReadingAttribution)
	}
	return nil
}

// SearchFold is the text list searches match against: the typed reference,
// the book's Indonesian name with the passage, and the text, folded the way
// the translation's language is folded (07 §4).
func (r Reading) SearchFold() string {
	canonical := r.Reference
	if ref, err := ParseReference(r.Reference); err == nil {
		canonical = ref.Canonical("id")
	}
	return FoldFor(r.Translation.Language, r.ReferenceDisplay+" "+canonical+" "+r.Text)
}

// Snippet is the first characters of the text on one line, for lists.
func Snippet(text string, n int) string {
	text = CollapseSpaces(text)
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	return strings.TrimSpace(string([]rune(text)[:n])) + "…"
}
