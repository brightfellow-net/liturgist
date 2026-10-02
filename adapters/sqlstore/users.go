// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

type userRepo struct{ *store }

func (s *store) Users() app.UserRepo { return userRepo{s} }

const userColumns = "id, name, email, phone, password_hash, preferences, last_seen_at, created_at, updated_at"

type scanner interface{ Scan(...any) error }

func (r userRepo) scan(row scanner) (domain.User, error) { return r.scanWith(row) }

// scanWith scans leading columns into before, then the user columns.
func (r userRepo) scanWith(row scanner, before ...any) (domain.User, error) {
	var (
		u            domain.User
		email, phone sql.NullString
		lastSeen     NullTime
		created, upd Time
	)
	dest := append(before, &u.ID, &u.Name, &email, &phone, &u.PasswordHash, jsonValue{&u.Preferences}, &lastSeen, &created, &upd)
	err := row.Scan(dest...)
	if err != nil {
		return domain.User{}, r.d.MapError(err)
	}
	u.Email, u.Phone, u.LastSeenAt, u.CreatedAt, u.UpdatedAt = email.String, phone.String, lastSeen.Time, created.Time, upd.Time
	return u, nil
}

func (r userRepo) ByIdentifier(ctx context.Context, id domain.Identifier) (domain.User, error) {
	col := "email"
	if id.Kind == domain.Phone {
		col = "phone"
	}
	return r.scan(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+userColumns+" FROM users WHERE "+col+" = ?"), id.Value))
}

func (r userRepo) ByID(ctx context.Context, id domain.UserID) (domain.User, error) {
	return r.scan(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+userColumns+" FROM users WHERE id = ?"), id))
}

func (r userRepo) Create(ctx context.Context, u domain.User) error {
	prefs, err := jsonArg(u.Preferences)
	if err != nil {
		return err
	}
	_, err = r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO users
		(id, name, email, phone, password_hash, preferences, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.Name, nullString(u.Email), nullString(u.Phone), u.PasswordHash, prefs,
		r.d.TimeArg(u.CreatedAt), r.d.TimeArg(u.UpdatedAt))
	return r.d.MapError(err)
}

func (r userRepo) SetPasswordHash(ctx context.Context, id domain.UserID, hash string, now time.Time) error {
	return r.exec1(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", hash, r.d.TimeArg(now), id)
}

func (r userRepo) UpdateProfile(ctx context.Context, id domain.UserID, name string, prefs domain.Preferences, now time.Time) error {
	p, err := jsonArg(prefs)
	if err != nil {
		return err
	}
	return r.exec1(ctx, "UPDATE users SET name = ?, preferences = ?, updated_at = ? WHERE id = ?", name, p, r.d.TimeArg(now), id)
}

func (r userRepo) Touch(ctx context.Context, id domain.UserID, now time.Time) error {
	return r.exec1(ctx, "UPDATE users SET last_seen_at = ? WHERE id = ?", r.d.TimeArg(now), id)
}

func (r userRepo) MembershipChurchIDs(ctx context.Context, id domain.UserID) ([]domain.ChurchID, error) {
	var ids []domain.ChurchID
	err := r.tx.SelectContext(ctx, &ids, r.d.Rebind("SELECT church_id FROM memberships WHERE user_id = ? ORDER BY church_id"), id)
	return ids, r.d.MapError(err)
}

func (r userRepo) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.tx.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY lower(name), id")
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.User
	for rows.Next() {
		u, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, r.d.MapError(rows.Err())
}

// exec1 runs a statement that must change exactly one row (else ErrNotFound).
func (s *store) exec1(ctx context.Context, q string, args ...any) error {
	res, err := s.tx.ExecContext(ctx, s.d.Rebind(q), args...)
	if err != nil {
		return s.d.MapError(err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return app.ErrNotFound
	}
	return nil
}

// prefixed qualifies each column of a comma-separated list.
func prefixed(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
