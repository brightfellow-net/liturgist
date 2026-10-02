// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Members holds the member list and role-assignment use cases (04 §6).
type Members struct {
	Tx           Tx
	Clock        Clock
	Entitlements Entitlements
}

// MemberActions are the advisory actions on a member (04 §5).
type MemberActions struct {
	EditRoles       bool `json:"edit_roles"`
	Remove          bool `json:"remove"`
	CreateResetLink bool `json:"create_reset_link"`
}

// ResetSummary is a member's latest reset link, never the link itself (03 §9).
type ResetSummary struct {
	CreatedByName *string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
}

// MemberView is a member as the API shows it.
type MemberView struct {
	Member    domain.Member
	Roles     []domain.Role // the member's roles, by name
	Scopes    domain.ScopeSet
	LastReset *ResetSummary // only for viewers with members.manage
	Actions   MemberActions
}

// MemberList is GET /members.
type MemberList struct {
	Members []MemberView
	Used    int
	Max     *int // nil = unlimited
}

// memberView computes a member's roles and actions as seen by the actor.
// Actions are true only if the scope check and the safeguards would pass.
func (c churchScope) memberView(ctx context.Context, s Store, m domain.Member, all []domain.Member, withReset bool) (MemberView, error) {
	v := MemberView{Member: m, Scopes: c.scopesOf(m.RoleIDs)}
	for _, id := range m.RoleIDs {
		if r, ok := c.roles[id]; ok {
			v.Roles = append(v.Roles, r)
		}
	}
	slices.SortFunc(v.Roles, func(a, b domain.Role) int { return compareFold(a.Name, b.Name) })
	a := c.actor.Scopes
	holds := len(a.Missing(v.Scopes)) == 0
	v.Actions.EditRoles = a.Has(domain.ScopeRolesManage) && holds
	if a.Has(domain.ScopeMembersManage) && holds {
		v.Actions.Remove = domain.SomeoneCanAdminister(membershipsWithout(all, m.ID), c.roles)
		elsewhere, err := memberElsewhere(ctx, s, m.UserID, c.actor.ChurchID)
		if err != nil {
			return MemberView{}, err
		}
		v.Actions.CreateResetLink = !elsewhere
	}
	if withReset && a.Has(domain.ScopeMembersManage) {
		latest, err := s.PasswordResets().Latest(ctx, []domain.UserID{m.UserID})
		if err != nil {
			return MemberView{}, err
		}
		if r, ok := latest[m.UserID]; ok {
			sum, err := resetSummary(ctx, s, r)
			if err != nil {
				return MemberView{}, err
			}
			v.LastReset = &sum
		}
	}
	return v, nil
}

func resetSummary(ctx context.Context, s Store, r domain.PasswordReset) (ResetSummary, error) {
	sum := ResetSummary{CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt}
	if !r.UsedAt.IsZero() {
		used := r.UsedAt
		sum.UsedAt = &used
	}
	if r.CreatedBy != "" {
		u, err := s.Users().ByID(ctx, r.CreatedBy)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return ResetSummary{}, err
		}
		if err == nil {
			sum.CreatedByName = &u.Name
		}
	}
	return sum, nil
}

func memberships(all []domain.Member) []domain.Membership {
	out := make([]domain.Membership, len(all))
	for i, m := range all {
		out[i] = m.Membership
	}
	return out
}

func membershipsWithout(all []domain.Member, id domain.MembershipID) []domain.Membership {
	out := make([]domain.Membership, 0, len(all))
	for _, m := range all {
		if m.ID != id {
			out = append(out, m.Membership)
		}
	}
	return out
}

// memberElsewhere reports whether the user belongs to a church other than this one.
func memberElsewhere(ctx context.Context, s Store, user domain.UserID, church domain.ChurchID) (bool, error) {
	ids, err := s.Users().MembershipChurchIDs(ctx, user)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id != church {
			return true, nil
		}
	}
	return false, nil
}

// teamUsage returns memberships + pending invites and the team-member limit.
func teamUsage(ctx context.Context, cs ChurchStore, now time.Time) (int, error) {
	members, err := cs.Memberships().Count(ctx)
	if err != nil {
		return 0, err
	}
	pending, err := cs.Invites().CountPending(ctx, now)
	return members + pending, err
}

// teamLimit asks Entitlements before the transaction (no outside calls inside
// a transaction); an error is unavailable, never "allowed" (04 §8).
func teamLimit(ctx context.Context, e Entitlements) (Limit, error) {
	t, err := TenantFrom(ctx)
	if err != nil {
		return Limit{}, err
	}
	l, err := e.Limit(ctx, t.ChurchID, LimitMaxTeamMembers)
	if err != nil {
		return Limit{}, errors.Join(ErrUnavailable, err)
	}
	return l, nil
}

// List returns the members (members.view) with usage.
func (u *Members) List(ctx context.Context, sess *domain.Session) (MemberList, error) {
	if _, err := TenantFrom(ctx); err != nil {
		return MemberList{}, err
	}
	limit, err := teamLimit(ctx, u.Entitlements)
	if err != nil {
		return MemberList{}, err
	}
	var res MemberList
	err = u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersView); err != nil {
			return err
		}
		all, err := sc.cs.Memberships().List(ctx)
		if err != nil {
			return err
		}
		res = MemberList{}
		for _, m := range all {
			v, err := sc.memberView(ctx, s, m, all, true)
			if err != nil {
				return err
			}
			res.Members = append(res.Members, v)
		}
		res.Used, err = teamUsage(ctx, sc.cs, u.Clock.Now())
		return err
	})
	if err == nil && !limit.Unlimited {
		res.Max = &limit.Max
	}
	return res, err
}

// SetRoles replaces a member's roles (roles.manage). The actor must hold the
// scopes of every role added or removed; the lock-out rule applies.
func (u *Members) SetRoles(ctx context.Context, sess *domain.Session, id domain.MembershipID, roleIDs []domain.RoleID) (MemberView, error) {
	var res MemberView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeRolesManage); err != nil {
			return err
		}
		m, err := sc.cs.Memberships().ByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return notFound(ReasonMissing)
		}
		if err != nil {
			return err
		}
		roleIDs = uniqueRoles(roleIDs)
		for _, r := range roleIDs {
			if _, ok := sc.roles[r]; !ok {
				return &domain.InvalidInputError{Field: "role_ids", Message: "Unknown role."}
			}
		}
		changed := domain.ScopeSet{}
		for _, r := range symmetricDiff(m.RoleIDs, roleIDs) {
			changed.Add(sc.roles[r].Scopes)
		}
		if err := sc.actor.RequireHeld(changed); err != nil {
			return err
		}
		if err := sc.cs.Memberships().SetRoles(ctx, id, roleIDs); err != nil {
			return err
		}
		all, err := checkLockout(ctx, sc.cs)
		if err != nil {
			return err
		}
		for _, mm := range all {
			if mm.ID == id {
				res, err = sc.memberView(ctx, s, mm, all, true)
				return err
			}
		}
		return ErrNotFound
	})
	return res, err
}

// Remove deletes a membership (members.manage); the user account and sessions
// remain. The actor must hold the member's scopes; the lock-out rule applies.
func (u *Members) Remove(ctx context.Context, sess *domain.Session, id domain.MembershipID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		m, err := sc.cs.Memberships().ByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return notFound(ReasonMissing)
		}
		if err != nil {
			return err
		}
		if err := sc.actor.RequireHeld(sc.scopesOf(m.RoleIDs)); err != nil {
			return err
		}
		if err := sc.cs.Memberships().Delete(ctx, id); err != nil {
			return err
		}
		_, err = checkLockout(ctx, sc.cs)
		return err
	})
}

// checkLockout re-reads memberships and roles after a change and returns
// ErrLockout if nobody holds both roles.manage and members.manage (03 §8 rule 1).
func checkLockout(ctx context.Context, cs ChurchStore) ([]domain.Member, error) {
	all, err := cs.Memberships().List(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := roleMap(ctx, cs)
	if err != nil {
		return nil, err
	}
	if !domain.SomeoneCanAdminister(memberships(all), roles) {
		return nil, ErrLockout
	}
	return all, nil
}

// reloadRoles returns c with the role map re-read (after roles changed in this transaction).
func (c churchScope) reloadRoles(ctx context.Context) (churchScope, error) {
	roles, err := roleMap(ctx, c.cs)
	c.roles = roles
	return c, err
}

func compareFold(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }

func uniqueRoles(ids []domain.RoleID) []domain.RoleID {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func symmetricDiff(a, b []domain.RoleID) []domain.RoleID {
	var out []domain.RoleID
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			out = append(out, x)
		}
	}
	return out
}
