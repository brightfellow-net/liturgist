// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func verse(key string, n int, text string) app.SectionInput {
	return app.SectionInput{Key: key, Kind: domain.SectionVerse, Number: n, Text: text}
}

func chorus(key, text string) app.SectionInput {
	return app.SectionInput{Key: key, Kind: domain.SectionChorus, Text: text}
}

// newSong creates "Besar Setia-Mu" with verse 1, a chorus and verse 2,
// arranged V1 C V2 C, as the church admin.
func (e cenv) newSong(title, lang string) app.SongView {
	e.t.Helper()
	v, err := e.songs.Create(e.ctx, e.admin, app.SongInput{Language: lang, Title: title,
		Sections:    []app.SectionInput{verse("v1", 1, "bait satu"), chorus("c", "reff"), verse("v2", 2, "bait dua")},
		Arrangement: []string{"v1", "c", "v2", "c"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func sectionIDs(s domain.Song) []domain.SectionID {
	out := make([]domain.SectionID, len(s.Sections))
	for i, sec := range s.Sections {
		out[i] = sec.ID
	}
	return out
}

func invalidField(err error) string {
	var in *domain.InvalidInputError
	if errors.As(err, &in) {
		return in.Field
	}
	return ""
}

// IT-S-001: who can do what.
func TestSongPermissions(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	team, _ := e.member("team@example.org")
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID)

	v := e.newSong("Besar Setia-Mu", "id")
	if !v.Actions.Edit || !v.Actions.Delete || v.Song.Version != 1 {
		t.Errorf("admin view: %+v", v)
	}
	if got := sectionIDs(v.Song); len(got) != 3 || len(v.Song.DefaultArrangement) != 4 ||
		v.Song.DefaultArrangement[0] != got[0] || v.Song.DefaultArrangement[1] != got[1] || v.Song.DefaultArrangement[3] != got[1] {
		t.Errorf("keys must resolve to the new IDs: %+v", v.Song)
	}

	// A team member can look but not change.
	tv, err := e.songs.Get(e.ctx, team, v.Song.ID)
	if err != nil || tv.Actions.Edit || tv.Actions.Delete || len(tv.Song.Sections) != 3 {
		t.Errorf("team member Get: %+v %v", tv, err)
	}
	list, err := e.songs.List(e.ctx, team, app.SongQuery{})
	if err != nil || list.Total != 1 || list.Items[0].Actions.Edit {
		t.Errorf("team member List: %+v %v", list, err)
	}
	if _, err := e.songs.Create(e.ctx, team, app.SongInput{Language: "id", Title: "X"}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member Create: %v", err)
	}
	title := "Hijack"
	if _, err := e.songs.Update(e.ctx, team, v.Song.ID, app.SongChange{Version: 1, Title: &title}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member Update: %v", err)
	}
	if err := e.songs.Delete(e.ctx, team, v.Song.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member Delete: %v", err)
	}
	if _, err := e.songs.Link(e.ctx, team, v.Song.ID, v.Song.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member Link: %v", err)
	}
	if err := e.songs.Unlink(e.ctx, team, v.Song.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member Unlink: %v", err)
	}
	// The editor (library.edit) can.
	if _, err := e.songs.Create(e.ctx, editor, app.SongInput{Language: "en", Title: "Editor's song"}); err != nil {
		t.Errorf("editor Create: %v", err)
	}
	// Anonymous and unknown.
	if _, err := e.songs.List(e.ctx, nil, app.SongQuery{}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
	if _, err := e.songs.Get(e.ctx, team, "01JNOSUCHSONG0000000000000"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown song: %v", err)
	}
}

// TC-S-005: section replacement keeps IDs, creates and deletes, resolves keys.
func TestSongSectionReplacement(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	v := e.newSong("Besar Setia-Mu", "id")
	s := v.Song
	v1, c, v2 := s.Sections[0].ID, s.Sections[1].ID, s.Sections[2].ID
	update := func(ch app.SongChange) (app.SongView, error) {
		return e.songs.Update(e.ctx, e.admin, s.ID, ch)
	}

	// Verse 2 first (renumbered), a new bridge, the chorus kept, verse 1 dropped.
	secs := []app.SectionInput{
		{ID: v2, Kind: domain.SectionVerse, Number: 1, Text: "bait dua, kini pertama"},
		{Key: "bridge", Kind: domain.SectionBridge, Text: "jembatan"},
		{ID: c, Kind: domain.SectionChorus, Label: "Reff", Text: "reff baru"},
	}
	arr := []string{string(v2), "bridge", string(c), string(v2)}
	got, err := update(app.SongChange{Version: 1, Sections: &secs, Arrangement: &arr})
	if err != nil {
		t.Fatal(err)
	}
	ids := sectionIDs(got.Song)
	if len(ids) != 3 || ids[0] != v2 || ids[2] != c || ids[1] == "" || slices.Contains(ids, v1) {
		t.Fatalf("IDs: kept stay, new is created, dropped is gone: %v (v1=%s)", ids, v1)
	}
	if got.Song.Version != 2 || got.Song.Sections[0].Number != 1 || got.Song.Sections[2].Label != "Reff" {
		t.Errorf("fields: %+v", got.Song)
	}
	if want := []domain.SectionID{v2, ids[1], c, v2}; !slices.Equal(got.Song.DefaultArrangement, want) {
		t.Errorf("arrangement %v, want %v", got.Song.DefaultArrangement, want)
	}

	// Sections replaced without an arrangement: entries of removed sections disappear.
	only := []app.SectionInput{{ID: v2, Kind: domain.SectionVerse, Number: 1, Text: "bait dua"}}
	got, err = update(app.SongChange{Version: 2, Sections: &only})
	if err != nil || !slices.Equal(got.Song.DefaultArrangement, []domain.SectionID{v2, v2}) {
		t.Errorf("arrangement after dropping sections: %v %v", got.Song.DefaultArrangement, err)
	}
	// [] clears the arrangement; omitting keeps it.
	title := "Renamed"
	if got, err = update(app.SongChange{Version: 3, Title: &title}); err != nil || len(got.Song.DefaultArrangement) != 2 {
		t.Errorf("omitted arrangement: %v %v", got.Song.DefaultArrangement, err)
	}
	empty := []string{}
	if got, err = update(app.SongChange{Version: 4, Arrangement: &empty}); err != nil || len(got.Song.DefaultArrangement) != 0 {
		t.Errorf("cleared arrangement: %v %v", got.Song.DefaultArrangement, err)
	}

	// Mistakes are 422s that change nothing.
	cur := got.Song.Version
	for name, tc := range map[string]struct {
		ch   app.SongChange
		want string
	}{
		"foreign section ID": {app.SongChange{Version: cur, Sections: &[]app.SectionInput{{ID: "01JOTHERSONGSECTION00000", Kind: domain.SectionOther, Text: "x"}}}, "sections.0.id"},
		"duplicate ID": {app.SongChange{Version: cur, Sections: &[]app.SectionInput{
			{ID: v2, Kind: domain.SectionVerse, Number: 1, Text: "a"}, {ID: v2, Kind: domain.SectionVerse, Number: 2, Text: "b"}}}, "sections.1.id"},
		"duplicate key": {app.SongChange{Version: cur, Sections: &[]app.SectionInput{
			{Key: "k", Kind: domain.SectionOther, Text: "a"}, {Key: "k", Kind: domain.SectionOther, Text: "b"}}}, "sections.1.key"},
		"key on an existing section": {app.SongChange{Version: cur, Sections: &[]app.SectionInput{{ID: v2, Key: "k", Kind: domain.SectionVerse, Number: 1, Text: "a"}}}, "sections.0.key"},
		"unknown arrangement entry":  {app.SongChange{Version: cur, Arrangement: &[]string{"nothing"}}, "default_arrangement.0"},
		"verse without number":       {app.SongChange{Version: cur, Sections: &[]app.SectionInput{{Kind: domain.SectionVerse, Text: "a"}}}, "sections.0.number"},
		"bad title":                  {app.SongChange{Version: cur, Title: new(string)}, "title"},
	} {
		if _, err := update(tc.ch); invalidField(err) != tc.want {
			t.Errorf("%s: invalid field %q (%v), want %q", name, invalidField(err), err, tc.want)
		}
	}
	after, err := e.songs.Get(e.ctx, e.admin, s.ID)
	if err != nil || after.Song.Version != cur {
		t.Errorf("rejected updates must not change the song: %+v %v", after.Song, err)
	}
}

// IT-S-002: two editors, one wins.
func TestSongVersionConflict(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	v := e.newSong("Besar Setia-Mu", "id")
	first, second := "First", "Second"
	if _, err := e.songs.Update(e.ctx, e.admin, v.Song.ID, app.SongChange{Version: 1, Title: &first}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.songs.Update(e.ctx, e.admin, v.Song.ID, app.SongChange{Version: 1, Title: &second}); !errors.Is(err, app.ErrVersionConflict) {
		t.Errorf("stale version: %v", err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, v.Song.ID); got.Song.Title != "First" || got.Song.Version != 2 {
		t.Errorf("the second update must change nothing: %+v", got.Song)
	}
}

// IT-S-005: a section in use cannot be removed; others can.
func TestSongSectionInUse(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	v := e.newSong("Besar Setia-Mu", "id")
	c, v2 := v.Song.Sections[1].ID, v.Song.Sections[2].ID
	lid := e.draft()
	e.useSections(lid, v.Song.ID, c)
	withoutChorus := []app.SectionInput{
		{ID: v.Song.Sections[0].ID, Kind: domain.SectionVerse, Number: 1, Text: "bait satu"},
		{ID: v2, Kind: domain.SectionVerse, Number: 2, Text: "bait dua"},
	}
	_, err := e.songs.Update(e.ctx, e.admin, v.Song.ID, app.SongChange{Version: 1, Sections: &withoutChorus})
	var busy *app.SectionInUseError
	if !errors.As(err, &busy) || !slices.Equal(busy.IDs, []domain.SectionID{c}) {
		t.Fatalf("removing a section in use: %v", err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, v.Song.ID); got.Song.Version != 1 || len(got.Song.Sections) != 3 {
		t.Errorf("nothing may change: %+v", got.Song)
	}
	withoutVerse := []app.SectionInput{
		{ID: v.Song.Sections[0].ID, Kind: domain.SectionVerse, Number: 1, Text: "bait satu"},
		{ID: c, Kind: domain.SectionChorus, Text: "reff"},
	}
	if _, err := e.songs.Update(e.ctx, e.admin, v.Song.ID, app.SongChange{Version: 1, Sections: &withoutVerse}); err != nil {
		t.Errorf("removing a section not in use: %v", err)
	}
	if err := e.songs.Delete(e.ctx, e.admin, v.Song.ID); !errors.Is(err, app.ErrSongInUse) {
		t.Errorf("deleting a song in use: %v", err)
	}
	if err := e.liturgies.Delete(e.ctx, e.admin, lid); err != nil {
		t.Fatal(err)
	}
	if err := e.songs.Delete(e.ctx, e.admin, v.Song.ID); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, err := e.songs.Get(e.ctx, e.admin, v.Song.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("deleted song: %v", err)
	}
}

func groupErr(err error) string {
	var g *app.GroupConflictError
	if errors.As(err, &g) {
		return g.Reason
	}
	return ""
}

// TC-S-007, IT-S-007: groups of language versions.
func TestSongGroups(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	id1, en1, en2, zh := e.newSong("Besar Setia-Mu", "id"), e.newSong("Great Is Thy Faithfulness", "en"),
		e.newSong("Another English", "en"), e.newSong("你的信实广大", "zh-Hans")
	a, b, c, z := id1.Song.ID, en1.Song.ID, en2.Song.ID, zh.Song.ID
	link := func(x, y domain.SongID) (app.SongView, error) { return e.songs.Link(e.ctx, e.admin, x, y) }

	if _, err := link(a, a); invalidField(err) != "other_song_id" {
		t.Errorf("linking a song to itself: %v", err)
	}
	if _, err := link(b, c); groupErr(err) != app.ReasonLanguageTaken {
		t.Errorf("two English songs: %v", err)
	}
	v, err := link(a, b) // new group
	if err != nil || len(v.Versions) != 1 || v.Versions[0].ID != b || v.Song.Version != 2 {
		t.Fatalf("new group: %+v %v", v, err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, b); got.Song.Version != 2 || got.Song.GroupID != v.Song.GroupID {
		t.Errorf("both songs gain a version: %+v", got.Song)
	}
	if v, err = link(z, a); err != nil || len(v.Versions) != 2 { // the Chinese song joins
		t.Fatalf("join: %+v %v", v, err)
	}
	if v, err = link(a, b); err != nil || v.Song.Version != 2 { // already together: no change (a's group did not change when z joined)
		t.Errorf("same group is a no-op: version %d %v", v.Song.Version, err)
	}
	if _, err = link(c, a); groupErr(err) != app.ReasonLanguageTaken {
		t.Errorf("English is taken in the group: %v", err)
	}
	other := e.newSong("Lagu Lain", "id")
	third := e.newSong("Ein Lied", "en")
	if _, err = link(third.Song.ID, other.Song.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = link(a, other.Song.ID); groupErr(err) != app.ReasonAlreadyGrouped {
		t.Errorf("two different groups: %v", err)
	}

	// Changing a grouped song's language to a taken one is refused.
	lang := "en"
	if _, err := e.songs.Update(e.ctx, e.admin, z, app.SongChange{Version: 2, Language: &lang}); groupErr(err) != app.ReasonLanguageTaken {
		t.Errorf("language change into a taken language: %v", err)
	}

	// Unlinking one of three keeps the group; unlinking again dissolves it.
	if err := e.songs.Unlink(e.ctx, e.admin, z); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, a); len(got.Versions) != 1 {
		t.Errorf("group of two: %+v", got.Versions)
	}
	if err := e.songs.Unlink(e.ctx, e.admin, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, a); got.Song.GroupID != "" || len(got.Versions) != 0 {
		t.Errorf("a group left with one song is dissolved: %+v", got)
	}
	if err := e.songs.Unlink(e.ctx, e.admin, b); err != nil { // not grouped: no change
		t.Errorf("unlink of an ungrouped song: %v", err)
	}

	// Deleting one of a pair dissolves the group.
	if err := e.songs.Delete(e.ctx, e.admin, third.Song.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.songs.Get(e.ctx, e.admin, other.Song.ID); got.Song.GroupID != "" {
		t.Errorf("group after deleting its other song: %+v", got.Song)
	}
}

// List requests are folded and checked (06 §5.2).
func TestSongListRequests(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	s, err := e.songs.Create(e.ctx, e.admin, app.SongInput{Language: "id", Title: "Besar Setia-Mu", HymnalSource: "KJ", HymnalNumber: "12",
		Sections: []app.SectionInput{verse("", 1, "bait satu")}})
	if err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]int{"setia": 1, "KJ 12": 1, "kj12": 1, "bait": 1, "zzz": 0, "": 1} {
		got, err := e.songs.List(e.ctx, e.admin, app.SongQuery{Q: q})
		if err != nil || got.Total != want {
			t.Errorf("q=%q: total %d (%v), want %d", q, got.Total, err, want)
		}
	}
	for name, q := range map[string]app.SongQuery{
		"201 characters":         {Q: strings.Repeat("a", 201)},
		"11 words":               {Q: "a b c d e f g h i j k"},
		"unknown language":       {Language: "fr"},
		"unknown licence status": {LicenceStatus: "free"},
		"number without hymnal":  {HymnalNumber: "12"},
		"hymnal without letters": {HymnalSource: "--"},
	} {
		if _, err := e.songs.List(e.ctx, e.admin, q); invalidField(err) == "" {
			t.Errorf("%s must be rejected, got %v", name, err)
		}
	}
	got, err := e.songs.List(e.ctx, e.admin, app.SongQuery{HymnalSource: "kj", HymnalNumber: "12", Limit: 500})
	if err != nil || got.Total != 1 || got.Items[0].ID != s.Song.ID || !got.Items[0].Actions.Edit {
		t.Errorf("hymnal filter: %+v %v", got, err)
	}
}

// The operator command rebuilds the index church by church (the repair of a
// damaged index is tested in the repository, IT-S-006).
func TestReindexSongs(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	e.newSong("Besar Setia-Mu", "id")
	e.newSong("Great Is Thy Faithfulness", "en")
	n, err := e.operator.ReindexSongs(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("reindex: %d %v", n, err)
	}
	if got, _ := e.songs.List(e.ctx, e.admin, app.SongQuery{Q: "setia"}); got.Total != 1 {
		t.Errorf("after reindex: %+v", got)
	}
}
