// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

type sessionRepo struct{ *store }

func (s *store) Sessions() app.SessionRepo { return sessionRepo{s} }

func (r sessionRepo) Create(ctx context.Context, s domain.Session) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO sessions
		(token_hash, user_id, user_agent, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`),
		s.TokenHash, s.UserID, s.UserAgent, r.d.TimeArg(s.CreatedAt), r.d.TimeArg(s.LastSeenAt), r.d.TimeArg(s.ExpiresAt))
	return r.d.MapError(err)
}

func (r sessionRepo) ByTokenHash(ctx context.Context, hash string) (domain.Session, error) {
	var (
		s                      domain.Session
		created, seen, expires Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT token_hash, user_id, user_agent, created_at, last_seen_at, expires_at
		FROM sessions WHERE token_hash = ?`), hash).Scan(&s.TokenHash, &s.UserID, &s.UserAgent, &created, &seen, &expires)
	if err != nil {
		return domain.Session{}, r.d.MapError(err)
	}
	s.CreatedAt, s.LastSeenAt, s.ExpiresAt = created.Time, seen.Time, expires.Time
	return s, nil
}

func (r sessionRepo) Extend(ctx context.Context, hash string, now, expires time.Time) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(
		"UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ? AND expires_at > ?"),
		r.d.TimeArg(now), r.d.TimeArg(expires), hash, r.d.TimeArg(now))
	if err != nil {
		return false, r.d.MapError(err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r sessionRepo) Delete(ctx context.Context, hash string) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM sessions WHERE token_hash = ?"), hash)
	return r.d.MapError(err)
}

func (r sessionRepo) DeleteOthers(ctx context.Context, user domain.UserID, keepHash string) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?"), user, keepHash)
	return r.d.MapError(err)
}

func (r sessionRepo) DeleteAllForUser(ctx context.Context, user domain.UserID) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM sessions WHERE user_id = ?"), user)
	return r.d.MapError(err)
}

func (r sessionRepo) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM sessions WHERE expires_at < ?"), r.d.TimeArg(now))
	return r.d.MapError(err)
}
