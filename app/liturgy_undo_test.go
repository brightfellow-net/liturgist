// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// uenv is a church with a liturgy and two people who may edit it: a is the
// admin, b an editor.
type uenv struct {
	cenv
	lid  domain.LiturgyID
	a, b *domain.Session
}

func newUndoEnv(t *testing.T, db *sqlstore.DB) uenv {
	t.Helper()
	e := newChurch(t, db, nil)
	b, _ := e.member("b@example.org", e.role(domain.OriginEditor).ID)
	return uenv{cenv: e, lid: e.draft(), a: e.admin, b: b}
}

func (u uenv) item(id domain.ItemID) (domain.Item, bool) {
	u.t.Helper()
	for _, v := range u.liturgy(u.lid).Items {
		if v.Item.ID == id {
			return v.Item, true
		}
	}
	return domain.Item{}, false
}

func (u uenv) mustItem(id domain.ItemID) domain.Item {
	u.t.Helper()
	it, ok := u.item(id)
	if !ok {
		u.t.Fatalf("item %s is gone", id)
	}
	return it
}

func (u uenv) order() []domain.ItemID {
	var out []domain.ItemID
	for _, v := range u.liturgy(u.lid).Items {
		out = append(out, v.Item.ID)
	}
	return out
}

// fresh starts clean stacks: whatever was set up so far is below the undo floor.
func (u uenv) fresh() {
	u.t.Helper()
	u.sql("UPDATE liturgies SET undo_floor_seq = edit_seq WHERE id = ?", string(u.lid))
}

func (u uenv) pray(title string) domain.ItemID { return u.addItem(u.lid, domain.ItemPrayer, title) }

// text sets the text of an item as sess, from the version it has now.
func (u uenv) text(sess *domain.Session, id domain.ItemID, text string) {
	u.t.Helper()
	if _, err := u.liturgies.UpdateItem(u.ctx, sess, u.lid, id, app.ItemChange{Version: u.mustItem(id).Version, Text: &text}); err != nil {
		u.t.Fatal(err)
	}
}

func (u uenv) undo(sess *domain.Session) (app.UndoResult, error) {
	return u.liturgies.Undo(u.ctx, sess, u.lid)
}

func (u uenv) redo(sess *domain.Session) (app.UndoResult, error) {
	return u.liturgies.Redo(u.ctx, sess, u.lid)
}

func (u uenv) mustUndo(sess *domain.Session) app.UndoResult {
	u.t.Helper()
	r, err := u.undo(sess)
	if err != nil {
		u.t.Fatalf("undo: %v", err)
	}
	return r
}

func (u uenv) mustRedo(sess *domain.Session) app.UndoResult {
	u.t.Helper()
	r, err := u.redo(sess)
	if err != nil {
		u.t.Fatalf("redo: %v", err)
	}
	return r
}

// refused checks that err is an undo refusal for the reason.
func refused(t *testing.T, err error, reason string) {
	t.Helper()
	var r *app.UndoRefusedError
	if !errors.As(err, &r) || r.Reason != reason {
		t.Fatalf("want undo_refused (%s), got %v", reason, err)
	}
}

// history returns the rows oldest first.
func (u uenv) history() []domain.Edit {
	u.t.Helper()
	views, err := u.liturgies.Edits(u.ctx, u.a, u.lid, domain.MaxLiturgyQuery)
	if err != nil {
		u.t.Fatal(err)
	}
	out := make([]domain.Edit, len(views))
	for i, v := range views {
		out[len(views)-1-i] = v.Edit
	}
	return out
}

// noGaps checks that seq runs without a gap (a refused undo rolls its number back);
// the history lists at most 100 rows, so a longer one is checked from its first row on.
func (u uenv) noGaps() {
	u.t.Helper()
	rows := u.history()
	for i, e := range rows {
		if e.Seq != rows[0].Seq+i {
			u.t.Fatalf("seq %d at position %d (the first is %d): a gap in the history", e.Seq, i+1, rows[0].Seq)
		}
	}
	if len(rows) < domain.MaxLiturgyQuery && rows[0].Seq != 1 {
		u.t.Fatalf("the history starts at seq %d", rows[0].Seq)
	}
}

func (u uenv) sql(query string, args ...any) {
	u.t.Helper()
	if err := u.db.ExecForTest(context.Background(), query, args...); err != nil {
		u.t.Fatal(err)
	}
}

func (u uenv) duty() domain.DutyID {
	u.t.Helper()
	duties, err := u.vocab.List(u.ctx, u.a, domain.KindDuty)
	if err != nil || len(duties) == 0 {
		u.t.Fatal("no duties", err)
	}
	return domain.DutyID(duties[0].Entry.ID)
}

// TC-U-001: each command, undone and redone right away.
func TestUndoEachCommand(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		L := u.liturgies

		t.Run("item.update", func(t *testing.T) {
			x := u.pray("X")
			u.text(u.a, x, "hello")
			r := u.mustUndo(u.a)
			if r.Edit.Command != domain.CmdItemUpdate || r.Edit.Status != domain.EditUndone || r.ItemID != x {
				t.Errorf("result %+v", r)
			}
			if got := u.mustItem(x); got.Text != "" || got.Version != 3 {
				t.Errorf("after undo: text %q version %d, want empty and 3 (versions rise)", got.Text, got.Version)
			}
			u.mustRedo(u.a)
			if got := u.mustItem(x); got.Text != "hello" || got.Version != 4 {
				t.Errorf("after redo: text %q version %d", got.Text, got.Version)
			}
		})

		t.Run("item.songs", func(t *testing.T) {
			song := u.newSong("Lagu", "id")
			x := u.useSong(u.lid, song.Song.ID)
			before := u.mustItem(x).Songs
			u.mustUndo(u.a)
			if got := u.mustItem(x); len(got.Songs) != 0 {
				t.Fatalf("songs after undo: %+v", got.Songs)
			}
			u.mustRedo(u.a)
			got := u.mustItem(x)
			if len(got.Songs) != 1 || got.Songs[0].ID != before[0].ID || len(got.Songs[0].Entries) != len(before[0].Entries) {
				t.Errorf("songs after redo: %+v, want %+v", got.Songs, before)
			}
			if got.Songs[0].Entries[0].ID != before[0].Entries[0].ID {
				t.Errorf("entry IDs changed: %+v", got.Songs[0].Entries)
			}
		})

		t.Run("item.add", func(t *testing.T) {
			x := u.pray("Added")
			n := len(u.order())
			u.mustUndo(u.a)
			if _, ok := u.item(x); ok || len(u.order()) != n-1 {
				t.Fatalf("the item is still there")
			}
			u.mustRedo(u.a)
			got := u.mustItem(x)
			if got.Version != 2 || len(u.order()) != n {
				t.Errorf("re-created: version %d (want 2: removed at 1, plus 1), %d items", got.Version, len(u.order()))
			}
		})

		t.Run("item.remove", func(t *testing.T) {
			x := u.pray("Removed")
			u.pray("After")
			pos := u.mustItem(x).Position
			if _, err := L.RemoveItem(u.ctx, u.a, u.lid, x, u.liturgy(u.lid).Liturgy.Version); err != nil {
				t.Fatal(err)
			}
			u.mustUndo(u.a)
			got := u.mustItem(x)
			if got.Position != pos || got.Version != 2 || got.Title != "Removed" {
				t.Errorf("restored: %+v (position %d)", got, pos)
			}
			u.mustRedo(u.a)
			if _, ok := u.item(x); ok {
				t.Error("the redo of a removal left the item")
			}
		})

		t.Run("items.reorder", func(t *testing.T) {
			before := u.order()
			rev := append([]domain.ItemID(nil), before...)
			for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
				rev[i], rev[j] = rev[j], rev[i]
			}
			if _, err := L.ReorderItems(u.ctx, u.a, u.lid, u.liturgy(u.lid).Liturgy.Version, rev); err != nil {
				t.Fatal(err)
			}
			u.mustUndo(u.a)
			if got := u.order(); !equalIDs(got, before) {
				t.Errorf("after undo %v, want %v", got, before)
			}
			u.mustRedo(u.a)
			if got := u.order(); !equalIDs(got, rev) {
				t.Errorf("after redo %v, want %v", got, rev)
			}
		})

		t.Run("liturgy.update", func(t *testing.T) {
			name := "Renamed"
			if _, err := L.Update(u.ctx, u.a, u.lid, app.LiturgyChange{Version: u.liturgy(u.lid).Liturgy.Version, ServiceName: &name}); err != nil {
				t.Fatal(err)
			}
			u.mustUndo(u.a)
			if got := u.liturgy(u.lid).Liturgy.ServiceName; got != "Test" {
				t.Errorf("name after undo %q", got)
			}
			u.mustRedo(u.a)
			if got := u.liturgy(u.lid).Liturgy.ServiceName; got != name {
				t.Errorf("name after redo %q", got)
			}
		})

		t.Run("assignments", func(t *testing.T) {
			asg, err := L.AddAssignment(u.ctx, u.a, u.lid, app.AssignmentInput{DutyID: u.duty(), Name: "Budi"})
			if err != nil {
				t.Fatal(err)
			}
			u.mustUndo(u.a)
			if n := len(u.liturgy(u.lid).Assignments); n != 0 {
				t.Fatalf("%d assignments after undo", n)
			}
			u.mustRedo(u.a)
			got := u.liturgy(u.lid).Assignments
			if len(got) != 1 || got[0].Assignment.ID != asg.Assignment.ID {
				t.Fatalf("after redo %+v, want the same ID", got)
			}
			// A removal is undone by putting the row back with its old ID.
			if err := L.RemoveAssignment(u.ctx, u.a, u.lid, asg.Assignment.ID); err != nil {
				t.Fatal(err)
			}
			u.mustUndo(u.a)
			if got := u.liturgy(u.lid).Assignments; len(got) != 1 || got[0].Assignment.ID != asg.Assignment.ID {
				t.Fatalf("after undoing the removal %+v", got)
			}
			u.mustRedo(u.a)
			if n := len(u.liturgy(u.lid).Assignments); n != 0 {
				t.Fatalf("%d assignments after redoing the removal", n)
			}
		})

		// Undo and redo rows are history too, never targets, and carry the target.
		u.noGaps()
		var undos int
		for _, e := range u.history() {
			if e.Command == domain.CmdUndo || e.Command == domain.CmdRedo {
				undos++
				if e.TargetEditID == "" {
					t.Errorf("row %d (%s) has no target", e.Seq, e.Command)
				}
			}
		}
		if undos == 0 {
			t.Error("no undo or redo rows in the history")
		}
	})
}

func equalIDs(a, b []domain.ItemID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TC-U-002 and TC-U-003: stacks, and redo in reverse order.
func TestUndoStacks(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		x := u.pray("X")
		y := u.pray("Y")
		u.fresh()

		// Three edits of one item, undone three times in a row although every undo raised the version.
		u.text(u.a, x, "1")
		u.text(u.a, x, "2")
		u.text(u.a, x, "3")
		for range 3 {
			u.mustUndo(u.a)
		}
		if got := u.mustItem(x).Text; got != "" {
			t.Errorf("after three undos %q", got)
		}
		_, err := u.undo(u.a)
		refused(t, err, app.UndoNothingToUndo)

		// Redone in reverse order: the most recently undone first.
		u.mustRedo(u.a)
		if got := u.mustItem(x).Text; got != "1" {
			t.Errorf("first redo %q, want 1", got)
		}
		u.mustRedo(u.a)
		u.mustRedo(u.a)
		if got := u.mustItem(x).Text; got != "3" {
			t.Errorf("after three redos %q", got)
		}
		_, err = u.redo(u.a)
		refused(t, err, app.UndoNothingToRedo)

		// A new edit drops the undone ones.
		u.mustUndo(u.a)
		u.text(u.a, y, "new")
		_, err = u.redo(u.a)
		refused(t, err, app.UndoNothingToRedo)

		// Two people: each undoes only their own newest edit.
		u.text(u.b, y, "by b")
		u.text(u.a, x, "by a")
		r := u.mustUndo(u.a)
		if r.ItemID != x {
			t.Errorf("a undid %s, want x", r.ItemID)
		}
		r = u.mustUndo(u.b)
		if r.ItemID != y || u.mustItem(y).Text != "new" {
			t.Errorf("b undid %s, y is %q", r.ItemID, u.mustItem(y).Text)
		}
		u.noGaps()
	})
}

// A redo after a colleague's change is refused and drops the whole stack; an
// undo after a colleague's change to the same item is refused, to another item allowed.
func TestUndoConflicts(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		x, y := u.pray("X"), u.pray("Y")
		u.fresh()

		u.text(u.a, x, "a1")
		u.text(u.b, y, "b on y") // another item: no touch
		if r := u.mustUndo(u.a); r.ItemID != x {
			t.Fatalf("undo %+v", r)
		}

		u.text(u.a, x, "a2")
		u.text(u.b, x, "b on x") // the same item: touches
		_, err := u.undo(u.a)
		refused(t, err, app.UndoChangedSince)
		if got := u.mustItem(x).Text; got != "b on x" {
			t.Errorf("a refused undo changed the item: %q", got)
		}

		// Redo: A undid, B edited the item, the redo is refused and the stack is dropped.
		u2 := newUndoEnv(t, sqlstoretest.NewSQLite(t))
		x2 := u2.pray("X")
		u2.text(u2.a, x2, "1")
		u2.text(u2.a, x2, "2")
		u2.mustUndo(u2.a)
		u2.mustUndo(u2.a)
		u2.text(u2.b, x2, "b")
		_, err = u2.redo(u2.a)
		refused(t, err, app.UndoChangedSince)
		_, err = u2.redo(u2.a)
		refused(t, err, app.UndoNothingToRedo)
		u.noGaps()
		u2.noGaps()
	})
}

// TC-U-005: a removed item comes back with its ID, position and a version that never goes back.
func TestUndoRestoresItem(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		L := u.liturgies

		// An item with songs: remove, undo, redo, undo.
		song := u.newSong("Lagu", "id")
		s := u.useSong(u.lid, song.Song.ID)
		u.pray("after")
		want := u.mustItem(s)
		if _, err := L.RemoveItem(u.ctx, u.a, u.lid, s, u.liturgy(u.lid).Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		u.mustUndo(u.a)
		got := u.mustItem(s)
		if got.Position != want.Position || got.Version != want.Version+1 || len(got.Songs) != 1 || got.Songs[0].ID != want.Songs[0].ID {
			t.Fatalf("restored %+v, want position %d version %d and the same songs %+v", got, want.Position, want.Version+1, want.Songs)
		}
		u.mustRedo(u.a)
		u.mustUndo(u.a)
		if got2 := u.mustItem(s); got2.Version != want.Version+2 {
			t.Errorf("version after the second restore %d, want %d (never a value it had)", got2.Version, want.Version+2)
		}

		// An item that is added, edited, and then everything undone and the add redone.
		x := u.pray("X") // version 1
		u.text(u.a, x, "t")
		u.mustUndo(u.a) // the edit: version 3
		u.mustUndo(u.a) // the add: the item goes at version 3
		u.mustRedo(u.a)
		if got := u.mustItem(x); got.Version != 4 {
			t.Errorf("re-created at version %d, want 4 (3 when removed, plus 1)", got.Version)
		}

		// The position is clamped to the current count.
		y := u.pray("last")
		if _, err := L.RemoveItem(u.ctx, u.a, u.lid, y, u.liturgy(u.lid).Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		u.mustUndo(u.a)
		if order := u.order(); order[len(order)-1] != y {
			t.Errorf("the last item came back at %v", order)
		}
		u.noGaps()
	})
}

// TC-U-008: skips, structural blocks, the redo drop and the floor.
func TestUndoSkipsAndFloor(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		x, y := u.pray("X"), u.pray("Y")
		u.fresh()

		// A refused non-structural edit is skipped: the older edit can be undone.
		u.text(u.a, x, "a on x")
		u.text(u.a, y, "a on y")
		u.text(u.b, y, "b on y")
		_, err := u.undo(u.a)
		refused(t, err, app.UndoChangedSince)
		if r := u.mustUndo(u.a); r.ItemID != x {
			t.Fatalf("after the refusal, undo took %+v, want the older edit of x", r)
		}
		_, err = u.undo(u.a)
		refused(t, err, app.UndoNothingToUndo)
		u.noGaps()

		// A refused structural edit keeps blocking everything older.
		p, q := u.pray("P"), u.pray("Q")
		u.fresh()
		u.text(u.a, p, "older")
		if _, err := u.liturgies.RemoveItem(u.ctx, u.a, u.lid, q, u.liturgy(u.lid).Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		if _, err := u.liturgies.ReorderItems(u.ctx, u.b, u.lid, u.liturgy(u.lid).Liturgy.Version, reversed(u.order())); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			_, err = u.undo(u.a)
			refused(t, err, app.UndoChangedSince) // the same answer: nothing was skipped
		}
		if got := u.mustItem(p).Text; got != "older" {
			t.Errorf("the blocked older edit changed: %q", got)
		}

		// The floor: nothing at or below it is a target; later edits are.
		u.sql("UPDATE liturgies SET undo_floor_seq = edit_seq WHERE id = ?", string(u.lid))
		_, err = u.undo(u.b)
		refused(t, err, app.UndoNothingToUndo)
		u.text(u.b, x, "after the floor")
		if r := u.mustUndo(u.b); r.ItemID != x {
			t.Errorf("undo after the floor took %+v", r)
		}
		u.noGaps()
	})
}

func reversed(ids []domain.ItemID) []domain.ItemID {
	out := make([]domain.ItemID, len(ids))
	for i, id := range ids {
		out[len(ids)-1-i] = id
	}
	return out
}

// A refused redo drops the stack even when the target is not structural, and skipped
// edits are not offered again.
func TestRedoRefusalDropsStack(t *testing.T) {
	u := newUndoEnv(t, sqlstoretest.NewSQLite(t))
	x := u.pray("X")
	u.text(u.a, x, "1")
	u.mustUndo(u.a)
	u.text(u.b, x, "b")
	_, err := u.redo(u.a)
	refused(t, err, app.UndoChangedSince)
	got := u.history()
	for _, e := range got {
		if e.Status == domain.EditUndone {
			t.Errorf("row %d is still undone after the refused redo", e.Seq)
		}
	}
}

// A removed item whose duty was deleted cannot come back (reference_gone).
func TestUndoItemWithGoneDuty(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		L := u.liturgies

		// A removed item whose duty was deleted meanwhile cannot come back (reference_gone) and keeps blocking.
		duty, err := u.vocab.Create(u.ctx, u.a, domain.KindDuty, "Pemandu")
		if err != nil {
			t.Fatal(err)
		}
		it, err := L.AddItem(u.ctx, u.a, u.lid, app.ItemInput{LiturgyVersion: u.liturgy(u.lid).Liturgy.Version, Title: "Doa",
			Type: domain.ItemPrayer, DutyID: domain.DutyID(duty.Entry.ID)})
		if err != nil {
			t.Fatal(err)
		}
		x := it.Item.Item.ID
		if _, err := L.RemoveItem(u.ctx, u.a, u.lid, x, u.liturgy(u.lid).Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		if err := u.vocab.Delete(u.ctx, u.a, domain.KindDuty, duty.Entry.ID); err != nil {
			t.Fatal(err)
		}
		_, err = u.undo(u.a)
		refused(t, err, app.UndoReferenceGone)
		if _, ok := u.item(x); ok {
			t.Error("a refused undo restored the item")
		}
		u.noGaps()
	})
}

// The assignment of a member who left cannot be put back (reference_gone).
func TestUndoAssignmentOfFormerMember(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		L := u.liturgies

		// An assignment of a member who left: its removal cannot be undone (reference_gone) and is skipped.
		member, mid := u.member("m@example.org", u.role(domain.OriginEditor).ID)
		x := u.pray("X")
		u.text(u.a, x, "older")
		asg, err := L.AddAssignment(u.ctx, u.a, u.lid, app.AssignmentInput{DutyID: u.duty(), UserID: member.UserID})
		if err != nil {
			t.Fatal(err)
		}
		if err := L.RemoveAssignment(u.ctx, u.a, u.lid, asg.Assignment.ID); err != nil {
			t.Fatal(err)
		}
		if err := u.members.Remove(u.ctx, u.a, mid); err != nil {
			t.Fatal(err)
		}
		_, err = u.undo(u.a)
		refused(t, err, app.UndoReferenceGone)
		// The refused edit was skipped (it is not structural). The older assignment.add cannot be
		// undone either, because its assignment was already removed; that refusal is skipped too,
		// and the text edit below them is next.
		_, err = u.undo(u.a)
		refused(t, err, app.UndoChangedSince)
		if r := u.mustUndo(u.a); r.Edit.Command != domain.CmdItemUpdate {
			t.Errorf("after two skips, undo took %s, want the item.update", r.Edit.Command)
		}
		u.noGaps()
	})
}

// A locked liturgy refuses first; a person who may not edit is refused too.
func TestUndoPermissionsAndState(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		u := newUndoEnv(t, db)
		x := u.pray("X")
		u.text(u.a, x, "t")
		// A locked liturgy refuses before anything else; a person without liturgy.edit is refused too.
		u.sql("UPDATE liturgies SET state = 'in_review' WHERE id = ?", string(u.lid))
		if _, err := u.undo(u.a); !errors.Is(err, app.ErrLiturgyLocked) {
			t.Errorf("locked: %v", err)
		}
		u.sql("UPDATE liturgies SET state = 'draft' WHERE id = ?", string(u.lid))
		team, _ := u.member("team@example.org")
		if _, err := u.undo(team); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("a team member: %v", err)
		}
		if _, err := u.liturgies.Undo(u.ctx, nil, u.lid); !errors.Is(err, app.ErrUnauthenticated) {
			t.Errorf("no session: %v", err)
		}
		if _, err := u.liturgies.Undo(u.ctx, u.a, domain.LiturgyID(u.ids.NewID())); !isNotFound(err, app.ReasonMissing) {
			t.Errorf("a liturgy that does not exist: %v", err)
		}
	})
}

// IT-E-003: an undo and an edit of the same item at the same moment. Exactly
// one wins; nothing is lost; versions stay consistent; a PostgreSQL deadlock
// between the two lock orders is absorbed by the retry (02 §3), never a 500.
func TestUndoRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		rounds := 25
		if db.Path() == "" {
			rounds = 100
		}
		u := newUndoEnv(t, db)
		x := u.pray("X")
		u.fresh()
		var undone, edited int
		for i := range rounds {
			text := "a" + string(rune('A'+i%26))
			prev := u.mustItem(x).Text
			u.text(u.a, x, text) // an edit of A's to undo
			base := u.mustItem(x)
			errs := race(2, func(n int) error {
				if n == 0 {
					_, err := u.undo(u.a)
					return err
				}
				b := "b"
				_, err := u.liturgies.UpdateItem(u.ctx, u.b, u.lid, x, app.ItemChange{Version: base.Version, Text: &b})
				return err
			})
			ok, others := succeeded(errs)
			if ok != 1 || len(others) != 1 {
				t.Fatalf("round %d: %d won, others %v", i, ok, others)
			}
			got := u.mustItem(x)
			if got.Version != base.Version+1 {
				t.Fatalf("round %d: version %d after one winner from %d", i, got.Version, base.Version)
			}
			switch {
			case errs[0] == nil: // the undo won: B is told that the item changed
				undone++
				if s, id := conflict(errs[1]); s != app.ScopeItem || id != x || got.Text != prev {
					t.Fatalf("round %d: undo won but B got %v and the text is %q, want %q", i, errs[1], got.Text, prev)
				}
			default: // B won: the undo is refused and A's edit is skipped
				edited++
				var ref *app.UndoRefusedError
				if !errors.As(errs[0], &ref) || ref.Reason != app.UndoChangedSince || got.Text != "b" {
					t.Fatalf("round %d: B won but A got %v and the text is %q", i, errs[0], got.Text)
				}
			}
			u.noGaps()
		}
		t.Logf("%d rounds: the undo won %d, the edit won %d", rounds, undone, edited)
	})
}
