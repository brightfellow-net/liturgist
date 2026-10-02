// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// mkSong builds a song whose verses are the given lyrics, numbered from 1.
func mkSong(sid, title, language string, now time.Time, lyrics ...string) domain.Song {
	s := domain.Song{ID: domain.SongID(id(sid)), Language: language, Title: title, AltTitles: []string{},
		LicenceStatus: domain.LicenceUnknown, Version: 1, CreatedAt: now, UpdatedAt: now}
	for i, l := range lyrics {
		s.Sections = append(s.Sections, domain.Section{ID: domain.SectionID(id(fmt.Sprintf("%sV%d", sid, i+1))),
			Kind: domain.SectionVerse, Number: i + 1, Text: l})
	}
	return s
}

func songsOf(t *testing.T, db *sqlstore.DB, fn func(r app.SongRepo) error) {
	t.Helper()
	mustWrite(t, db, func(s app.Store) error {
		cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
		if err != nil {
			return err
		}
		return fn(cs.Songs())
	})
}

// rawSong inserts a song row with raw SQL.
func (f fixture) rawSong(sid, church, lang string) error {
	return f.exec(`INSERT INTO songs (id, church_id, language, title, title_key, alt_titles, hymnal_source, hymnal_number,
		hymnal_key, lyricist, composer, translator, default_key, copyright_holder, copyright_line, ccli_song_number,
		licence_status, licence_notes, version, created_at, updated_at)
		VALUES (?, ?, ?, 't', 't', '[]', '', '', NULL, '', '', '', '', '', '', '', 'unknown', '', 1, ?, ?)`,
		id(sid), id(church), lang, f.ts(0), f.ts(0))
}

func (f fixture) churchRepo(fn func(r app.SongRepo) error) { songsOf(f.t, f.db, fn) }

// IT-S-001 (repository part): rows written are read back, an update replaces
// sections in place and keeps their IDs, a stale version changes nothing.
func TestSongRoundTripAndUpdate(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		s := mkSong("S1", "Besar Setia-Mu", "id", f.now, "bait satu", "bait dua")
		s.AltTitles = []string{"Great Is Thy Faithfulness"}
		s.HymnalSource, s.HymnalNumber = "KJ", "12"
		s.Lyricist, s.DefaultKey, s.CCLISongNumber = "T. Chisholm", "G", "123"
		chorus := domain.Section{ID: domain.SectionID(id("S1C")), Kind: domain.SectionChorus, Label: "Reff", Text: "reff\nbaris dua"}
		s.Sections = append(s.Sections, chorus)
		v1, v2 := s.Sections[0].ID, s.Sections[1].ID
		s.DefaultArrangement = []domain.SectionID{v1, chorus.ID, v2, chorus.ID}
		f.churchRepo(func(r app.SongRepo) error { return r.Create(ctx, s) })

		var got domain.Song
		f.churchRepo(func(r app.SongRepo) (err error) { got, err = r.ByID(ctx, s.ID); return })
		if !reflect.DeepEqual(got, s) {
			t.Fatalf("round trip:\n got %+v\nwant %+v", got, s)
		}

		// Swap the two verse numbers, drop the chorus, add a bridge, new arrangement.
		next := got
		next.Title = "Besar Setia-Mu (edit)"
		next.Version, next.UpdatedAt = 2, f.now.Add(time.Minute)
		next.Sections = []domain.Section{
			{ID: v2, Kind: domain.SectionVerse, Number: 1, Text: "bait dua, sekarang pertama"},
			{ID: v1, Kind: domain.SectionVerse, Number: 2, Text: "bait satu, sekarang kedua"},
			{ID: domain.SectionID(id("S1B")), Kind: domain.SectionBridge, Text: "jembatan"},
		}
		next.DefaultArrangement = []domain.SectionID{v2, v1, v2}
		stale := false
		f.churchRepo(func(r app.SongRepo) error {
			bad := next
			ok, err := r.Update(ctx, bad, 7) // wrong expected version
			stale = ok
			return err
		})
		if stale {
			t.Error("an update with a stale version must change nothing")
		}
		f.churchRepo(func(r app.SongRepo) error {
			ok, err := r.Update(ctx, next, 1)
			if err != nil || !ok {
				return fmt.Errorf("update: %v %w", ok, err)
			}
			return nil
		})
		f.churchRepo(func(r app.SongRepo) (err error) { got, err = r.ByID(ctx, s.ID); return })
		if !reflect.DeepEqual(got, next) {
			t.Fatalf("after update:\n got %+v\nwant %+v", got, next)
		}

		// Deleting removes sections, arrangement, search rows.
		f.churchRepo(func(r app.SongRepo) error { return r.Delete(ctx, s.ID) })
		for _, table := range []string{"songs", "song_sections", "song_arrangement_entries", "song_search"} {
			if n := count(t, db, "SELECT count(*) FROM "+table+" WHERE church_id = ?", id("CHA")); n != 0 {
				t.Errorf("%s: %d rows left", table, n)
			}
		}
		if db.Dialect().Name() == "sqlite" {
			if n := count(t, db, "SELECT count(*) FROM song_fts"); n != 0 {
				t.Errorf("song_fts: %d rows left", n)
			}
		}
		f.churchRepo(func(r app.SongRepo) error {
			if err := r.Delete(ctx, s.ID); !errors.Is(err, app.ErrNotFound) {
				return fmt.Errorf("delete again: %w", err)
			}
			return nil
		})
	})
}

// searchFixture is the library of IT-S-003 and IT-S-004.
func searchFixture(now time.Time) []domain.Song {
	mk := func(sid, title, lang string, lyrics ...string) domain.Song {
		return mkSong(sid, title, lang, now, lyrics...)
	}
	var out []domain.Song
	add := func(s domain.Song, src, num string, alts ...string) {
		s.HymnalSource, s.HymnalNumber = src, num
		s.AltTitles = append(s.AltTitles, alts...)
		out = append(out, s)
	}
	add(mk("S01", "Besar Setia-Mu", "id", "besar setia-Mu ya Tuhan"), "KJ", "12", "Great Is Thy Faithfulness")
	add(mk("S02", "Allah Mahabesar", "id", "Allah mahabesar dan setia"), "", "")
	add(mk("S03", "Café Kasih", "id", "Tuhan baik"), "", "")
	add(mk("S04", "Kasih Setia", "id", "kasih setia Tuhan besar"), "PKJ", "12a")
	add(mk("S05", "Dan Yang Terbaik", "id", "dan yang terbaik"), "", "")
	add(mk("S06", "Terima Kasih Tuhan", "id", "terima kasih"), "KJ", "120")
	add(mk("S10", "Great Is Thy Faithfulness", "en", "great is thy faithfulness morning by morning"), "", "")
	add(mk("S11", "Amazing Grace", "en", "amazing grace how sweet the sound"), "", "")
	add(mk("S12", "Allah's Grace", "en", "grace upon grace"), "", "")
	add(mk("S13", "O Come, All Ye Faithful", "en", "come and behold him"), "", "")
	add(mk("S20", "主，我愿意", "zh-Hans", "主我愿意跟随你，你是我的主"), "", "")
	add(mk("S21", "奇异恩典", "zh-Hans", "奇异恩典，何等甘甜"), "", "")
	add(mk("S22", "Grace 恩典", "zh-Hans", "恩典够用"), "", "")
	add(mk("S30", "Lagu 2026", "id", "tahun 2026 baru"), "", "")
	add(mk("S31", "ＦＵＬＬ Width", "en", "wide letters"), "", "")
	add(mk("S32", "Café Rohani", "id", "Roh Kudus"), "", "")
	return out
}

func ids(page app.SongPage) []string {
	out := make([]string, len(page.Items))
	for i, it := range page.Items {
		out[i] = string(it.ID)[:3]
	}
	return out
}

// IT-S-003, IT-S-004: matches, tiers and the exact order of result IDs. The
// same expectations hold on SQLite and PostgreSQL.
func TestSongSearch(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		songs := searchFixture(f.now)
		songs[1].LicenceStatus = domain.LicenceChurch // S02
		f.churchRepo(func(r app.SongRepo) error {
			for _, s := range songs {
				if err := r.Create(ctx, s); err != nil {
					return err
				}
			}
			return nil
		})

		search := func(q string, mod ...func(*app.SongSearch)) app.SongSearch {
			s := app.SongSearch{Terms: domain.SearchTerms(q), Limit: 50}
			if key, ok := domain.ParseHymnalQuery(q); ok {
				s.HymnalQueryKey = key
			}
			for _, m := range mod {
				m(&s)
			}
			return s
		}
		cases := []struct {
			name string
			q    app.SongSearch
			want []string
		}{
			{"title token and lyrics only", search("setia"), []string{"S01", "S04", "S02"}},
			{"prefix, lyrics-only second", search("bes"), []string{"S01", "S04"}},
			{"two terms across head and lyrics", search("besar setia"), []string{"S01", "S04"}},
			{"hymnal query then text match", search("kj 12"), []string{"S01", "S06"}},
			{"hymnal query without spaces", search("pkj12a"), []string{"S04"}},
			{"precomposed and decomposed accents", search("cafe"), []string{"S03", "S32"}},
			{"accent in the query", search("café"), []string{"S03", "S32"}},
			{"Latin title of a Chinese song", search("grace"), []string{"S12", "S11", "S22"}},
			{"stop word is a word", search("dan"), []string{"S05", "S02"}},
			{"one Chinese character", search("恩"), []string{"S22", "S21"}},
			{"two Chinese terms", search("愿意 主"), []string{"S20"}},
			{"digits", search("2026"), []string{"S30"}},
			{"full-width letters", search("full"), []string{"S31"}},
			{"alternative title in the head", search("gre"), []string{"S01", "S10"}},
			{"punctuation boundary", search("setia mu"), []string{"S01"}},
			{"apostrophe splits the word; a one-letter prefix matches lyrics", search("allah s"), []string{"S12", "S02"}},
			{"no match", search("zzz"), []string{}},
			{"language filter", search("", func(s *app.SongSearch) { s.Language = "en" }), []string{"S12", "S11", "S31", "S10", "S13"}},
			{"hymnal source filter", search("", func(s *app.SongSearch) { s.HymnalSourceKey = "kj" }), []string{"S01", "S06"}},
			{"hymnal key filter", search("", func(s *app.SongSearch) { s.HymnalKey = "kj:12" }), []string{"S01"}},
			{"licence filter", search("", func(s *app.SongSearch) { s.LicenceStatus = domain.LicenceChurch }), []string{"S02"}},
			{"filter with query", search("faithful", func(s *app.SongSearch) { s.Language = "en" }), []string{"S10", "S13"}},
		}
		for _, c := range cases {
			var page app.SongPage
			f.churchRepo(func(r app.SongRepo) (err error) { page, err = r.Search(ctx, c.q); return })
			if got := ids(page); !slices.Equal(got, c.want) || page.Total != len(c.want) {
				t.Errorf("%s: got %v (total %d), want %v", c.name, got, page.Total, c.want)
			}
		}

		// Without a query: all songs by title key (bytewise), then ID; paging keeps the total.
		var all, paged app.SongPage
		f.churchRepo(func(r app.SongRepo) (err error) { all, err = r.Search(ctx, search("")); return })
		f.churchRepo(func(r app.SongRepo) (err error) {
			paged, err = r.Search(ctx, search("", func(s *app.SongSearch) { s.Limit, s.Offset = 3, 2 }))
			return
		})
		var want []string
		sorted := slices.Clone(songs)
		slices.SortFunc(sorted, func(a, b domain.Song) int {
			if c := compareBytes(a.TitleKey(), b.TitleKey()); c != 0 {
				return c
			}
			return compareBytes(string(a.ID), string(b.ID))
		})
		for _, s := range sorted {
			want = append(want, string(s.ID)[:3])
		}
		if got := ids(all); !slices.Equal(got, want) || all.Total != len(songs) {
			t.Errorf("all songs: got %v (total %d), want %v", got, all.Total, want)
		}
		if got := ids(paged); !slices.Equal(got, want[2:5]) || paged.Total != len(songs) {
			t.Errorf("page: got %v (total %d), want %v", got, paged.Total, want[2:5])
		}
	})
}

func compareBytes(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

type indexRow struct{ SongID, Language, Head, Lyrics string }

func indexRows(t *testing.T, db *sqlstore.DB) []indexRow {
	t.Helper()
	var out []indexRow
	mustRead(t, db, func(s app.Store) error {
		rows, err := sqlstore.RawTx(s).QueryxContext(ctx, db.Dialect().Rebind(`SELECT song_id, language, head_fold, lyrics_fold
			FROM song_search WHERE church_id = ? ORDER BY song_id`), id("CHA"))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var r indexRow
			if err := rows.Scan(&r.SongID, &r.Language, &r.Head, &r.Lyrics); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out
}

// IT-S-006: the index follows every change, a rolled-back change leaves it
// untouched, and reindexing a consistent index changes nothing, even while
// songs are being created.
func TestSongIndexConsistency(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		songs := searchFixture(f.now)
		f.churchRepo(func(r app.SongRepo) error {
			for _, s := range songs {
				if err := r.Create(ctx, s); err != nil {
					return err
				}
			}
			return nil
		})
		check := func(when string) {
			t.Helper()
			rows := indexRows(t, db)
			nSongs := count(t, db, "SELECT count(*) FROM songs WHERE church_id = ?", id("CHA"))
			if len(rows) != nSongs {
				t.Errorf("%s: %d index rows for %d songs", when, len(rows), nSongs)
			}
			if db.Dialect().Name() == "sqlite" {
				if n := count(t, db, "SELECT count(*) FROM song_fts"); n != nSongs {
					t.Errorf("%s: %d song_fts rows for %d songs", when, n, nSongs)
				}
			}
		}
		check("after creates")
		var bySong = map[string]indexRow{}
		for _, r := range indexRows(t, db) {
			bySong[r.SongID] = r
		}
		if r := bySong[id("S01")]; r.Head != "besar setia mu great is thy faithfulness kj 12" || r.Lyrics != "besar setia mu ya tuhan" {
			t.Errorf("index of S01: %+v", r)
		}
		if r := bySong[id("S20")]; r.Head != "主我愿意" || r.Lyrics != "主我愿意跟随你你是我的主" {
			t.Errorf("index of the Chinese song: %+v", r)
		}

		// A change that fails leaves everything as it was.
		before := indexRows(t, db)
		err := db.Write(ctx, func(s app.Store) error {
			cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			if err := cs.Songs().Create(ctx, mkSong("S99", "Never saved", "id", f.now, "x")); err != nil {
				return err
			}
			return errors.New("rollback")
		})
		if err == nil || !slices.Equal(indexRows(t, db), before) {
			t.Errorf("rolled-back create changed the index (err %v)", err)
		}
		check("after rollback")

		// Edit and delete keep the index in step.
		edited := songs[2]
		edited.Title, edited.Version, edited.UpdatedAt = "Kasih Baru", 2, f.now.Add(time.Minute)
		f.churchRepo(func(r app.SongRepo) error {
			if ok, err := r.Update(ctx, edited, 1); err != nil || !ok {
				return fmt.Errorf("update: %v %w", ok, err)
			}
			return r.Delete(ctx, songs[3].ID)
		})
		check("after edit and delete")

		// A damaged index is repaired by reindexing.
		good := indexRows(t, db)
		f.must("DELETE FROM song_search WHERE song_id = ?", id("S05"))
		if db.Dialect().Name() == "sqlite" {
			f.must("DELETE FROM song_fts")
		}
		f.churchRepo(func(r app.SongRepo) error { return r.Reindex(ctx) })
		if after := indexRows(t, db); !slices.Equal(after, good) {
			t.Error("reindex did not restore the damaged index")
		}
		check("after repairing a damaged index")

		// Reindex of a consistent index changes nothing; creates racing with it are not lost.
		before = indexRows(t, db)
		f.churchRepo(func(r app.SongRepo) error { return r.Reindex(ctx) })
		if after := indexRows(t, db); !slices.Equal(after, before) {
			t.Error("reindexing a consistent index changed it")
		}
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := db.Write(ctx, func(s app.Store) error {
					cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
					if err != nil {
						return err
					}
					if err := cs.LockChurch(ctx); err != nil { // as the use cases and the CLI do
						return err
					}
					if i == 3 {
						return cs.Songs().Reindex(ctx)
					}
					return cs.Songs().Create(ctx, mkSong(fmt.Sprintf("R%d", i), fmt.Sprintf("Racing %d", i), "id", f.now, "baris"))
				})
				if err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		check("after racing creates and reindex")
		before = indexRows(t, db)
		f.churchRepo(func(r app.SongRepo) error { return r.Reindex(ctx) })
		if after := indexRows(t, db); !slices.Equal(after, before) {
			t.Error("the index after the race differs from a fresh rebuild")
		}
	})
}

// Groups (06 §2.3): one song per language, members ordered by language, and a
// group still referenced cannot be deleted.
func TestSongGroups(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		f.churchRepo(func(r app.SongRepo) error {
			for _, s := range []domain.Song{
				mkSong("S1", "Besar Setia-Mu", "id", f.now, "a"),
				mkSong("S2", "Great Is Thy Faithfulness", "en", f.now, "b"),
				mkSong("S3", "Another English", "en", f.now, "c"),
			} {
				if err := r.Create(ctx, s); err != nil {
					return err
				}
			}
			g := domain.SongGroupID(id("G1"))
			if err := r.CreateGroup(ctx, g, f.now); err != nil {
				return err
			}
			if err := r.SetGroup(ctx, []domain.SongID{domain.SongID(id("S1")), domain.SongID(id("S2"))}, g, f.now.Add(time.Minute)); err != nil {
				return err
			}
			members, err := r.GroupMembers(ctx, g)
			if err != nil || len(members) != 2 || members[0].Language != "en" || members[1].Language != "id" {
				return fmt.Errorf("members: %+v %w", members, err)
			}
			s1, err := r.ByID(ctx, domain.SongID(id("S1")))
			if err != nil || s1.GroupID != g || s1.Version != 2 || !s1.UpdatedAt.Equal(f.now.Add(time.Minute)) {
				return fmt.Errorf("version and group after SetGroup: %+v %w", s1, err)
			}
			return nil
		})
		// A failed statement aborts a PostgreSQL transaction, so this runs on its own.
		err := db.Write(ctx, func(s app.Store) error {
			cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			return cs.Songs().DeleteGroup(ctx, domain.SongGroupID(id("G1")))
		})
		if !errors.Is(err, app.ErrReferenced) {
			t.Errorf("deleting a group in use: %v", err)
		}
		// A second English song cannot join: unique index songs_group_language_key.
		err = db.Write(ctx, func(s app.Store) error {
			cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			return cs.Songs().SetGroup(ctx, []domain.SongID{domain.SongID(id("S3"))}, domain.SongGroupID(id("G1")), f.now)
		})
		var u *app.UniqueError
		if !errors.As(err, &u) || u.Constraint != "songs_group_language_key" {
			t.Errorf("second song of a language in a group: %v", err)
		}
		f.churchRepo(func(r app.SongRepo) error {
			if err := r.SetGroup(ctx, []domain.SongID{domain.SongID(id("S1")), domain.SongID(id("S2"))}, "", f.now); err != nil {
				return err
			}
			return r.DeleteGroup(ctx, domain.SongGroupID(id("G1")))
		})
	})
}

// IT-S-008: the database itself rejects invalid song rows.
func TestSongConstraints(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		for _, s := range []domain.Song{mkSong("SA", "A", "id", f.now, "x"), mkSong("SB", "B", "en", f.now, "y")} {
			f.churchRepo(func(r app.SongRepo) error { return r.Create(ctx, s) })
		}
		f.churchRepo(func(r app.SongRepo) error {
			s := mkSong("SC", "C", "id", f.now, "z")
			s.DefaultArrangement = []domain.SectionID{s.Sections[0].ID}
			return r.Create(ctx, s)
		})
		insertSection := func(sec, church, song, kind string, number any) error {
			return f.exec(`INSERT INTO song_sections (id, church_id, song_id, position, kind, number, label, text)
				VALUES (?, ?, ?, 5, ?, ?, NULL, 'text')`, id(sec), id(church), id(song), kind, number)
		}
		insertSong := func(sid, lang, source, number string, hymnalKey any, version int, alts string) error {
			return f.exec(`INSERT INTO songs (id, church_id, language, title, title_key, alt_titles, hymnal_source, hymnal_number,
				hymnal_key, lyricist, composer, translator, default_key, copyright_holder, copyright_line, ccli_song_number,
				licence_status, licence_notes, version, created_at, updated_at)
				VALUES (?, ?, ?, 't', 't', ?, ?, ?, ?, '', '', '', '', '', '', '', 'unknown', '', ?, ?, ?)`,
				id(sid), id("CHA"), lang, alts, source, number, hymnalKey, version, f.ts(0), f.ts(0))
		}
		invalid := map[string]func() error{
			"verse without number":            func() error { return insertSection("X1", "CHA", "SA", "verse", nil) },
			"verse number 100":                func() error { return insertSection("X2", "CHA", "SA", "verse", 100) },
			"chorus with a number":            func() error { return insertSection("X3", "CHA", "SA", "chorus", 1) },
			"unknown kind":                    func() error { return insertSection("X4", "CHA", "SA", "refrain", nil) },
			"unknown language":                func() error { return insertSong("X5", "fr", "", "", nil, 1, "[]") },
			"version 0":                       func() error { return insertSong("X6", "id", "", "", nil, 0, "[]") },
			"alternative titles not an array": func() error { return insertSong("X7", "id", "", "", nil, 1, "{}") },
			"hymnal source without number":    func() error { return insertSong("X8", "id", "KJ", "", nil, 1, "[]") },
			"hymnal number without key":       func() error { return insertSong("X9", "id", "KJ", "1", nil, 1, "[]") },
			"hymnal key without number":       func() error { return insertSong("XA", "id", "", "", "kj:1", 1, "[]") },
			"negative section position": func() error {
				return f.exec(`UPDATE song_sections SET position = -1 WHERE id = ?`, id("SAV1"))
			},
			"arrangement position 100": func() error {
				return f.exec(`INSERT INTO song_arrangement_entries (church_id, song_id, position, section_id) VALUES (?, ?, 100, ?)`,
					id("CHA"), id("SA"), id("SAV1"))
			},
		}
		for name, write := range invalid {
			if err := write(); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", name, err)
			}
		}
		referenced := map[string]func() error{
			"section of a song of another church": func() error { return insertSection("Y1", "CHB", "SA", "other", nil) },
			"arrangement entry naming another song's section": func() error {
				return f.exec(`INSERT INTO song_arrangement_entries (church_id, song_id, position, section_id) VALUES (?, ?, 3, ?)`,
					id("CHA"), id("SA"), id("SBV1"))
			},
			"song in a group of another church": func() error {
				f.must(`INSERT INTO song_groups (id, church_id, created_at) VALUES (?, ?, ?)`, id("GB"), id("CHB"), f.ts(0))
				return f.exec(`UPDATE songs SET song_group_id = ? WHERE id = ?`, id("GB"), id("SA"))
			},
		}
		for name, write := range referenced {
			if err := write(); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("%s: want ErrReferenced, got %v", name, err)
			}
		}
		// Two verses with one number are a unique violation.
		var u *app.UniqueError
		if err := insertSection("Z1", "CHA", "SA", "verse", 1); !errors.As(err, &u) || u.Constraint != "song_sections_verse_key" {
			t.Errorf("second verse 1: %v", err)
		}
		// Deleting a section removes the arrangement entries that name it.
		f.must(`DELETE FROM song_sections WHERE id = ?`, id("SCV1"))
		if n := count(t, db, "SELECT count(*) FROM song_arrangement_entries WHERE song_id = ?", id("SC")); n != 0 {
			t.Errorf("%d arrangement entries left after deleting their section", n)
		}
	})
}
