// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (f fixture) importRepo(fn func(s app.ChurchStore) error) {
	f.t.Helper()
	err := f.db.Write(ctx, func(s app.Store) error {
		cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
		if err != nil {
			return err
		}
		return fn(cs)
	})
	if err != nil && !errors.Is(err, errRollbackFixture) {
		f.t.Fatal(err)
	}
}

// The import repository behaves the same on both databases (08 §7).
func TestImportRepo(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		batch := func(bid string, age time.Duration, status domain.ImportStatus) domain.ImportBatch {
			at := f.now.Add(age)
			return domain.ImportBatch{ID: domain.ImportBatchID(id(bid)), Format: domain.FormatOpenLyrics, Status: status,
				CreatedBy: domain.UserID(id("U1")), CreatedAt: at, UpdatedAt: at}
		}
		f.importRepo(func(cs app.ChurchStore) error {
			r := cs.Imports()
			d := domain.SongDraft{Language: "zh-Hans", Title: "大哉主恩", AltTitles: []string{"Besar", `Quote " and \`}, HymnalSource: "Pelengkap",
				HymnalNumber: "12", Sections: []domain.DraftSection{
					{Kind: domain.SectionVerse, Number: 1, Text: "神爱世人\n第二行"}, {Kind: domain.SectionChorus, Label: "Reff", Text: "x"}},
				DefaultArrangement: []int{0, 1, 0}}
			b1 := batch("B1", 0, domain.ImportOpen)
			c1 := testCandidate(id("C1"), b1.ID, 0)
			c1.Draft, c1.Warnings, c1.DuplicateOfID = d, []string{"blocks_numbered", "chorus_guessed"}, domain.SongID(id("S9"))
			c2, c3 := testCandidate(id("C2"), b1.ID, 1), testCandidate(id("C3"), b1.ID, 2)
			if err := r.CreateBatch(ctx, b1, []domain.ImportCandidate{c3, c1, c2}); err != nil { // order of the slice does not matter
				return err
			}
			if err := r.CreateBatch(ctx, batch("B2", time.Hour, domain.ImportOpen), nil); err != nil {
				return err
			}
			if err := r.CreateBatch(ctx, batch("B3", 2*time.Hour, domain.ImportClosed), nil); err != nil {
				return err
			}

			got, err := r.Batch(ctx, b1.ID)
			if err != nil || got.Format != domain.FormatOpenLyrics || got.Status != domain.ImportOpen || got.CreatedBy != b1.CreatedBy ||
				!got.CreatedAt.Equal(b1.CreatedAt) || !got.UpdatedAt.Equal(b1.UpdatedAt) {
				t.Errorf("Batch: %+v %v", got, err)
			}
			if _, err := r.Batch(ctx, domain.ImportBatchID(id("NOPE"))); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("missing batch: %v", err)
			}
			list, err := r.Candidates(ctx, b1.ID)
			if err != nil || len(list) != 3 || list[0].ID != c1.ID || list[1].ID != c2.ID || list[2].ID != c3.ID {
				t.Fatalf("Candidates: %+v %v", list, err)
			}
			if !reflect.DeepEqual(list[0].Draft, d) || !reflect.DeepEqual(list[0].Warnings, c1.Warnings) || list[0].DuplicateOfID != c1.DuplicateOfID ||
				list[0].Decision != domain.DecisionPending || list[0].MergeInto != "" || list[0].MergeTargetVersion != 0 || list[0].Outcome != "" {
				t.Errorf("the candidate did not round-trip:\n%+v\nwant draft %+v", list[0], d)
			}

			// Open batches, newest change first; a closed one is left out.
			open, err := r.OpenBatches(ctx)
			if err != nil || len(open) != 2 || open[0].ID != domain.ImportBatchID(id("B2")) || open[1].ID != b1.ID {
				t.Errorf("OpenBatches: %+v %v", open, err)
			}

			// Every mutable field is written.
			c1.Decision, c1.MergeInto, c1.MergeTargetVersion, c1.RemoveUnmatched = domain.DecisionMerge, domain.SongID(id("S1")), 4, true
			c1.Outcome, c1.ErrorCode = domain.OutcomeFailed, "section_in_use"
			c1.Draft.Title = "Changed"
			if err := r.UpdateCandidate(ctx, c1); err != nil {
				return err
			}
			one, err := r.Candidate(ctx, b1.ID, c1.ID)
			if err != nil || one.Decision != domain.DecisionMerge || one.MergeInto != c1.MergeInto || one.MergeTargetVersion != 4 ||
				!one.RemoveUnmatched || one.Outcome != domain.OutcomeFailed || one.ErrorCode != "section_in_use" || one.Draft.Title != "Changed" ||
				one.AppliedSongID != "" {
				t.Errorf("after update: %+v %v", one, err)
			}
			c1.Outcome, c1.ErrorCode, c1.AppliedSongID = domain.OutcomeApplied, "", domain.SongID(id("S2"))
			if err := r.UpdateCandidate(ctx, c1); err != nil {
				return err
			}
			if one, _ = r.Candidate(ctx, b1.ID, c1.ID); one.Outcome != domain.OutcomeApplied || one.AppliedSongID != c1.AppliedSongID || one.ErrorCode != "" {
				t.Errorf("applied: %+v", one)
			}
			if _, err := r.Candidate(ctx, domain.ImportBatchID(id("B2")), c1.ID); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("a candidate is only found in its own batch: %v", err)
			}

			// Unfinished: c1 applied, c2 pending, c3 skipped.
			c3.Decision = domain.DecisionSkip
			if err := r.UpdateCandidate(ctx, c3); err != nil {
				return err
			}
			if n, err := r.Unfinished(ctx, b1.ID); err != nil || n != 1 {
				t.Errorf("Unfinished = %d %v, want 1", n, err)
			}
			c2.Decision, c2.Outcome, c2.ErrorCode = domain.DecisionAccept, domain.OutcomeFailed, "validation_failed"
			if err := r.UpdateCandidate(ctx, c2); err != nil {
				return err
			}
			if n, _ := r.Unfinished(ctx, b1.ID); n != 1 {
				t.Errorf("a failed candidate is unfinished: %d", n)
			}

			// SetBatch changes the status and the time.
			later := f.now.Add(5 * time.Hour)
			if err := r.SetBatch(ctx, b1.ID, domain.ImportClosed, later); err != nil {
				return err
			}
			if got, _ = r.Batch(ctx, b1.ID); got.Status != domain.ImportClosed || !got.UpdatedAt.Equal(later) {
				t.Errorf("SetBatch: %+v", got)
			}

			// Older than the cutoff goes (with its candidates); B3 is two hours old, B1 was touched later.
			if err := r.DeleteOlderThan(ctx, f.now.Add(150*time.Minute)); err != nil {
				return err
			}
			if _, err := r.Batch(ctx, domain.ImportBatchID(id("B2"))); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("old batch kept: %v", err)
			}
			if _, err := r.Batch(ctx, domain.ImportBatchID(id("B3"))); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("old closed batch kept: %v", err)
			}
			if _, err := r.Batch(ctx, b1.ID); err != nil {
				t.Errorf("recent batch deleted: %v", err)
			}
			if err := r.DeleteBatch(ctx, b1.ID); err != nil {
				return err
			}
			if err := r.DeleteBatch(ctx, b1.ID); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("second delete: %v", err)
			}
			if rows, _ := r.Candidates(ctx, b1.ID); len(rows) != 0 {
				t.Errorf("%d candidates left", len(rows))
			}
			return errRollbackFixture
		})
	})
}

// Duplicate detection (08 §3): hymnal key first, then folded title and language.
func TestFindDuplicate(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		f.importRepo(func(cs app.ChurchStore) error {
			r := cs.Songs()
			mk := func(sid, title, lang, source, number string, age time.Duration) domain.Song {
				s := testSong(id(sid), title, lang, f.now.Add(age))
				s.HymnalSource, s.HymnalNumber = source, number
				return s
			}
			for _, s := range []domain.Song{
				mk("SD1", "Besar Setia-Mu", "id", "Pelengkap", "12", 2*time.Minute),
				mk("SD2", "Besar setia mu", "id", "", "", time.Minute), // older, same folded title
				mk("SD3", "Great Is Thy Faithfulness", "en", "Pelengkap", "12", 3*time.Minute),
				mk("SD4", "Besar Setia-Mu", "en", "", "", 4*time.Minute),
			} {
				if err := r.Create(ctx, s); err != nil {
					return err
				}
			}
			for name, tc := range map[string]struct {
				hymnal, title, lang string
				want                string
				found               bool
			}{
				"hymnal key wins over the title": {domain.HymnalKey("Pelengkap", "12"), domain.Fold("Besar Setia-Mu"), "id", "SD1", true},
				"oldest of equal hymnal keys":    {domain.HymnalKey("Pelengkap", "12"), "", "en", "SD1", true},
				"title and language":             {"", domain.Fold("BESAR setia  mu"), "id", "SD2", true},
				"same title, other language":     {"", domain.Fold("Besar Setia-Mu"), "en", "SD4", true},
				"a title is not a substring":     {"", domain.Fold("Besar Setiamu"), "id", "", false},
				"unknown hymnal falls back":      {domain.HymnalKey("Other", "1"), domain.Fold("Besar Setia-Mu"), "id", "SD2", true},
				"nothing":                        {"", "", "id", "", false},
				"no title match in the language": {"", domain.Fold("Besar Setia-Mu"), "zh-Hans", "", false},
			} {
				d, ok, err := r.FindDuplicate(ctx, tc.hymnal, tc.title, tc.lang)
				want := domain.SongID(id(tc.want))
				if tc.want == "" {
					want = ""
				}
				if err != nil || ok != tc.found || d.ID != want {
					t.Errorf("%s: got %+v %v %v, want %s %v", name, d, ok, err, tc.want, tc.found)
				}
			}
			d, _, _ := r.FindDuplicate(ctx, "", domain.Fold("Besar Setia-Mu"), "id")
			if d.Title != "Besar setia mu" || d.HymnalSource != "" {
				t.Errorf("reference = %+v", d)
			}
			return errRollbackFixture
		})
	})
}
