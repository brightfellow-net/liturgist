// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (c *churchStore) Published() app.PublishedRepo { return publishedRepo{c} }

type publishedRepo struct{ *churchStore }

func (r publishedRepo) Create(ctx context.Context, v domain.PublishedVersion, assignees []domain.UserID) (int, error) {
	var number int
	err := r.tx.GetContext(ctx, &number, r.d.Rebind(
		"SELECT COALESCE(MAX(number), 0) + 1 FROM published_versions WHERE church_id = ? AND liturgy_id = ?"), r.churchID, v.LiturgyID)
	if err != nil {
		return 0, r.d.MapError(err)
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO published_versions
		(id, church_id, liturgy_id, number, content, published_by, published_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		v.ID, r.churchID, v.LiturgyID, number, string(v.Content), v.PublishedBy, r.d.TimeArg(v.PublishedAt)); err != nil {
		return 0, r.d.MapError(err)
	}
	for _, u := range assignees {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(
			"INSERT INTO published_assignees (church_id, version_id, user_id) VALUES (?, ?, ?)"), r.churchID, v.ID, u); err != nil {
			return 0, r.d.MapError(err)
		}
	}
	return number, nil
}

func (r publishedRepo) Info(ctx context.Context, liturgy domain.LiturgyID) (domain.PublishedVersion, error) {
	var (
		v  domain.PublishedVersion
		at Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT id, liturgy_id, number, published_by, published_at FROM published_versions
		WHERE church_id = ? AND liturgy_id = ? ORDER BY number DESC LIMIT 1`), r.churchID, liturgy).Scan(&v.ID, &v.LiturgyID, &v.Number, &v.PublishedBy, &at)
	if err != nil {
		return domain.PublishedVersion{}, r.d.MapError(err)
	}
	v.PublishedAt = at.Time
	return v, nil
}

func (r publishedRepo) Exists(ctx context.Context, liturgy domain.LiturgyID) (bool, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind(
		"SELECT COUNT(*) FROM published_versions WHERE church_id = ? AND liturgy_id = ?"), r.churchID, liturgy)
	return n > 0, r.d.MapError(err)
}
