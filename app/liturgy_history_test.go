// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// history returns the rows oldest first.
func (e cenv) history(lid domain.LiturgyID) []app.EditView {
	e.t.Helper()
	h, err := e.liturgies.Edits(e.ctx, e.admin, lid, 100)
	if err != nil {
		e.t.Fatal(err)
	}
	slices.Reverse(h)
	return h
}

func image(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if raw == nil {
		return nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("image %q: %v", raw, err)
	}
	return m
}

// TC-L-010, IT-L-012: every change writes one history row, in order.
func TestHistory(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		L := e.liturgies
		tpl, _ := e.seededTemplate()
		v, err := L.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", ServiceName: "Ibadah", TemplateID: &tpl})
		if err != nil {
			t.Fatal(err)
		}
		lid := v.Liturgy.ID
		song, _ := e.songs.Create(e.ctx, e.admin, app.SongInput{Language: "id", Title: "Lagu", Sections: []app.SectionInput{verse("v1", 1, "RAHASIA LIRIK")}})
		duties, _ := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
		duty := domain.DutyID(duties[0].Entry.ID)

		item := e.addItem(lid, domain.ItemSong, "Lagu baru") // item.add
		title := "Lagu 2"                                    // item.update
		if _, err := L.UpdateItem(e.ctx, e.admin, lid, item, app.ItemChange{Version: 1, Title: &title}); err != nil {
			t.Fatal(err)
		}
		if _, err := L.AddSong(e.ctx, e.admin, lid, item, 2, song.Song.ID, nil); err != nil { // item.songs
			t.Fatal(err)
		}
		cur := e.liturgy(lid)
		order := slices.Clone(itemIDs(cur))
		slices.Reverse(order)
		if _, err := L.ReorderItems(e.ctx, e.admin, lid, cur.Liturgy.Version, order); err != nil { // items.reorder
			t.Fatal(err)
		}
		if _, err := L.RemoveItem(e.ctx, e.admin, lid, item, cur.Liturgy.Version+1); err != nil { // item.remove
			t.Fatal(err)
		}
		name := "Ibadah Pagi"
		if _, err := L.Update(e.ctx, e.admin, lid, app.LiturgyChange{Version: cur.Liturgy.Version + 2, ServiceName: &name}); err != nil { // liturgy.update
			t.Fatal(err)
		}
		a, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: duty, Name: "Pak Yan"}) // assignment.add
		if err != nil {
			t.Fatal(err)
		}
		if err := L.RemoveAssignment(e.ctx, e.admin, lid, a.Assignment.ID); err != nil { // assignment.remove
			t.Fatal(err)
		}

		// Refused writes leave no row and no gap in seq.
		if _, err := L.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: 1, Title: "x", Type: domain.ItemPrayer}); err == nil {
			t.Fatal("stale add accepted")
		}
		if _, err := L.AddAssignment(e.ctx, e.admin, lid, app.AssignmentInput{DutyID: duty}); err == nil {
			t.Fatal("empty assignment accepted")
		}

		h := e.history(lid)
		want := []string{domain.CmdLiturgyCreate, domain.CmdItemAdd, domain.CmdItemUpdate, domain.CmdItemSongs, domain.CmdItemsReorder,
			domain.CmdItemRemove, domain.CmdLiturgyUpdate, domain.CmdAssignmentAdd, domain.CmdAssignmentRemove}
		var got []string
		for i, row := range h {
			got = append(got, row.Edit.Command)
			if row.Edit.Seq != i+1 || row.Edit.Status != "done" || row.UserName != "Admin" || row.Edit.UserID != e.admin.UserID {
				t.Errorf("row %d: %+v (%s)", i, row.Edit, row.UserName)
			}
			if strings.Contains(string(row.Edit.Before)+string(row.Edit.After), "RAHASIA") {
				t.Errorf("row %d holds lyrics", i)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("commands: %v", got)
		}
		if after := image(t, h[0].Edit.After); h[0].Edit.Before != nil || len(after["items"].([]any)) != 7 || after["service_name"] != "Ibadah" {
			t.Errorf("create image: %v %v", h[0].Edit.Before, after)
		}
		if b, a := image(t, h[2].Edit.Before), image(t, h[2].Edit.After); len(b) != 1 || b["title"] != "Lagu baru" || a["title"] != "Lagu 2" {
			t.Errorf("update image: %v %v", b, a)
		}
		if h[1].Edit.ItemID != item || h[1].Edit.ItemVersionAfter != 1 || h[2].Edit.ItemVersionAfter != 2 || h[3].Edit.ItemVersionAfter != 3 {
			t.Errorf("item versions: %+v %+v %+v", h[1].Edit, h[2].Edit, h[3].Edit)
		}
		if b := image(t, h[5].Edit.Before); b["id"] != string(item) || h[5].Edit.After != nil {
			t.Errorf("remove image: %v", b)
		}
		if h[4].Edit.LiturgyVersionAfter != cur.Liturgy.Version+1 || h[5].Edit.LiturgyVersionAfter != cur.Liturgy.Version+2 {
			t.Errorf("liturgy versions: %d %d", h[4].Edit.LiturgyVersionAfter, h[5].Edit.LiturgyVersionAfter)
		}
		if b := image(t, h[7].Edit.After); h[7].Edit.Before != nil || b["name"] != "Pak Yan" || image(t, h[8].Edit.Before)["id"] != string(a.Assignment.ID) {
			t.Errorf("assignment images: %v", b)
		}
		if got := e.liturgy(lid).Liturgy.EditSeq; got != 9 {
			t.Errorf("edit_seq %d", got)
		}

		// Everyone who can see the liturgy reads it; the limit caps the page.
		team, _ := e.member("team@example.org")
		if _, err := L.Edits(e.ctx, team, lid, 10); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member reads the history: %v", err)
		}
		if h, _ := L.Edits(e.ctx, e.admin, lid, 3); len(h) != 3 || h[0].Edit.Seq != 9 {
			t.Errorf("newest first, limited: %+v", h)
		}
	})
}

// TC-L-010: the largest legal images are stored and read back whole.
func TestHistoryWorstCase(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		L := e.liturgies

		// liturgy.create: a template of 60 items of 5,000 characters.
		var items []domain.TemplateItem
		text := strings.Repeat("ä", domain.MaxTemplateText)
		for range domain.MaxTemplateItems {
			items = append(items, domain.TemplateItem{Title: strings.Repeat("t", 200), Type: domain.ItemFreeText, DefaultText: text})
		}
		big, err := e.tpls.Create(e.ctx, e.admin, app.TemplateInput{Name: "Besar", Items: items})
		if err != nil {
			t.Fatal(err)
		}
		v, err := L.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", ServiceName: "Besar", TemplateID: &big.Template.ID})
		if err != nil {
			t.Fatal(err)
		}
		lid := v.Liturgy.ID
		h := e.history(lid)
		var img struct {
			Items []struct{ Text string }
		}
		if err := json.Unmarshal(h[0].Edit.After, &img); err != nil || len(img.Items) != 60 || len(img.Items[59].Text) != 2*domain.MaxTemplateText {
			t.Fatalf("create image: %d items, %v", len(img.Items), err)
		}

		// item.songs and item.remove: 10 songs of 100 entries (a liturgy of 60 items is full).
		lid = e.draft()
		song := e.newSong("Lagu", "id")
		item := e.addItem(lid, domain.ItemSong, "Lagu")
		var in []app.ItemSongInput
		for range domain.MaxItemSongs {
			s := app.ItemSongInput{SongID: song.Song.ID, Key: "F#m", Note: strings.Repeat("n", 200)}
			for range domain.MaxSequenceEntries {
				s.Entries = append(s.Entries, app.EntryInput{SectionID: song.Song.Sections[1].ID, KeyChange: "Bb", Note: strings.Repeat("e", 100)})
			}
			in = append(in, s)
		}
		if _, err := L.SetSongs(e.ctx, e.admin, lid, item, 1, in); err != nil {
			t.Fatal(err)
		}
		cur := e.liturgy(lid)
		if _, err := L.RemoveItem(e.ctx, e.admin, lid, item, cur.Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		h = e.history(lid)
		count := func(raw []byte, wrapped bool) (songs, entries int) {
			type song struct{ Entries []struct{ ID string } }
			var list []song
			if wrapped {
				var im struct{ Songs []song }
				if err := json.Unmarshal(raw, &im); err != nil {
					t.Fatal(err)
				}
				list = im.Songs
			} else if err := json.Unmarshal(raw, &list); err != nil {
				t.Fatal(err)
			}
			for _, s := range list {
				entries += len(s.Entries)
			}
			return len(list), entries
		}
		var setSongs, removed domain.Edit
		for _, row := range h {
			switch row.Edit.Command {
			case domain.CmdItemSongs:
				setSongs = row.Edit
			case domain.CmdItemRemove:
				removed = row.Edit
			}
		}
		if _, n := count(setSongs.After, false); n != 1000 {
			t.Errorf("item.songs after image: %d entries", n)
		}
		if s, n := count(removed.Before, true); s != 10 || n != 1000 {
			t.Errorf("item.remove before image: %d songs, %d entries", s, n)
		}

		// item.update: two copies of a 20,000-character text.
		pr := e.addItem(lid, domain.ItemPrayer, "Doa")
		long := strings.Repeat("ü", domain.MaxItemText)
		other := strings.Repeat("ö", domain.MaxItemText)
		for i, tx := range []string{long, other} {
			if _, err := L.UpdateItem(e.ctx, e.admin, lid, pr, app.ItemChange{Version: i + 1, Text: &tx}); err != nil {
				t.Fatal(err)
			}
		}
		h = e.history(lid)
		last := h[len(h)-1].Edit
		if b, a := image(t, last.Before), image(t, last.After); len(b["text"].(string)) != 2*domain.MaxItemText || len(a["text"].(string)) != 2*domain.MaxItemText {
			t.Errorf("item.update image: %d %d", len(b["text"].(string)), len(a["text"].(string)))
		}
	})
}
