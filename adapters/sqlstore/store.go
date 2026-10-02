// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/jmoiron/sqlx"
)

type store struct {
	tx *sqlx.Tx
	d  Dialect
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
