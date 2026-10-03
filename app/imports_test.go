// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// stubImporter returns fixed candidates.
type stubImporter struct {
	cands []app.ImportCandidate
	err   error
}

func (s stubImporter) Parse(context.Context, io.Reader, app.ImportHint) ([]app.ImportCandidate, error) {
	return s.cands, s.err
}

func pasteReq(title, text string) app.ImportRequest {
	return app.ImportRequest{Format: "paste", Files: []app.ImportFile{{Name: title, Text: text}}}
}

func (e cenv) batch(req app.ImportRequest) app.BatchView {
	e.t.Helper()
	v, err := e.imports.Create(e.ctx, e.admin, req)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e cenv) decide(batch domain.ImportBatchID, ds ...app.DecisionInput) app.BatchView {
	e.t.Helper()
	v, err := e.imports.Decide(e.ctx, e.admin, batch, ds)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func accept(id domain.ImportCandidateID) app.DecisionInput {
	return app.DecisionInput{ID: id, Decision: domain.DecisionAccept}
}

func (e cenv) songCount() int {
	e.t.Helper()
	l, err := e.songs.List(e.ctx, e.admin, app.SongQuery{})
	if err != nil {
		e.t.Fatal(err)
	}
	return l.Total
}

func importReason(err error) string {
	var u *app.ImportUnreadableError
	var c *app.ImportConflictError
	switch {
	case errors.As(err, &u):
		return u.Reason
	case errors.As(err, &c):
		return "conflict:" + c.Reason
	}
	return fmt.Sprint(err)
}

// IT-I-001: paste, review, apply. Nothing is saved before Apply.
func TestImportPasteReviewApply(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	b := e.batch(pasteReq("Besar Setia-Mu", "1. Besar setia-Mu\nTuhan\n\nReff\nBesar setia-Mu\n\n2. Pagi demi pagi\n\nReff"))
	if len(b.Candidates) != 1 || b.Batch.Status != domain.ImportOpen || e.songCount() != 0 {
		t.Fatalf("created: %+v songs=%d", b, e.songCount())
	}
	c := b.Candidates[0]
	if c.Draft.Title != "Besar Setia-Mu" || len(c.Draft.Sections) != 3 || len(c.Draft.DefaultArrangement) != 4 || c.Decision != domain.DecisionPending {
		t.Errorf("candidate: %+v", c)
	}
	// Language defaults to the church's content language.
	if c.Draft.Language != "id" {
		t.Errorf("language = %q", c.Draft.Language)
	}
	// Pending candidates are not applied.
	res, err := e.imports.Apply(e.ctx, e.admin, b.Batch.ID)
	if err != nil || res.Created != 0 || res.Status != domain.ImportOpen || e.songCount() != 0 {
		t.Fatalf("apply of a pending batch: %+v %v", res, err)
	}
	e.decide(b.Batch.ID, accept(c.ID))
	res, err = e.imports.Apply(e.ctx, e.admin, b.Batch.ID)
	if err != nil || res.Created != 1 || len(res.Failed) != 0 || res.Status != domain.ImportClosed {
		t.Fatalf("apply: %+v %v", res, err)
	}
	// The song is found by search, with the sections and arrangement of the draft.
	l, err := e.songs.List(e.ctx, e.admin, app.SongQuery{Q: "besar setia"})
	if err != nil || l.Total != 1 {
		t.Fatalf("search: %+v %v", l, err)
	}
	song, err := e.songs.Get(e.ctx, e.admin, l.Items[0].ID)
	if err != nil || len(song.Song.Sections) != 3 || len(song.Song.DefaultArrangement) != 4 ||
		song.Song.DefaultArrangement[0] != song.Song.Sections[0].ID || song.Song.DefaultArrangement[1] != song.Song.Sections[1].ID {
		t.Errorf("song: %+v %v", song.Song, err)
	}
	got, _ := e.imports.Get(e.ctx, e.admin, b.Batch.ID)
	if got.Candidates[0].Outcome != domain.OutcomeApplied || got.Candidates[0].AppliedSongID != song.Song.ID {
		t.Errorf("outcome: %+v", got.Candidates[0])
	}
	// Applied candidates cannot be edited or decided again.
	_, err = e.imports.EditDraft(e.ctx, e.admin, b.Batch.ID, c.ID, c.Draft)
	if importReason(err) != "conflict:already_applied" {
		t.Errorf("edit of an applied candidate: %v", err)
	}
	_, err = e.imports.Decide(e.ctx, e.admin, b.Batch.ID, []app.DecisionInput{{ID: c.ID, Decision: domain.DecisionSkip}})
	if importReason(err) != "conflict:already_applied" {
		t.Errorf("decision on an applied candidate: %v", err)
	}
	// A second Apply creates nothing.
	if res, err = e.imports.Apply(e.ctx, e.admin, b.Batch.ID); err != nil || res.Created != 0 || e.songCount() != 1 {
		t.Errorf("second apply: %+v %v songs=%d", res, err, e.songCount())
	}
}

// IT-I-002: one failing candidate does not stop the others; it can be fixed and applied later.
func TestImportPartialFailureAndRetry(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	imp := *e.imports
	imp.Importers = map[domain.ImportFormat]app.Importer{domain.FormatChordPro: stubImporter{cands: []app.ImportCandidate{
		{Draft: oneVerse("Satu")}, {Draft: oneVerse("Dua")}, {Draft: oneVerse("Tiga")}}}}
	b, err := imp.Create(e.ctx, e.admin, app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "x.cho", Text: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	var decisions []app.DecisionInput
	for _, c := range b.Candidates {
		decisions = append(decisions, accept(c.ID))
	}
	if _, err := imp.Decide(e.ctx, e.admin, b.Batch.ID, decisions); err != nil {
		t.Fatal(err)
	}
	// The second candidate becomes invalid behind the application's back.
	bad := b.Candidates[1]
	bad.Draft.Title = strings.Repeat("x", 201)
	bad.Decision = domain.DecisionAccept
	e.write(func(s app.Store) error {
		cs, err := s.ForChurch(ctx, e.church)
		if err != nil {
			return err
		}
		return cs.Imports().UpdateCandidate(ctx, bad.ImportCandidate)
	})
	res, err := imp.Apply(e.ctx, e.admin, b.Batch.ID)
	if err != nil || res.Created != 2 || len(res.Failed) != 1 || res.Failed[0].CandidateID != bad.ID ||
		res.Failed[0].Code != domain.CodeValidationFailed || res.Status != domain.ImportOpen {
		t.Fatalf("apply: %+v %v", res, err)
	}
	if e.songCount() != 2 {
		t.Errorf("songs = %d, want the 2 good ones", e.songCount())
	}
	got, _ := imp.Get(e.ctx, e.admin, b.Batch.ID)
	if c := got.Candidates[1]; c.Outcome != domain.OutcomeFailed || c.ErrorCode != domain.CodeValidationFailed {
		t.Errorf("failed candidate: %+v", c)
	}
	// Applying again retries only the failed one, which fails again.
	if res, _ = imp.Apply(e.ctx, e.admin, b.Batch.ID); res.Created != 0 || len(res.Failed) != 1 || e.songCount() != 2 {
		t.Errorf("retry without a fix: %+v", res)
	}
	// Fix the draft: the failure is cleared and the next Apply creates the song and closes the batch.
	fixed := b.Candidates[1].Draft
	if _, err := imp.EditDraft(e.ctx, e.admin, b.Batch.ID, bad.ID, fixed); err != nil {
		t.Fatal(err)
	}
	got, _ = imp.Get(e.ctx, e.admin, b.Batch.ID)
	if c := got.Candidates[1]; c.Outcome != "" || c.ErrorCode != "" || c.Decision != domain.DecisionAccept {
		t.Errorf("after the edit: %+v", c)
	}
	if res, err = imp.Apply(e.ctx, e.admin, b.Batch.ID); err != nil || res.Created != 1 || res.Status != domain.ImportClosed || e.songCount() != 3 {
		t.Errorf("after the fix: %+v %v songs=%d", res, err, e.songCount())
	}
}

func oneVerse(title string) domain.SongDraft {
	return domain.SongDraft{Language: "id", Title: title, AltTitles: []string{}, DefaultArrangement: []int{},
		Sections: []domain.DraftSection{{Kind: domain.SectionVerse, Number: 1, Text: "lirik " + title}}}
}

// IT-I-003: two simultaneous Applies create every song once, on both dialects.
func TestImportConcurrentApply(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		imp := *e.imports
		var cands []app.ImportCandidate
		for i := range 6 {
			cands = append(cands, app.ImportCandidate{Draft: oneVerse(fmt.Sprintf("Lagu %d", i))})
		}
		imp.Importers = map[domain.ImportFormat]app.Importer{domain.FormatChordPro: stubImporter{cands: cands}}
		b, err := imp.Create(e.ctx, e.admin, app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "x.cho", Text: "x"}}})
		if err != nil {
			t.Fatal(err)
		}
		var ds []app.DecisionInput
		for _, c := range b.Candidates {
			ds = append(ds, accept(c.ID))
		}
		if _, err := imp.Decide(e.ctx, e.admin, b.Batch.ID, ds); err != nil {
			t.Fatal(err)
		}
		results := make([]app.ApplyResult, 2)
		errs := race(2, func(i int) (err error) {
			results[i], err = imp.Apply(e.ctx, e.admin, b.Batch.ID)
			return err
		})
		if ok, others := succeeded(errs); ok != 2 {
			t.Fatalf("applies: %v", others)
		}
		if got := results[0].Created + results[1].Created; got != 6 || e.songCount() != 6 {
			t.Errorf("created %d+%d, songs %d; want each song once", results[0].Created, results[1].Created, e.songCount())
		}
		if got, _ := imp.Get(e.ctx, e.admin, b.Batch.ID); got.Batch.Status != domain.ImportClosed {
			t.Errorf("status = %s", got.Batch.Status)
		}
	})
}

// IT-I-008: an edit and an Apply of the same candidate never mix; exactly one wins.
func TestImportEditVersusApply(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		for round := range 5 {
			b := e.batch(pasteReq(fmt.Sprintf("Lagu %d", round), "satu\n\ndua"))
			c := b.Candidates[0]
			e.decide(b.Batch.ID, accept(c.ID))
			edited := c.Draft
			edited.Title = fmt.Sprintf("Edited %d", round)
			var editErr error
			errs := race(2, func(i int) error {
				if i == 0 {
					_, editErr = e.imports.EditDraft(e.ctx, e.admin, b.Batch.ID, c.ID, edited)
					return nil
				}
				_, err := e.imports.Apply(e.ctx, e.admin, b.Batch.ID)
				return err
			})
			if ok, others := succeeded(errs); ok != 2 {
				t.Fatalf("round %d: %v", round, others)
			}
			after, _ := e.imports.Get(e.ctx, e.admin, b.Batch.ID)
			got := after.Candidates[0]
			l, _ := e.songs.List(e.ctx, e.admin, app.SongQuery{Q: fmt.Sprintf("lagu %d", round)})
			l2, _ := e.songs.List(e.ctx, e.admin, app.SongQuery{Q: fmt.Sprintf("edited %d", round)})
			switch {
			case editErr == nil && got.Outcome == domain.OutcomeApplied && l2.Total == 1 && l.Total == 0:
				// The edit came first; Apply used the edited draft.
			case importReason(editErr) == "conflict:already_applied" && got.Outcome == domain.OutcomeApplied && l.Total == 1 && l2.Total == 0:
				// Apply came first; the edit was refused.
			default:
				t.Errorf("round %d: edit=%v outcome=%q songs(old)=%d songs(edited)=%d", round, editErr, got.Outcome, l.Total, l2.Total)
			}
		}
	})
}

// IT-I-004: decisions, duplicates and merge safety.
func TestImportDecisionsAndMerge(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	existing := e.newSong("Besar Setia-Mu", "id") // V1 C V2, arranged V1 C V2 C
	b := e.batch(pasteReq("besar setia mu", "1. baru satu\n\nReff\nreff baru\n\n3. baru tiga"))
	c := b.Candidates[0]
	if c.DuplicateOf == nil || c.DuplicateOf.ID != existing.Song.ID || c.Decision != domain.DecisionPending {
		t.Fatalf("duplicate: %+v", c)
	}
	// A different language is no duplicate.
	other := e.batch(app.ImportRequest{Format: "paste", Language: "en", Files: []app.ImportFile{{Name: "Besar Setia-Mu", Text: "x"}}})
	if other.Candidates[0].DuplicateOf != nil {
		t.Errorf("other language: %+v", other.Candidates[0].DuplicateOf)
	}
	// merge needs the song and the version of its preview.
	for name, d := range map[string]app.DecisionInput{
		"no target":  {ID: c.ID, Decision: domain.DecisionMerge, MergeTargetVersion: 1},
		"no version": {ID: c.ID, Decision: domain.DecisionMerge, MergeInto: existing.Song.ID},
		"unknown":    {ID: c.ID, Decision: "maybe"},
	} {
		if _, err := e.imports.Decide(e.ctx, e.admin, b.Batch.ID, []app.DecisionInput{d}); invalidField(err) == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Another church's song is not found.
	e2 := newSecondChurch(t, e)
	foreign := e2.newSong("Lain", "id")
	_, err := e.imports.Decide(e.ctx, e.admin, b.Batch.ID, []app.DecisionInput{{ID: c.ID, Decision: domain.DecisionMerge,
		MergeInto: foreign.Song.ID, MergeTargetVersion: 1}})
	if !errors.Is(err, app.ErrNotFound) {
		t.Errorf("merge into another church's song: %v", err)
	}
	// A wrong version is refused when the decision is saved.
	_, err = e.imports.Decide(e.ctx, e.admin, b.Batch.ID, []app.DecisionInput{{ID: c.ID, Decision: domain.DecisionMerge,
		MergeInto: existing.Song.ID, MergeTargetVersion: 9}})
	if importReason(err) != "conflict:target_changed" {
		t.Errorf("stale version: %v", err)
	}

	// The preview shows what Apply does.
	pv, err := e.imports.MergePreview(e.ctx, e.admin, b.Batch.ID, c.ID, existing.Song.ID, false)
	if err != nil || pv.TargetVersion != 1 {
		t.Fatalf("preview: %+v %v", pv, err)
	}
	statuses := ""
	for _, s := range pv.Sections {
		statuses += s.Status[:1]
	}
	if statuses != "uunk" { // verse 1 and the chorus updated, verse 3 new, verse 2 kept
		t.Errorf("preview statuses = %q: %+v", statuses, pv.Sections)
	}
	if pv.Sections[0].OldText != "bait satu" || pv.Sections[0].NewText != "baru satu" {
		t.Errorf("preview texts: %+v", pv.Sections[0])
	}
	pvRemove, _ := e.imports.MergePreview(e.ctx, e.admin, b.Batch.ID, c.ID, existing.Song.ID, true)
	if last := pvRemove.Sections[len(pvRemove.Sections)-1]; last.Status != "removed" {
		t.Errorf("remove preview: %+v", pvRemove.Sections)
	}

	e.decide(b.Batch.ID, app.DecisionInput{ID: c.ID, Decision: domain.DecisionMerge, MergeInto: existing.Song.ID, MergeTargetVersion: pv.TargetVersion})
	// The target is edited after the decision: Apply fails with target_changed and changes nothing.
	title := "Besar Setia-Mu!"
	if _, err := e.songs.Update(e.ctx, e.admin, existing.Song.ID, app.SongChange{Version: 1, Title: &title}); err != nil {
		t.Fatal(err)
	}
	res, err := e.imports.Apply(e.ctx, e.admin, b.Batch.ID)
	if err != nil || res.Merged != 0 || len(res.Failed) != 1 || res.Failed[0].Code != domain.CodeTargetChanged || res.Status != domain.ImportOpen {
		t.Fatalf("apply after the target changed: %+v %v", res, err)
	}
	cur, _ := e.songs.Get(e.ctx, e.admin, existing.Song.ID)
	if cur.Song.Version != 2 || cur.Song.Sections[0].Text != "bait satu" {
		t.Errorf("the failed merge changed the song: %+v", cur.Song)
	}
	// The member looks at the preview again, decides again, and the merge is exactly what the preview said.
	pv, _ = e.imports.MergePreview(e.ctx, e.admin, b.Batch.ID, c.ID, existing.Song.ID, false)
	e.decide(b.Batch.ID, app.DecisionInput{ID: c.ID, Decision: domain.DecisionMerge, MergeInto: existing.Song.ID, MergeTargetVersion: pv.TargetVersion})
	if res, err = e.imports.Apply(e.ctx, e.admin, b.Batch.ID); err != nil || res.Merged != 1 || res.Status != domain.ImportClosed {
		t.Fatalf("apply: %+v %v", res, err)
	}
	cur, _ = e.songs.Get(e.ctx, e.admin, existing.Song.ID)
	s := cur.Song
	if s.Version != 3 || len(s.Sections) != 4 {
		t.Fatalf("merged song: %+v", s)
	}
	// Result order: matched and new in draft order, then the kept section.
	want := []struct {
		kind domain.SectionKind
		num  int
		text string
	}{{domain.SectionVerse, 1, "baru satu"}, {domain.SectionChorus, 0, "reff baru"}, {domain.SectionVerse, 3, "baru tiga"}, {domain.SectionVerse, 2, "bait dua"}}
	for i, w := range want {
		if s.Sections[i].Kind != w.kind || s.Sections[i].Number != w.num || s.Sections[i].Text != w.text {
			t.Errorf("section %d = %+v, want %+v", i, s.Sections[i], w)
		}
	}
	// Matched sections kept their IDs; the old arrangement still points at them (the draft has none).
	if s.Sections[0].ID != existing.Song.Sections[0].ID || s.Sections[1].ID != existing.Song.Sections[1].ID || s.Sections[3].ID != existing.Song.Sections[2].ID {
		t.Errorf("IDs not reused: %v vs %v", sectionIDs(s), sectionIDs(existing.Song))
	}
	if len(s.DefaultArrangement) != 4 || s.DefaultArrangement[0] != s.Sections[0].ID || s.DefaultArrangement[2] != s.Sections[3].ID {
		t.Errorf("arrangement = %v", s.DefaultArrangement)
	}
	if s.Title != "Besar Setia-Mu!" {
		t.Errorf("title = %q", s.Title)
	}
}

// Merge removal respects sections in use, and a deleted target fails the candidate.
func TestImportMergeRemovalAndMissingTarget(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	existing := e.newSong("Besar Setia-Mu", "id")
	b := e.batch(pasteReq("Besar Setia-Mu", "1. satu baru"))
	c := b.Candidates[0]
	merge := func(remove bool) {
		pv, err := e.imports.MergePreview(e.ctx, e.admin, b.Batch.ID, c.ID, existing.Song.ID, remove)
		if err != nil {
			t.Fatal(err)
		}
		e.decide(b.Batch.ID, app.DecisionInput{ID: c.ID, Decision: domain.DecisionMerge, MergeInto: existing.Song.ID,
			MergeTargetVersion: pv.TargetVersion, RemoveUnmatched: remove})
	}
	e.usage.busy = []domain.SectionID{existing.Song.Sections[1].ID} // the chorus is in use
	merge(true)
	res, err := e.imports.Apply(e.ctx, e.admin, b.Batch.ID)
	if err != nil || len(res.Failed) != 1 || res.Failed[0].Code != domain.CodeSectionInUse {
		t.Fatalf("removal of a section in use: %+v %v", res, err)
	}
	// Without removal the same merge works.
	merge(false)
	if res, err = e.imports.Apply(e.ctx, e.admin, b.Batch.ID); err != nil || res.Merged != 1 {
		t.Fatalf("merge without removal: %+v %v", res, err)
	}
	// A target deleted after the decision: not_found.
	b2 := e.batch(pasteReq("Besar Setia-Mu", "1. lagi"))
	e.usage.busy = nil
	pv, _ := e.imports.MergePreview(e.ctx, e.admin, b2.Batch.ID, b2.Candidates[0].ID, existing.Song.ID, false)
	e.decide(b2.Batch.ID, app.DecisionInput{ID: b2.Candidates[0].ID, Decision: domain.DecisionMerge, MergeInto: existing.Song.ID, MergeTargetVersion: pv.TargetVersion})
	if err := e.songs.Delete(e.ctx, e.admin, existing.Song.ID); err != nil {
		t.Fatal(err)
	}
	res, err = e.imports.Apply(e.ctx, e.admin, b2.Batch.ID)
	if err != nil || len(res.Failed) != 1 || res.Failed[0].Code != domain.CodeNotFound {
		t.Errorf("deleted target: %+v %v", res, err)
	}
	got, _ := e.imports.Get(e.ctx, e.admin, b2.Batch.ID)
	if got.Candidates[0].DuplicateOf != nil {
		t.Errorf("a deleted song still counts as a duplicate: %+v", got.Candidates[0].DuplicateOf)
	}
}

func TestImportDuplicatesInBatchAndSkip(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	imp := *e.imports
	a, b, c := oneVerse("Besar Setia-Mu"), oneVerse("besar setia mu"), oneVerse("Besar Setiamu")
	a.HymnalSource, a.HymnalNumber = "Pelengkap", "12"
	d := oneVerse("Lain")
	d.HymnalSource, d.HymnalNumber = "pelengkap", "12" // same hymnal key as a
	imp.Importers = map[domain.ImportFormat]app.Importer{domain.FormatChordPro: stubImporter{cands: []app.ImportCandidate{
		{Draft: a}, {Draft: b}, {Draft: c}, {Draft: d}}}}
	batch, err := imp.Create(e.ctx, e.admin, app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "x", Text: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	has := func(c app.CandidateView) bool {
		for _, w := range c.Warnings {
			if w == domain.WarnDuplicateInBatch {
				return true
			}
		}
		return false
	}
	// a, b (same folded title) and d (same hymnal key as a) warn; "besar setiamu" does not.
	if !has(batch.Candidates[0]) || !has(batch.Candidates[1]) || has(batch.Candidates[2]) || !has(batch.Candidates[3]) {
		t.Errorf("duplicate_in_batch: %v %v %v %v", batch.Candidates[0].Warnings, batch.Candidates[1].Warnings,
			batch.Candidates[2].Warnings, batch.Candidates[3].Warnings)
	}
	// Skipping everything closes the batch on Apply.
	var ds []app.DecisionInput
	for _, c := range batch.Candidates {
		ds = append(ds, app.DecisionInput{ID: c.ID, Decision: domain.DecisionSkip})
	}
	if _, err := imp.Decide(e.ctx, e.admin, batch.Batch.ID, ds); err != nil {
		t.Fatal(err)
	}
	res, err := imp.Apply(e.ctx, e.admin, batch.Batch.ID)
	if err != nil || res.Skipped != 4 || res.Created != 0 || res.Status != domain.ImportClosed || e.songCount() != 0 {
		t.Errorf("apply of skipped candidates: %+v %v", res, err)
	}
	// Changing a mind about a skipped candidate in a closed batch re-opens it.
	got, _ := imp.Decide(e.ctx, e.admin, batch.Batch.ID, []app.DecisionInput{accept(batch.Candidates[2].ID)})
	if got.Batch.Status != domain.ImportOpen {
		t.Errorf("status = %s, want open", got.Batch.Status)
	}
}

// IT-I-005: permissions and limits.
func TestImportPermissionsAndLimits(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	team, _ := e.member("team@example.org")
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID)
	b := e.batch(pasteReq("T", "a"))
	for name, call := range map[string]func() error{
		"create": func() error { _, err := e.imports.Create(e.ctx, team, pasteReq("T", "a")); return err },
		"get":    func() error { _, err := e.imports.Get(e.ctx, team, b.Batch.ID); return err },
		"open":   func() error { _, err := e.imports.Open(e.ctx, team); return err },
		"edit": func() error {
			_, err := e.imports.EditDraft(e.ctx, team, b.Batch.ID, b.Candidates[0].ID, oneVerse("x"))
			return err
		},
		"decide": func() error { _, err := e.imports.Decide(e.ctx, team, b.Batch.ID, nil); return err },
		"preview": func() error {
			_, err := e.imports.MergePreview(e.ctx, team, b.Batch.ID, b.Candidates[0].ID, "x", false)
			return err
		},
		"apply":   func() error { _, err := e.imports.Apply(e.ctx, team, b.Batch.ID); return err },
		"discard": func() error { return e.imports.Discard(e.ctx, team, b.Batch.ID) },
	} {
		if err := call(); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("%s by a member without library.edit: %v", name, err)
		}
	}
	// Another member holding the scope sees the first member's batch.
	if got, err := e.imports.Get(e.ctx, editor, b.Batch.ID); err != nil || got.Batch.ID != b.Batch.ID {
		t.Errorf("a second editor: %v", err)
	}
	if open, err := e.imports.Open(e.ctx, editor); err != nil || len(open) != 1 {
		t.Errorf("open batches: %v %v", open, err)
	}

	req := func(n int) app.ImportRequest {
		r := app.ImportRequest{Format: "chordpro"}
		for i := range n {
			r.Files = append(r.Files, app.ImportFile{Name: fmt.Sprintf("%d.cho", i), Text: "{title: T}\nx"})
		}
		return r
	}
	for name, tc := range map[string]struct {
		req   app.ImportRequest
		check func(error) bool
	}{
		"no files":           {req(0), func(err error) bool { return invalidField(err) == "files" }},
		"201 files":          {req(201), func(err error) bool { return invalidField(err) == "files" }},
		"unknown format":     {app.ImportRequest{Format: "docx", Files: req(1).Files}, func(err error) bool { return invalidField(err) == "format" }},
		"unknown language":   {app.ImportRequest{Format: "chordpro", Language: "fr", Files: req(1).Files}, func(err error) bool { return invalidField(err) == "language" }},
		"two pasted texts":   {app.ImportRequest{Format: "paste", Files: req(2).Files}, func(err error) bool { return invalidField(err) == "files" }},
		"no title":           {pasteReq(" ", "a"), func(err error) bool { return invalidField(err) == "files.0.name" }},
		"200,001 characters": {pasteReq("T", strings.Repeat("é", 200_001)), func(err error) bool { return invalidField(err) == "files.0.text" }},
		"over 5 MiB":         {app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "a", Text: strings.Repeat("x", 3<<20)}, {Name: "b", Text: strings.Repeat("x", 3<<20)}}}, func(err error) bool { return errors.Is(err, app.ErrImportTooLarge) }},
		"nothing readable":   {app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "a", Text: strings.Repeat("x", 1<<20+1)}}}, func(err error) bool { return importReason(err) == app.ImportFileTooLarge }},
		"no lyrics":          {app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{{Name: "a", Text: "# nothing"}}}, func(err error) bool { return importReason(err) == app.ImportNoSong }},
	} {
		if _, err := e.imports.Create(e.ctx, e.admin, tc.req); !tc.check(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A file over 1 MiB is rejected alone; the valid ones still produce candidates.
	mixed := app.ImportRequest{Format: "chordpro", Files: []app.ImportFile{
		{Name: "big.cho", Text: strings.Repeat("x", 1<<20+1)}, {Name: "ok.cho", Text: "{title: Ok}\nlyrics"}}}
	got, err := e.imports.Create(e.ctx, e.admin, mixed)
	if err != nil || len(got.Candidates) != 1 || len(got.Rejected) != 1 || got.Rejected[0].Name != "big.cho" || got.Rejected[0].Reason != app.ImportFileTooLarge {
		t.Errorf("mixed files: %+v %v", got, err)
	}
	// More than 500 candidates are refused.
	imp := *e.imports
	var many []app.ImportCandidate
	for i := range 501 {
		many = append(many, app.ImportCandidate{Draft: oneVerse(fmt.Sprint("S", i))})
	}
	imp.Importers = map[domain.ImportFormat]app.Importer{domain.FormatChordPro: stubImporter{cands: many}}
	if _, err := imp.Create(e.ctx, e.admin, req(1)); invalidField(err) != "files" {
		t.Errorf("501 candidates: %v", err)
	}
	// An importer error that is not about the file is passed on.
	imp.Importers[domain.FormatChordPro] = stubImporter{err: errors.New("boom")}
	if _, err := imp.Create(e.ctx, e.admin, req(1)); err == nil || err.Error() != "boom" {
		t.Errorf("importer failure: %v", err)
	}
	// Rejected songs of a multi-song file are listed with their index.
	imp.Importers[domain.FormatChordPro] = stubImporter{cands: []app.ImportCandidate{
		{Draft: oneVerse("A")}, {Reject: app.ImportTooComplex}, {Draft: oneVerse("C")}}}
	if got, err := imp.Create(e.ctx, e.admin, req(1)); err != nil || len(got.Candidates) != 2 || len(got.Rejected) != 1 ||
		got.Rejected[0].SongIndex != 1 || got.Rejected[0].Reason != app.ImportTooComplex {
		t.Errorf("multi-song: %+v %v", got, err)
	}
}

// IT-I-006: another church sees nothing of the batch.
func TestImportIsolation(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	b := e.batch(pasteReq("T", "a"))
	e2 := newSecondChurch(t, e)
	c := b.Candidates[0]
	for name, call := range map[string]func() error{
		"get": func() error { _, err := e2.imports.Get(e2.ctx, e2.admin, b.Batch.ID); return err },
		"edit": func() error {
			_, err := e2.imports.EditDraft(e2.ctx, e2.admin, b.Batch.ID, c.ID, oneVerse("x"))
			return err
		},
		"decide": func() error {
			_, err := e2.imports.Decide(e2.ctx, e2.admin, b.Batch.ID, []app.DecisionInput{accept(c.ID)})
			return err
		},
		"preview": func() error {
			_, err := e2.imports.MergePreview(e2.ctx, e2.admin, b.Batch.ID, c.ID, "x", false)
			return err
		},
		"apply":   func() error { _, err := e2.imports.Apply(e2.ctx, e2.admin, b.Batch.ID); return err },
		"discard": func() error { return e2.imports.Discard(e2.ctx, e2.admin, b.Batch.ID) },
	} {
		if err := call(); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s from another church: %v", name, err)
		}
	}
	if open, _ := e2.imports.Open(e2.ctx, e2.admin); len(open) != 0 {
		t.Errorf("open batches of another church: %v", open)
	}
	if got, err := e.imports.Get(e.ctx, e.admin, b.Batch.ID); err != nil || got.Candidates[0].Decision != domain.DecisionPending {
		t.Errorf("the batch was changed: %+v %v", got, err)
	}
}

// IT-I-007: working data is deleted after 7 days without a change.
func TestImportCleanup(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	cl := &app.Cleanup{Tx: e.db, Clock: e.clock}
	old := e.batch(pasteReq("Old", "a"))
	closed := e.batch(pasteReq("Closed", "a"))
	e.decide(closed.Batch.ID, app.DecisionInput{ID: closed.Candidates[0].ID, Decision: domain.DecisionSkip})
	if _, err := e.imports.Apply(e.ctx, e.admin, closed.Batch.ID); err != nil {
		t.Fatal(err)
	}
	e.clock.add(6 * 24 * time.Hour)
	touched := e.batch(pasteReq("Recent", "a"))
	e.clock.add(25 * time.Hour) // old: 7 days and an hour; recent: one day and an hour
	if err := cl.Hourly(ctx); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int { return sqlstoretest.Count(t, e.db, "SELECT count(*) FROM "+table) }
	if count("import_batches") != 1 || count("import_candidates") != 1 {
		t.Fatalf("batches %d candidates %d, want only the recent one", count("import_batches"), count("import_candidates"))
	}
	if _, err := e.imports.Get(e.ctx, e.admin, touched.Batch.ID); err != nil {
		t.Errorf("recent batch: %v", err)
	}
	if _, err := e.imports.Get(e.ctx, e.admin, old.Batch.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("old batch: %v", err)
	}
	// Editing a candidate counts as a change.
	e.clock.add(6 * 24 * time.Hour)
	if _, err := e.imports.EditDraft(e.ctx, e.admin, touched.Batch.ID, touched.Candidates[0].ID, oneVerse("Recent")); err != nil {
		t.Fatal(err)
	}
	e.clock.add(2 * 24 * time.Hour)
	if err := cl.Hourly(ctx); err != nil || count("import_batches") != 1 {
		t.Errorf("an edited batch must stay: %d %v", count("import_batches"), err)
	}
	// Discarding deletes at once.
	if err := e.imports.Discard(e.ctx, e.admin, touched.Batch.ID); err != nil || count("import_batches") != 0 || count("import_candidates") != 0 {
		t.Errorf("discard: %v", err)
	}
	if err := e.imports.Discard(e.ctx, e.admin, touched.Batch.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("second discard: %v", err)
	}
}

// Edits of a draft are validated like a typed song, and a bad one changes nothing.
func TestImportEditDraft(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	b := e.batch(pasteReq("Lagu", "satu\n\ndua"))
	c := b.Candidates[0]
	bad := c.Draft
	bad.Title = ""
	if _, err := e.imports.EditDraft(e.ctx, e.admin, b.Batch.ID, c.ID, bad); invalidField(err) != "title" {
		t.Errorf("empty title: %v", err)
	}
	ok := c.Draft
	ok.Title, ok.Lyricist = "Lagu Baru", "Seseorang"
	got, err := e.imports.EditDraft(e.ctx, e.admin, b.Batch.ID, c.ID, ok)
	if err != nil || got.Draft.Title != "Lagu Baru" || got.Draft.Lyricist != "Seseorang" {
		t.Errorf("edit: %+v %v", got, err)
	}
	// The duplicate is recomputed.
	e.newSong("Lagu Baru", "id")
	again, _ := e.imports.Get(e.ctx, e.admin, b.Batch.ID)
	if again.Candidates[0].DuplicateOf == nil {
		t.Error("the edited title should now match the existing song")
	}
	if _, err := e.imports.EditDraft(e.ctx, e.admin, b.Batch.ID, domain.ImportCandidateID(e.ids.NewID()), ok); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown candidate: %v", err)
	}
}

// newSecondChurch adds a church of its own to the same database, with an editor who can import.
func newSecondChurch(t *testing.T, e cenv) cenv {
	t.Helper()
	e2 := e
	e2.church = domain.ChurchID(e.ids.NewID())
	e2.ctx = app.WithTenant(ctx, e2.church)
	uid, now := domain.UserID(e.ids.NewID()), e.clock.Now()
	rm, _ := domain.ReadyMade(domain.OriginEditor)
	e2.admin = &domain.Session{UserID: uid}
	e2.write(func(s app.Store) error {
		c := domain.Church{ID: e2.church, Name: "Other", DefaultUILanguage: "en", DefaultLanguage: "id", DefaultTranslationID: "01M3XY2HBEKN8PETK6KK6A7NB2",
			TimeZone: "UTC", Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: now, UpdatedAt: now}
		if err := s.Churches().Create(ctx, c); err != nil {
			return err
		}
		if err := s.Users().Create(ctx, domain.User{ID: uid, Name: "Other", Email: "other@example.org", PasswordHash: "x", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		cs, err := s.ForChurch(ctx, e2.church)
		if err != nil {
			return err
		}
		role := domain.Role{ID: domain.RoleID(e.ids.NewID()), Name: "Editor", Origin: domain.OriginEditor, Scopes: domain.NewScopeSet(rm.Scopes...), CreatedAt: now, UpdatedAt: now}
		if err := cs.Roles().Create(ctx, role); err != nil {
			return err
		}
		return cs.Memberships().Create(ctx, domain.Membership{ID: domain.MembershipID(e.ids.NewID()), UserID: uid, RoleIDs: []domain.RoleID{role.ID}, CreatedAt: now})
	})
	return e2
}
