// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const sqliteTimeLayout = "2006-01-02T15:04:05.000000Z"

func sqliteDSN(path string, writer bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	if writer {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(NORMAL)")
		q.Set("_txlock", "immediate")
	} else {
		q.Add("_pragma", "query_only(1)")
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

func openSQLite(ctx context.Context, path string, readers int) (writer, reader *sqlx.DB, err error) {
	writer, err = sqlx.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		return nil, nil, err
	}
	writer.SetMaxOpenConns(1) // one writer: writes queue in Go, never "database is locked" (02 §3)
	if err := writer.PingContext(ctx); err != nil {
		_ = writer.Close()
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	reader, err = sqlx.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	reader.SetMaxOpenConns(readers)
	return writer, reader, nil
}

type sqliteDialect struct{}

func (sqliteDialect) Name() string           { return "sqlite" }
func (sqliteDialect) Rebind(q string) string { return q } // already uses ?
func (sqliteDialect) TimeArg(t time.Time) any {
	return t.UTC().Truncate(time.Microsecond).Format(sqliteTimeLayout)
}

// Writes are already serialised by the single writer connection, so locks are no-ops.
func (sqliteDialect) LockInstall(context.Context, *sqlx.Tx) error        { return nil }
func (sqliteDialect) LockUser(context.Context, *sqlx.Tx, string) error   { return nil }
func (sqliteDialect) LockChurch(context.Context, *sqlx.Tx, string) error { return nil }
func (sqliteDialect) SetTenant(context.Context, *sqlx.Tx, string) error  { return nil }

func (sqliteDialect) Retryable(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	primary := e.Code() & 0xff
	return primary == sqlite3.SQLITE_BUSY || primary == sqlite3.SQLITE_LOCKED
}

func (sqliteDialect) MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return &app.UniqueError{Constraint: sqliteConstraintName(e.Error())}
	case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return fmt.Errorf("%w: %w", app.ErrReferenced, err)
	case sqlite3.SQLITE_CONSTRAINT_CHECK:
		return fmt.Errorf("%w: %w", app.ErrInvalid, err)
	}
	// ON DELETE RESTRICT reports its failure with another extended code.
	if e.Code()&0xff == sqlite3.SQLITE_CONSTRAINT && strings.Contains(e.Error(), "FOREIGN KEY constraint failed") {
		return fmt.Errorf("%w: %w", app.ErrReferenced, err)
	}
	if e.Code()&0xff == sqlite3.SQLITE_FULL {
		return fmt.Errorf("%w: disk full", app.ErrUnavailable)
	}
	return err
}

// sqliteUniqueNames maps the columns SQLite reports for unique violations
// ("UNIQUE constraint failed: users.email") to the constraint names used in both
// dialects (02 §8). Every unique constraint, primary key and unique index needs an
// entry; TestUniqueConstraintNames fails if one is missing.
var sqliteUniqueNames = map[string]string{ //nolint:gosec // constraint names, not credentials
	"translations.id":   "translations_pkey",
	"translations.code": "translations_code_key",

	"churches.id": "churches_pkey",

	"users.id":    "users_pkey",
	"users.email": "users_email_key",
	"users.phone": "users_phone_key",

	"memberships.id": "memberships_pkey",
	"memberships.church_id, memberships.user_id": "memberships_church_user_key",
	"memberships.church_id, memberships.id":      "memberships_church_id_key",

	"import_batches.id":                           "import_batches_pkey",
	"import_batches.church_id, import_batches.id": "import_batches_church_id_key",
	"import_candidates.id":                        "import_candidates_pkey",

	"duties.id":                                       "duties_pkey",
	"duties.church_id, duties.name_key":               "duties_church_name_key",
	"duties.church_id, duties.id":                     "duties_church_id_key",
	"singing_parts.id":                                "singing_parts_pkey",
	"singing_parts.church_id, singing_parts.name_key": "singing_parts_church_name_key",
	"singing_parts.church_id, singing_parts.id":       "singing_parts_church_id_key",
	"templates.id":                                    "templates_pkey",
	"templates.church_id, templates.name_key":         "templates_church_name_key",
	"templates.church_id, templates.id":               "templates_church_id_key",
	"template_items.id":                               "template_items_pkey",
	"services.id":                                     "services_pkey",
	"services.church_id, services.name_key":           "services_church_name_key",
	"services.church_id, services.id":                 "services_church_id_key",
	"service_times.id":                                "service_times_pkey",
	"service_times.service_id, service_times.weekday, service_times.time": "service_times_slot_key",
	"church_seeds.church_id, church_seeds.seed_key":                       "church_seeds_pkey",

	"readings.id": "readings_pkey",
	"readings.church_id, readings.reference, readings.translation_id": "readings_church_ref_key",

	"roles.id":                        "roles_pkey",
	"roles.church_id, roles.name_key": "roles_church_name_key",
	"roles.church_id, roles.origin":   "roles_church_origin_key",
	"roles.church_id, roles.id":       "roles_church_id_key",

	"role_scopes.role_id, role_scopes.scope": "role_scopes_pkey",

	"membership_roles.membership_id, membership_roles.role_id": "membership_roles_pkey",

	"invites.id":                       "invites_pkey",
	"invites.token_hash":               "invites_token_hash_key",
	"invites.church_id, invites.id":    "invites_church_id_key",
	"invites.church_id, invites.email": "invites_church_email_open_key",
	"invites.church_id, invites.phone": "invites_church_phone_open_key",

	"invite_roles.invite_id, invite_roles.role_id": "invite_roles_pkey",

	"sessions.token_hash": "sessions_pkey",

	"password_resets.id":         "password_resets_pkey",
	"password_resets.token_hash": "password_resets_token_hash_key",
	"password_resets.user_id":    "password_resets_user_open_key",

	"auth_throttle.key": "auth_throttle_pkey",

	"setup_tokens.id": "setup_tokens_pkey",

	"songs.church_id, songs.song_group_id, songs.language": "songs_group_language_key",
	"song_sections.song_id, song_sections.number":          "song_sections_verse_key",

	"song_groups.id":                        "song_groups_pkey",
	"song_groups.church_id, song_groups.id": "song_groups_church_id_key",

	"songs.id":                  "songs_pkey",
	"songs.church_id, songs.id": "songs_church_id_key",

	"song_sections.id": "song_sections_pkey",
	"song_sections.church_id, song_sections.song_id, song_sections.id": "song_sections_church_song_id_key",

	"song_arrangement_entries.song_id, song_arrangement_entries.position": "song_arrangement_entries_pkey",

	"song_search.church_id, song_search.song_id": "song_search_pkey",
}

var sqliteUniqueRe = regexp.MustCompile(`UNIQUE constraint failed: (.+?)(?: \(|$)`)

func sqliteConstraintName(msg string) string {
	m := sqliteUniqueRe.FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	cols := strings.TrimSpace(m[1])
	if name, ok := sqliteUniqueNames[cols]; ok {
		return name
	}
	if strings.HasPrefix(cols, "index '") { // partial unique index: "index 'name'"
		return strings.TrimSuffix(strings.TrimPrefix(cols, "index '"), "'")
	}
	return cols
}

// --- song search (06 §5.3) ---

func (d sqliteDialect) WriteSongIndex(ctx context.Context, tx *sqlx.Tx, churchID, songID, language, head, lyrics string) error {
	if err := d.DeleteSongIndex(ctx, tx, churchID, songID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO song_search (church_id, song_id, language, head_fold, lyrics_fold)
		VALUES (?, ?, ?, ?, ?)`, churchID, songID, language, head, lyrics); err != nil {
		return d.MapError(err)
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO song_fts (head, lyrics, church_id, song_id) VALUES (?, ?, ?, ?)",
		head, lyrics, churchID, songID)
	return d.MapError(err)
}

func (d sqliteDialect) DeleteSongIndex(ctx context.Context, tx *sqlx.Tx, churchID, songID string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM song_fts WHERE church_id = ? AND song_id = ?", churchID, songID); err != nil {
		return d.MapError(err)
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM song_search WHERE church_id = ? AND song_id = ?", churchID, songID)
	return d.MapError(err)
}

func (d sqliteDialect) DeleteChurchIndex(ctx context.Context, tx *sqlx.Tx, churchID string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM song_fts WHERE church_id = ?", churchID); err != nil {
		return d.MapError(err)
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM song_search WHERE church_id = ?", churchID)
	return d.MapError(err)
}

// TermMatch asks the FTS5 index for a prefix match; terms hold only letters
// and digits (Fold), so quoting them is enough.
func (sqliteDialect) TermMatch(term string, headOnly bool) (string, []any) {
	match := `"` + term + `"*`
	if headOnly {
		match = "head : " + match
	}
	return "ss.song_id IN (SELECT song_id FROM song_fts WHERE song_fts MATCH ? AND church_id = ss.church_id)", []any{match}
}

func (sqliteDialect) Contains(col string) string { return "instr(" + col + ", ?) > 0" }

func (sqliteDialect) OrderBytes(col string) string { return col }
