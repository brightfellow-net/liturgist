// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"

	"github.com/brightfellow-net/liturgist/domain"
)

// PlanningActions are the advisory actions on duties, singing parts,
// templates and services (04 §5, 09 §4): permission only. Whether a row is in
// use needs a query per row, so the 409 explains it instead.
type PlanningActions struct {
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

func planningActions(a Actor) PlanningActions {
	edit := a.Scopes.Has(domain.ScopeTemplatesEdit)
	return PlanningActions{Edit: edit, Delete: edit}
}

// requirePlanningView lets templates.edit and liturgy.edit holders read
// templates and services (09 §4).
func requirePlanningView(a Actor) error {
	return a.RequireAny(domain.ScopeTemplatesEdit, domain.ScopeLiturgyEdit)
}

// --- duties and singing parts ---

// Vocabulary holds the use cases of the two ordered name lists, duties and
// singing parts (09 §2.1). Every member can read them; templates.edit changes them.
type Vocabulary struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
	Usage PlanningUsage
}

// NameEntryView is an entry with its actions.
type NameEntryView struct {
	Entry   domain.NameEntry
	Actions PlanningActions
}

func listRepo(cs ChurchStore, kind domain.ListKind) NameListRepo {
	if kind == domain.KindDuty {
		return cs.Duties()
	}
	return cs.SingingParts()
}

// mapNameTaken turns a clash on a unique name key into name_taken.
func mapNameTaken(err error, reason string) error {
	var uq *UniqueError
	if errors.As(err, &uq) && (uq.Constraint == "duties_church_name_key" || uq.Constraint == "singing_parts_church_name_key" ||
		uq.Constraint == "templates_church_name_key" || uq.Constraint == "services_church_name_key") {
		return &NameTakenError{Reason: reason}
	}
	return err
}

// List returns the entries in order (any member).
func (u *Vocabulary) List(ctx context.Context, sess *domain.Session, kind domain.ListKind) ([]NameEntryView, error) {
	var res []NameEntryView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		entries, err := listRepo(sc.cs, kind).List(ctx)
		if err != nil {
			return err
		}
		res = entryViews(entries, sc.actor)
		return nil
	})
	return res, err
}

func entryViews(entries []domain.NameEntry, a Actor) []NameEntryView {
	actions := planningActions(a)
	out := make([]NameEntryView, len(entries))
	for i, e := range entries {
		out[i] = NameEntryView{Entry: e, Actions: actions}
	}
	return out
}

// Create adds an entry at the end of the list (templates.edit). The count and
// the insert run under the church lock, so two requests cannot both take the
// last place.
func (u *Vocabulary) Create(ctx context.Context, sess *domain.Session, kind domain.ListKind, name string) (NameEntryView, error) {
	var res NameEntryView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		key, err := domain.ValidateEntryName(kind, &name)
		if err != nil {
			return err
		}
		repo := listRepo(sc.cs, kind)
		n, err := repo.Count(ctx)
		if err != nil {
			return err
		}
		if n >= kind.Limit() {
			return domain.LimitError("name", kind.Limit(), n)
		}
		e := domain.NameEntry{ID: u.IDs.NewID(), Name: name, NameKey: key, Position: n, CreatedAt: u.Clock.Now()}
		if err := repo.Create(ctx, e); err != nil {
			return mapNameTaken(err, string(kind))
		}
		res = NameEntryView{Entry: e, Actions: planningActions(sc.actor)}
		return nil
	})
	return res, err
}

// Rename changes the name of an entry (templates.edit).
func (u *Vocabulary) Rename(ctx context.Context, sess *domain.Session, kind domain.ListKind, id, name string) (NameEntryView, error) {
	var res NameEntryView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		key, err := domain.ValidateEntryName(kind, &name)
		if err != nil {
			return err
		}
		repo := listRepo(sc.cs, kind)
		if err := repo.Rename(ctx, id, name, key); err != nil {
			return missing(mapNameTaken(err, string(kind)))
		}
		e, err := repo.ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		res = NameEntryView{Entry: e, Actions: planningActions(sc.actor)}
		return nil
	})
	return res, err
}

// Reorder sets the order of the list; ids must be exactly the current entries
// once each, else ErrVersionConflict (someone changed the list) (templates.edit).
func (u *Vocabulary) Reorder(ctx context.Context, sess *domain.Session, kind domain.ListKind, ids []string) ([]NameEntryView, error) {
	var res []NameEntryView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		repo := listRepo(sc.cs, kind)
		cur, err := repo.List(ctx)
		if err != nil {
			return err
		}
		if !sameIDs(cur, ids) {
			return ErrVersionConflict
		}
		if err := repo.SetOrder(ctx, ids); err != nil {
			return err
		}
		if cur, err = repo.List(ctx); err != nil {
			return err
		}
		res = entryViews(cur, sc.actor)
		return nil
	})
	return res, err
}

// sameIDs reports whether ids lists every entry exactly once.
func sameIDs(cur []domain.NameEntry, ids []string) bool {
	if len(cur) != len(ids) {
		return false
	}
	have := make([]string, len(cur))
	for i, e := range cur {
		have[i] = e.ID
	}
	want := slices.Clone(ids)
	slices.Sort(have)
	slices.Sort(want)
	return slices.Equal(have, want)
}

// Delete removes an entry that no liturgy uses (templates.edit). Template
// items that name a deleted duty lose their default duty.
func (u *Vocabulary) Delete(ctx context.Context, sess *domain.Session, kind domain.ListKind, id string) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		repo := listRepo(sc.cs, kind)
		if _, err := repo.ByID(ctx, id); err != nil {
			return missing(err)
		}
		var inUse bool
		if kind == domain.KindDuty {
			inUse, err = u.Usage.DutyInUse(ctx, sc.actor.ChurchID, id)
		} else {
			inUse, err = u.Usage.SingingPartInUse(ctx, sc.actor.ChurchID, id)
		}
		if err != nil {
			return err
		}
		if inUse {
			if kind == domain.KindDuty {
				return ErrDutyInUse
			}
			return ErrSingingPartInUse
		}
		if err := repo.Delete(ctx, id); err != nil {
			return missing(err)
		}
		// Close the gap so positions stay dense 0..n-1.
		rest, err := repo.List(ctx)
		if err != nil {
			return err
		}
		ids := make([]string, len(rest))
		for i, e := range rest {
			ids[i] = e.ID
		}
		return repo.SetOrder(ctx, ids)
	})
}
