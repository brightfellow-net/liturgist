// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// renv is a church with a liturgy, an editor (b, who submits) and the admin (a,
// who also holds liturgy.approve).
type renv struct {
	uenv
	team *domain.Session // a member with no role
}

func newReviewEnv(t *testing.T, db *sqlstore.DB) renv {
	t.Helper()
	u := newUndoEnv(t, db)
	team, _ := u.member("team@example.org")
	u.pray("Opening prayer")
	return renv{uenv: u, team: team}
}

func (r renv) review(sess *domain.Session, act domain.ReviewAction, seq *int, note string) (app.LiturgyView, error) {
	return r.liturgies.Review(r.ctx, sess, r.lid, app.ReviewInput{Action: act, EditSeq: seq, Note: note})
}

func (r renv) must(sess *domain.Session, act domain.ReviewAction, seq *int, note string) app.LiturgyView {
	r.t.Helper()
	v, err := r.review(sess, act, seq, note)
	if err != nil {
		r.t.Fatalf("%s: %v", act, err)
	}
	return v
}

func (r renv) state() domain.LiturgyState { return r.liturgy(r.lid).Liturgy.State }

func (r renv) seq() *int { return ptr(r.liturgy(r.lid).Liturgy.EditSeq) }

func (r renv) changes(sess *domain.Session) []app.StateChangeView {
	r.t.Helper()
	p, err := r.liturgies.StateChanges(r.ctx, sess, r.lid, 0, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	return p.Items
}

func isInvalidTransition(err error, state domain.LiturgyState) bool {
	var e *app.InvalidTransitionError
	return errors.As(err, &e) && e.State == state
}

// IT-R-001: the whole round, with notes, locks and the editor's actions.
func TestReviewRound(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		v := r.must(r.b, domain.ActionSubmit, nil, "  ready \r\n ")
		if v.Liturgy.State != domain.StateInReview {
			t.Fatalf("after submit: %s", v.Liturgy.State)
		}
		if v.Actions.Edit || v.Actions.Submit || v.Actions.Approve {
			t.Errorf("editor's actions in review: %+v", v.Actions)
		}
		// Locked: edits are refused, the history is not.
		if _, err := r.liturgies.AddItem(r.ctx, r.b, r.lid, app.ItemInput{LiturgyVersion: v.Liturgy.Version, Title: "x", Type: domain.ItemPrayer}); !errors.Is(err, app.ErrLiturgyLocked) {
			t.Fatalf("edit in review: %v", err)
		}
		if v.Review == nil || v.Review.LastChange == nil || v.Review.LastChange.Change.Note != "ready" ||
			v.Review.LastChange.UserName != "b" {
			t.Fatalf("last change: %+v", v.Review)
		}

		v = r.must(r.a, domain.ActionRequestChanges, ptr(v.Liturgy.EditSeq), "Fix the prayer")
		if v.Liturgy.State != domain.StateNeedsRevision || !v.Actions.Edit || !v.Actions.Submit {
			t.Fatalf("needs_revision: %s %+v", v.Liturgy.State, v.Actions)
		}
		x := r.pray("Second")
		r.text(r.b, x, "words")
		r.must(r.b, domain.ActionSubmit, nil, "")
		v = r.must(r.a, domain.ActionApprove, r.seq(), "")
		if v.Liturgy.State != domain.StateApproved {
			t.Fatalf("approved: %s", v.Liturgy.State)
		}
		if got := len(r.changes(r.a)); got != 4 {
			t.Errorf("%d state changes, want 4", got)
		}
	})
}

// IT-R-006: the submitter may approve; reopen records itself.
func TestReviewSelfApprovalAndReopen(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.must(r.a, domain.ActionSubmit, nil, "")
		v := r.must(r.a, domain.ActionApprove, r.seq(), "ok")
		if v.Liturgy.State != domain.StateApproved || v.Actions.Edit || !v.Actions.Reopen {
			t.Fatalf("approved: %+v %+v", v.Liturgy.State, v.Actions)
		}
		v = r.must(r.a, domain.ActionReopen, nil, "one more change")
		if v.Liturgy.State != domain.StateDraft || !v.Actions.Edit {
			t.Fatalf("reopened: %+v", v.Liturgy.State)
		}
		ch := r.changes(r.a)
		if len(ch) != 3 || ch[0].Change.To != domain.StateDraft || ch[0].Change.From != domain.StateApproved ||
			ch[0].Change.Note != "one more change" || ch[2].Change.From != domain.StateDraft || ch[2].Change.To != domain.StateInReview {
			t.Fatalf("history newest first: %+v", ch)
		}
		for i, c := range ch {
			if c.UserName != "Admin" || c.Change.EditSeq != *r.seq() {
				t.Errorf("row %d: %+v", i, c)
			}
		}
	})
}

// TC-R-007, IT-R-002: the order of checks and the stale review.
func TestReviewChecks(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)

		// 404 before 403: a member with no liturgy scope.
		if _, err := r.review(r.team, domain.ActionSubmit, nil, ""); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member: %v", err)
		}
		// 403: an editor may not approve; an approver without edit may not submit.
		if _, err := r.review(r.b, domain.ActionApprove, r.seq(), ""); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("editor approving: %v", err)
		}
		// 422: missing edit_seq and a long note come before the state check.
		var inv *domain.InvalidInputError
		if _, err := r.review(r.a, domain.ActionApprove, nil, ""); !errors.As(err, &inv) || inv.Field != "edit_seq" {
			t.Errorf("missing edit_seq: %v", err)
		}
		if _, err := r.review(r.b, domain.ActionSubmit, nil, strings.Repeat("é", 501)); !errors.As(err, &inv) || inv.Field != "note" {
			t.Errorf("long note: %v", err)
		}
		if v, err := r.review(r.b, domain.ActionSubmit, nil, strings.Repeat("é", 500)); err != nil || v.Liturgy.State != domain.StateInReview {
			t.Errorf("500 runes: %v", err)
		}
		// 409 invalid_transition: in review a second submit, reopen, approve of a draft ...
		for _, act := range []domain.ReviewAction{domain.ActionSubmit, domain.ActionReopen} {
			if _, err := r.review(r.a, act, r.seq(), ""); !isInvalidTransition(err, domain.StateInReview) {
				t.Errorf("%s in review: %v", act, err)
			}
		}
		// The state wins over a stale edit_seq.
		if _, err := r.review(r.a, domain.ActionSubmit, ptr(-5), ""); !isInvalidTransition(err, domain.StateInReview) {
			t.Errorf("state before seq: %v", err)
		}
		// 409 review_stale: a wrong edit_seq.
		if _, err := r.review(r.a, domain.ActionApprove, ptr(*r.seq()-1), ""); !errors.Is(err, app.ErrReviewStale) {
			t.Errorf("stale approve: %v", err)
		}
		if r.state() != domain.StateInReview {
			t.Fatalf("a refused request changed the state to %s", r.state())
		}
		if _, err := r.review(r.a, "publish", nil, ""); err == nil {
			t.Error("publish is step 5")
		}
		if got := len(r.changes(r.a)); got != 1 {
			t.Errorf("%d state changes after refusals, want 1", got)
		}
	})
}

// TC-R-003, P-69: nothing unfinished goes into review.
func TestReviewSubmitGate(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		lid := e.draft()
		submit := func() error {
			_, err := e.liturgies.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: domain.ActionSubmit})
			return err
		}
		if err := submit(); !errors.Is(err, app.ErrEmptyLiturgy) {
			t.Fatalf("empty: %v", err)
		}
		song := e.addItem(lid, domain.ItemSong, "Song")
		reading := e.addItem(lid, domain.ItemReading, "Reading")
		var hp *app.HasProblemsError
		if err := submit(); !errors.As(err, &hp) || len(hp.Problems) != 2 || hp.Problems[0].ItemID != song || hp.Problems[1].ItemID != reading {
			t.Fatalf("problems: %v %+v", err, hp)
		}
		if e.liturgy(lid).Liturgy.State != domain.StateDraft {
			t.Fatal("a refused submit changed the state")
		}
		for _, id := range []domain.ItemID{song, reading} {
			v := e.liturgy(lid)
			if _, err := e.liturgies.RemoveItem(e.ctx, e.admin, lid, id, v.Liturgy.Version); err != nil {
				t.Fatal(err)
			}
		}
		e.addItem(lid, domain.ItemPrayer, "Prayer")
		if err := submit(); err != nil {
			t.Fatalf("finished liturgy: %v", err)
		}
	})
}

// IT-R-005: a transition starts a new undo window.
func TestReviewUndoFloor(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		x := r.pray("X")
		r.text(r.b, x, "before review")
		r.must(r.b, domain.ActionSubmit, nil, "")
		l := r.liturgy(r.lid).Liturgy
		if l.UndoFloorSeq != l.EditSeq {
			t.Fatalf("floor %d, edit_seq %d", l.UndoFloorSeq, l.EditSeq)
		}
		r.must(r.a, domain.ActionRequestChanges, r.seq(), "")
		refused(t, errOf(r.undo(r.b)), app.UndoNothingToUndo)
		r.text(r.b, x, "after review")
		r.mustUndo(r.b)
		if got := r.mustItem(x).Text; got != "before review" {
			t.Errorf("text %q", got)
		}
		// Undo meeting a locked liturgy is liturgy_locked, never changed_since.
		r.must(r.b, domain.ActionSubmit, nil, "")
		if _, err := r.undo(r.b); !errors.Is(err, app.ErrLiturgyLocked) {
			t.Errorf("undo in review: %v", err)
		}
		if _, err := r.redo(r.b); !errors.Is(err, app.ErrLiturgyLocked) {
			t.Errorf("redo in review: %v", err)
		}
	})
}

func errOf[T any](_ T, err error) error { return err }

// IT-R-009, IT-R-012: review data needs a liturgy scope, whatever the state.
func TestReviewVisibility(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.must(r.b, domain.ActionSubmit, nil, "")
		// Step 5 publishes; here the state is set directly.
		r.sql("UPDATE liturgies SET state = 'published' WHERE id = ?", string(r.lid))
		v, err := r.liturgies.Get(r.ctx, r.team, r.lid)
		if err != nil {
			t.Fatal(err)
		}
		if v.Review != nil || v.Actions != (app.LiturgyActions{}) {
			t.Errorf("a team member sees review data on a published liturgy: %+v %+v", v.Review, v.Actions)
		}
		if _, err := r.liturgies.StateChanges(r.ctx, r.team, r.lid, 0, 0); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member: %v", err)
		}
		if _, err := r.liturgies.StateChanges(r.ctx, r.b, r.lid, 0, 0); err != nil {
			t.Errorf("editor: %v", err)
		}
		if _, err := r.review(r.team, domain.ActionReopen, nil, ""); !isNotFound(err, app.ReasonNotVisible) && !errors.Is(err, app.ErrForbidden) {
			t.Errorf("team member reopening: %v", err)
		}
	})
}

// Another church never sees or moves this liturgy: church A's admin has no
// membership in church B, and B's context finds no liturgy of A's.
func TestReviewOtherChurch(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		bctx := app.WithTenant(ctx, r.oldChurch("id"))
		if _, err := r.liturgies.Review(bctx, r.a, r.lid, app.ReviewInput{Action: domain.ActionSubmit}); err == nil {
			t.Error("review of church A's liturgy through church B")
		}
		if _, err := r.liturgies.StateChanges(bctx, r.a, r.lid, 0, 0); err == nil {
			t.Error("history of church A's liturgy through church B")
		}
		if r.state() != domain.StateDraft {
			t.Error("state changed")
		}
	})
}

// IT-R-003: two approvers, one winner.
func TestReviewSimultaneous(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.must(r.b, domain.ActionSubmit, nil, "")
		seq := r.seq()
		var (
			wg   sync.WaitGroup
			errs [2]error
		)
		for i, act := range []domain.ReviewAction{domain.ActionApprove, domain.ActionRequestChanges} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = r.review(r.a, act, seq, "")
			}()
		}
		wg.Wait()
		wins := 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case isInvalidTransition(err, domain.StateApproved) || isInvalidTransition(err, domain.StateNeedsRevision):
			default:
				t.Errorf("loser: %v", err)
			}
		}
		if wins != 1 || len(r.changes(r.a)) != 2 {
			t.Fatalf("%d winners, %d state changes", wins, len(r.changes(r.a)))
		}
	})
}

// raceClock runs hook the first time the use case asks for the time, which is
// after its early checks and before its last statements: a forced interleaving
// on PostgreSQL, where a second transaction can commit in between.
type raceClock struct {
	*clock
	once sync.Once
	hook func()
}

func (c *raceClock) Now() time.Time {
	c.once.Do(c.hook)
	return c.clock.Now()
}

func postgresOnly(t *testing.T, db *sqlstore.DB) {
	t.Helper()
	if db.DialectName() != "postgres" {
		t.Skip("one writer at a time on SQLite: the interleaving cannot happen")
	}
}

// IT-R-004: an edit that passed its early state check loses to a transition that
// commits first; it ends in liturgy_locked, not 404 or 500, and writes nothing.
func TestEditLosesToTransition(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		postgresOnly(t, db)
		r := newReviewEnv(t, db)
		x := r.pray("X")
		before := len(r.history())
		slow := *r.liturgies
		slow.Clock = &raceClock{clock: r.clock, hook: func() { r.must(r.a, domain.ActionSubmit, nil, "") }}
		text := "late"
		_, err := slow.UpdateItem(r.ctx, r.b, r.lid, x, app.ItemChange{Version: r.mustItem(x).Version, Text: &text})
		if !errors.Is(err, app.ErrLiturgyLocked) {
			t.Fatalf("lost race: %v", err)
		}
		if got := r.mustItem(x).Text; got != "" {
			t.Errorf("the edit was written: %q", got)
		}
		if len(r.history()) != before || r.state() != domain.StateInReview {
			t.Errorf("history %d rows (was %d), state %s", len(r.history()), before, r.state())
		}
		l := r.liturgy(r.lid).Liturgy
		if l.UndoFloorSeq != l.EditSeq {
			t.Errorf("floor %d, edit_seq %d", l.UndoFloorSeq, l.EditSeq)
		}
	})
}

// IT-R-011: an edit between the submit check and its update (P-69).
func TestSubmitLosesToEdit(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		postgresOnly(t, db)
		e := newChurch(t, db, nil)
		lid := e.draft()
		only := e.addItem(lid, domain.ItemPrayer, "Only")
		slow := *e.liturgies
		slow.Clock = &raceClock{clock: e.clock, hook: func() {
			if _, err := e.liturgies.RemoveItem(e.ctx, e.admin, lid, only, e.liturgy(lid).Liturgy.Version); err != nil {
				t.Error(err)
			}
		}}
		_, err := slow.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: domain.ActionSubmit})
		if !errors.Is(err, app.ErrReviewStale) {
			t.Fatalf("submit lost to an edit: %v", err)
		}
		if e.liturgy(lid).Liturgy.State != domain.StateDraft || len(e.liturgy(lid).Items) != 0 {
			t.Error("an empty liturgy went into review")
		}
	})
}

// Deleting a liturgy takes its state changes with it.
func TestReviewDeleteCascades(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.must(r.b, domain.ActionSubmit, nil, "")
		if n, err := db.CountForTest(r.ctx, "liturgy_state_changes"); err != nil || n != 1 {
			t.Fatalf("before: %d %v", n, err)
		}
		if err := r.liturgies.Delete(r.ctx, r.a, r.lid); err != nil {
			t.Fatal(err)
		}
		if n, err := db.CountForTest(r.ctx, "liturgy_state_changes"); err != nil || n != 0 {
			t.Fatalf("after: %d %v", n, err)
		}
	})
}

// IT-R-004, free-running: edits and a submit race 60 times; whatever the order,
// a locked liturgy has no history row above its undo floor, and an edit that
// lost says liturgy_locked (or a version/stale conflict), never anything else.
func TestEditAndSubmitRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		for round := 0; round < 60; round++ {
			lid := r.draft()
			x := r.addItem(lid, domain.ItemPrayer, "X")
			item, err := r.liturgies.Get(r.ctx, r.a, lid)
			if err != nil {
				t.Fatal(err)
			}
			version := item.Items[0].Item.Version
			var wg sync.WaitGroup
			var editErr, submitErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				text := "late"
				_, editErr = r.liturgies.UpdateItem(r.ctx, r.b, lid, x, app.ItemChange{Version: version, Text: &text})
			}()
			go func() {
				defer wg.Done()
				_, submitErr = r.liturgies.Review(r.ctx, r.b, lid, app.ReviewInput{Action: domain.ActionSubmit})
			}()
			wg.Wait()
			var vc *app.VersionConflictError
			switch {
			case editErr == nil, errors.Is(editErr, app.ErrLiturgyLocked), errors.As(editErr, &vc):
			default:
				t.Fatalf("round %d: edit: %v", round, editErr)
			}
			if submitErr != nil && !errors.Is(submitErr, app.ErrReviewStale) {
				t.Fatalf("round %d: submit: %v", round, submitErr)
			}
			l := r.liturgy(lid).Liturgy
			if submitErr == nil {
				if l.State != domain.StateInReview || l.UndoFloorSeq != l.EditSeq {
					t.Fatalf("round %d: state %s, floor %d, edit_seq %d", round, l.State, l.UndoFloorSeq, l.EditSeq)
				}
				views, err := r.liturgies.Edits(r.ctx, r.a, lid, domain.MaxLiturgyQuery)
				if err != nil {
					t.Fatal(err)
				}
				if views[0].Edit.Seq > l.UndoFloorSeq {
					t.Fatalf("round %d: history row %d above the floor %d in a locked liturgy", round, views[0].Edit.Seq, l.UndoFloorSeq)
				}
			}
		}
	})
}
