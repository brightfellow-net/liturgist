// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strconv"

	"github.com/brightfellow-net/liturgist/domain"
)

// Templates holds the template use cases (09 §2.2). templates.edit or
// liturgy.edit holders read them; templates.edit changes them.
type Templates struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// TemplateSummary is a template in a list.
type TemplateSummary struct {
	Row     TemplateRow
	Actions PlanningActions
}

// TemplateView is a whole template.
type TemplateView struct {
	Template domain.Template
	Actions  PlanningActions
}

// TemplateInput creates a template. An empty Language means the church's
// default content language.
type TemplateInput struct {
	Name, Language string
	Items          []domain.TemplateItem
}

// TemplateChange is a PATCH: nil fields are unchanged. Items replaces the
// complete list (P-57).
type TemplateChange struct {
	Version        int
	Name, Language *string
	Items          *[]domain.TemplateItem
}

// List returns the templates by name (templates.edit or liturgy.edit).
func (u *Templates) List(ctx context.Context, sess *domain.Session) ([]TemplateSummary, error) {
	var res []TemplateSummary
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := requirePlanningView(sc.actor); err != nil {
			return err
		}
		rows, err := sc.cs.Templates().List(ctx)
		if err != nil {
			return err
		}
		actions := planningActions(sc.actor)
		res = make([]TemplateSummary, len(rows))
		for i, r := range rows {
			res[i] = TemplateSummary{Row: r, Actions: actions}
		}
		return nil
	})
	return res, err
}

// Get returns one template with its items.
func (u *Templates) Get(ctx context.Context, sess *domain.Session, id domain.TemplateID) (TemplateView, error) {
	var res TemplateView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := requirePlanningView(sc.actor); err != nil {
			return err
		}
		t, err := sc.cs.Templates().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		res = TemplateView{Template: t, Actions: planningActions(sc.actor)}
		return nil
	})
	return res, err
}

// checkDuties rejects items that name a duty the church does not have.
func checkDuties(ctx context.Context, cs ChurchStore, items []domain.TemplateItem) error {
	var known map[string]bool
	for i, it := range items {
		if it.DefaultDutyID == "" {
			continue
		}
		if known == nil {
			duties, err := cs.Duties().List(ctx)
			if err != nil {
				return err
			}
			known = make(map[string]bool, len(duties))
			for _, d := range duties {
				known[d.ID] = true
			}
		}
		if !known[string(it.DefaultDutyID)] {
			return &domain.InvalidInputError{Field: "items." + strconv.Itoa(i) + ".default_duty_id", Message: "Unknown duty."}
		}
	}
	return nil
}

func (u *Templates) withIDs(items []domain.TemplateItem) []domain.TemplateItem {
	out := make([]domain.TemplateItem, len(items))
	for i, it := range items {
		it.ID = u.IDs.NewID()
		out[i] = it
	}
	return out
}

// Create saves a template (templates.edit).
func (u *Templates) Create(ctx context.Context, sess *domain.Session, in TemplateInput) (TemplateView, error) {
	var res TemplateView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		now := u.Clock.Now()
		t := domain.Template{ID: domain.TemplateID(u.IDs.NewID()), Name: in.Name, Language: in.Language,
			Items: in.Items, Version: 1, CreatedAt: now, UpdatedAt: now}
		if t.Language == "" {
			ch, err := sc.cs.Church().Get(ctx)
			if err != nil {
				return err
			}
			t.Language = ch.DefaultLanguage
		}
		if err := domain.ValidateTemplate(&t); err != nil {
			return err
		}
		if err := checkDuties(ctx, sc.cs, t.Items); err != nil {
			return err
		}
		n, err := sc.cs.Templates().Count(ctx)
		if err != nil {
			return err
		}
		if n >= domain.MaxTemplates {
			return domain.LimitError("name", domain.MaxTemplates, n)
		}
		t.Items = u.withIDs(t.Items)
		if err := sc.cs.Templates().Create(ctx, t); err != nil {
			return mapNameTaken(err, "template")
		}
		res = TemplateView{Template: t, Actions: planningActions(sc.actor)}
		return nil
	})
	return res, err
}

// Update changes a template (templates.edit). A stale version is
// ErrVersionConflict and nothing is written.
func (u *Templates) Update(ctx context.Context, sess *domain.Session, id domain.TemplateID, ch TemplateChange) (TemplateView, error) {
	var res TemplateView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		cur, err := sc.cs.Templates().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		if cur.Version != ch.Version {
			return ErrVersionConflict
		}
		next := cur
		if ch.Name != nil {
			next.Name = *ch.Name
		}
		if ch.Language != nil {
			next.Language = *ch.Language
		}
		if ch.Items != nil {
			next.Items = *ch.Items
		}
		if err := domain.ValidateTemplate(&next); err != nil {
			return err
		}
		if ch.Items != nil {
			if err := checkDuties(ctx, sc.cs, next.Items); err != nil {
				return err
			}
			next.Items = u.withIDs(next.Items)
		}
		next.Version, next.UpdatedAt = cur.Version+1, u.Clock.Now()
		ok, err := sc.cs.Templates().Update(ctx, next, cur.Version)
		if err != nil {
			return mapNameTaken(err, "template")
		}
		if !ok {
			return ErrVersionConflict
		}
		res = TemplateView{Template: next, Actions: planningActions(sc.actor)}
		return nil
	})
	return res, err
}

// Delete removes a template that no service has as its default
// (templates.edit); liturgies made from it keep working.
func (u *Templates) Delete(ctx context.Context, sess *domain.Session, id domain.TemplateID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		if _, err := sc.cs.Templates().ByID(ctx, id); err != nil {
			return missing(err)
		}
		used, err := sc.cs.Templates().ServicesUsing(ctx, id)
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return &TemplateInUseError{ServiceIDs: used}
		}
		return missing(sc.cs.Templates().Delete(ctx, id))
	})
}
