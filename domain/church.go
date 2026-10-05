// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Church is the row that owns all church data (schema churches).
type Church struct {
	ID                   ChurchID
	Name                 string
	DefaultUILanguage    string // en | id
	DefaultLanguage      string // id | en | zh-Hans | zh-Hant
	DefaultTranslationID TranslationID
	TimeZone             string // IANA name
	Settings             ChurchSettings
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// PrintDefaults are the church's defaults for the print view (13 §6).
type PrintDefaults struct {
	Paper       string `json:"paper"`  // a4 | f4
	Lyrics      string `json:"lyrics"` // full | first_lines
	Readings    bool   `json:"readings"`
	Assignments bool   `json:"assignments"`
	Keys        bool   `json:"keys"`
	Notes       bool   `json:"notes"`
	Size        string `json:"size"` // normal | large
}

// DefaultPrint is what a church that never set the options gets.
var DefaultPrint = PrintDefaults{Paper: "a4", Lyrics: "full", Readings: true, Assignments: true, Keys: true, Notes: true, Size: "normal"}

// Allowed print values (13 §6).
var (
	PrintPapers = []string{"a4", "f4"}
	PrintLyrics = []string{"full", "first_lines"}
	PrintSizes  = []string{"normal", "large"}
)

// Validate checks the values of a complete print object.
func (p PrintDefaults) Validate(field string) error {
	switch {
	case !slices.Contains(PrintPapers, p.Paper):
		return &InvalidInputError{Field: field + ".paper", Message: "Must be a4 or f4."}
	case !slices.Contains(PrintLyrics, p.Lyrics):
		return &InvalidInputError{Field: field + ".lyrics", Message: "Must be full or first_lines."}
	case !slices.Contains(PrintSizes, p.Size):
		return &InvalidInputError{Field: field + ".size", Message: "Must be normal or large."}
	}
	return nil
}

// ChurchSettings is the settings JSON object. Keys this version doesn't know
// are kept in Extra and written back unchanged (schema churches.settings).
type ChurchSettings struct {
	KeyDisplay     string         // do | letter
	FeedbackURL    string         // "" = none
	PrivacyContact string         // "" = none
	ShowCredits    *bool          // nil = not set = true (13 §6)
	LicenceFooter  string         // "" = none
	Print          *PrintDefaults // nil = DefaultPrint
	Extra          map[string]json.RawMessage
}

// CreditsShown is show_credits with its default.
func (s ChurchSettings) CreditsShown() bool { return s.ShowCredits == nil || *s.ShowCredits }

// PrintOrDefault is the print options with their defaults.
func (s ChurchSettings) PrintOrDefault() PrintDefaults {
	if s.Print == nil {
		return DefaultPrint
	}
	return *s.Print
}

var knownSettings = []string{"key_display", "feedback_url", "privacy_contact", "show_credits", "licence_footer", "print"}

// MarshalJSON writes known keys (omitting empty ones) plus the preserved extras.
func (s ChurchSettings) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range s.Extra {
		m[k] = v
	}
	for _, k := range knownSettings {
		delete(m, k)
	}
	for k, v := range map[string]string{"key_display": s.KeyDisplay, "feedback_url": s.FeedbackURL, "privacy_contact": s.PrivacyContact, "licence_footer": s.LicenceFooter} {
		if v != "" {
			m[k] = v
		}
	}
	if s.ShowCredits != nil {
		m["show_credits"] = *s.ShowCredits
	}
	if s.Print != nil {
		m["print"] = *s.Print
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads known keys and keeps the rest. A known key of the wrong
// type reads as not set.
func (s *ChurchSettings) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*s = ChurchSettings{}
	dst := map[string]*string{"key_display": &s.KeyDisplay, "feedback_url": &s.FeedbackURL, "privacy_contact": &s.PrivacyContact, "licence_footer": &s.LicenceFooter}
	for _, k := range knownSettings {
		raw, ok := m[k]
		if !ok {
			continue
		}
		switch k {
		case "show_credits":
			var v bool
			if json.Unmarshal(raw, &v) == nil {
				s.ShowCredits = &v
			}
		case "print":
			var v PrintDefaults
			if json.Unmarshal(raw, &v) == nil && v.Validate("print") == nil {
				s.Print = &v
			}
		default:
			_ = json.Unmarshal(raw, dst[k]) // a non-string value reads as empty
		}
		delete(m, k)
	}
	if len(m) > 0 {
		s.Extra = m
	}
	return nil
}

// Allowed values (03 §10).
var (
	UILanguages      = []string{"en", "id"}
	ContentLanguages = []string{"id", "en", "zh-Hans", "zh-Hant"}
	KeyDisplays      = []string{"do", "letter"}
)

// ValidateChurch checks every field except the translation (looked up by the
// use case) and the time zone (loaded by the use case). Name is trimmed.
func ValidateChurch(c *Church, fieldPrefix string) error {
	c.Name = strings.TrimSpace(c.Name)
	if n := utf8.RuneCountInString(c.Name); n < 1 || n > 120 {
		return &InvalidInputError{Field: fieldPrefix + "name", Message: "Name must be 1 to 120 characters."}
	}
	if !slices.Contains(UILanguages, c.DefaultUILanguage) {
		return &InvalidInputError{Field: fieldPrefix + "default_ui_language", Message: "Must be en or id."}
	}
	if !slices.Contains(ContentLanguages, c.DefaultLanguage) {
		return &InvalidInputError{Field: fieldPrefix + "default_language", Message: "Must be id, en, zh-Hans or zh-Hant."}
	}
	if !slices.Contains(KeyDisplays, c.Settings.KeyDisplay) {
		return &InvalidInputError{Field: fieldPrefix + "key_display", Message: "Must be do or letter."}
	}
	if u := c.Settings.FeedbackURL; u != "" {
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "https" || p.Host == "" || len(u) > 2000 {
			return &InvalidInputError{Field: fieldPrefix + "feedback_url", Message: "Must be an absolute https URL."}
		}
	}
	if utf8.RuneCountInString(c.Settings.PrivacyContact) > 500 {
		return &InvalidInputError{Field: fieldPrefix + "privacy_contact", Message: "At most 500 characters."}
	}
	c.Settings.LicenceFooter = strings.TrimSpace(c.Settings.LicenceFooter)
	if utf8.RuneCountInString(c.Settings.LicenceFooter) > 200 {
		return &InvalidInputError{Field: fieldPrefix + "licence_footer", Message: "At most 200 characters."}
	}
	if c.Settings.Print != nil {
		if err := c.Settings.Print.Validate(fieldPrefix + "print"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRole trims and checks a role's name and description (03 §8).
func ValidateRole(name, description *string, scopes []Scope) error {
	if name != nil {
		*name = strings.TrimSpace(*name)
		if n := utf8.RuneCountInString(*name); n < 1 || n > 60 {
			return &InvalidInputError{Field: "name", Message: "Name must be 1 to 60 characters."}
		}
	}
	if description != nil {
		*description = strings.TrimSpace(*description)
		if utf8.RuneCountInString(*description) > 200 {
			return &InvalidInputError{Field: "description", Message: "At most 200 characters."}
		}
	}
	for _, s := range scopes {
		if !ValidScope(s) {
			return &InvalidInputError{Field: "scopes", Message: "Unknown scope " + string(s) + "."}
		}
	}
	return nil
}

// ValidatePersonName trims and checks a person's name (1–120 characters).
func ValidatePersonName(name *string, field string) error {
	*name = strings.TrimSpace(*name)
	if n := utf8.RuneCountInString(*name); n < 1 || n > 120 {
		return &InvalidInputError{Field: field, Message: "Name must be 1 to 120 characters."}
	}
	return nil
}

// Translation is a Bible translation (schema translations).
type Translation struct {
	ID       TranslationID
	Code     string
	Name     string
	Language string
}
