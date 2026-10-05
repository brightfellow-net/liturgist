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

// ListKind says which of the two short, ordered name lists an entry is in (09 §2.1).
type ListKind string

// The two lists.
const (
	KindDuty        ListKind = "duty"
	KindSingingPart ListKind = "singing_part"
)

// Limits of the planning setup (09 §2).
const (
	MaxDuties        = 50
	MaxSingingParts  = 30
	MaxDutyName      = 60
	MaxPartName      = 40
	MaxTemplates     = 50
	MaxTemplateItems = 60
	MaxServices      = 30
	MaxServiceTimes  = 14
	MaxPlanningName  = 100
	MaxItemTitle     = 200
	MaxTemplateText  = 5000
)

// Reasons of an InvalidInputError (01 §10).
const (
	ReasonLimit            = "limit"
	ReasonRequired         = "required"
	ReasonLanguageMismatch = "language_mismatch"
)

// MaxLen is the longest name an entry of the list may have.
func (k ListKind) MaxLen() int {
	if k == KindDuty {
		return MaxDutyName
	}
	return MaxPartName
}

// Limit is the number of entries a church may have in the list.
func (k ListKind) Limit() int {
	if k == KindDuty {
		return MaxDuties
	}
	return MaxSingingParts
}

// NameEntry is a duty or a singing part: a name in an ordered list (09 §2.1).
// ID is a DutyID or a SingingPartID, by Kind.
type NameEntry struct {
	ID        string
	Name      string
	NameKey   string // Fold(Name), unique per church and kind
	Position  int    // dense 0..n-1
	CreatedAt time.Time
}

// ValidateEntryName trims and checks a duty or singing-part name and returns
// its folded key. Failures are InvalidInputError values.
func ValidateEntryName(kind ListKind, name *string) (key string, err error) {
	return validateName(name, "name", kind.MaxLen())
}

// validateName trims a one-line name of 1..limit characters with at least one
// letter or digit, and returns Fold(name).
func validateName(name *string, field string, limit int) (string, error) {
	*name = strings.TrimSpace(*name)
	n := utf8.RuneCountInString(*name)
	if n < 1 || n > limit {
		return "", &InvalidInputError{Field: field, Message: "1 to " + strconv.Itoa(limit) + " characters."}
	}
	if strings.IndexFunc(*name, unicode.IsControl) >= 0 {
		return "", &InvalidInputError{Field: field, Message: "No line breaks."}
	}
	key := Fold(*name)
	if key == "" {
		return "", &InvalidInputError{Field: field, Message: "Use at least one letter or digit."}
	}
	return key, nil
}

// LimitError is the error of a fixed count that would be exceeded (09 §2.1).
func LimitError(field string, limit, used int) error {
	return &InvalidInputError{Field: field, Message: "At most " + strconv.Itoa(limit) + ".", Reason: ReasonLimit, Max: limit, Used: used}
}

// ItemType is the kind of a template or liturgy item (09 §2.3, 10 §2.2).
type ItemType string

// Item types.
const (
	ItemSong     ItemType = "song"
	ItemReading  ItemType = "reading"
	ItemPrayer   ItemType = "prayer"
	ItemSermon   ItemType = "sermon"
	ItemFreeText ItemType = "free_text"
	ItemOther    ItemType = "other"
)

// ItemTypes lists every item type in display order.
var ItemTypes = []ItemType{ItemSong, ItemReading, ItemPrayer, ItemSermon, ItemFreeText, ItemOther}

// TakesText reports whether items of the type carry text of their own.
func (t ItemType) TakesText() bool {
	return t == ItemPrayer || t == ItemSermon || t == ItemFreeText || t == ItemOther
}

// TemplateItem is one item of a template. Template items have no stable IDs
// and nothing refers to them (09 §2.3).
type TemplateItem struct {
	ID            string // storage key, made by the use case when the template is saved; never in the API
	Title         string
	Type          ItemType
	DefaultText   string
	DefaultDutyID DutyID // "" = none
}

// Template is a named list of items a liturgy can be made from (09 §2.2).
type Template struct {
	ID        TemplateID
	Name      string
	NameKey   string
	Language  string
	Items     []TemplateItem
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ValidateTemplate trims and checks a template and its items (09 §2.2, §2.3)
// and sets NameKey. The duties are checked by the use case.
func ValidateTemplate(t *Template) error {
	key, err := validateName(&t.Name, "name", MaxPlanningName)
	if err != nil {
		return err
	}
	t.NameKey = key
	if !slices.Contains(ContentLanguages, t.Language) {
		return &InvalidInputError{Field: "language", Message: "Must be id, en, zh-Hans or zh-Hant."}
	}
	if len(t.Items) > MaxTemplateItems {
		return LimitError("items", MaxTemplateItems, len(t.Items))
	}
	for i := range t.Items {
		it := &t.Items[i]
		field := "items." + strconv.Itoa(i) + "."
		it.Title = strings.TrimSpace(it.Title)
		if n := utf8.RuneCountInString(it.Title); n < 1 || n > MaxItemTitle {
			return &InvalidInputError{Field: field + "title", Message: "1 to 200 characters."}
		}
		if !slices.Contains(ItemTypes, it.Type) {
			return &InvalidInputError{Field: field + "item_type", Message: "Unknown item type."}
		}
		it.DefaultText = NormalizeLyrics(it.DefaultText)
		if it.DefaultText != "" && !it.Type.TakesText() {
			return &InvalidInputError{Field: field + "default_text", Message: "Songs and readings take no text."}
		}
		if utf8.RuneCountInString(it.DefaultText) > MaxTemplateText {
			return tooLong(field+"default_text", MaxTemplateText)
		}
	}
	return nil
}

// ServiceTime is one weekly time of a service: ISO weekday 1 (Monday) to 7
// (Sunday) and a wall-clock time "HH:MM" in the church's time zone (09 §2.4).
type ServiceTime struct {
	ID      string // storage key, made by the use case when the service is saved; never in the API
	Weekday int
	Time    string
}

// Service is a regular service of the church (09 §2.4).
type Service struct {
	ID                ServiceID
	Name              string
	NameKey           string
	Language          string
	DefaultTemplateID TemplateID // "" = none
	Times             []ServiceTime
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

var clockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidClock reports whether s is a 24-hour time "HH:MM" from 00:00 to 23:59.
func ValidClock(s string) bool { return clockRe.MatchString(s) }

// ValidateService trims and checks a service, sorts its times by weekday and
// time, and sets NameKey. The template is checked by the use case.
func ValidateService(s *Service) error {
	key, err := validateName(&s.Name, "name", MaxPlanningName)
	if err != nil {
		return err
	}
	s.NameKey = key
	if !slices.Contains(ContentLanguages, s.Language) {
		return &InvalidInputError{Field: "language", Message: "Must be id, en, zh-Hans or zh-Hant."}
	}
	if len(s.Times) == 0 {
		return &InvalidInputError{Field: "times", Message: "A service needs at least one time.", Reason: ReasonRequired}
	}
	if len(s.Times) > MaxServiceTimes {
		return LimitError("times", MaxServiceTimes, len(s.Times))
	}
	for i, t := range s.Times {
		field := "times." + strconv.Itoa(i) + "."
		if t.Weekday < 1 || t.Weekday > 7 {
			return &InvalidInputError{Field: field + "weekday", Message: "1 (Monday) to 7 (Sunday)."}
		}
		if !ValidClock(t.Time) {
			return &InvalidInputError{Field: field + "time", Message: "Use HH:MM, 00:00 to 23:59."}
		}
	}
	slices.SortFunc(s.Times, func(a, b ServiceTime) int {
		if a.Weekday != b.Weekday {
			return a.Weekday - b.Weekday
		}
		return strings.Compare(a.Time, b.Time)
	})
	for i := 1; i < len(s.Times); i++ {
		if s.Times[i].Weekday == s.Times[i-1].Weekday && s.Times[i].Time == s.Times[i-1].Time {
			return &InvalidInputError{Field: "times." + strconv.Itoa(i), Message: "This time is listed twice."}
		}
	}
	return nil
}
