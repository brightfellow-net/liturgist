// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func conflict(err error) (scope string, item domain.ItemID) {
	var c *app.VersionConflictError
	if errors.As(err, &c) && errors.Is(err, app.ErrVersionConflict) {
		return c.Scope, c.ItemID
	}
	return "", ""
}

func itemIDs(v app.LiturgyView) []domain.ItemID {
	out := make([]domain.ItemID, len(v.Items))
	for i, it := range v.Items {
		out[i] = it.Item.ID
	}
	return out
}

// TC-L-006, IT-L-002: the two version counters.
func TestItemsAndVersions(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		lid := e.draft()
		L := e.liturgies

		add := func(version int, title string) (app.ItemResult, error) {
			return L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: version, Title: title, Type: domain.ItemPrayer})
		}
		a, err := add(1, "A")
		if err != nil || a.LiturgyVersion != 2 || a.Item.Item.Version != 1 || a.Item.Item.Position != 0 {
			t.Fatalf("add A: %+v %v", a, err)
		}
		b, _ := add(2, "B")
		c, _ := add(3, "C")
		if v := e.liturgy(lid); v.Liturgy.Version != 4 || !slices.Equal(itemIDs(v), []domain.ItemID{a.Item.Item.ID, b.Item.Item.ID, c.Item.Item.ID}) {
			t.Fatalf("after three adds: %d %v", v.Liturgy.Version, itemIDs(v))
		}
		if _, err := add(3, "stale"); func() bool { s, _ := conflict(err); return s != app.ScopeLiturgy }() {
			t.Errorf("stale add: %v", err)
		}
		if v := e.liturgy(lid); len(v.Items) != 3 || v.Liturgy.Version != 4 {
			t.Errorf("a refused add changed the liturgy: %d items, version %d", len(v.Items), v.Liturgy.Version)
		}

		// Editing items bumps only the item; different items never conflict.
		title := "A2"
		ra, err := L.UpdateItem(e.ctx, e.admin, lid, a.Item.Item.ID, app.ItemChange{Version: 1, Title: &title})
		if err != nil || ra.Item.Item.Version != 2 || ra.LiturgyVersion != 4 || ra.Item.Item.Title != "A2" {
			t.Fatalf("update A: %+v %v", ra, err)
		}
		title = "B2"
		if rb, err := L.UpdateItem(e.ctx, e.admin, lid, b.Item.Item.ID, app.ItemChange{Version: 1, Title: &title}); err != nil || rb.Item.Item.Version != 2 {
			t.Fatalf("update B from the same liturgy version: %+v %v", rb, err)
		}
		title = "A3"
		_, err = L.UpdateItem(e.ctx, e.admin, lid, a.Item.Item.ID, app.ItemChange{Version: 1, Title: &title})
		if s, id := conflict(err); s != app.ScopeItem || id != a.Item.Item.ID {
			t.Errorf("stale item update: %v", err)
		}
		if got := e.liturgy(lid).Items[0].Item; got.Title != "A2" || got.Version != 2 {
			t.Errorf("a refused update changed the item: %+v", got)
		}
		if _, err := L.UpdateItem(e.ctx, e.admin, lid, "01ARZ3NDEKTSV4RRFFQ69G5FAV", app.ItemChange{Version: 1, Title: &title}); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("unknown item: %v", err)
		}

		// Item edits do not make a reorder stale; structure changes do.
		order := []domain.ItemID{c.Item.Item.ID, a.Item.Item.ID, b.Item.Item.ID}
		r, err := L.ReorderItems(e.ctx, e.admin, lid, 4, order)
		if err != nil || r.LiturgyVersion != 5 || !slices.Equal(r.ItemIDs, order) {
			t.Fatalf("reorder: %+v %v", r, err)
		}
		if got := e.liturgy(lid); !slices.Equal(itemIDs(got), order) || got.Items[0].Item.Position != 0 || got.Items[2].Item.Position != 2 {
			t.Errorf("order after reorder: %v", itemIDs(got))
		}
		if _, err := L.ReorderItems(e.ctx, e.admin, lid, 4, order); func() bool { s, _ := conflict(err); return s != app.ScopeLiturgy }() {
			t.Errorf("stale reorder: %v", err)
		}
		for name, ids := range map[string][]domain.ItemID{"missing": order[:2], "extra": append(slices.Clone(order), "01ARZ3NDEKTSV4RRFFQ69G5FAV"),
			"duplicate": {order[0], order[0], order[1]}} {
			if _, err := L.ReorderItems(e.ctx, e.admin, lid, 5, ids); func() bool { s, _ := conflict(err); return s != app.ScopeLiturgy }() {
				t.Errorf("reorder with %s ids: %v", name, err)
			}
		}
		if v := e.liturgy(lid); v.Liturgy.Version != 5 {
			t.Errorf("refused reorders bumped the version: %d", v.Liturgy.Version)
		}

		// Removing closes the gap, bumps the liturgy, and checks the version.
		if _, err := L.RemoveItem(e.ctx, e.admin, lid, a.Item.Item.ID, 4); func() bool { s, _ := conflict(err); return s != app.ScopeLiturgy }() {
			t.Errorf("stale remove: %v", err)
		}
		rm, err := L.RemoveItem(e.ctx, e.admin, lid, a.Item.Item.ID, 5)
		if err != nil || rm.LiturgyVersion != 6 || !slices.Equal(rm.ItemIDs, []domain.ItemID{c.Item.Item.ID, b.Item.Item.ID}) {
			t.Fatalf("remove: %+v %v", rm, err)
		}
		if got := e.liturgy(lid); got.Items[1].Item.Position != 1 {
			t.Errorf("positions after remove: %+v", got.Items[1].Item)
		}
		if _, err := L.RemoveItem(e.ctx, e.admin, lid, a.Item.Item.ID, 6); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("remove twice: %v", err)
		}

		// The liturgy's own fields are structural.
		date, name := "2026-10-18", "Ibadah Pagi"
		u, err := L.Update(e.ctx, e.admin, lid, app.LiturgyChange{Version: 6, Date: &date, ServiceName: &name})
		if err != nil || u.Liturgy.Version != 7 || u.Liturgy.Date != date || u.Liturgy.ServiceName != name {
			t.Fatalf("update liturgy: %+v %v", u.Liturgy, err)
		}
		if _, err := L.Update(e.ctx, e.admin, lid, app.LiturgyChange{Version: 6, Date: &date}); func() bool { s, _ := conflict(err); return s != app.ScopeLiturgy }() {
			t.Errorf("stale liturgy update: %v", err)
		}
		bad := "2026-02-30"
		if _, err := L.Update(e.ctx, e.admin, lid, app.LiturgyChange{Version: 7, Date: &bad}); invalidField(err) != "date" {
			t.Errorf("bad date: %v", err)
		}
	})
}

// TC-L-007: what items may hold.
func TestItemRules(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	lid := e.draft()
	L := e.liturgies
	ver := func() int { return e.liturgy(lid).Liturgy.Version }
	reading, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Yoh 3:16-21", Translation: "TB", Text: "Karena begitu besar"})
	if err != nil {
		t.Fatal(err)
	}

	bad := map[string]app.ItemInput{
		"empty title":      {Title: " ", Type: domain.ItemPrayer},
		"long title":       {Title: strings.Repeat("x", 201), Type: domain.ItemPrayer},
		"unknown type":     {Title: "X", Type: "video"},
		"text on a song":   {Title: "X", Type: domain.ItemSong, Text: "la la"},
		"text on reading":  {Title: "X", Type: domain.ItemReading, Text: "x"},
		"unknown duty":     {Title: "X", Type: domain.ItemPrayer, DutyID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		"long text":        {Title: "X", Type: domain.ItemPrayer, Text: strings.Repeat("x", 20001)},
		"position too big": {Title: "X", Type: domain.ItemPrayer, Position: ptr(1)},
		"position below 0": {Title: "X", Type: domain.ItemPrayer, Position: ptr(-1)},
	}
	for name, in := range bad {
		in.LiturgyVersion = ver()
		if _, err := L.AddItem(e.ctx, e.admin, lid, in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if v := e.liturgy(lid); len(v.Items) != 0 || v.Liturgy.Version != 1 {
		t.Fatalf("refused adds changed the liturgy: %d items, version %d", len(v.Items), v.Liturgy.Version)
	}
	// Text takes CRLF; positions 0 and n insert.
	x, err := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: ver(), Title: "X", Type: domain.ItemFreeText, Text: "a\r\nb"})
	if err != nil || x.Item.Item.Text != "a\nb" {
		t.Fatalf("text: %+v %v", x.Item.Item, err)
	}
	y, _ := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: ver(), Title: "Y", Type: domain.ItemPrayer, Position: ptr(0)})
	z, _ := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: ver(), Title: "Z", Type: domain.ItemPrayer, Position: ptr(2)})
	if got := itemIDs(e.liturgy(lid)); !slices.Equal(got, []domain.ItemID{y.Item.Item.ID, x.Item.Item.ID, z.Item.Item.ID}) {
		t.Errorf("insert positions: %v", got)
	}

	// Changes of one item: reading only on a reading item, text only on a text item.
	rd := e.addItem(lid, domain.ItemReading, "Bacaan")
	sg := e.addItem(lid, domain.ItemSong, "Lagu")
	ch := func(item domain.ItemID, v int, c app.ItemChange) error {
		c.Version = v
		_, err := L.UpdateItem(e.ctx, e.admin, lid, item, c)
		return err
	}
	id := reading.Reading.ID
	unknown := domain.ReadingID("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	txt := "text"
	if invalidField(ch(y.Item.Item.ID, 1, app.ItemChange{ReadingID: &id})) != "reading_id" ||
		invalidField(ch(rd, 1, app.ItemChange{ReadingID: &unknown})) != "reading_id" ||
		invalidField(ch(sg, 1, app.ItemChange{Text: &txt})) != "text" {
		t.Error("reading and text rules")
	}
	res, err := L.UpdateItem(e.ctx, e.admin, lid, rd, app.ItemChange{Version: 1, ReadingID: &id})
	if err != nil || res.Item.Item.ReadingID != id || res.Item.Item.ReadingLabel != "Yohanes 3:16-21 (TB)" || res.Item.Reading == nil || res.Item.Reading.Reading.Text != "Karena begitu besar" {
		t.Fatalf("set reading: %+v %v", res.Item, err)
	}
	none := domain.ReadingID("")
	if res, err = L.UpdateItem(e.ctx, e.admin, lid, rd, app.ItemChange{Version: 2, ReadingID: &none}); err != nil || res.Item.Item.ReadingID != "" || res.Item.Item.ReadingLabel != "" {
		t.Errorf("clear reading: %+v %v", res.Item.Item, err)
	}
	// A duty can be set and cleared.
	duties, _ := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
	duty := domain.DutyID(duties[0].Entry.ID)
	if res, err = L.UpdateItem(e.ctx, e.admin, lid, z.Item.Item.ID, app.ItemChange{Version: 1, DutyID: &duty}); err != nil || res.Item.Item.DutyID != duty {
		t.Errorf("set duty: %+v %v", res.Item.Item, err)
	}
	noDuty := domain.DutyID("")
	if res, err = L.UpdateItem(e.ctx, e.admin, lid, z.Item.Item.ID, app.ItemChange{Version: 2, DutyID: &noDuty}); err != nil || res.Item.Item.DutyID != "" {
		t.Errorf("clear duty: %+v %v", res.Item.Item, err)
	}

	// 60 items at most.
	for len(e.liturgy(lid).Items) < domain.MaxLiturgyItems {
		e.addItem(lid, domain.ItemOther, "x")
	}
	_, err = L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: ver(), Title: "61", Type: domain.ItemOther})
	if r, max, used := limitReason(err); r != domain.ReasonLimit || max != 60 || used != 60 {
		t.Errorf("61st item: %v", err)
	}
}

func entryIn(sec domain.SectionID) app.EntryInput { return app.EntryInput{SectionID: sec} }

func labels(s domain.LiturgySong) []string {
	out := make([]string, len(s.Entries))
	for i, e := range s.Entries {
		out[i] = e.SectionLabel
	}
	return out
}

// TC-L-004, TC-L-005, IT-L-002: songs in a song item and their sequences.
func TestItemSongs(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		lid := e.draft()
		L := e.liturgies
		withArr, err := e.songs.Create(e.ctx, e.admin, app.SongInput{Language: "id", Title: "Besar Setia-Mu", DefaultKey: "G",
			Sections:    []app.SectionInput{verse("v1", 1, "bait satu"), chorus("c", "reff"), verse("v2", 2, "bait dua")},
			Arrangement: []string{"v1", "c", "v2", "c"}})
		if err != nil {
			t.Fatal(err)
		}
		noArr, err := e.songs.Create(e.ctx, e.admin, app.SongInput{Language: "id", Title: "Tanpa urutan",
			Sections: []app.SectionInput{verse("v1", 1, "satu"), chorus("c", "reff"), verse("v2", 2, "dua")}})
		if err != nil {
			t.Fatal(err)
		}
		item := e.addItem(lid, domain.ItemSong, "Lagu")

		// P-62: the default arrangement, else every section in order; the key from the song.
		r, err := L.AddSong(e.ctx, e.admin, lid, item, 1, withArr.Song.ID, nil)
		if err != nil || r.Item.Item.Version != 2 || len(r.Item.Songs) != 1 {
			t.Fatalf("add song: %+v %v", r.Item, err)
		}
		s1 := r.Item.Songs[0]
		if !slices.Equal(labels(s1.Song), []string{"Verse 1", "Chorus", "Verse 2", "Chorus"}) || s1.Song.Key != "G" || s1.Song.SongTitle != "Besar Setia-Mu" {
			t.Errorf("sequence from the arrangement: %v key %q", labels(s1.Song), s1.Song.Key)
		}
		if s1.Summary == nil || s1.Summary.Title != "Besar Setia-Mu" || len(s1.Summary.Sections) != 3 {
			t.Errorf("summary: %+v", s1.Summary)
		}
		r, err = L.AddSong(e.ctx, e.admin, lid, item, 2, noArr.Song.ID, ptr(0))
		if err != nil || len(r.Item.Songs) != 2 || r.Item.Songs[0].Song.SongID != noArr.Song.ID || r.Item.Songs[0].Song.Position != 0 || r.Item.Songs[1].Song.Position != 1 {
			t.Fatalf("add at 0: %+v %v", r.Item.Songs, err)
		}
		if !slices.Equal(labels(r.Item.Songs[0].Song), []string{"Verse 1", "Chorus", "Verse 2"}) || r.Item.Songs[0].Song.Key != "" {
			t.Errorf("sequence without an arrangement: %v", labels(r.Item.Songs[0].Song))
		}
		// Rules of adding.
		if _, err := L.AddSong(e.ctx, e.admin, lid, item, 2, noArr.Song.ID, nil); func() bool { s, id := conflict(err); return s != app.ScopeItem || id != item }() {
			t.Errorf("stale add song: %v", err)
		}
		if _, err := L.AddSong(e.ctx, e.admin, lid, item, 3, "01ARZ3NDEKTSV4RRFFQ69G5FAV", nil); invalidField(err) != "song_id" {
			t.Errorf("unknown song: %v", err)
		}
		if _, err := L.AddSong(e.ctx, e.admin, lid, item, 3, noArr.Song.ID, ptr(3)); invalidField(err) != "position" {
			t.Errorf("position: %v", err)
		}
		prayer := e.addItem(lid, domain.ItemPrayer, "Doa")
		if _, err := L.AddSong(e.ctx, e.admin, lid, prayer, 1, noArr.Song.ID, nil); invalidField(err) != "item" {
			t.Errorf("a song in a prayer: %v", err)
		}

		// PUT: the complete list, every ID new, each rule checked.
		secs := withArr.Song.Sections
		put := func(version int, songs ...app.ItemSongInput) (app.ItemResult, error) {
			return L.SetSongs(e.ctx, e.admin, lid, item, version, songs)
		}
		old := e.liturgy(lid).Items[0].Item
		ok := app.ItemSongInput{SongID: withArr.Song.ID, Key: "Bb", Note: "pelan", Entries: []app.EntryInput{
			entryIn(secs[0].ID), entryIn(secs[1].ID), entryIn(secs[1].ID), {SectionID: secs[2].ID, KeyChange: "A", Note: "2x"}}}
		r, err = put(3, ok)
		if err != nil || len(r.Item.Songs) != 1 || r.Item.Item.Version != 4 || !slices.Equal(labels(r.Item.Songs[0].Song), []string{"Verse 1", "Chorus", "Chorus", "Verse 2"}) {
			t.Fatalf("put: %+v %v", r.Item, err)
		}
		got := r.Item.Songs[0].Song
		if got.Key != "Bb" || got.Note != "pelan" || got.Entries[3].KeyChange != "A" || got.Entries[3].Note != "2x" {
			t.Errorf("fields: %+v", got)
		}
		for _, o := range old.Songs {
			if o.ID == got.ID {
				t.Error("an item song kept its ID")
			}
			for _, oe := range o.Entries {
				for _, ne := range got.Entries {
					if oe.ID == ne.ID {
						t.Error("an entry kept its ID")
					}
				}
			}
		}
		other := noArr.Song.Sections[0].ID
		for name, in := range map[string]app.ItemSongInput{
			"section of another song": {SongID: withArr.Song.ID, Entries: []app.EntryInput{entryIn(other)}},
			"unknown song":            {SongID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			"unknown part":            {SongID: withArr.Song.ID, Entries: []app.EntryInput{{SectionID: secs[0].ID, SingingPartID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}}},
			"key H":                   {SongID: withArr.Song.ID, Key: "H"},
			"key change H":            {SongID: withArr.Song.ID, Entries: []app.EntryInput{{SectionID: secs[0].ID, KeyChange: "H"}}},
			"long note":               {SongID: withArr.Song.ID, Note: strings.Repeat("x", 201)},
			"long entry note":         {SongID: withArr.Song.ID, Entries: []app.EntryInput{{SectionID: secs[0].ID, Note: strings.Repeat("x", 101)}}},
		} {
			if _, err := put(4, in); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
		if _, err := put(3, ok); func() bool { s, id := conflict(err); return s != app.ScopeItem || id != item }() {
			t.Errorf("stale put: %v", err)
		}
		if v := e.liturgy(lid).Items[0].Item; v.Version != 4 || len(v.Songs) != 1 || v.Songs[0].ID != got.ID {
			t.Errorf("refused puts changed the item: %+v", v)
		}
		if _, err := L.SetSongs(e.ctx, e.admin, lid, prayer, 1, []app.ItemSongInput{ok}); invalidField(err) != "item" {
			t.Errorf("put on a prayer: %v", err)
		}
		// Limits: 100 and 101 entries, 10 and 11 songs.
		many := func(n int) app.ItemSongInput {
			in := app.ItemSongInput{SongID: withArr.Song.ID}
			for range n {
				in.Entries = append(in.Entries, entryIn(secs[1].ID))
			}
			return in
		}
		if r, err = put(4, many(100)); err != nil || len(r.Item.Songs[0].Song.Entries) != 100 {
			t.Fatalf("100 entries: %v", err)
		}
		if _, err := put(5, many(101)); func() bool { r, max, _ := limitReason(err); return r != domain.ReasonLimit || max != 100 }() {
			t.Errorf("101 entries: %v", err)
		}
		ten := make([]app.ItemSongInput, 11)
		for i := range ten {
			ten[i] = many(1)
		}
		if _, err = put(5, ten[:10]...); err != nil {
			t.Errorf("10 songs: %v", err)
		}
		if _, err = put(6, ten...); func() bool { r, max, _ := limitReason(err); return r != domain.ReasonLimit || max != 10 }() {
			t.Errorf("11 songs: %v", err)
		}
		if r, err = put(6); err != nil || len(r.Item.Songs) != 0 {
			t.Errorf("an empty list: %+v %v", r.Item.Songs, err)
		}
	})
}

// IT-L-001, TC-L-008: assignments.
func TestAssignments(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		lid := e.draft()
		L := e.liturgies
		duties, _ := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
		d1, d2 := domain.DutyID(duties[0].Entry.ID), domain.DutyID(duties[1].Entry.ID)
		member, mid := e.member("budi@example.org", e.role(domain.OriginEditor).ID)

		a, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d1, UserID: member.UserID})
		if err != nil || a.UserName != "budi" || a.FormerMember {
			t.Fatalf("assign a member: %+v %v", a, err)
		}
		if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d2, UserID: member.UserID}); err != nil {
			t.Errorf("a person with two duties: %v", err)
		}
		n, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d1, Name: "  Pak Yan "})
		if err != nil || n.Assignment.Name != "Pak Yan" || n.UserName != "Pak Yan" {
			t.Fatalf("assign a name: %+v %v", n, err)
		}
		if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d1, UserID: e.admin.UserID}); err != nil {
			t.Errorf("several people on a duty: %v", err)
		}
		for name, in := range map[string]app.AssignmentInput{
			"same member":       {DutyID: d1, UserID: member.UserID},
			"same name":         {DutyID: d1, Name: "pak yan"},
			"same name, folded": {DutyID: d1, Name: "PAK  YAN"},
		} {
			if _, err := L.AddAssignment(e.ctx, e.admin, lid, in); !errors.Is(err, app.ErrAssignmentExists) {
				t.Errorf("%s: %v", name, err)
			}
		}
		stranger := domain.UserID("01ARZ3NDEKTSV4RRFFQ69G5FAV")
		for name, in := range map[string]app.AssignmentInput{
			"a non-member": {DutyID: d1, UserID: stranger},
			"both":         {DutyID: d1, UserID: member.UserID, Name: "X"},
			"neither":      {DutyID: d1},
			"unknown duty": {DutyID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Name: "X"},
			"no duty":      {Name: "X"},
			"empty name":   {DutyID: d1, Name: "  "},
			"long name":    {DutyID: d1, Name: strings.Repeat("x", 101)},
		} {
			if _, err := L.AddAssignment(e.ctx, e.admin, lid, in); err == nil || errors.Is(err, app.ErrAssignmentExists) {
				t.Errorf("%s: %v", name, err)
			}
		}

		// A removed member's assignments stay and are flagged on every read.
		e.write(func(s app.Store) error {
			cs, err := s.ForChurch(ctx, e.church)
			if err != nil {
				return err
			}
			return cs.Memberships().Delete(ctx, mid)
		})
		var flagged, total int
		for _, av := range e.liturgy(lid).Assignments {
			total++
			if av.FormerMember {
				flagged++
				if av.UserName != "budi" {
					t.Errorf("the former member's name: %+v", av)
				}
			}
		}
		if total != 4 || flagged != 2 {
			t.Errorf("%d assignments, %d flagged", total, flagged)
		}
		if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d1, UserID: member.UserID}); invalidField(err) != "user_id" {
			t.Errorf("assigning a removed member: %v", err)
		}

		// Removing one.
		if err := L.RemoveAssignment(e.ctx, e.admin, lid, n.Assignment.ID); err != nil {
			t.Fatal(err)
		}
		if err := L.RemoveAssignment(e.ctx, e.admin, lid, n.Assignment.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("remove twice: %v", err)
		}
		if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d1, Name: "Pak Yan"}); err != nil {
			t.Errorf("assign again after removal: %v", err)
		}
		// 200 at most.
		for i := len(e.liturgy(lid).Assignments); i < domain.MaxAssignments; i++ {
			if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d2, Name: fmt.Sprintf("Person %d", i)}); err != nil {
				t.Fatalf("fill %d: %v", i, err)
			}
		}
		_, err = L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: d2, Name: "One more"})
		if r, max, _ := limitReason(err); r != domain.ReasonLimit || max != 200 {
			t.Errorf("201st assignment: %v", err)
		}
		// A duty in use cannot be deleted; a liturgy with assignments can be.
		if err := e.vocab.Delete(e.ctx, e.admin, domain.KindDuty, string(d1)); !errors.Is(err, app.ErrDutyInUse) {
			t.Errorf("delete a duty with assignments: %v", err)
		}
		if err := L.Delete(e.ctx, e.admin, lid); err != nil {
			t.Fatal(err)
		}
		if err := e.vocab.Delete(e.ctx, e.admin, domain.KindDuty, string(d1)); err != nil {
			t.Errorf("delete the duty after the liturgy: %v", err)
		}
	})
}

// TC-L-009: problems the editor shows. (The removed-reference codes need a
// published liturgy; they are tested in the sqlstore package.)
func TestProblems(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	lid := e.draft()
	song := e.newSong("Besar Setia-Mu", "id")
	reading, _ := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Mzm 23", Text: "TUHAN adalah gembalaku"})
	empty := e.addItem(lid, domain.ItemSong, "Lagu kosong")
	unset := e.addItem(lid, domain.ItemReading, "Bacaan kosong")
	e.useSong(lid, song.Song.ID)
	e.useReading(lid, reading.Reading.ID)
	e.addItem(lid, domain.ItemPrayer, "Doa")
	got := e.liturgy(lid).Problems
	want := []domain.Problem{{Code: domain.ProblemSongMissing, ItemID: empty}, {Code: domain.ProblemReadingMissing, ItemID: unset}}
	if !slices.Equal(got, want) {
		t.Errorf("problems: %+v", got)
	}
}
