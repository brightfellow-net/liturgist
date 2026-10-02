// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "time"

// Role is a church-defined set of scopes (03 §8).
type Role struct {
	ID          RoleID
	Name        string
	Description string
	Origin      RoleOrigin // "" for custom roles
	Scopes      ScopeSet
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Membership links a user to a church with roles.
type Membership struct {
	ID        MembershipID
	UserID    UserID
	RoleIDs   []RoleID
	CreatedAt time.Time
}

// Member is a membership with its user, for listings.
type Member struct {
	Membership
	User User
}

// EffectiveScopes is the union of the scopes of the given roles (03 §8);
// role IDs not in roles grant nothing.
func EffectiveScopes(roleIDs []RoleID, roles map[RoleID]Role) ScopeSet {
	s := ScopeSet{}
	for _, id := range roleIDs {
		if r, ok := roles[id]; ok {
			s.Add(r.Scopes)
		}
	}
	return s
}

// SomeoneCanAdminister reports whether at least one membership holds both
// roles.manage and members.manage (03 §8 rule 1, the lock-out rule).
func SomeoneCanAdminister(members []Membership, roles map[RoleID]Role) bool {
	for _, m := range members {
		if EffectiveScopes(m.RoleIDs, roles).CanAdminister() {
			return true
		}
	}
	return false
}
