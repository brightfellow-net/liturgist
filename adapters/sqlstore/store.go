// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"log/slog"
	"strings"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/jmoiron/sqlx"
)

type store struct {
	tx  *sqlx.Tx
	d   Dialect
	log *slog.Logger
}

func (s *store) LockInstall(ctx context.Context) error { return s.d.LockInstall(ctx, s.tx) }

func (s *store) LockUser(ctx context.Context, id domain.UserID) error {
	return s.d.LockUser(ctx, s.tx, string(id))
}

func (s *store) ForChurch(ctx context.Context, id domain.ChurchID) (app.ChurchStore, error) {
	if err := s.d.SetTenant(ctx, s.tx, string(id)); err != nil {
		return nil, err
	}
	return &churchStore{store: s, churchID: id}, nil
}

type churchStore struct {
	*store
	churchID domain.ChurchID
}

func (c *churchStore) ChurchID() domain.ChurchID { return c.churchID }

func (c *churchStore) LockChurch(ctx context.Context) error {
	return c.d.LockChurch(ctx, c.tx, string(c.churchID))
}

func (c *churchStore) Church() app.ChurchSettingsRepo  { return churchSettingsRepo{c} }
func (c *churchStore) Memberships() app.MembershipRepo { return membershipRepo{c} }
func (c *churchStore) Roles() app.RoleRepo             { return roleRepo{c} }
func (c *churchStore) Invites() app.InviteRepo         { return inviteRepo{c} }
func (c *churchStore) Songs() app.SongRepo             { return songRepo{c} }
func (c *churchStore) Readings() app.ReadingRepo       { return readingRepo{c} }

// in builds "col IN (?, ?, …)" with its arguments; n must be > 0.
func in(col string, n int) string {
	return col + " IN (?" + strings.Repeat(", ?", n-1) + ")"
}

func anys[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
