// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (c *churchStore) Comments() app.CommentRepo { return commentRepo{c} }

type commentRepo struct{ *churchStore }

const commentColumns = `id, liturgy_id, item_id, item_title, author_id, body, resolved_at, resolved_by, created_at`

func scanComment(scan func(...any) error) (domain.Comment, error) {
	var (
		c          domain.Comment
		item, by   *string
		resolvedAt NullTime
		created    Time
	)
	if err := scan(&c.ID, &c.LiturgyID, &item, &c.ItemTitle, &c.AuthorID, &c.Body, &resolvedAt, &by, &created); err != nil {
		return domain.Comment{}, err
	}
	c.ItemID, c.ResolvedBy, c.CreatedAt = domain.ItemID(strOf(item)), domain.UserID(strOf(by)), created.Time
	if resolvedAt.Valid {
		t := resolvedAt.Time
		c.ResolvedAt = &t
	}
	return c, nil
}

func (r commentRepo) Create(ctx context.Context, c domain.Comment) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgy_comments
		(id, church_id, liturgy_id, item_id, item_title, author_id, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, r.churchID, c.LiturgyID, nullString(string(c.ItemID)), c.ItemTitle, c.AuthorID, c.Body, r.d.TimeArg(c.CreatedAt))
	return r.d.MapError(err)
}

func (r commentRepo) List(ctx context.Context, liturgy domain.LiturgyID, resolved *bool) ([]domain.Comment, error) {
	q := `SELECT ` + commentColumns + ` FROM liturgy_comments WHERE church_id = ? AND liturgy_id = ?`
	if resolved != nil {
		if *resolved {
			q += " AND resolved_at IS NOT NULL"
		} else {
			q += " AND resolved_at IS NULL"
		}
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(q+" ORDER BY created_at, id"), r.churchID, liturgy)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Comment
	for rows.Next() {
		c, err := scanComment(rows.Scan)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, c)
	}
	return out, r.d.MapError(rows.Err())
}

func (r commentRepo) count(ctx context.Context, liturgy domain.LiturgyID, where string) (int, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM liturgy_comments WHERE church_id = ? AND liturgy_id = ?"+where), r.churchID, liturgy)
	return n, r.d.MapError(err)
}

func (r commentRepo) Count(ctx context.Context, liturgy domain.LiturgyID) (int, error) {
	return r.count(ctx, liturgy, "")
}

func (r commentRepo) Open(ctx context.Context, liturgy domain.LiturgyID) (int, error) {
	return r.count(ctx, liturgy, " AND resolved_at IS NULL")
}

func (r commentRepo) ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.CommentID) (domain.Comment, error) {
	c, err := scanComment(r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT `+commentColumns+` FROM liturgy_comments
		WHERE church_id = ? AND liturgy_id = ? AND id = ?`), r.churchID, liturgy, id).Scan)
	return c, r.d.MapError(err)
}

func (r commentRepo) SetResolved(ctx context.Context, liturgy domain.LiturgyID, id domain.CommentID, resolved bool, by domain.UserID, now time.Time) (bool, error) {
	if resolved {
		res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_comments SET resolved_at = ?, resolved_by = ?
			WHERE church_id = ? AND liturgy_id = ? AND id = ? AND resolved_at IS NULL`), r.d.TimeArg(now), by, r.churchID, liturgy, id)
		return changed(r.d, res, err)
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_comments SET resolved_at = NULL, resolved_by = NULL
		WHERE church_id = ? AND liturgy_id = ? AND id = ? AND resolved_at IS NOT NULL`), r.churchID, liturgy, id)
	return changed(r.d, res, err)
}
