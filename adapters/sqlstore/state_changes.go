// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (c *churchStore) StateChanges() app.StateChangeRepo { return stateChangeRepo{c} }

type stateChangeRepo struct{ *churchStore }

const stateChangeColumns = `id, liturgy_id, from_state, to_state, user_id, note, edit_seq, created_at`

func scanStateChange(scan func(...any) error) (domain.StateChange, error) {
	var (
		c       domain.StateChange
		created Time
	)
	if err := scan(&c.ID, &c.LiturgyID, &c.From, &c.To, &c.UserID, &c.Note, &c.EditSeq, &created); err != nil {
		return domain.StateChange{}, err
	}
	c.CreatedAt = created.Time
	return c, nil
}

func (r stateChangeRepo) Append(ctx context.Context, c domain.StateChange) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgy_state_changes
		(id, church_id, liturgy_id, from_state, to_state, user_id, note, edit_seq, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, r.churchID, c.LiturgyID, string(c.From), string(c.To), c.UserID, c.Note, c.EditSeq, r.d.TimeArg(c.CreatedAt))
	return r.d.MapError(err)
}

func (r stateChangeRepo) List(ctx context.Context, liturgy domain.LiturgyID, limit, offset int) ([]domain.StateChange, int, error) {
	var total int
	if err := r.tx.GetContext(ctx, &total, r.d.Rebind(
		"SELECT COUNT(*) FROM liturgy_state_changes WHERE church_id = ? AND liturgy_id = ?"), r.churchID, liturgy); err != nil {
		return nil, 0, r.d.MapError(err)
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT `+stateChangeColumns+` FROM liturgy_state_changes
		WHERE church_id = ? AND liturgy_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`), r.churchID, liturgy, limit, offset)
	if err != nil {
		return nil, 0, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.StateChange
	for rows.Next() {
		c, err := scanStateChange(rows.Scan)
		if err != nil {
			return nil, 0, r.d.MapError(err)
		}
		out = append(out, c)
	}
	return out, total, r.d.MapError(rows.Err())
}

func (r stateChangeRepo) Last(ctx context.Context, liturgy domain.LiturgyID) (domain.StateChange, error) {
	c, err := scanStateChange(r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT `+stateChangeColumns+` FROM liturgy_state_changes
		WHERE church_id = ? AND liturgy_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`), r.churchID, liturgy).Scan)
	return c, r.d.MapError(err)
}
