// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// songItem makes a song item with one song whose sequence has one entry that
// names a section and a singing part, then removes the item, so that undo has
// to bring the whole thing back. It returns what the item refers to.
func (u uenv) removedSongItem() (item domain.ItemID, song domain.SongID, section domain.SectionID, part domain.SingingPartID) {
	u.t.Helper()
	L := u.liturgies
	v, err := u.songs.Create(u.ctx, u.a, app.SongInput{Language: "id", Title: "Besar Setia-Mu",
		Sections: []app.SectionInput{verse("v1", 1, "bait satu"), chorus("c", "reff")}})
	if err != nil {
		u.t.Fatal(err)
	}
	pt, err := u.vocab.Create(u.ctx, u.a, domain.KindSingingPart, "Solis")
	if err != nil {
		u.t.Fatal(err)
	}
	item = u.addItem(u.lid, domain.ItemSong, "Pujian")
	section, part = v.Song.Sections[0].ID, domain.SingingPartID(pt.Entry.ID)
	in := []app.ItemSongInput{{SongID: v.Song.ID, Key: "G", Note: "pelan",
		Entries: []app.EntryInput{{SectionID: section, SingingPartID: part, KeyChange: "A"}, {SectionID: v.Song.Sections[1].ID}}}}
	if _, err := L.SetSongs(u.ctx, u.a, u.lid, item, u.mustItem(item).Version, in); err != nil {
		u.t.Fatal(err)
	}
	if _, err := L.RemoveItem(u.ctx, u.a, u.lid, item, u.liturgy(u.lid).Liturgy.Version); err != nil {
		u.t.Fatal(err)
	}
	return item, v.Song.ID, section, part
}

// 16C.1, pinned before songsOf is split: undoing the removal of an item with
// songs puts the songs, their sequence and its notes back.
func TestUndoRestoresSongs(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		x, song, section, part := u.removedSongItem()
		u.mustUndo(u.a)
		it := u.mustItem(x)
		if len(it.Songs) != 1 {
			t.Fatalf("songs after undo: %+v", it.Songs)
		}
		s := it.Songs[0]
		if s.SongID != song || s.Key != "G" || s.Note != "pelan" || s.SongTitle != "Besar Setia-Mu" || s.Position != 0 || len(s.Entries) != 2 {
			t.Errorf("song after undo: %+v", s)
		}
		if e := s.Entries[0]; e.SectionID != section || e.SingingPartID != part || e.KeyChange != "A" || e.Position != 0 {
			t.Errorf("first entry after undo: %+v", e)
		}
		if e := s.Entries[1]; e.SingingPartID != "" || e.Position != 1 {
			t.Errorf("second entry after undo: %+v", e)
		}
		u.noGaps()
	})
}

// 16C.1: the same undo is refused when the song, one of its sections or the
// singing part no longer exists, and nothing is restored.
func TestUndoSongReferencesGone(t *testing.T) {
	cases := map[string]func(u uenv, song domain.SongID, section domain.SectionID, part domain.SingingPartID){
		"the song was deleted": func(u uenv, song domain.SongID, _ domain.SectionID, _ domain.SingingPartID) {
			if err := u.songs.Delete(u.ctx, u.a, song); err != nil {
				u.t.Fatal(err)
			}
		},
		"a section was deleted": func(u uenv, _ domain.SongID, section domain.SectionID, _ domain.SingingPartID) {
			u.sql("DELETE FROM song_sections WHERE id = ?", string(section))
		},
		"the singing part was deleted": func(u uenv, _ domain.SongID, _ domain.SectionID, part domain.SingingPartID) {
			if err := u.vocab.Delete(u.ctx, u.a, domain.KindSingingPart, string(part)); err != nil {
				u.t.Fatal(err)
			}
		},
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
				u := newUndoEnv(t, db)
				x, song, section, part := u.removedSongItem()
				spoil(u, song, section, part)
				_, err := u.undo(u.a)
				refused(t, err, app.UndoReferenceGone)
				if _, ok := u.item(x); ok {
					t.Error("a refused undo restored the item")
				}
				u.noGaps()
			})
		})
	}
}
