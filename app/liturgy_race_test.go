// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// IT-L-003: concurrent editing of one liturgy (both dialects).
func TestConcurrentEditing(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		L := e.liturgies
		lid := e.draft()
		a := e.addItem(lid, domain.ItemPrayer, "A")
		b := e.addItem(lid, domain.ItemPrayer, "B")
		edit := func(item domain.ItemID, version int, title string) error {
			_, err := L.UpdateItem(e.ctx, e.admin, lid, item, app.ItemChange{Version: version, Title: &title})
			return err
		}

		// Different items never conflict.
		errs := race(2, func(i int) error { return edit([]domain.ItemID{a, b}[i], 1, fmt.Sprint("new ", i)) })
		if ok, others := succeeded(errs); ok != 2 {
			t.Errorf("different items: %v", others)
		}
		// The same item from one version: one wins, the other is told which item changed.
		errs = race(2, func(i int) error { return edit(a, 2, fmt.Sprint("fight ", i)) })
		ok, others := succeeded(errs)
		if s, id := conflict(others[0]); ok != 1 || s != app.ScopeItem || id != a {
			t.Fatalf("same item: %d won, %v", ok, others)
		}
		if got := e.liturgy(lid).Items[0].Item; got.Version != 3 || (got.Title != "fight 0" && got.Title != "fight 1") {
			t.Errorf("the item after the fight: %+v", got)
		}
		// A reorder and an add from one liturgy version: one wins, the other gets a conflict on the liturgy.
		v := e.liturgy(lid)
		errs = race(2, func(i int) error {
			if i == 0 {
				_, err := L.ReorderItems(e.ctx, e.admin, lid, v.Liturgy.Version, []domain.ItemID{b, a})
				return err
			}
			_, err := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: v.Liturgy.Version, Title: "C", Type: domain.ItemPrayer})
			return err
		})
		ok, others = succeeded(errs)
		if s, _ := conflict(others[0]); ok != 1 || s != app.ScopeLiturgy {
			t.Fatalf("reorder against add: %d won, %v", ok, others)
		}
		if after := e.liturgy(lid); after.Liturgy.Version != v.Liturgy.Version+1 {
			t.Errorf("version %d after one structural write from %d", after.Liturgy.Version, v.Liturgy.Version)
		}
		// 50 edits of distinct items all succeed.
		lid = e.draft()
		var items []domain.ItemID
		for i := range 50 {
			items = append(items, e.addItem(lid, domain.ItemPrayer, fmt.Sprint("item ", i)))
		}
		errs = race(50, func(i int) error {
			title := fmt.Sprint("edited ", i)
			_, err := L.UpdateItem(e.ctx, e.admin, lid, items[i], app.ItemChange{Version: 1, Title: &title})
			return err
		})
		if ok, others := succeeded(errs); ok != 50 {
			t.Errorf("50 parallel edits: %d ok, %v", ok, others)
		}
		// Each wrote one history row with its own seq, no gaps (the newest 100 are listed).
		h := e.history(lid)
		if len(h) != 100 || h[99].Edit.Seq != 101 {
			t.Fatalf("%d history rows, newest seq %d", len(h), h[len(h)-1].Edit.Seq)
		}
		for i, row := range h {
			if row.Edit.Seq != i+2 {
				t.Fatalf("row %d has seq %d", i, row.Edit.Seq)
			}
		}
	})
}

// IT-L-004, IT-L-012: one slot is created once; a second writer from the same
// version is refused before it changes anything.
func TestSlotAndWriteOrderRaces(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		L := e.liturgies
		tpl, _ := e.seededTemplate()
		svc := e.newService("Pagi", "id", tpl, sunday("07:00"))

		errs := race(3, func(int) error {
			_, err := L.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{{ServiceID: svc, Date: "2026-10-18", Time: "07:00"}})
			return err
		})
		ok, others := succeeded(errs)
		if ok != 1 || existsID(others[0]) == "" || existsID(others[1]) == "" {
			t.Errorf("concurrent prepares: %d created, %v", ok, others)
		}
		errs = race(3, func(int) error {
			_, err := L.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-25", Time: "07:00", ServiceID: svc})
			return err
		})
		if ok, others := succeeded(errs); ok != 1 || existsID(others[0]) == "" {
			t.Errorf("concurrent creates: %d created, %v", ok, others)
		}
		if page, _ := L.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50}); page.Total != 2 {
			t.Errorf("%d liturgies", page.Total)
		}

		// Two replacements of one item's songs from one version: the loser changes nothing.
		lid := e.draft()
		song := e.newSong("Lagu", "id")
		item := e.addItem(lid, domain.ItemSong, "Lagu")
		before := len(e.history(lid))
		secs := song.Song.Sections
		errs = race(2, func(i int) error {
			in := app.ItemSongInput{SongID: song.Song.ID, Note: fmt.Sprint("writer ", i)}
			for range 5 + i { // different lengths, so a mixture would show
				in.Entries = append(in.Entries, entryIn(secs[i].ID))
			}
			_, err := L.SetSongs(e.ctx, e.admin, lid, item, 1, []app.ItemSongInput{in})
			return err
		})
		ok, others = succeeded(errs)
		if s, id := conflict(others[0]); ok != 1 || s != app.ScopeItem || id != item {
			t.Fatalf("two puts: %d won, %v", ok, others)
		}
		got := e.liturgy(lid).Items[0]
		winner := 0
		if got.Item.Songs[0].Note == "writer 1" {
			winner = 1
		}
		if got.Item.Version != 2 || len(got.Item.Songs) != 1 || len(got.Item.Songs[0].Entries) != 5+winner ||
			!slices.ContainsFunc(got.Item.Songs[0].Entries, func(en domain.Entry) bool { return en.SectionID != secs[winner].ID }) == false {
			t.Errorf("the item after two puts: %+v", got.Item)
		}
		if h := e.history(lid); len(h) != before+1 || h[len(h)-1].Edit.Command != domain.CmdItemSongs {
			t.Errorf("history after two puts: %d rows (was %d)", len(h), before)
		}
	})
}

// IT-L-010: removing a section against adding it to a liturgy.
func TestSectionRemovalRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		var removed, kept int
		for round := range 6 {
			song := e.newSong(fmt.Sprint("Lagu ", round), "id")
			lid := e.draft()
			item := e.addItem(lid, domain.ItemSong, "Lagu")
			secs := song.Song.Sections
			chorusID := secs[1].ID
			without := []app.SectionInput{
				{ID: secs[0].ID, Kind: domain.SectionVerse, Number: 1, Text: "bait satu"},
				{ID: secs[2].ID, Kind: domain.SectionVerse, Number: 2, Text: "bait dua"},
			}
			errs := race(2, func(i int) error {
				if i == 0 {
					_, err := e.songs.Update(e.ctx, e.admin, song.Song.ID, app.SongChange{Version: 1, Sections: &without})
					return err
				}
				_, err := e.liturgies.SetSongs(e.ctx, e.admin, lid, item, 1,
					[]app.ItemSongInput{{SongID: song.Song.ID, Entries: []app.EntryInput{entryIn(secs[0].ID), entryIn(chorusID)}}})
				return err
			})
			ok, others := succeeded(errs)
			if ok != 1 {
				t.Fatalf("round %d: %d won, %v", round, ok, others)
			}
			got, _ := e.songs.Get(e.ctx, e.admin, song.Song.ID)
			entries := e.liturgy(lid).Items[0].Item.Songs
			switch errs[0] {
			case nil: // the section is gone, so the entry never existed
				removed++
				if invalidField(errs[1]) != "songs.0.entries.1.section_id" || len(got.Song.Sections) != 2 || len(entries) != 0 {
					t.Errorf("round %d, section removed: %v %d sections, %d songs", round, errs[1], len(got.Song.Sections), len(entries))
				}
			default: // the liturgy won, so the section stays
				kept++
				var busy *app.SectionInUseError
				if !errors.As(errs[0], &busy) || !slices.Equal(busy.IDs, []domain.SectionID{chorusID}) || len(got.Song.Sections) != 3 || len(entries) != 1 {
					t.Errorf("round %d, section kept: %v %d sections, %d songs", round, errs[0], len(got.Song.Sections), len(entries))
				}
			}
		}
		t.Logf("section removed %d times, kept %d times", removed, kept)
	})
}

// IT-L-011: assigning a member against removing the membership.
func TestAssignmentAndMemberRemovalRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		duties, _ := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
		duty := domain.DutyID(duties[0].Entry.ID)
		for round := range 6 {
			lid := e.draft()
			member, mid := e.member(fmt.Sprintf("m%d@example.org", round))
			errs := race(2, func(i int) error {
				if i == 0 {
					_, err := e.liturgies.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: duty, UserID: member.UserID})
					return err
				}
				return e.members.Remove(e.ctx, e.admin, mid)
			})
			if errs[1] != nil {
				t.Fatalf("round %d, remove: %v", round, errs[1])
			}
			list := e.liturgy(lid).Assignments
			switch {
			case errs[0] == nil: // made while the person was a member; now flagged
				if len(list) != 1 || !list[0].FormerMember {
					t.Errorf("round %d, assignment kept: %+v", round, list)
				}
			case invalidField(errs[0]) == "user_id": // refused
				if len(list) != 0 {
					t.Errorf("round %d, refused but stored: %+v", round, list)
				}
			default:
				t.Errorf("round %d: %v", round, errs[0])
			}
		}
	})
}

// Item edits take the item first and the liturgy's history counter last; structure
// writes take the liturgy first. Mixed at once they may deadlock in PostgreSQL;
// the transaction retry must absorb it, so every writer ends in a success or a
// conflict, never an internal error.
func TestMixedWritesEndInSuccessOrConflict(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		L := e.liturgies
		lid := e.draft()
		var items []domain.ItemID
		for i := range 20 {
			items = append(items, e.addItem(lid, domain.ItemPrayer, fmt.Sprint("item ", i)))
		}
		v := e.liturgy(lid).Liturgy.Version
		errs := race(20, func(i int) error {
			switch i % 4 {
			case 0, 1: // edit an item
				title := fmt.Sprint("edited ", i)
				_, err := L.UpdateItem(e.ctx, e.admin, lid, items[i], app.ItemChange{Version: 1, Title: &title})
				return err
			case 2: // remove another item
				_, err := L.RemoveItem(e.ctx, e.admin, lid, items[i], v)
				return err
			default: // add an item
				_, err := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: v, Title: "new", Type: domain.ItemPrayer})
				return err
			}
		})
		for i, err := range errs {
			if s, _ := conflict(err); err != nil && s == "" && !errors.Is(err, app.ErrNotFound) {
				t.Errorf("writer %d: %v", i, err)
			}
		}
		// The history has no gap: seq is 1..n.
		h, _ := L.Edits(e.ctx, e.admin, lid, 100)
		for i := 1; i < len(h); i++ {
			if h[i].Edit.Seq != h[i-1].Edit.Seq-1 {
				t.Fatalf("a gap in the history between seq %d and %d", h[i].Edit.Seq, h[i-1].Edit.Seq)
			}
		}
		if got := e.liturgy(lid).Liturgy.EditSeq; got != h[0].Edit.Seq {
			t.Errorf("edit_seq %d, newest row %d", got, h[0].Edit.Seq)
		}
	})
}
