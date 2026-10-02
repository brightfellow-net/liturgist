// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// Tenant is the church a request is for, set by the tenant middleware (04 §2).
type Tenant struct{ ChurchID domain.ChurchID }

type tenantKey struct{}

// WithTenant returns ctx carrying the tenant.
func WithTenant(ctx context.Context, id domain.ChurchID) context.Context {
	return context.WithValue(ctx, tenantKey{}, Tenant{ChurchID: id})
}

// TenantFrom returns the request's tenant, or ErrNotSetUp when there is none.
func TenantFrom(ctx context.Context) (Tenant, error) {
	if t, ok := ctx.Value(tenantKey{}).(Tenant); ok && t.ChurchID != "" {
		return t, nil
	}
	return Tenant{}, ErrNotSetUp
}

// Actor is the logged-in member acting in the tenant church (04 §5).
type Actor struct {
	UserID       domain.UserID
	ChurchID     domain.ChurchID
	MembershipID domain.MembershipID
	RoleIDs      []domain.RoleID
	Scopes       domain.ScopeSet
}

// Require returns ErrForbidden unless the actor holds s.
func (a Actor) Require(s domain.Scope) error {
	if a.Scopes.Has(s) {
		return nil
	}
	return ErrForbidden
}

// RequireAny returns ErrForbidden unless the actor holds one of scopes.
func (a Actor) RequireAny(scopes ...domain.Scope) error {
	for _, s := range scopes {
		if a.Scopes.Has(s) {
			return nil
		}
	}
	return ErrForbidden
}

// RequireHeld is safeguard 2 (no escalation): the actor must hold every scope in want.
func (a Actor) RequireHeld(want domain.ScopeSet) error {
	if m := a.Scopes.Missing(want); len(m) > 0 {
		return &ScopeNotHeldError{Scopes: m}
	}
	return nil
}

// churchScope is what a church-scoped use case works with inside one transaction.
type churchScope struct {
	cs    ChurchStore
	actor Actor
	roles map[domain.RoleID]domain.Role
}

// actorIn opens the tenant church in s and loads the actor (04 §5): no
// session → ErrUnauthenticated; not a member → 404 not_member. With lock set,
// LockChurch is taken first, so the actor's scopes and every safeguard are
// evaluated under the lock (03 §8).
func actorIn(ctx context.Context, s Store, sess *domain.Session, lock bool) (churchScope, error) {
	if sess == nil {
		return churchScope{}, ErrUnauthenticated
	}
	t, err := TenantFrom(ctx)
	if err != nil {
		return churchScope{}, err
	}
	cs, err := s.ForChurch(ctx, t.ChurchID)
	if err != nil {
		return churchScope{}, err
	}
	if lock {
		if err := cs.LockChurch(ctx); err != nil {
			return churchScope{}, err
		}
	}
	m, err := cs.Memberships().ByUser(ctx, sess.UserID)
	if errors.Is(err, ErrNotFound) {
		return churchScope{}, notFound(ReasonNotMember)
	}
	if err != nil {
		return churchScope{}, err
	}
	roles, err := roleMap(ctx, cs)
	if err != nil {
		return churchScope{}, err
	}
	return churchScope{cs: cs, roles: roles, actor: Actor{
		UserID: sess.UserID, ChurchID: t.ChurchID, MembershipID: m.ID, RoleIDs: m.RoleIDs,
		Scopes: domain.EffectiveScopes(m.RoleIDs, roles),
	}}, nil
}

func roleMap(ctx context.Context, cs ChurchStore) (map[domain.RoleID]domain.Role, error) {
	list, err := cs.Roles().List(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[domain.RoleID]domain.Role, len(list))
	for _, r := range list {
		m[r.ID] = r
	}
	return m, nil
}

// scopesOf returns the union of the scopes of roleIDs.
func (c churchScope) scopesOf(roleIDs []domain.RoleID) domain.ScopeSet {
	return domain.EffectiveScopes(roleIDs, c.roles)
}
