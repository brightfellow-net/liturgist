// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// Advisory lock keys (02 §2.1, §5).
const (
	lockKeyInstall   = 7420116
	lockKeyMigration = 7420115
)

func openPostgres(ctx context.Context, dsn string, maxConns int) (*sqlx.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("LITURGIST_DB_URL: %w", err)
	}
	cfg.RuntimeParams["statement_timeout"] = "5000" // ms (02 §2)
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	db := sqlx.NewDb(stdlib.OpenDB(*cfg), "pgx")
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(max(1, maxConns/2))
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return db, nil
}

type postgresDialect struct{}

func (postgresDialect) Name() string            { return "postgres" }
func (postgresDialect) Rebind(q string) string  { return sqlx.Rebind(sqlx.DOLLAR, q) }
func (postgresDialect) TimeArg(t time.Time) any { return t.UTC().Truncate(time.Microsecond) }

func (d postgresDialect) LockInstall(ctx context.Context, tx *sqlx.Tx) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKeyInstall)
	return d.MapError(err)
}

func (d postgresDialect) LockUser(ctx context.Context, tx *sqlx.Tx, id string) error {
	return d.lockRow(ctx, tx, "SELECT id FROM users WHERE id = $1 FOR UPDATE", id)
}

func (d postgresDialect) LockChurch(ctx context.Context, tx *sqlx.Tx, id string) error {
	return d.lockRow(ctx, tx, "SELECT id FROM churches WHERE id = $1 FOR UPDATE", id)
}

func (d postgresDialect) lockRow(ctx context.Context, tx *sqlx.Tx, q, id string) error {
	var got string
	return d.MapError(tx.QueryRowContext(ctx, q, id).Scan(&got))
}

func (d postgresDialect) SetTenant(ctx context.Context, tx *sqlx.Tx, churchID string) error {
	_, err := tx.ExecContext(ctx, "SELECT set_config('liturgist.church_id', $1, true)", churchID)
	return d.MapError(err)
}

func (postgresDialect) Retryable(err error) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && (e.Code == "40001" || e.Code == "40P01")
}

func (postgresDialect) MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	var e *pgconn.PgError
	if !errors.As(err, &e) {
		return err
	}
	switch e.Code {
	case "23505":
		return &app.UniqueError{Constraint: e.ConstraintName}
	case "23503":
		return fmt.Errorf("%w: %w", app.ErrReferenced, err)
	case "23514":
		return fmt.Errorf("%w: %w", app.ErrInvalid, err)
	case "53100": // disk_full
		return fmt.Errorf("%w: %w", app.ErrStorageFull, err)
	case "57014": // statement timeout
		return fmt.Errorf("%w: %w", app.ErrUnavailable, err)
	}
	return err
}

// --- song search (06 §5.3) ---

func (d postgresDialect) WriteSongIndex(ctx context.Context, tx *sqlx.Tx, churchID, songID, language, head, lyrics string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO song_search (church_id, song_id, language, head_fold, lyrics_fold, fts_head, fts_lyrics)
		VALUES ($1, $2, $3, $4, $5, to_tsvector('simple', $4), to_tsvector('simple', $5))
		ON CONFLICT (church_id, song_id) DO UPDATE SET language = EXCLUDED.language, head_fold = EXCLUDED.head_fold,
		lyrics_fold = EXCLUDED.lyrics_fold, fts_head = EXCLUDED.fts_head, fts_lyrics = EXCLUDED.fts_lyrics`,
		churchID, songID, language, head, lyrics)
	return d.MapError(err)
}

func (d postgresDialect) DeleteSongIndex(ctx context.Context, tx *sqlx.Tx, churchID, songID string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM song_search WHERE church_id = $1 AND song_id = $2", churchID, songID)
	return d.MapError(err)
}

func (d postgresDialect) DeleteChurchIndex(ctx context.Context, tx *sqlx.Tx, churchID string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM song_search WHERE church_id = $1", churchID)
	return d.MapError(err)
}

// TermMatch builds a prefix query; the term holds only letters and digits
// (Fold) and is bound as a parameter, never concatenated into the SQL.
func (postgresDialect) TermMatch(term string, headOnly bool) (string, []any) {
	arg := term + ":*"
	if headOnly {
		return "ss.fts_head @@ to_tsquery('simple', ?)", []any{arg}
	}
	return "(ss.fts_head @@ to_tsquery('simple', ?) OR ss.fts_lyrics @@ to_tsquery('simple', ?))", []any{arg, arg}
}

func (postgresDialect) Contains(col string) string { return "strpos(" + col + ", ?) > 0" }

func (postgresDialect) OrderBytes(col string) string { return col + ` COLLATE "C"` }
