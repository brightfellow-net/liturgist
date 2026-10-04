// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/brightfellow-net/liturgist/domain"
)

// UndoResult is the answer of an undo or redo: the edit that was acted on
// (with its new status) and the versions the change left behind.
type UndoResult struct {
	Edit           domain.Edit
	LiturgyVersion int
	ItemID         domain.ItemID // "" when the change is not about one item
	ItemVersion    int           // 0 when the change is not about one item
}

// refusal ends the write transaction of an undo or redo that is refused; the
// bookkeeping that follows (skip, drop) runs in a second, short transaction
// so that the first one rolls back and leaves no gap in seq (11 §7.2).
type refusal struct {
	reason string
	skip   domain.EditID // set: mark this edit skipped
	drop   bool          // set: drop the caller's undone edits
}

func (r *refusal) Error() string { return "undo refused: " + r.reason }

// applied is what applying an image changed.
type applied struct {
	liturgyVersion, itemVersion int
	item                        domain.ItemID
	before, after               any
}

// Undo reverses the caller's newest edit that nobody else has touched since (liturgy.edit, 11 §7.2).
func (u *Liturgies) Undo(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (UndoResult, error) {
	return u.step(ctx, sess, id, true)
}

// Redo applies again the edit the caller undid last (liturgy.edit, 11 §7.2).
func (u *Liturgies) Redo(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (UndoResult, error) {
	return u.step(ctx, sess, id, false)
}

func (u *Liturgies) step(ctx context.Context, sess *domain.Session, id domain.LiturgyID, undo bool) (UndoResult, error) {
	var res UndoResult
	err := u.Tx.Write(ctx, func(s Store) error {
		// Always adds references (a restored item, song or assignment), so the church lock comes first.
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if _, err := sc.openEdit(ctx, id); err != nil {
			return err
		}
		// The history counter first: the test below then sees every committed edit (10 §5).
		seq, err := sc.nextSeq(ctx, id)
		if err != nil {
			return err
		}
		l, err := sc.cs.Liturgies().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		if !l.State.Editable() {
			return ErrLiturgyLocked
		}
		var (
			target domain.Edit
			from   int // foreign rows count after this seq
		)
		if undo {
			target, err = sc.cs.Edits().Newest(ctx, sc.actor.UserID, id, l.UndoFloorSeq, domain.UndoWindow)
			from = target.Seq
		} else {
			target, err = sc.cs.Edits().NewestUndone(ctx, sc.actor.UserID, id, l.UndoFloorSeq)
			from = target.UndoSeq
		}
		if errors.Is(err, ErrNotFound) {
			if undo {
				return &refusal{reason: UndoNothingToUndo}
			}
			return &refusal{reason: UndoNothingToRedo}
		}
		if err != nil {
			return err
		}
		refuse := func(reason string) error {
			r := &refusal{reason: reason}
			switch {
			case !undo:
				r.drop = true
			case domain.Skippable(target.Command):
				r.skip = target.ID
			}
			return r
		}
		foreign, err := sc.cs.Edits().Foreign(ctx, id, from, sc.actor.UserID)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(foreign, target.TouchedBy) {
			return refuse(UndoChangedSince)
		}
		ap, err := u.apply(ctx, sc, l, target, undo)
		var gone *UndoRefusedError
		if errors.As(err, &gone) {
			return refuse(gone.Reason)
		}
		if err != nil {
			return err
		}
		cmd := domain.CmdRedo
		if undo {
			cmd = domain.CmdUndo
		}
		row := domain.Edit{Seq: seq, Command: cmd, TargetEditID: target.ID, ItemID: ap.item,
			LiturgyVersionAfter: ap.liturgyVersion, ItemVersionAfter: ap.itemVersion}
		if err := u.append(ctx, sc, id, row, ap.before, ap.after); err != nil {
			return err
		}
		to, undoSeq, wasStatus := domain.EditUndone, seq, domain.EditDone
		if !undo {
			to, undoSeq, wasStatus = domain.EditDone, 0, domain.EditUndone
		}
		ok, err := sc.cs.Edits().SetStatus(ctx, target.ID, wasStatus, to, undoSeq)
		if err != nil {
			return err
		}
		if !ok { // cannot happen under the liturgy's row lock
			return errors.New("undo: the edit changed status while it was being acted on")
		}
		target.Status, target.UndoSeq = to, undoSeq
		res = UndoResult{Edit: target, LiturgyVersion: ap.liturgyVersion, ItemID: ap.item, ItemVersion: ap.itemVersion}
		return nil
	})
	var ref *refusal
	if !errors.As(err, &ref) {
		return res, err
	}
	if ref.skip != "" || ref.drop {
		// The bookkeeping must not hide the refusal if it fails: it only keeps the stack usable.
		_ = u.Tx.Write(ctx, func(s Store) error {
			sc, err := actorIn(ctx, s, sess, false)
			if err != nil {
				return err
			}
			if ref.drop {
				return sc.cs.Edits().DropUndone(ctx, sc.actor.UserID, id)
			}
			return sc.cs.Edits().MarkSkipped(ctx, ref.skip)
		})
	}
	return res, &UndoRefusedError{Reason: ref.reason}
}

func gone() error    { return &UndoRefusedError{Reason: UndoReferenceGone} }
func changed() error { return &UndoRefusedError{Reason: UndoChangedSince} }

// apply puts the before image of t (undo) or its after image (redo) back
// through the same repository calls as a normal write. A refusal is returned
// as an *UndoRefusedError.
func (u *Liturgies) apply(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit, undo bool) (applied, error) {
	img := t.After
	if undo {
		img = t.Before
	}
	switch t.Command {
	case domain.CmdItemUpdate:
		return u.applyFields(ctx, sc, l, t, img)
	case domain.CmdItemSongs:
		return u.applySongs(ctx, sc, l, t, img)
	case domain.CmdItemAdd, domain.CmdItemRemove:
		// An add is undone by removing the item and redone by creating it; a removal the other way round.
		if (t.Command == domain.CmdItemAdd) == undo {
			return u.removeItem(ctx, sc, l, t.ItemID)
		}
		return u.restoreItem(ctx, sc, l, t, undo)
	case domain.CmdItemsReorder:
		return u.applyOrder(ctx, sc, l, img)
	case domain.CmdLiturgyUpdate:
		return u.applyLiturgy(ctx, sc, l, img)
	case domain.CmdAssignmentAdd, domain.CmdAssignmentRemove:
		if (t.Command == domain.CmdAssignmentAdd) == undo {
			return u.removeAssignment(ctx, sc, l, t)
		}
		return u.restoreAssignment(ctx, sc, l, t)
	}
	return applied{}, errors.New("undo: no rule for command " + t.Command)
}

// applyFields sets the fields an item.update changed back (or again).
func (u *Liturgies) applyFields(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit, img []byte) (applied, error) {
	var fields map[string]string
	if err := json.Unmarshal(img, &fields); err != nil {
		return applied{}, err
	}
	cur, err := sc.cs.LiturgyItems().ByID(ctx, l.ID, t.ItemID)
	if errors.Is(err, ErrNotFound) {
		return applied{}, changed()
	}
	if err != nil {
		return applied{}, err
	}
	next := cur
	before := map[string]string{}
	for name, v := range fields {
		switch name {
		case "title":
			before[name], next.Title = cur.Title, v
		case "text":
			before[name], next.Text = cur.Text, v
		case "duty_id":
			before[name], next.DutyID = string(cur.DutyID), domain.DutyID(v)
		case "reading_id":
			before[name], next.ReadingID = string(cur.ReadingID), domain.ReadingID(v)
		case "reading_label":
			before[name], next.ReadingLabel = cur.ReadingLabel, v
		}
	}
	if next.DutyID != cur.DutyID && next.DutyID != "" {
		if err := dutyExists(ctx, sc.cs, next.DutyID); err != nil {
			return applied{}, err
		}
	}
	if next.ReadingID != cur.ReadingID && next.ReadingID != "" {
		if _, err := sc.cs.Readings().ByID(ctx, next.ReadingID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return applied{}, gone()
			}
			return applied{}, err
		}
	}
	next.UpdatedAt = u.Clock.Now()
	ok, err := sc.cs.LiturgyItems().Update(ctx, next, cur.Version)
	if err != nil {
		return applied{}, err
	}
	if !ok {
		return applied{}, changed()
	}
	return applied{liturgyVersion: l.Version, itemVersion: cur.Version + 1, item: t.ItemID, before: before, after: fields}, nil
}

func dutyExists(ctx context.Context, cs ChurchStore, id domain.DutyID) error {
	if _, err := cs.Duties().ByID(ctx, string(id)); err != nil {
		if errors.Is(err, ErrNotFound) {
			return gone()
		}
		return err
	}
	return nil
}

// songsOf rebuilds songs from an image, checking that what they refer to still exists.
func songsOf(ctx context.Context, cs ChurchStore, img []songImage) ([]domain.LiturgySong, error) {
	cache := map[domain.SongID]domain.Song{}
	out := make([]domain.LiturgySong, len(img))
	for i, si := range img {
		ls := domain.LiturgySong{ID: domain.ItemSongID(si.ID), Position: i, SongID: domain.SongID(si.SongID), SongTitle: si.SongTitle,
			Key: si.Key, Note: si.Note, Entries: make([]domain.Entry, len(si.Entries))}
		var sections map[domain.SectionID]bool
		if ls.SongID != "" {
			song, ok := cache[ls.SongID]
			if !ok {
				var err error
				if song, err = cs.Songs().ByID(ctx, ls.SongID); err != nil {
					if errors.Is(err, ErrNotFound) {
						return nil, gone()
					}
					return nil, err
				}
				cache[ls.SongID] = song
			}
			sections = make(map[domain.SectionID]bool, len(song.Sections))
			for _, sec := range song.Sections {
				sections[sec.ID] = true
			}
		}
		for j, ei := range si.Entries {
			if ei.SectionID != "" && !sections[domain.SectionID(ei.SectionID)] {
				return nil, gone()
			}
			if ei.SingingPartID != "" {
				if _, err := cs.SingingParts().ByID(ctx, ei.SingingPartID); err != nil {
					if errors.Is(err, ErrNotFound) {
						return nil, gone()
					}
					return nil, err
				}
			}
			ls.Entries[j] = domain.Entry{ID: domain.EntryID(ei.ID), Position: j, SectionID: domain.SectionID(ei.SectionID),
				SingingPartID: domain.SingingPartID(ei.SingingPartID), KeyChange: ei.KeyChange, Note: ei.Note, SectionLabel: ei.SectionLabel}
		}
		out[i] = ls
	}
	return out, nil
}

// applySongs puts the whole list of songs of an item back (or again).
func (u *Liturgies) applySongs(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit, img []byte) (applied, error) {
	var want []songImage
	if err := json.Unmarshal(img, &want); err != nil {
		return applied{}, err
	}
	cur, err := sc.cs.LiturgyItems().ByID(ctx, l.ID, t.ItemID)
	if errors.Is(err, ErrNotFound) {
		return applied{}, changed()
	}
	if err != nil {
		return applied{}, err
	}
	songs, err := songsOf(ctx, sc.cs, want)
	if err != nil {
		return applied{}, err
	}
	ok, err := sc.cs.LiturgyItems().Bump(ctx, t.ItemID, cur.Version, u.Clock.Now())
	if err != nil {
		return applied{}, err
	}
	if !ok {
		return applied{}, changed()
	}
	if err := sc.cs.LiturgyItems().ReplaceSongs(ctx, t.ItemID, songs); err != nil {
		return applied{}, err
	}
	return applied{liturgyVersion: l.Version, itemVersion: cur.Version + 1, item: t.ItemID,
		before: songsImage(cur.Songs), after: songsImage(songs)}, nil
}

// removeItem deletes an item (undo of an add, redo of a removal). Structural.
func (u *Liturgies) removeItem(ctx context.Context, sc churchScope, l domain.Liturgy, id domain.ItemID) (applied, error) {
	cur, err := sc.cs.LiturgyItems().ByID(ctx, l.ID, id)
	if errors.Is(err, ErrNotFound) {
		return applied{}, changed()
	}
	if err != nil {
		return applied{}, err
	}
	ok, err := sc.cs.Liturgies().Bump(ctx, l.ID, l.Version, u.Clock.Now())
	if err != nil {
		return applied{}, err
	}
	if !ok {
		return applied{}, changed()
	}
	if err := sc.cs.LiturgyItems().Delete(ctx, l.ID, id); err != nil {
		return applied{}, missing(err)
	}
	order, err := sc.cs.LiturgyItems().IDs(ctx, l.ID)
	if err != nil {
		return applied{}, err
	}
	if err := sc.cs.LiturgyItems().SetPositions(ctx, l.ID, order); err != nil {
		return applied{}, err
	}
	return applied{liturgyVersion: l.Version + 1, item: id, before: imageOfItem(cur)}, nil
}

// restoreItem re-creates a removed item with its old ID and position, and a
// version above every version it has had (11 §7.2).
func (u *Liturgies) restoreItem(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit, undo bool) (applied, error) {
	// The image that holds the item as it was when it was last removed: the removal itself
	// the first time; once the removal was undone and redone, the redo row; for the redo
	// of an add, the undo row that removed it (11 §7.2).
	src := t.Before
	switch row, err := sc.cs.Edits().LastActing(ctx, l.ID, t.ID); {
	case err == nil:
		src = row.Before
	case !errors.Is(err, ErrNotFound) || !undo:
		return applied{}, err
	}
	var img itemImage
	if err := json.Unmarshal(src, &img); err != nil {
		return applied{}, err
	}
	if _, err := sc.cs.LiturgyItems().ByID(ctx, l.ID, t.ItemID); err == nil {
		return applied{}, changed()
	} else if !errors.Is(err, ErrNotFound) {
		return applied{}, err
	}
	order, err := sc.cs.LiturgyItems().IDs(ctx, l.ID)
	if err != nil {
		return applied{}, err
	}
	if len(order) >= domain.MaxLiturgyItems {
		return applied{}, domain.LimitError("items", domain.MaxLiturgyItems, len(order))
	}
	if err := dutyIfAny(ctx, sc.cs, domain.DutyID(img.DutyID)); err != nil {
		return applied{}, err
	}
	if img.ReadingID != "" {
		if _, err := sc.cs.Readings().ByID(ctx, domain.ReadingID(img.ReadingID)); err != nil {
			if errors.Is(err, ErrNotFound) {
				return applied{}, gone()
			}
			return applied{}, err
		}
	}
	songs, err := songsOf(ctx, sc.cs, img.Songs)
	if err != nil {
		return applied{}, err
	}
	now := u.Clock.Now()
	it := domain.Item{ID: t.ItemID, LiturgyID: l.ID, Title: img.Title, Type: domain.ItemType(img.Type), DutyID: domain.DutyID(img.DutyID),
		Text: img.Text, ReadingID: domain.ReadingID(img.ReadingID), ReadingLabel: img.ReadingLabel, Version: img.Version + 1,
		Songs: songs, CreatedAt: now, UpdatedAt: now}
	ok, err := sc.cs.Liturgies().Bump(ctx, l.ID, l.Version, now)
	if err != nil {
		return applied{}, err
	}
	if !ok {
		return applied{}, changed()
	}
	at := min(max(img.Position, 0), len(order))
	it.Position = len(order)
	if err := sc.cs.LiturgyItems().Insert(ctx, it); err != nil {
		return applied{}, err
	}
	if err := sc.cs.LiturgyItems().SetPositions(ctx, l.ID, slices.Insert(order, at, it.ID)); err != nil {
		return applied{}, err
	}
	it.Position = at
	return applied{liturgyVersion: l.Version + 1, itemVersion: it.Version, item: it.ID, after: imageOfItem(it)}, nil
}

func dutyIfAny(ctx context.Context, cs ChurchStore, id domain.DutyID) error {
	if id == "" {
		return nil
	}
	return dutyExists(ctx, cs, id)
}

// applyOrder sets the order of the items back (or again).
func (u *Liturgies) applyOrder(ctx context.Context, sc churchScope, l domain.Liturgy, img []byte) (applied, error) {
	var want []domain.ItemID
	if err := json.Unmarshal(img, &want); err != nil {
		return applied{}, err
	}
	cur, err := sc.cs.LiturgyItems().IDs(ctx, l.ID)
	if err != nil {
		return applied{}, err
	}
	a, b := slices.Clone(cur), slices.Clone(want)
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		return applied{}, changed()
	}
	ok, err := sc.cs.Liturgies().Bump(ctx, l.ID, l.Version, u.Clock.Now())
	if err != nil {
		return applied{}, err
	}
	if !ok {
		return applied{}, changed()
	}
	if err := sc.cs.LiturgyItems().SetPositions(ctx, l.ID, want); err != nil {
		return applied{}, err
	}
	return applied{liturgyVersion: l.Version + 1, before: cur, after: want}, nil
}

// applyLiturgy sets the date, time and name back (or again).
func (u *Liturgies) applyLiturgy(ctx context.Context, sc churchScope, l domain.Liturgy, img []byte) (applied, error) {
	var want liturgyFieldsImage
	if err := json.Unmarshal(img, &want); err != nil {
		return applied{}, err
	}
	next := l
	next.Date, next.Time, next.ServiceName = want.Date, want.Time, want.ServiceName
	if err := slotTaken(ctx, sc.cs, next); err != nil {
		return applied{}, err
	}
	next.UpdatedAt = u.Clock.Now()
	ok, err := sc.cs.Liturgies().Update(ctx, next, l.Version)
	if err != nil {
		return applied{}, mapSlotTaken(err)
	}
	if !ok {
		return applied{}, changed()
	}
	return applied{liturgyVersion: l.Version + 1,
		before: liturgyFieldsImage{Date: l.Date, Time: l.Time, ServiceName: l.ServiceName}, after: want}, nil
}

// removeAssignment takes an assignment away (undo of an add, redo of a removal).
func (u *Liturgies) removeAssignment(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit) (applied, error) {
	img := t.After // the row an add wrote
	if t.Command == domain.CmdAssignmentRemove {
		img = t.Before // the row a removal took away
	}
	var a assignmentImage
	if err := json.Unmarshal(img, &a); err != nil {
		return applied{}, err
	}
	cur, err := sc.cs.Assignments().ByID(ctx, l.ID, domain.AssignmentID(a.ID))
	if errors.Is(err, ErrNotFound) {
		return applied{}, changed()
	}
	if err != nil {
		return applied{}, err
	}
	if err := sc.cs.Assignments().Remove(ctx, l.ID, cur.ID); err != nil {
		return applied{}, missing(err)
	}
	return applied{liturgyVersion: l.Version, before: imageOfAssignment(cur)}, nil
}

// restoreAssignment puts an assignment back with its old ID (undo of a
// removal, redo of an add), if the duty and the person still exist.
func (u *Liturgies) restoreAssignment(ctx context.Context, sc churchScope, l domain.Liturgy, t domain.Edit) (applied, error) {
	img := t.Before // the row a removal took away
	if t.Command == domain.CmdAssignmentAdd {
		img = t.After // the row an add wrote
	}
	var ai assignmentImage
	if err := json.Unmarshal(img, &ai); err != nil {
		return applied{}, err
	}
	a := domain.Assignment{ID: domain.AssignmentID(ai.ID), LiturgyID: l.ID, DutyID: domain.DutyID(ai.DutyID),
		UserID: domain.UserID(ai.UserID), Name: ai.Name, CreatedAt: u.Clock.Now()}
	if a.Name != "" {
		var err error
		if a.NameKey, err = domain.ValidateAssignmentName(&a.Name); err != nil {
			return applied{}, err
		}
	}
	if err := dutyExists(ctx, sc.cs, a.DutyID); err != nil {
		return applied{}, err
	}
	if a.UserID != "" {
		if _, err := sc.cs.Memberships().ByUser(ctx, a.UserID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return applied{}, gone() // the member has left
			}
			return applied{}, err
		}
	}
	n, err := sc.cs.Assignments().Count(ctx, l.ID)
	if err != nil {
		return applied{}, err
	}
	if n >= domain.MaxAssignments {
		return applied{}, domain.LimitError("assignments", domain.MaxAssignments, n)
	}
	if err := sc.cs.Assignments().Add(ctx, a); err != nil {
		var uq *UniqueError
		if errors.As(err, &uq) {
			return applied{}, changed()
		}
		return applied{}, err
	}
	return applied{liturgyVersion: l.Version, after: imageOfAssignment(a)}, nil
}
