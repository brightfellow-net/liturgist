// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"

	"github.com/brightfellow-net/liturgist/domain"
)

// Roles holds the role editor use cases (03 §8, 04 §6).
type Roles struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// RoleActions are the advisory actions on a role.
type RoleActions struct {
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

// RoleView is a role with its member count and actions.
type RoleView struct {
	Role        domain.Role
	MemberCount int
	Actions     RoleActions
}

// ScopeInfo is a scope with its description in the caller's language.
type ScopeInfo struct {
	Scope       domain.Scope
	Description string
}

// RoleInput creates a role.
type RoleInput struct {
	Name, Description string
	Scopes            []domain.Scope
}

// RoleChange is a PATCH: nil fields are unchanged.
type RoleChange struct {
	Name, Description *string
	Scopes            *[]domain.Scope
}

// List returns the church's roles (roles.manage or members.manage).
func (u *Roles) List(ctx context.Context, sess *domain.Session) ([]RoleView, error) {
	var res []RoleView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.RequireAny(domain.ScopeRolesManage, domain.ScopeMembersManage); err != nil {
			return err
		}
		res, err = sc.roleViews(ctx)
		return err
	})
	return res, err
}

// Scopes lists every scope with its description in the caller's UI
// language: their preference, else the church default.
func (u *Roles) Scopes(ctx context.Context, sess *domain.Session) ([]ScopeInfo, error) {
	var lang string
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.RequireAny(domain.ScopeRolesManage, domain.ScopeMembersManage); err != nil {
			return err
		}
		usr, err := s.Users().ByID(ctx, sess.UserID)
		if err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		lang = usr.Preferences.UILanguage
		if lang == "" {
			lang = church.DefaultUILanguage
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]ScopeInfo, len(domain.AllScopes))
	for i, sc := range domain.AllScopes {
		out[i] = ScopeInfo{Scope: sc, Description: domain.ScopeDescription(sc, lang)}
	}
	return out, nil
}

// Create adds a custom role (roles.manage); the actor must hold every scope in it.
func (u *Roles) Create(ctx context.Context, sess *domain.Session, in RoleInput) (RoleView, error) {
	if err := domain.ValidateRole(&in.Name, &in.Description, in.Scopes); err != nil {
		return RoleView{}, err
	}
	var res RoleView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeRolesManage); err != nil {
			return err
		}
		scopes := domain.NewScopeSet(in.Scopes...)
		if err := sc.actor.RequireHeld(scopes); err != nil {
			return err
		}
		now := u.Clock.Now()
		r := domain.Role{ID: domain.RoleID(u.IDs.NewID()), Name: in.Name, Description: in.Description,
			Scopes: scopes, CreatedAt: now, UpdatedAt: now}
		if err := sc.cs.Roles().Create(ctx, r); err != nil {
			return mapRoleName(err)
		}
		res, err = sc.roleView(ctx, r.ID)
		return err
	})
	return res, err
}

// Update renames a role or changes its description or scopes (roles.manage).
// The actor must hold every scope added; the lock-out rule applies.
func (u *Roles) Update(ctx context.Context, sess *domain.Session, id domain.RoleID, ch RoleChange) (RoleView, error) {
	var scopes []domain.Scope
	if ch.Scopes != nil {
		scopes = *ch.Scopes
	}
	if err := domain.ValidateRole(ch.Name, ch.Description, scopes); err != nil {
		return RoleView{}, err
	}
	var res RoleView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeRolesManage); err != nil {
			return err
		}
		r, ok := sc.roles[id]
		if !ok {
			return notFound(ReasonMissing)
		}
		if ch.Name != nil {
			r.Name = *ch.Name
		}
		if ch.Description != nil {
			r.Description = *ch.Description
		}
		if ch.Scopes != nil {
			next := domain.NewScopeSet(scopes...)
			if err := sc.actor.RequireHeld(domain.NewScopeSet(r.Scopes.Missing(next)...)); err != nil {
				return err
			}
			r.Scopes = next
		}
		r.UpdatedAt = u.Clock.Now()
		if err := sc.cs.Roles().Update(ctx, r); err != nil {
			return mapRoleName(err)
		}
		if _, err := checkLockout(ctx, sc.cs); err != nil {
			return err
		}
		res, err = sc.roleView(ctx, id)
		return err
	})
	return res, err
}

// Delete removes a role from the church and from every member and open
// invite (roles.manage). If members hold it, the actor must hold its scopes
// (removing a role from members is rule 2); the lock-out rule applies.
func (u *Roles) Delete(ctx context.Context, sess *domain.Session, id domain.RoleID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeRolesManage); err != nil {
			return err
		}
		r, ok := sc.roles[id]
		if !ok {
			return notFound(ReasonMissing)
		}
		counts, err := sc.cs.Roles().MemberCounts(ctx)
		if err != nil {
			return err
		}
		if counts[id] > 0 {
			if err := sc.actor.RequireHeld(r.Scopes); err != nil {
				return err
			}
		}
		if err := sc.cs.Roles().Delete(ctx, id); err != nil {
			return err
		}
		_, err = checkLockout(ctx, sc.cs)
		return err
	})
}

func (c churchScope) roleViews(ctx context.Context) ([]RoleView, error) {
	counts, err := c.cs.Roles().MemberCounts(ctx)
	if err != nil {
		return nil, err
	}
	all, err := c.cs.Memberships().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoleView, 0, len(c.roles))
	for _, r := range c.roles {
		out = append(out, c.roleViewOf(r, counts[r.ID], all))
	}
	slices.SortFunc(out, func(a, b RoleView) int { return compareFold(a.Role.Name, b.Role.Name) })
	return out, nil
}

// roleView re-reads the roles (they changed in this transaction) and returns one.
func (c churchScope) roleView(ctx context.Context, id domain.RoleID) (RoleView, error) {
	c, err := c.reloadRoles(ctx)
	if err != nil {
		return RoleView{}, err
	}
	views, err := c.roleViews(ctx)
	if err != nil {
		return RoleView{}, err
	}
	for _, v := range views {
		if v.Role.ID == id {
			return v, nil
		}
	}
	return RoleView{}, ErrNotFound
}

// roleViewOf computes the actions: edit needs roles.manage; delete also needs
// the role's scopes when members hold it, and must not lock everyone out.
func (c churchScope) roleViewOf(r domain.Role, count int, all []domain.Member) RoleView {
	v := RoleView{Role: r, MemberCount: count}
	if !c.actor.Scopes.Has(domain.ScopeRolesManage) {
		return v
	}
	v.Actions.Edit = true
	if count > 0 && len(c.actor.Scopes.Missing(r.Scopes)) > 0 {
		return v
	}
	without := make(map[domain.RoleID]domain.Role, len(c.roles))
	for id, rr := range c.roles {
		if id != r.ID {
			without[id] = rr
		}
	}
	v.Actions.Delete = domain.SomeoneCanAdminister(memberships(all), without)
	return v
}

func mapRoleName(err error) error {
	var u *UniqueError
	if errors.As(err, &u) && u.Constraint == "roles_church_name_key" {
		return ErrRoleNameTaken
	}
	return err
}
