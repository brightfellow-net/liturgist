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

func (r publishedRepo) Latest(ctx context.Context, liturgy domain.LiturgyID) (domain.PublishedVersion, error) {
	var (
		v       domain.PublishedVersion
		content string
		at      Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT id, liturgy_id, number, content, published_by, published_at FROM published_versions
		WHERE church_id = ? AND liturgy_id = ? ORDER BY number DESC LIMIT 1`), r.churchID, liturgy).
		Scan(&v.ID, &v.LiturgyID, &v.Number, &content, &v.PublishedBy, &at)
	if err != nil {
		return domain.PublishedVersion{}, r.d.MapError(err)
	}
	v.Content, v.PublishedAt = []byte(content), at.Time
	return v, nil
}

// newestVersion is true for the row of v that has the highest number of its liturgy.
const newestVersion = `v.number = (SELECT MAX(x.number) FROM published_versions x WHERE x.church_id = v.church_id AND x.liturgy_id = v.liturgy_id)`

func (r publishedRepo) ListLatest(ctx context.Context, f app.PublishedFilter) ([]app.PublishedRow, int, error) {
	where, args := `church_id = ? AND EXISTS (SELECT 1 FROM published_versions v WHERE v.church_id = liturgies.church_id AND v.liturgy_id = liturgies.id)`, []any{r.churchID}
	if f.From != "" {
		where += " AND date >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND date <= ?"
		args = append(args, f.To)
	}
	switch f.Archived {
	case app.ArchivedOnly:
		where += " AND archived_at IS NOT NULL"
	case app.ArchivedAll:
	default:
		where += " AND archived_at IS NULL"
	}
	var total int
	if err := r.tx.GetContext(ctx, &total, r.d.Rebind("SELECT COUNT(*) FROM liturgies WHERE "+where), args...); err != nil {
		return nil, 0, r.d.MapError(err)
	}
	// Newest first; a liturgy without a time sorts after those with one.
	q := "SELECT " + liturgyColumns + `,
		(SELECT MAX(v.number) FROM published_versions v WHERE v.church_id = liturgies.church_id AND v.liturgy_id = liturgies.id),
		(SELECT v.published_at FROM published_versions v WHERE v.church_id = liturgies.church_id AND v.liturgy_id = liturgies.id AND ` + newestVersion + `)
		FROM liturgies WHERE ` + where + " ORDER BY date DESC, (CASE WHEN time = '' THEN 1 ELSE 0 END), time DESC, id DESC LIMIT ? OFFSET ?"
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(q), append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []app.PublishedRow
	for rows.Next() {
		var (
			row app.PublishedRow
			at  Time
		)
		row.Liturgy, err = scanLiturgy(func(dest ...any) error { return rows.Scan(append(dest, &row.Number, &at)...) })
		if err != nil {
			return nil, 0, r.d.MapError(err)
		}
		row.PublishedAt = at.Time
		out = append(out, row)
	}
	return out, total, r.d.MapError(rows.Err())
}

func (r publishedRepo) Upcoming(ctx context.Context, user domain.UserID, today string, limit int) ([]app.PublishedUpcoming, error) {
	q := "SELECT " + liturgyColumns + ` FROM liturgies WHERE church_id = ? AND archived_at IS NULL AND date >= ?
		AND EXISTS (SELECT 1 FROM published_versions v JOIN published_assignees a ON a.church_id = v.church_id AND a.version_id = v.id
			WHERE v.church_id = liturgies.church_id AND v.liturgy_id = liturgies.id AND a.user_id = ? AND ` + newestVersion + `)
		ORDER BY date, (CASE WHEN time = '' THEN 1 ELSE 0 END), time, id LIMIT ?`
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(q), r.churchID, today, user, limit)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	var out []app.PublishedUpcoming
	for rows.Next() {
		l, err := scanLiturgy(rows.Scan)
		if err != nil {
			_ = rows.Close()
			return nil, r.d.MapError(err)
		}
		out = append(out, app.PublishedUpcoming{Liturgy: l})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, r.d.MapError(err)
	}
	_ = rows.Close()
	for i := range out {
		if out[i].Version, err = r.Latest(ctx, out[i].Liturgy.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
