// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// Services holds the service use cases (09 §2.4). templates.edit or
// liturgy.edit holders read them; templates.edit changes them.
type Services struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// ServiceView is a service with its default template's name.
type ServiceView struct {
	Service             domain.Service
	DefaultTemplateName string
	Actions             PlanningActions
}

// ServiceInput creates a service. An empty Language means the church's
// default content language.
type ServiceInput struct {
	Name, Language    string
	DefaultTemplateID domain.TemplateID
	Times             []domain.ServiceTime
}

// ServiceChange is a PATCH: nil fields are unchanged; a DefaultTemplateID of
// "" clears it. Times replaces the complete list (P-57).
type ServiceChange struct {
	Version           int
	Name, Language    *string
	DefaultTemplateID *domain.TemplateID
	Times             *[]domain.ServiceTime
}

// names maps the church's template IDs to their names.
func templateNames(ctx context.Context, cs ChurchStore) (map[domain.TemplateID]string, error) {
	rows, err := cs.Templates().List(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[domain.TemplateID]string, len(rows))
	for _, r := range rows {
		m[r.ID] = r.Name
	}
	return m, nil
}

// List returns the services by name (templates.edit or liturgy.edit).
func (u *Services) List(ctx context.Context, sess *domain.Session) ([]ServiceView, error) {
	var res []ServiceView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := requirePlanningView(sc.actor); err != nil {
			return err
		}
		list, err := sc.cs.Services().List(ctx)
		if err != nil {
			return err
		}
		names, err := templateNames(ctx, sc.cs)
		if err != nil {
			return err
		}
		actions := planningActions(sc.actor)
		res = make([]ServiceView, len(list))
		for i, sv := range list {
			res[i] = ServiceView{Service: sv, DefaultTemplateName: names[sv.DefaultTemplateID], Actions: actions}
		}
		return nil
	})
	return res, err
}

// Get returns one service.
func (u *Services) Get(ctx context.Context, sess *domain.Session, id domain.ServiceID) (ServiceView, error) {
	var res ServiceView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := requirePlanningView(sc.actor); err != nil {
			return err
		}
		sv, err := sc.cs.Services().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		res, err = serviceView(ctx, sc, sv)
		return err
	})
	return res, err
}

func serviceView(ctx context.Context, sc churchScope, sv domain.Service) (ServiceView, error) {
	v := ServiceView{Service: sv, Actions: planningActions(sc.actor)}
	if sv.DefaultTemplateID != "" {
		t, err := sc.cs.Templates().ByID(ctx, sv.DefaultTemplateID)
		if err != nil {
			return ServiceView{}, err
		}
		v.DefaultTemplateName = t.Name
	}
	return v, nil
}

// checkTemplate rejects a default template the church does not have.
func checkTemplate(ctx context.Context, cs ChurchStore, id domain.TemplateID) error {
	if id == "" {
		return nil
	}
	if _, err := cs.Templates().ByID(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return &domain.InvalidInputError{Field: "default_template_id", Message: "Unknown template."}
		}
		return err
	}
	return nil
}

func (u *Services) withIDs(times []domain.ServiceTime) []domain.ServiceTime {
	out := make([]domain.ServiceTime, len(times))
	for i, t := range times {
		t.ID = u.IDs.NewID()
		out[i] = t
	}
	return out
}

// Create saves a service (templates.edit).
func (u *Services) Create(ctx context.Context, sess *domain.Session, in ServiceInput) (ServiceView, error) {
	var res ServiceView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		now := u.Clock.Now()
		sv := domain.Service{ID: domain.ServiceID(u.IDs.NewID()), Name: in.Name, Language: in.Language,
			DefaultTemplateID: in.DefaultTemplateID, Times: in.Times, Version: 1, CreatedAt: now, UpdatedAt: now}
		if sv.Language == "" {
			ch, err := sc.cs.Church().Get(ctx)
			if err != nil {
				return err
			}
			sv.Language = ch.DefaultLanguage
		}
		if err := domain.ValidateService(&sv); err != nil {
			return err
		}
		if err := checkTemplate(ctx, sc.cs, sv.DefaultTemplateID); err != nil {
			return err
		}
		n, err := sc.cs.Services().Count(ctx)
		if err != nil {
			return err
		}
		if n >= domain.MaxServices {
			return domain.LimitError("name", domain.MaxServices, n)
		}
		sv.Times = u.withIDs(sv.Times)
		if err := sc.cs.Services().Create(ctx, sv); err != nil {
			return mapNameTaken(err, "service")
		}
		res, err = serviceView(ctx, sc, sv)
		return err
	})
	return res, err
}

// Update changes a service (templates.edit). A stale version is
// ErrVersionConflict and nothing is written.
func (u *Services) Update(ctx context.Context, sess *domain.Session, id domain.ServiceID, ch ServiceChange) (ServiceView, error) {
	var res ServiceView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		cur, err := sc.cs.Services().ByID(ctx, id)
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
		if ch.DefaultTemplateID != nil {
			next.DefaultTemplateID = *ch.DefaultTemplateID
		}
		if ch.Times != nil {
			next.Times = *ch.Times
		}
		if err := domain.ValidateService(&next); err != nil {
			return err
		}
		if ch.DefaultTemplateID != nil {
			if err := checkTemplate(ctx, sc.cs, next.DefaultTemplateID); err != nil {
				return err
			}
		}
		if ch.Times != nil {
			next.Times = u.withIDs(next.Times)
		}
		next.Version, next.UpdatedAt = cur.Version+1, u.Clock.Now()
		ok, err := sc.cs.Services().Update(ctx, next, cur.Version)
		if err != nil {
			return mapNameTaken(err, "service")
		}
		if !ok {
			return ErrVersionConflict
		}
		res, err = serviceView(ctx, sc, next)
		return err
	})
	return res, err
}

// Delete removes a service; liturgies made from it keep a copy of its name
// (templates.edit).
func (u *Services) Delete(ctx context.Context, sess *domain.Session, id domain.ServiceID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeTemplatesEdit); err != nil {
			return err
		}
		return missing(sc.cs.Services().Delete(ctx, id))
	})
}
