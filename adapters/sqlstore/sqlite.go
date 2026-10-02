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
