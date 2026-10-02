// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

const inviteColumns = `id, church_id, token_hash, name, email, phone, created_by, created_at, expires_at,
	accepted_at, accepted_user_id, cancelled_at`

func scanInvite(d Dialect, row scanner) (domain.Invite, error) {
	var (
		i                                  domain.Invite
		email, phone, createdBy, acceptedU sql.NullString
		created, expires                   Time
		accepted, cancelled                NullTime
	)
	err := row.Scan(&i.ID, &i.ChurchID, &i.TokenHash, &i.Name, &email, &phone, &createdBy, &created, &expires,
		&accepted, &acceptedU, &cancelled)
	if err != nil {
		return domain.Invite{}, d.MapError(err)
	}
	i.Email, i.Phone, i.CreatedBy, i.AcceptedUserID = email.String, phone.String, domain.UserID(createdBy.String), domain.UserID(acceptedU.String)
	i.CreatedAt, i.ExpiresAt, i.AcceptedAt, i.CancelledAt = created.Time, expires.Time, accepted.Time, cancelled.Time
	return i, nil
}

// openInvite is the condition for invites neither accepted nor cancelled.
const openInvite = "accepted_at IS NULL AND cancelled_at IS NULL"

// --- church-scoped ---

type inviteRepo struct{ *churchStore }

func (r inviteRepo) query(ctx context.Context, where string, args ...any) ([]domain.Invite, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+inviteColumns+" FROM invites WHERE church_id = ? "+where),
		append([]any{r.churchID}, args...)...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Invite
	for rows.Next() {
		i, err := scanInvite(r.d, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, r.d.MapError(err)
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]domain.InviteID, len(out))
	for i, inv := range out {
		ids[i] = inv.ID
	}
	var roles []struct {
		InviteID domain.InviteID `db:"invite_id"`
		RoleID   domain.RoleID   `db:"role_id"`
	}
	if err := r.tx.SelectContext(ctx, &roles, r.d.Rebind("SELECT invite_id, role_id FROM invite_roles WHERE church_id = ? AND "+
		in("invite_id", len(ids))+" ORDER BY role_id"), append([]any{r.churchID}, anys(ids)...)...); err != nil {
		return nil, r.d.MapError(err)
	}
	byInvite := map[domain.InviteID][]domain.RoleID{}
	for _, x := range roles {
		byInvite[x.InviteID] = append(byInvite[x.InviteID], x.RoleID)
	}
	for i := range out {
		out[i].RoleIDs = byInvite[out[i].ID]
	}
	return out, nil
}

func (r inviteRepo) List(ctx context.Context) ([]domain.Invite, error) {
	return r.query(ctx, "AND "+openInvite+" ORDER BY created_at DESC, id DESC")
}

func (r inviteRepo) ByID(ctx context.Context, id domain.InviteID) (domain.Invite, error) {
	list, err := r.query(ctx, "AND id = ?", id)
	if err != nil {
		return domain.Invite{}, err
	}
	if len(list) == 0 {
		return domain.Invite{}, app.ErrNotFound
	}
	return list[0], nil
}

func (r inviteRepo) Create(ctx context.Context, i domain.Invite) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO invites
		(id, church_id, token_hash, name, email, phone, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		i.ID, r.churchID, i.TokenHash, i.Name, nullString(i.Email), nullString(i.Phone), nullString(string(i.CreatedBy)),
		r.d.TimeArg(i.CreatedAt), r.d.TimeArg(i.ExpiresAt))
	if err != nil {
		return r.d.MapError(err)
	}
	for _, role := range i.RoleIDs {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO invite_roles (church_id, invite_id, role_id) VALUES (?, ?, ?)"),
			r.churchID, i.ID, role); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r inviteRepo) CancelExpired(ctx context.Context, email, phone string, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE invites SET cancelled_at = ?
		WHERE church_id = ? AND `+openInvite+` AND expires_at <= ? AND (email = ? OR phone = ?)`),
		r.d.TimeArg(now), r.churchID, r.d.TimeArg(now), nullString(email), nullString(phone))
	return r.d.MapError(err)
}

func (r inviteRepo) CountPending(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT count(*) FROM invites WHERE church_id = ? AND "+openInvite+" AND expires_at > ?"),
		r.churchID, r.d.TimeArg(now)).Scan(&n)
	return n, r.d.MapError(err)
}

func (r inviteRepo) changed(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, r.d.MapError(err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r inviteRepo) Renew(ctx context.Context, id domain.InviteID, hash string, expires time.Time) (bool, error) {
	return r.changed(r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE invites SET token_hash = ?, expires_at = ?
		WHERE church_id = ? AND id = ? AND `+openInvite), hash, r.d.TimeArg(expires), r.churchID, id))
}

func (r inviteRepo) Cancel(ctx context.Context, id domain.InviteID, now time.Time) (bool, error) {
	return r.changed(r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE invites SET cancelled_at = ?
		WHERE church_id = ? AND id = ? AND `+openInvite), r.d.TimeArg(now), r.churchID, id))
}

// --- platform: by token ---

type inviteTokenRepo struct{ *store }

func (s *store) InviteTokens() app.InviteTokenRepo { return inviteTokenRepo{s} }

func (r inviteTokenRepo) ByTokenHash(ctx context.Context, hash string) (domain.Invite, error) {
	return scanInvite(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+inviteColumns+" FROM invites WHERE token_hash = ?"), hash))
}

func (r inviteTokenRepo) Claim(ctx context.Context, hash string, user domain.UserID, now time.Time) (domain.Invite, bool, error) {
	i, err := scanInvite(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind(`UPDATE invites SET accepted_at = ?, accepted_user_id = ?
		WHERE token_hash = ? AND `+openInvite+` AND expires_at > ? RETURNING `+inviteColumns),
		r.d.TimeArg(now), nullString(string(user)), hash, r.d.TimeArg(now)))
	if errors.Is(err, app.ErrNotFound) {
		return domain.Invite{}, false, nil
	}
	return i, err == nil, err
}

func (r inviteTokenRepo) SetAcceptedUser(ctx context.Context, id domain.InviteID, user domain.UserID) error {
	return r.exec1(ctx, "UPDATE invites SET accepted_user_id = ? WHERE id = ? AND accepted_at IS NOT NULL", user, id)
}

// --- platform: password resets ---

type resetRepo struct{ *store }

func (s *store) PasswordResets() app.PasswordResetRepo { return resetRepo{s} }

const resetColumns = "id, user_id, token_hash, created_by, created_at, expires_at, used_at"

func scanReset(d Dialect, row scanner) (domain.PasswordReset, error) {
	var (
		p                domain.PasswordReset
		createdBy        sql.NullString
		created, expires Time
		used             NullTime
	)
	if err := row.Scan(&p.ID, &p.UserID, &p.TokenHash, &createdBy, &created, &expires, &used); err != nil {
		return domain.PasswordReset{}, d.MapError(err)
	}
	p.CreatedBy, p.CreatedAt, p.ExpiresAt, p.UsedAt = domain.UserID(createdBy.String), created.Time, expires.Time, used.Time
	return p, nil
}

func (r resetRepo) Create(ctx context.Context, p domain.PasswordReset) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO password_resets ("+resetColumns+") VALUES (?, ?, ?, ?, ?, ?, NULL)"),
		p.ID, p.UserID, p.TokenHash, nullString(string(p.CreatedBy)), r.d.TimeArg(p.CreatedAt), r.d.TimeArg(p.ExpiresAt))
	return r.d.MapError(err)
}

func (r resetRepo) CloseOpen(ctx context.Context, user domain.UserID, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE password_resets SET used_at = ? WHERE user_id = ? AND used_at IS NULL"),
		r.d.TimeArg(now), user)
	return r.d.MapError(err)
}

func (r resetRepo) ByTokenHash(ctx context.Context, hash string) (domain.PasswordReset, error) {
	return scanReset(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+resetColumns+" FROM password_resets WHERE token_hash = ?"), hash))
}

func (r resetRepo) Claim(ctx context.Context, hash string, now time.Time) (domain.PasswordReset, bool, error) {
	p, err := scanReset(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind(`UPDATE password_resets SET used_at = ?
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ? RETURNING `+resetColumns),
		r.d.TimeArg(now), hash, r.d.TimeArg(now)))
	if errors.Is(err, app.ErrNotFound) {
		return domain.PasswordReset{}, false, nil
	}
	return p, err == nil, err
}

func (r resetRepo) Latest(ctx context.Context, users []domain.UserID) (map[domain.UserID]domain.PasswordReset, error) {
	out := map[domain.UserID]domain.PasswordReset{}
	if len(users) == 0 {
		return out, nil
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+resetColumns+" FROM password_resets WHERE "+
		in("user_id", len(users))+" ORDER BY created_at, id"), anys(users)...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		p, err := scanReset(r.d, rows)
		if err != nil {
			return nil, err
		}
		out[p.UserID] = p // ordered oldest first, so the newest wins
	}
	return out, r.d.MapError(rows.Err())
}

func (r resetRepo) DeleteOld(ctx context.Context, cutoff time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM password_resets WHERE expires_at < ? OR used_at < ?"),
		r.d.TimeArg(cutoff), r.d.TimeArg(cutoff))
	return r.d.MapError(err)
}
