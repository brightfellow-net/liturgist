// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"reflect"
	"testing"
)

func pitem(id, title string, duty string, songs ...PublishedSong) PublishedItem {
	it := PublishedItem{ID: ItemID(id), Title: title, Songs: songs}
	if duty != "" {
		it.Duty = &PublishedRef{ID: duty, Name: "Duty " + duty}
	}
	return it
}

func psong(id, key string, changes ...string) PublishedSong {
	s := PublishedSong{SongID: SongID(id), Title: "Song " + id, Key: key}
	for _, c := range changes {
		s.Entries = append(s.Entries, PublishedEntry{KeyChange: c})
	}
	return s
}

func kinds(ch []ItemChange) []string {
	var out []string
	for _, c := range ch {
		out = append(out, c.Kind+":"+string(c.ItemID))
	}
	return out
}

// TC-P-003
func TestCompare(t *testing.T) {
	base := PublishedContent{Items: []PublishedItem{pitem("a", "Opening", "d1"), pitem("b", "Song time", "d2", psong("s1", "G")), pitem("c", "Prayer", "d1")}}

	t.Run("identical versions change nothing", func(t *testing.T) {
		if ch := Compare(base, base); !ch.Empty() {
			t.Errorf("got %+v", ch)
		}
	})
	t.Run("an item inserted at the top is one addition and no move", func(t *testing.T) {
		now := PublishedContent{Items: append([]PublishedItem{pitem("z", "Welcome", "")}, base.Items...)}
		got := kinds(Compare(base, now).Items)
		if !reflect.DeepEqual(got, []string{"added:z"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a removed item", func(t *testing.T) {
		now := PublishedContent{Items: []PublishedItem{base.Items[0], base.Items[2]}}
		if got := kinds(Compare(base, now).Items); !reflect.DeepEqual(got, []string{"removed:b"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a swap is one move", func(t *testing.T) {
		now := PublishedContent{Items: []PublishedItem{base.Items[0], base.Items[2], base.Items[1]}}
		got := kinds(Compare(base, now).Items)
		if len(got) != 1 || got[0][:6] != "moved:" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("an item moved far is one move", func(t *testing.T) {
		now := PublishedContent{Items: []PublishedItem{base.Items[2], base.Items[0], base.Items[1]}}
		if got := kinds(Compare(base, now).Items); !reflect.DeepEqual(got, []string{"moved:c"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a new title and a new duty", func(t *testing.T) {
		now := PublishedContent{Items: []PublishedItem{pitem("a", "Welcome", "d1"), pitem("b", "Song time", "d3", psong("s1", "G")), base.Items[2]}}
		ch := Compare(base, now).Items
		if got := kinds(ch); !reflect.DeepEqual(got, []string{"retitled:a", "duty_changed:b"}) {
			t.Fatalf("got %v", got)
		}
		if ch[0].OldTitle != "Opening" || ch[1].Duty.ID != "d3" || ch[1].OldDuty.ID != "d2" {
			t.Errorf("details: %+v", ch)
		}
	})
	t.Run("songs added, removed and in another key, also by an entry's key change", func(t *testing.T) {
		before := PublishedContent{Items: []PublishedItem{pitem("b", "Songs", "", psong("s1", "G"), psong("s2", "C"), psong("s3", "D", "E"))}}
		now := PublishedContent{Items: []PublishedItem{pitem("b", "Songs", "", psong("s1", "A"), psong("s3", "D", "F"), psong("s4", "E"))}}
		var got []string
		for _, c := range Compare(before, now).Songs {
			got = append(got, c.Kind+":"+string(c.Song.SongID)+":"+c.OldKey+">"+c.NewKey)
		}
		want := []string{"key_changed:s1:G>A", "key_changed:s3:D>D", "added:s4:>E", "removed:s2:C>"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
	t.Run("the same song twice in an item is matched by occurrence", func(t *testing.T) {
		before := PublishedContent{Items: []PublishedItem{pitem("b", "Songs", "", psong("s1", "G"), psong("s1", "G"))}}
		now := PublishedContent{Items: []PublishedItem{pitem("b", "Songs", "", psong("s1", "G"))}}
		if ch := Compare(before, now).Songs; len(ch) != 1 || ch[0].Kind != SongRemoved {
			t.Errorf("got %+v", ch)
		}
	})
	t.Run("a reading with another reference, added, or removed; its words do not count", func(t *testing.T) {
		r := func(ref, text string) *PublishedReading {
			return &PublishedReading{ReferenceDisplay: ref, TranslationCode: "TB", Text: text}
		}
		before := PublishedContent{Items: []PublishedItem{{ID: "r1", Title: "R1", Reading: r("Yoh 3:16", "a")}, {ID: "r2", Title: "R2", Reading: r("Mat 1:1", "x")}, {ID: "r3", Title: "R3"}, {ID: "r4", Title: "R4", Reading: r("Rom 1:1", "x")}}}
		now := PublishedContent{Items: []PublishedItem{{ID: "r1", Title: "R1", Reading: r("Yoh 3:17", "a")}, {ID: "r2", Title: "R2", Reading: r("Mat 1:1", "changed words")}, {ID: "r3", Title: "R3", Reading: r("Luk 1:1", "")}, {ID: "r4", Title: "R4"}}}
		var got []string
		for _, c := range Compare(before, now).Reading {
			got = append(got, string(c.ItemID)+":"+c.Old+">"+c.New)
		}
		want := []string{"r1:Yoh 3:16 (TB)>Yoh 3:17 (TB)", "r3:>Luk 1:1 (TB)", "r4:Rom 1:1 (TB)>"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
	t.Run("people added and removed, members by ID, free text by trimmed case-folded spelling", func(t *testing.T) {
		d := PublishedRef{ID: "d1", Name: "Liturgis"}
		before := PublishedContent{Assignments: []PublishedAssignment{
			{Duty: d, UserID: "u1", Name: "Ruth"}, {Duty: d, Name: "Pak Budi"}, {Duty: d, UserID: "u3", Name: "Old"}}}
		now := PublishedContent{Assignments: []PublishedAssignment{
			{Duty: d, UserID: "u1", Name: "Ruth Renamed"}, {Duty: d, Name: "  pak budi "}, {Duty: d, UserID: "u2", Name: "Dewi"},
			{Duty: PublishedRef{ID: "d2", Name: "Kolektan"}, UserID: "u1", Name: "Ruth"}}}
		var got []string
		for _, c := range Compare(before, now).Assignments {
			got = append(got, c.Kind+":"+c.Duty.ID+":"+c.Assignment.Name)
		}
		want := []string{"added:d1:Dewi", "added:d2:Ruth", "removed:d1:Old"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
	t.Run("lyrics and free text edits are not reported", func(t *testing.T) {
		before := base
		now := PublishedContent{Items: []PublishedItem{base.Items[0], base.Items[1], base.Items[2]}}
		now.Items[0].Text = "new words"
		now.Items[1].Songs = []PublishedSong{psong("s1", "G")}
		now.Items[1].Songs[0].Sections = []PublishedSection{{ID: "x", Text: "new lyrics"}}
		if ch := Compare(before, now); !ch.Empty() {
			t.Errorf("got %+v", ch)
		}
	})
}
