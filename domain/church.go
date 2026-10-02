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

// ChurchSettings is the settings JSON object. Keys this version doesn't know
// are kept in Extra and written back unchanged (schema churches.settings).
type ChurchSettings struct {
	KeyDisplay     string // do | letter
	FeedbackURL    string // "" = none
	PrivacyContact string // "" = none
	Extra          map[string]json.RawMessage
}

var knownSettings = []string{"key_display", "feedback_url", "privacy_contact"}

// MarshalJSON writes known keys (omitting empty ones) plus the preserved extras.
func (s ChurchSettings) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range s.Extra {
		m[k] = v
	}
	for k, v := range map[string]string{"key_display": s.KeyDisplay, "feedback_url": s.FeedbackURL, "privacy_contact": s.PrivacyContact} {
		if v != "" {
			m[k] = v
		} else {
			delete(m, k)
		}
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads known keys and keeps the rest.
func (s *ChurchSettings) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*s = ChurchSettings{}
	dst := map[string]*string{"key_display": &s.KeyDisplay, "feedback_url": &s.FeedbackURL, "privacy_contact": &s.PrivacyContact}
	for _, k := range knownSettings {
		if raw, ok := m[k]; ok {
			_ = json.Unmarshal(raw, dst[k]) // a non-string value reads as empty
			delete(m, k)
		}
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
