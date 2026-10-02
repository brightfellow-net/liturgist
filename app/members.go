// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"slices"
	"strings"

	"github.com/brightfellow-net/liturgist/domain"
)

// MemberActions are the advisory actions on a member (04 §5).
type MemberActions struct {
	EditRoles bool `json:"edit_roles"`
	Remove    bool `json:"remove"`
}

// MemberView is a member as the API shows it.
type MemberView struct {
	Member  domain.Member
	Roles   []domain.Role // the member's roles, by name
	Scopes  domain.ScopeSet
	Actions MemberActions
}

// memberView computes a member's roles and actions as seen by the actor.
// Actions are true only if the scope check and the safeguards would pass.
func (c churchScope) memberView(m domain.Member, all []domain.Member) MemberView {
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
	}
	return v
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

func compareFold(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }
