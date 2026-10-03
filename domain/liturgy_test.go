// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-L-001: the fields of a liturgy.
func TestLiturgyFields(t *testing.T) {
	for date, want := range map[string]bool{"2026-10-04": true, "2026-02-30": false, "4-10-2026": false, "2026-10-4": false, "2028-02-29": true, "2026-13-01": false, "": false} {
		if got := domain.ValidDate(date); got != want {
			t.Errorf("ValidDate(%q) = %v", date, got)
		}
	}
	for _, c := range []struct {
		name, time, service string
		ok                  bool
	}{
		{"service, 07:00", "07:00", "S", true}, {"one-off, no time", "", "", true}, {"service, no time", "", "S", false},
		{"7:00", "7:00", "", false}, {"24:00", "24:00", "", false}, {"23:59", "23:59", "", true},
	} {
		l := domain.Liturgy{Date: "2026-10-04", Time: c.time, ServiceName: " Ibadah "}
		if c.service != "" {
			l.ServiceID = domain.ServiceID(c.service)
		}
		err := domain.ValidateLiturgyFields(&l)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
		if err == nil && l.ServiceName != "Ibadah" {
			t.Errorf("%s: name %q not trimmed", c.name, l.ServiceName)
		}
	}
	for _, name := range []string{"", "  ", strings.Repeat("x", 101), "a\nb"} {
		if err := domain.ValidateLiturgyFields(&domain.Liturgy{Date: "2026-10-04", ServiceName: name}); err == nil {
			t.Errorf("service name %q accepted", name)
		}
	}
}

// TC-L-005: keys, notes and the shape of an item song.
func TestItemSongRules(t *testing.T) {
	for k, want := range map[string]bool{"": true, "C": true, "F#": true, "Bb": true, "Em": true, "F#m": true, "H": false, "c": false, "Cb#": false, "Do": false} {
		if got := domain.ValidKey(k); got != want {
			t.Errorf("ValidKey(%q) = %v", k, got)
		}
	}
	entries := func(n int) []domain.Entry { return make([]domain.Entry, n) }
	for name, s := range map[string]domain.LiturgySong{
		"100 entries":   {Entries: entries(100)},
		"key":           {Key: " Bb "},
		"200-char note": {Note: strings.Repeat("x", 200)},
	} {
		if err := domain.ValidateItemSong(&s, ""); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, s := range map[string]domain.LiturgySong{
		"101 entries":         {Entries: entries(101)},
		"key H":               {Key: "H"},
		"201-char note":       {Note: strings.Repeat("x", 201)},
		"key change H":        {Entries: []domain.Entry{{KeyChange: "H"}}},
		"101-char entry note": {Entries: []domain.Entry{{Note: strings.Repeat("x", 101)}}},
	} {
		if err := domain.ValidateItemSong(&s, "songs.0."); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TC-L-007: what an item may hold.
func TestItemFields(t *testing.T) {
	ok := []domain.Item{{Title: " Doa ", Type: domain.ItemPrayer, Text: "a\r\nb"}, {Title: "Lagu", Type: domain.ItemSong}, {Title: "x", Type: domain.ItemReading}}
	for _, it := range ok {
		if err := domain.ValidateItemFields(&it, ""); err != nil {
			t.Errorf("%+v: %v", it, err)
		}
	}
	if it := ok[0]; domain.ValidateItemFields(&it, "") != nil || it.Title != "Doa" || it.Text != "a\nb" {
		t.Errorf("trimmed: %+v", it)
	}
	for name, it := range map[string]domain.Item{
		"no title": {Type: domain.ItemPrayer}, "title 201": {Title: strings.Repeat("x", 201), Type: domain.ItemPrayer}, "type": {Title: "x", Type: "video"},
		"text on song": {Title: "x", Type: domain.ItemSong, Text: "t"}, "text on reading": {Title: "x", Type: domain.ItemReading, Text: "t"},
		"text 20001": {Title: "x", Type: domain.ItemPrayer, Text: strings.Repeat("x", 20001)},
	} {
		var in *domain.InvalidInputError
		if err := domain.ValidateItemFields(&it, "items.2."); !errors.As(err, &in) || !strings.HasPrefix(in.Field, "items.2.") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := domain.ValidateItemFields(&domain.Item{Title: "x", Type: domain.ItemSermon, Text: strings.Repeat("ä", 20000)}, ""); err != nil {
		t.Errorf("20000 characters: %v", err)
	}
}

func TestSectionDisplayAndStates(t *testing.T) {
	for _, c := range []struct {
		sec  domain.Section
		want string
	}{
		{domain.Section{Kind: domain.SectionVerse, Number: 2}, "Verse 2"}, {domain.Section{Kind: domain.SectionChorus}, "Chorus"},
		{domain.Section{Kind: domain.SectionPreChorus}, "Pre-chorus"}, {domain.Section{Kind: domain.SectionBridge, Label: "Jembatan"}, "Jembatan"},
		{domain.Section{Kind: domain.SectionOther}, "Section"},
	} {
		if got := domain.SectionDisplay(c.sec); got != c.want {
			t.Errorf("%+v: %q", c.sec, got)
		}
	}
	for _, s := range domain.LiturgyStates {
		wantEdit := s == domain.StateDraft || s == domain.StateNeedsRevision
		if s.Editable() != wantEdit || s.Deletable() != (s != domain.StatePublished) {
			t.Errorf("%s: editable %v deletable %v", s, s.Editable(), s.Deletable())
		}
	}
	if domain.LiturgyState("archived").Valid() || domain.LiturgyState("archived").Deletable() {
		t.Error("an unknown state")
	}
}

// TC-L-009: the five problems, and none for a complete item.
func TestProblemCodes(t *testing.T) {
	items := []domain.Item{
		{ID: "A", Type: domain.ItemSong}, // song_missing
		{ID: "B", Type: domain.ItemSong, Songs: []domain.LiturgySong{ // complete
			{ID: "S1", SongID: "x", Entries: []domain.Entry{{ID: "E1", SectionID: "v"}}}}},
		{ID: "C", Type: domain.ItemSong, Songs: []domain.LiturgySong{ // song_removed, section_removed
			{ID: "S2", Entries: []domain.Entry{{ID: "E2"}}}}},
		{ID: "D", Type: domain.ItemReading},                                    // reading_missing
		{ID: "E", Type: domain.ItemReading, ReadingID: "r", ReadingLabel: "x"}, // complete
		{ID: "F", Type: domain.ItemReading, ReadingLabel: "Yohanes 3:16 (TB)"}, // reading_removed
		{ID: "G", Type: domain.ItemPrayer},                                     // none
	}
	want := []domain.Problem{
		{Code: domain.ProblemSongMissing, ItemID: "A"},
		{Code: domain.ProblemSongRemoved, ItemID: "C", ItemSongID: "S2"},
		{Code: domain.ProblemSectionRemoved, ItemID: "C", ItemSongID: "S2", EntryID: "E2"},
		{Code: domain.ProblemReadingMissing, ItemID: "D"},
		{Code: domain.ProblemReadingRemoved, ItemID: "F"},
	}
	if got := domain.Problems(items); !slices.Equal(got, want) {
		t.Errorf("problems: %+v", got)
	}
	if got := domain.Problems(nil); got == nil || len(got) != 0 {
		t.Errorf("no items: %#v", got)
	}
}

// TC-L-003: weeks run Monday to Sunday.
func TestWeeks(t *testing.T) {
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	for _, c := range []struct {
		date, monday string
		weekday      int
	}{
		{"2026-10-12", "2026-10-12", 1}, {"2026-10-14", "2026-10-12", 3}, {"2026-10-18", "2026-10-12", 7},
		{"2026-10-19", "2026-10-19", 1}, {"2026-01-01", "2025-12-29", 4}, {"2024-03-03", "2024-02-26", 7},
	} {
		if got := domain.WeekStart(day(c.date)).Format("2006-01-02"); got != c.monday {
			t.Errorf("WeekStart(%s) = %s", c.date, got)
		}
		if got := domain.ISOWeekday(day(c.date)); got != c.weekday {
			t.Errorf("ISOWeekday(%s) = %d", c.date, got)
		}
	}
}

func TestReadingLabel(t *testing.T) {
	r := domain.Reading{Reference: "JHN 3:16-21", ReferenceDisplay: "Yoh 3:16-21", Translation: domain.Translation{Code: "TB"}}
	if got := domain.ReadingLabel(r); got != "Yohanes 3:16-21 (TB)" {
		t.Errorf("label %q", got)
	}
	r.Translation.Code = ""
	if got := domain.ReadingLabel(r); got != "Yohanes 3:16-21" {
		t.Errorf("label without a translation %q", got)
	}
}
