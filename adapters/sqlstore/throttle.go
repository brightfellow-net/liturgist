// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/jmoiron/sqlx"
)

type throttleRepo struct{ *store }

func (s *store) AuthThrottle() app.ThrottleRepo { return throttleRepo{s} }

func (r throttleRepo) LockedUntil(ctx context.Context, keys []string, now time.Time) (time.Time, error) {
	q, args, err := sqlx.In("SELECT max(locked_until) FROM auth_throttle WHERE key IN (?) AND locked_until > ?", keys, r.d.TimeArg(now))
	if err != nil {
		return time.Time{}, err
	}
	var until NullTime
	if err := r.tx.QueryRowContext(ctx, r.d.Rebind(q), args...).Scan(&until); err != nil {
		return time.Time{}, r.d.MapError(err)
	}
	return until.Time, nil
}

// recordFailureSQL is one atomic statement on both dialects (02 §2.1, 03 §5):
// a new row starts at 1; an existing row restarts at 1 when its window has
// ended, otherwise adds 1; the lock is set when the count reaches the limit.
// A restart clears the lock: failures are only recorded when no counter is
// locked, so any old lock has already ended (and keeping it would break
// auth_throttle_lock_check). Every limit is at least 2, so 1 never locks.
const recordFailureSQL = `INSERT INTO auth_throttle (key, kind, failures, window_started_at, locked_until)
VALUES (?, ?, 1, ?, NULL)
ON CONFLICT (key) DO UPDATE SET
    failures = CASE WHEN auth_throttle.window_started_at <= ? THEN 1
                    ELSE auth_throttle.failures + 1 END,
    window_started_at = CASE WHEN auth_throttle.window_started_at <= ? THEN excluded.window_started_at
                             ELSE auth_throttle.window_started_at END,
    locked_until = CASE WHEN auth_throttle.window_started_at <= ? THEN NULL
                        WHEN auth_throttle.failures + 1 >= ? THEN ?
                        ELSE auth_throttle.locked_until END`

func (r throttleRepo) RecordFailure(ctx context.Context, key string, rule domain.ThrottleRule, now time.Time) error {
	cutoff := r.d.TimeArg(now.Add(-rule.Window))
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(recordFailureSQL),
		key, string(rule.Kind), r.d.TimeArg(now), cutoff, cutoff, cutoff, rule.Limit, r.d.TimeArg(now.Add(rule.Lock)))
	return r.d.MapError(err)
}

func (r throttleRepo) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	q := "DELETE FROM auth_throttle WHERE key IN (?" + strings.Repeat(", ?", len(keys)-1) + ")"
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(q), args...)
	return r.d.MapError(err)
}

func (r throttleRepo) DeleteAll(ctx context.Context) error {
	_, err := r.tx.ExecContext(ctx, "DELETE FROM auth_throttle")
	return r.d.MapError(err)
}

// Keys contain only hex digits, IP text and ':' '.' '/', never LIKE wildcards.
func (r throttleRepo) DeleteForIdentifier(ctx context.Context, identifier string) error {
	h := domain.ThrottleIdentifierHash(identifier)
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM auth_throttle WHERE key = ? OR key LIKE ?"),
		"id:"+h, "idip:"+h+":%")
	return r.d.MapError(err)
}

func (r throttleRepo) DeleteForAddr(ctx context.Context, addrKey string) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM auth_throttle WHERE key = ? OR key LIKE ?"),
		"ip:"+addrKey, "idip:%:"+addrKey)
	return r.d.MapError(err)
}

func (r throttleRepo) DeleteEnded(ctx context.Context, kind domain.ThrottleKind, windowStart, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`DELETE FROM auth_throttle WHERE kind = ? AND window_started_at < ?
		AND (locked_until IS NULL OR locked_until <= ?)`), string(kind), r.d.TimeArg(windowStart), r.d.TimeArg(now))
	return r.d.MapError(err)
}
