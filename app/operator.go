// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Operator holds the command-line recovery use cases (01 §6, 03 §9). They
// need no session: whoever runs the binary controls the server.
type Operator struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// UserLine is one line of "liturgist user list".
type UserLine struct {
	User  domain.User
	Roles []string // role names in the church; nil when not a member
}

// ListUsers lists every user with their roles in the church (if set up).
func (o *Operator) ListUsers(ctx context.Context, church domain.ChurchID) ([]UserLine, error) {
	var out []UserLine
	err := o.Tx.Read(ctx, func(s Store) error {
		users, err := s.Users().List(ctx)
		if err != nil {
			return err
		}
		byUser := map[domain.UserID][]string{}
		member := map[domain.UserID]bool{}
		if church != "" {
			cs, err := s.ForChurch(ctx, church)
			if err != nil {
				return err
			}
			roles, err := roleMap(ctx, cs)
			if err != nil {
				return err
			}
			all, err := cs.Memberships().List(ctx)
			if err != nil {
				return err
			}
			for _, m := range all {
				member[m.UserID] = true
				names := []string{}
				for _, id := range m.RoleIDs {
					names = append(names, roles[id].Name)
				}
				byUser[m.UserID] = names
			}
		}
		out = make([]UserLine, len(users))
		for i, u := range users {
			out[i] = UserLine{User: u}
			if member[u.ID] {
				out[i].Roles = byUser[u.ID]
			}
		}
		return nil
	})
	return out, err
}

// GrantAdmin gives a member the ready-made Church admin role, recreating it
// with its default scopes if it was deleted. ErrNotFound: no such user;
// ErrNotMember: the user isn't a member.
func (o *Operator) GrantAdmin(ctx context.Context, church domain.ChurchID, identifier string) (domain.User, error) {
	ident, err := domain.ParseIdentifier(identifier)
	if err != nil {
		return domain.User{}, err
	}
	var usr domain.User
	err = o.Tx.Write(ctx, func(s Store) error {
		cs, err := s.ForChurch(ctx, church)
		if err != nil {
			return err
		}
		if err := cs.LockChurch(ctx); err != nil {
			return err
		}
		if usr, err = s.Users().ByIdentifier(ctx, ident); err != nil {
			return err
		}
		m, err := cs.Memberships().ByUser(ctx, usr.ID)
		if errors.Is(err, ErrNotFound) {
			return ErrNotMember
		}
		if err != nil {
			return err
		}
		role, err := cs.Roles().ByOrigin(ctx, domain.OriginChurchAdmin)
		if errors.Is(err, ErrNotFound) {
			if role, err = o.recreateAdminRole(ctx, cs); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		for _, id := range m.RoleIDs {
			if id == role.ID {
				return nil
			}
		}
		return cs.Memberships().SetRoles(ctx, m.ID, append(m.RoleIDs, role.ID))
	})
	return usr, err
}

func (o *Operator) recreateAdminRole(ctx context.Context, cs ChurchStore) (domain.Role, error) {
	church, err := cs.Church().Get(ctx)
	if err != nil {
		return domain.Role{}, err
	}
	rm, ok := domain.ReadyMade(domain.OriginChurchAdmin)
	if !ok {
		return domain.Role{}, errNoReadyMadeOrigin
	}
	now := o.Clock.Now()
	r := readyMadeRole(rm, church.DefaultUILanguage, domain.RoleID(o.IDs.NewID()), now)
	err = cs.Roles().Create(ctx, r)
	var u *UniqueError
	if errors.As(err, &u) && u.Constraint == "roles_church_name_key" {
		r.Name += " (" + now.Format("2006-01-02") + ")" // a custom role took the default name
		err = cs.Roles().Create(ctx, r)
	}
	return r, err
}

// ClearThrottle deletes login-throttle counters: for one identifier, one
// client address, or all.
func (o *Operator) ClearThrottle(ctx context.Context, identifier, addr string, all bool) error {
	return o.Tx.Write(ctx, func(s Store) error {
		switch {
		case all:
			return s.AuthThrottle().DeleteAll(ctx)
		case identifier != "":
			ident, err := domain.ParseIdentifier(identifier)
			key := identifier
			if err == nil {
				key = ident.Value
			}
			return s.AuthThrottle().DeleteForIdentifier(ctx, key)
		default:
			return s.AuthThrottle().DeleteForAddr(ctx, addr)
		}
	})
}

// Cleanup is storage housekeeping; expiry itself is decided at use time (03 §11).
type Cleanup struct {
	Tx    Tx
	Clock Clock
}

// Throttle deletes counters whose window and lock have ended (every 10 minutes).
func (c *Cleanup) Throttle(ctx context.Context) error {
	now := c.Clock.Now()
	return c.Tx.Write(ctx, func(s Store) error {
		for kind, rule := range domain.ThrottleRules {
			if err := s.AuthThrottle().DeleteEnded(ctx, kind, now.Add(-rule.Window), now); err != nil {
				return err
			}
		}
		return nil
	})
}

// Hourly deletes expired sessions, old reset links and expired setup tokens.
// Invites are never deleted (history).
func (c *Cleanup) Hourly(ctx context.Context) error {
	now := c.Clock.Now()
	return c.Tx.Write(ctx, func(s Store) error {
		if err := s.Sessions().DeleteExpired(ctx, now); err != nil {
			return err
		}
		if err := s.PasswordResets().DeleteOld(ctx, now.Add(-7*24*time.Hour)); err != nil {
			return err
		}
		return s.SetupTokens().DeleteExpired(ctx, now)
	})
}
