// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits of a liturgy (10 §2).
const (
	MaxLiturgyItems     = 60
	MaxItemSongs        = 10
	MaxSequenceEntries  = MaxArrangement
	MaxItemText         = 20000
	MaxSongNote         = 200
	MaxEntryNote        = 100
	MaxAssignmentName   = 100
	MaxAssignments      = 200
	MaxPrepareOccurence = 50
	MaxLiturgyQuery     = 100
)

// LiturgyState is the review state of a liturgy (SPEC §5.5). Step 3 creates only drafts.
type LiturgyState string

// The states.
const (
	StateDraft         LiturgyState = "draft"
	StateInReview      LiturgyState = "in_review"
	StateNeedsRevision LiturgyState = "needs_revision"
	StateApproved      LiturgyState = "approved"
	StatePublished     LiturgyState = "published"
)

// LiturgyStates lists every state.
var LiturgyStates = []LiturgyState{StateDraft, StateInReview, StateNeedsRevision, StateApproved, StatePublished}

// Valid reports whether s is a known state.
func (s LiturgyState) Valid() bool { return slices.Contains(LiturgyStates, s) }

// Editable reports whether items may be written in the state (10 §2.1).
func (s LiturgyState) Editable() bool { return s == StateDraft || s == StateNeedsRevision }

// Deletable reports whether a liturgy in the state may be deleted (10 §2.1).
func (s LiturgyState) Deletable() bool { return s.Valid() && s != StatePublished }

// Liturgy is the service of one date (10 §2.1).
type Liturgy struct {
	ID          LiturgyID
	Date        string // YYYY-MM-DD
	Time        string // HH:MM or ""
	ServiceID   ServiceID
	ServiceName string
	Language    string
	TemplateID  TemplateID
	State       LiturgyState
	Version     int
	ArchivedAt  *time.Time
	ArchivedBy  UserID
	CreatedBy   UserID
	EditSeq     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Item is one item of a liturgy (10 §2.2).
type Item struct {
	ID           ItemID
	LiturgyID    LiturgyID
	Position     int
	Title        string
	Type         ItemType
	DutyID       DutyID
	Text         string
	ReadingID    ReadingID
	ReadingLabel string
	Version      int
	Songs        []LiturgySong
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// LiturgySong is one song sung in a song item, with its sequence (10 §2.3).
type LiturgySong struct {
	ID        ItemSongID
	Position  int
	SongID    SongID // "" = the song was deleted
	SongTitle string // snapshot
	Key       string
	Note      string
	Entries   []Entry
}

// Entry is one entry of a sequence (10 §2.3).
type Entry struct {
	ID            EntryID
	Position      int
	SectionID     SectionID // "" = the section was deleted
	SingingPartID SingingPartID
	KeyChange     string
	Note          string
	SectionLabel  string // snapshot
}

// Assignment puts a person on a duty in one liturgy (10 §2.4).
type Assignment struct {
	ID        AssignmentID
	LiturgyID LiturgyID
	DutyID    DutyID
	UserID    UserID // "" for a name
	Name      string
	NameKey   string
	CreatedAt time.Time
}

// The commands of a history row (10 §7).
const (
	CmdLiturgyCreate     = "liturgy.create"
	CmdLiturgyUpdate     = "liturgy.update"
	CmdItemAdd           = "item.add"
	CmdItemRemove        = "item.remove"
	CmdItemUpdate        = "item.update"
	CmdItemSongs         = "item.songs"
	CmdItemsReorder      = "items.reorder"
	CmdAssignmentAdd     = "assignment.add"
	CmdAssignmentRemove  = "assignment.remove"
	EditDone             = "done"
	maxLiturgyServiceLen = MaxPlanningName
)

// Edit is one row of a liturgy's history (10 §7). Before and After are JSON images.
type Edit struct {
	ID                  EditID
	LiturgyID           LiturgyID
	Seq                 int
	UserID              UserID
	Command             string
	ItemID              ItemID
	Before, After       []byte // nil = none
	LiturgyVersionAfter int
	ItemVersionAfter    int // 0 = none
	Status              string
	CreatedAt           time.Time
}

// ValidDate reports whether s is a real calendar date written YYYY-MM-DD.
func ValidDate(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

// ValidateLiturgyFields checks and trims the editable fields of a liturgy
// (10 §2.1): the date, the time (required with a service) and the name.
func ValidateLiturgyFields(l *Liturgy) error {
	if !ValidDate(l.Date) {
		return &InvalidInputError{Field: "date", Message: "Use a real date as YYYY-MM-DD."}
	}
	if l.Time == "" && l.ServiceID != "" {
		return &InvalidInputError{Field: "time", Message: "A service needs a time.", Reason: ReasonRequired}
	}
	if l.Time != "" && !ValidClock(l.Time) {
		return &InvalidInputError{Field: "time", Message: "Use HH:MM, 00:00 to 23:59."}
	}
	l.ServiceName = strings.TrimSpace(l.ServiceName)
	if n := utf8.RuneCountInString(l.ServiceName); n < 1 || n > maxLiturgyServiceLen {
		return &InvalidInputError{Field: "service_name", Message: "1 to 100 characters."}
	}
	if strings.IndexFunc(l.ServiceName, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
		return &InvalidInputError{Field: "service_name", Message: "No line breaks."}
	}
	return nil
}

// ValidateItemFields checks and trims the title and text of an item for its
// type (10 §2.2). field is the JSON path prefix, "" or "items.3.".
func ValidateItemFields(it *Item, field string) error {
	it.Title = strings.TrimSpace(it.Title)
	if n := utf8.RuneCountInString(it.Title); n < 1 || n > MaxItemTitle {
		return &InvalidInputError{Field: field + "title", Message: "1 to 200 characters."}
	}
	if !slices.Contains(ItemTypes, it.Type) {
		return &InvalidInputError{Field: field + "item_type", Message: "Unknown item type."}
	}
	it.Text = NormalizeLyrics(it.Text)
	if it.Text != "" && !it.Type.TakesText() {
		return &InvalidInputError{Field: field + "text", Message: "Songs and readings take no text."}
	}
	if utf8.RuneCountInString(it.Text) > MaxItemText {
		return tooLong(field+"text", MaxItemText)
	}
	return nil
}

// ValidKey reports whether k is empty or a key such as C, F#, Bb or Em (10 §2.3).
func ValidKey(k string) bool { return k == "" || keyRe.MatchString(k) }

// ValidateItemSong checks the key and note of an item song.
func ValidateItemSong(s *LiturgySong, field string) error {
	s.Key = strings.TrimSpace(s.Key)
	if !ValidKey(s.Key) {
		return &InvalidInputError{Field: field + "key", Message: "Use a key such as C, F#, Bb or Em."}
	}
	s.Note = strings.TrimSpace(s.Note)
	if utf8.RuneCountInString(s.Note) > MaxSongNote {
		return tooLong(field+"note", MaxSongNote)
	}
	if len(s.Entries) > MaxSequenceEntries {
		return LimitError(field+"entries", MaxSequenceEntries, len(s.Entries))
	}
	for i := range s.Entries {
		e := &s.Entries[i]
		f := field + "entries." + strconv.Itoa(i) + "."
		e.KeyChange = strings.TrimSpace(e.KeyChange)
		if !ValidKey(e.KeyChange) {
			return &InvalidInputError{Field: f + "key_change", Message: "Use a key such as C, F#, Bb or Em."}
		}
		e.Note = strings.TrimSpace(e.Note)
		if utf8.RuneCountInString(e.Note) > MaxEntryNote {
			return tooLong(f+"note", MaxEntryNote)
		}
	}
	return nil
}

// ValidateAssignmentName trims and checks a free-text name and returns Fold(name).
func ValidateAssignmentName(name *string) (string, error) {
	return validateName(name, "name", MaxAssignmentName)
}

// SectionDisplay is the text a section shows in a sequence: its own label, or
// one derived from the kind and number ("Verse 1", "Chorus") (10 §2.3).
func SectionDisplay(sec Section) string {
	if sec.Label != "" {
		return sec.Label
	}
	var name string
	switch sec.Kind {
	case SectionVerse:
		name = "Verse"
	case SectionPreChorus:
		name = "Pre-chorus"
	case SectionChorus:
		name = "Chorus"
	case SectionBridge:
		name = "Bridge"
	case SectionTag:
		name = "Tag"
	case SectionIntro:
		name = "Intro"
	case SectionEnding:
		name = "Ending"
	default:
		name = "Section"
	}
	if sec.Kind == SectionVerse && sec.Number > 0 {
		return name + " " + strconv.Itoa(sec.Number)
	}
	return name
}

// ReadingLabel is the display text kept with a reading item: the passage as
// the Indonesian book name and the translation code, "Yohanes 3:16-21 (TB)" (10 §2.2).
func ReadingLabel(r Reading) string {
	canonical := r.ReferenceDisplay
	if ref, err := ParseReference(r.Reference); err == nil {
		canonical = ref.Canonical("id")
	}
	if r.Translation.Code == "" {
		return canonical
	}
	return canonical + " (" + r.Translation.Code + ")"
}

// Problem codes of GET /liturgies/{id} (10 §2.5).
const (
	ProblemSongMissing    = "song_missing"
	ProblemReadingMissing = "reading_missing"
	ProblemSongRemoved    = "song_removed"
	ProblemReadingRemoved = "reading_removed"
	ProblemSectionRemoved = "section_removed"
)

// Problem is one thing the editor shows and step 4 will check before a submit.
type Problem struct {
	Code       string
	ItemID     ItemID
	ItemSongID ItemSongID
	EntryID    EntryID
}

// Problems computes the problems of the items of a liturgy (10 §2.5).
func Problems(items []Item) []Problem {
	out := []Problem{}
	for _, it := range items {
		switch it.Type {
		case ItemSong:
			if len(it.Songs) == 0 {
				out = append(out, Problem{Code: ProblemSongMissing, ItemID: it.ID})
			}
			for _, s := range it.Songs {
				if s.SongID == "" {
					out = append(out, Problem{Code: ProblemSongRemoved, ItemID: it.ID, ItemSongID: s.ID})
				}
				for _, e := range s.Entries {
					if e.SectionID == "" {
						out = append(out, Problem{Code: ProblemSectionRemoved, ItemID: it.ID, ItemSongID: s.ID, EntryID: e.ID})
					}
				}
			}
		case ItemReading:
			switch {
			case it.ReadingID != "":
			case it.ReadingLabel != "":
				out = append(out, Problem{Code: ProblemReadingRemoved, ItemID: it.ID})
			default:
				out = append(out, Problem{Code: ProblemReadingMissing, ItemID: it.ID})
			}
		}
	}
	return out
}

// ISOWeekday is the ISO 8601 weekday of a date: 1 (Monday) to 7 (Sunday).
func ISOWeekday(t time.Time) int {
	if w := int(t.Weekday()); w != 0 {
		return w
	}
	return 7
}

// WeekStart is the Monday of the ISO week that contains the calendar date d.
func WeekStart(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day()-(ISOWeekday(d)-1), 0, 0, 0, 0, time.UTC)
}
