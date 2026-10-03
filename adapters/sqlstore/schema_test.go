// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/brightfellow-net/liturgist/migrations"
)

// fixture inserts rows with raw SQL so database constraints can be tested on their own.
type fixture struct {
	t   *testing.T
	db  *sqlstore.DB
	now time.Time
}

func id(s string) string { return (s + strings.Repeat("0", 26))[:26] }

func hash(c string) string { return strings.Repeat(c, 64) }

const tb = "01M3XY2HBEKN8PETK6KCN370RJ" // seeded translation TB

func (f fixture) ts(d time.Duration) any { return f.db.Dialect().TimeArg(f.now.Add(d)) }

func (f fixture) exec(q string, args ...any) error { return f.db.ExecForTest(ctx, q, args...) }

func (f fixture) must(q string, args ...any) {
	f.t.Helper()
	if err := f.exec(q, args...); err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
}

func (f fixture) church(cid string) {
	f.must(`INSERT INTO churches (id, name, default_ui_language, default_language, default_translation_id,
		time_zone, settings, created_at, updated_at) VALUES (?, 'Church', 'en', 'id', ?, 'Asia/Jakarta', '{}', ?, ?)`,
		cid, tb, f.ts(0), f.ts(0))
}

func (f fixture) user(uid, email, phone string) error {
	return f.exec(`INSERT INTO users (id, name, email, phone, password_hash, preferences, created_at, updated_at)
		VALUES (?, 'User', ?, ?, 'x', '{}', ?, ?)`, uid, nullable(email), nullable(phone), f.ts(0), f.ts(0))
}

func (f fixture) role(rid, cid, nameKey string, origin any) error {
	return f.exec(`INSERT INTO roles (id, church_id, name, name_key, description, origin, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', ?, ?, ?)`, rid, cid, nameKey, nameKey, origin, f.ts(0), f.ts(0))
}

func (f fixture) invite(iid, cid, token, email, phone string) error {
	return f.exec(`INSERT INTO invites (id, church_id, token_hash, name, email, phone, created_at, expires_at)
		VALUES (?, ?, ?, 'Invitee', ?, ?, ?, ?)`, iid, cid, token, nullable(email), nullable(phone), f.ts(0), f.ts(time.Hour))
}

func (f fixture) reset(pid, uid, token string) error {
	return f.exec(`INSERT INTO password_resets (id, user_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`, pid, uid, token, f.ts(0), f.ts(time.Hour))
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// base creates churches A and B and one row in every step-1 table.
func base(t *testing.T, db *sqlstore.DB) fixture {
	f := fixture{t: t, db: db, now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	f.church(id("CHA"))
	f.church(id("CHB"))
	if err := f.user(id("U1"), "u1@example.org", "+6281234567890"); err != nil {
		t.Fatal(err)
	}
	f.must(`INSERT INTO memberships (id, church_id, user_id, created_at) VALUES (?, ?, ?, ?)`, id("M1"), id("CHA"), id("U1"), f.ts(0))
	for _, err := range []error{
		f.role(id("RA"), id("CHA"), "admin", "church_admin"),
		f.role(id("RB"), id("CHB"), "admin", "church_admin"),
		f.invite(id("I1"), id("CHA"), hash("1"), "inv@example.org", "+6281111111111"),
		f.reset(id("P1"), id("U1"), hash("2")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.must(`INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, 'members.view')`, id("CHA"), id("RA"))
	f.must(`INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("M1"), id("RA"))
	f.must(`INSERT INTO invite_roles (church_id, invite_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("I1"), id("RA"))
	f.must(`INSERT INTO sessions (token_hash, user_id, user_agent, created_at, last_seen_at, expires_at)
		VALUES (?, ?, '', ?, ?, ?)`, hash("3"), id("U1"), f.ts(0), f.ts(0), f.ts(time.Hour))
	f.must(`INSERT INTO auth_throttle (key, kind, failures, window_started_at) VALUES ('ip:203.0.113.5', 'ip', 1, ?)`, f.ts(0))
	f.must(`INSERT INTO setup_tokens (id, token_hash, created_at, expires_at) VALUES (1, ?, ?, ?)`, hash("4"), f.ts(0), f.ts(time.Hour))
	return f
}

// TC-P-007
func TestMigrationFoldersMatch(t *testing.T) {
	names := func(dialect string) []string {
		m, err := fs.Glob(migrations.For(dialect), "*.sql")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	sqlite, pg := names("sqlite"), names("postgres")
	if len(sqlite) == 0 || !slices.Equal(sqlite, pg) {
		t.Errorf("migration folders differ:\nsqlite:   %v\npostgres: %v", sqlite, pg)
	}
}

// IT-P-006
func TestSeededTranslations(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		var codes []string
		err := db.Read(ctx, func(s app.Store) error {
			return sqlstore.RawTx(s).SelectContext(ctx, &codes, "SELECT code FROM translations ORDER BY id")
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"TB", "TB2", "BIS", "CUV", "KJV", "WEB"}; !slices.Equal(codes, want) {
			t.Errorf("got %v, want %v", codes, want)
		}
	})
}

// TC-P-010: every unique constraint, primary key and unique index reports the
// same name on both dialects. The (church_id, id) keys that only exist as
// foreign-key targets can't be violated without violating the primary key first.
func TestUniqueConstraintNames(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		f.churchRepo(func(r app.SongRepo) error {
			sa, sc := mkSong("SA", "A", "id", f.now, "x"), mkSong("SC", "C", "id", f.now, "z")
			sa.DefaultArrangement = []domain.SectionID{sa.Sections[0].ID}
			for _, song := range []domain.Song{sa, sc} {
				if err := r.Create(ctx, song); err != nil {
					return err
				}
			}
			if err := r.CreateGroup(ctx, domain.SongGroupID(id("G1")), f.now); err != nil {
				return err
			}
			return r.SetGroup(ctx, []domain.SongID{sa.ID}, domain.SongGroupID(id("G1")), f.now)
		})
		for _, u := range []string{"U8", "U9"} { // users without an open reset link
			if err := f.user(id(u), strings.ToLower(u)+"@example.org", ""); err != nil {
				t.Fatal(err)
			}
		}
		cases := map[string]func() error{
			"translations_pkey": func() error {
				return f.exec(`INSERT INTO translations (id, code, name, language) VALUES (?, 'NEW', 'n', 'en')`, tb)
			},
			"translations_code_key": func() error {
				return f.exec(`INSERT INTO translations (id, code, name, language) VALUES (?, 'TB', 'n', 'en')`, id("TR"))
			},
			"churches_pkey": func() error {
				return f.exec(`INSERT INTO churches (id, name, default_ui_language, default_language, default_translation_id,
					time_zone, settings, created_at, updated_at) VALUES (?, 'c', 'en', 'id', ?, 'Asia/Jakarta', '{}', ?, ?)`,
					id("CHA"), tb, f.ts(0), f.ts(0))
			},
			"users_pkey":      func() error { return f.user(id("U1"), "other@example.org", "") },
			"users_email_key": func() error { return f.user(id("U2"), "u1@example.org", "") },
			"users_phone_key": func() error { return f.user(id("U3"), "", "+6281234567890") },
			"memberships_pkey": func() error {
				return f.exec(`INSERT INTO memberships (id, church_id, user_id, created_at) VALUES (?, ?, ?, ?)`, id("M1"), id("CHB"), id("U1"), f.ts(0))
			},
			"memberships_church_user_key": func() error {
				return f.exec(`INSERT INTO memberships (id, church_id, user_id, created_at) VALUES (?, ?, ?, ?)`, id("M2"), id("CHA"), id("U1"), f.ts(0))
			},
			// Each case violates exactly one constraint: with several, the dialects may report different ones.
			"roles_pkey":              func() error { return f.role(id("RA"), id("CHB"), "x", nil) },
			"roles_church_name_key":   func() error { return f.role(id("R2"), id("CHA"), "admin", nil) },
			"roles_church_origin_key": func() error { return f.role(id("R3"), id("CHA"), "other", "church_admin") },
			"role_scopes_pkey": func() error {
				return f.exec(`INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, 'members.view')`, id("CHA"), id("RA"))
			},
			"membership_roles_pkey": func() error {
				return f.exec(`INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("M1"), id("RA"))
			},
			"invites_pkey":                  func() error { return f.invite(id("I1"), id("CHB"), hash("5"), "x@example.org", "") },
			"invites_token_hash_key":        func() error { return f.invite(id("I2"), id("CHA"), hash("1"), "y@example.org", "") },
			"invites_church_email_open_key": func() error { return f.invite(id("I3"), id("CHA"), hash("6"), "inv@example.org", "") },
			"invites_church_phone_open_key": func() error { return f.invite(id("I4"), id("CHA"), hash("7"), "", "+6281111111111") },
			"invite_roles_pkey": func() error {
				return f.exec(`INSERT INTO invite_roles (church_id, invite_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("I1"), id("RA"))
			},
			"sessions_pkey": func() error {
				return f.exec(`INSERT INTO sessions (token_hash, user_id, user_agent, created_at, last_seen_at, expires_at)
					VALUES (?, ?, '', ?, ?, ?)`, hash("3"), id("U1"), f.ts(0), f.ts(0), f.ts(time.Hour))
			},
			"password_resets_pkey":           func() error { return f.reset(id("P1"), id("U8"), hash("8")) },
			"password_resets_token_hash_key": func() error { return f.reset(id("P2"), id("U9"), hash("2")) },
			"password_resets_user_open_key":  func() error { return f.reset(id("P3"), id("U1"), hash("9")) },
			"auth_throttle_pkey": func() error {
				return f.exec(`INSERT INTO auth_throttle (key, kind, failures, window_started_at) VALUES ('ip:203.0.113.5', 'ip', 1, ?)`, f.ts(0))
			},
			"setup_tokens_pkey": func() error {
				return f.exec(`INSERT INTO setup_tokens (id, token_hash, created_at, expires_at) VALUES (1, ?, ?, ?)`, hash("a"), f.ts(0), f.ts(time.Hour))
			},
			"song_groups_pkey": func() error {
				return f.exec(`INSERT INTO song_groups (id, church_id, created_at) VALUES (?, ?, ?)`, id("G1"), id("CHB"), f.ts(0))
			},
			"songs_pkey": func() error { return f.rawSong("SA", "CHB", "en") },
			"songs_group_language_key": func() error {
				return f.exec(`UPDATE songs SET song_group_id = ? WHERE id = ?`, id("G1"), id("SC")) // SA is already the group's "id" song
			},
			"song_sections_pkey": func() error {
				return f.exec(`INSERT INTO song_sections (id, church_id, song_id, position, kind, number, label, text)
					VALUES (?, ?, ?, 7, 'other', NULL, NULL, 'x')`, id("SAV1"), id("CHA"), id("SC")) // another song: only the primary key collides
			},
			"song_sections_verse_key": func() error {
				return f.exec(`INSERT INTO song_sections (id, church_id, song_id, position, kind, number, label, text)
					VALUES (?, ?, ?, 7, 'verse', 1, NULL, 'x')`, id("SAV9"), id("CHA"), id("SA"))
			},
			"song_arrangement_entries_pkey": func() error {
				return f.exec(`INSERT INTO song_arrangement_entries (church_id, song_id, position, section_id) VALUES (?, ?, 0, ?)`,
					id("CHA"), id("SA"), id("SAV1"))
			},
			"readings_pkey": func() error {
				f.must(readingInsert, id("RD1"), id("CHA"), "PSA 1", "PSA 1", tb, "x", "", "manual", "x", 1, f.ts(0), f.ts(0))
				return f.exec(readingInsert, id("RD1"), id("CHB"), "PSA 1", "PSA 1", tb, "x", "", "manual", "x", 1, f.ts(0), f.ts(0))
			},
			"readings_church_ref_key": func() error {
				f.must(readingInsert, id("RD2"), id("CHA"), "PSA 2", "PSA 2", tb, "x", "", "manual", "x", 1, f.ts(0), f.ts(0))
				return f.exec(readingInsert, id("RD3"), id("CHA"), "PSA 2", "Mzm 2", tb, "y", "", "manual", "y", 1, f.ts(0), f.ts(0))
			},
			"import_batches_pkey": func() error {
				f.must(batchInsert, id("BT1"), id("CHA"), "paste", "open", id("U1"), f.ts(0), f.ts(0))
				return f.exec(batchInsert, id("BT1"), id("CHB"), "paste", "open", id("U1"), f.ts(0), f.ts(0))
			},
			"import_candidates_pkey": func() error {
				f.must(batchInsert, id("BT2"), id("CHA"), "paste", "open", id("U1"), f.ts(0), f.ts(0))
				f.must(candInsert, id("CN1"), id("CHA"), id("BT2"), 0, "pending", nil, nil, false, nil, nil, nil)
				return f.exec(candInsert, id("CN1"), id("CHA"), id("BT2"), 1, "pending", nil, nil, false, nil, nil, nil)
			},
			"song_search_pkey": func() error {
				if db.Dialect().Name() == "postgres" { // its tsvector columns are NOT NULL
					return f.exec(`INSERT INTO song_search (church_id, song_id, language, head_fold, lyrics_fold, fts_head, fts_lyrics)
						VALUES (?, ?, 'id', '', '', to_tsvector('simple', ''), to_tsvector('simple', ''))`, id("CHA"), id("SA"))
				}
				return f.exec(`INSERT INTO song_search (church_id, song_id, language, head_fold, lyrics_fold) VALUES (?, ?, 'id', '', '')`,
					id("CHA"), id("SA"))
			},
		}
		for want, violate := range cases {
			var u *app.UniqueError
			if err := violate(); !errors.As(err, &u) || u.Constraint != want {
				t.Errorf("%s: got %v", want, err)
			}
		}
	})
}

// IT-P-009: the database itself rejects invalid rows.
func TestDatabaseRejectsInvalidRows(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		invalid := map[string]func() error{
			"second setup-token row": func() error {
				return f.exec(`INSERT INTO setup_tokens (id, token_hash, created_at, expires_at) VALUES (2, ?, ?, ?)`, hash("a"), f.ts(0), f.ts(time.Hour))
			},
			"accepted and cancelled invite": func() error {
				return f.exec(`UPDATE invites SET accepted_at = ?, cancelled_at = ? WHERE id = ?`, f.ts(time.Minute), f.ts(time.Minute), id("I1"))
			},
			"accepted user without accepted_at": func() error {
				return f.exec(`UPDATE invites SET accepted_user_id = ? WHERE id = ?`, id("U1"), id("I1"))
			},
			"malformed token hash":         func() error { return f.reset(id("P4"), id("U1"), "XYZ") },
			"upper-case token hash":        func() error { return f.invite(id("I5"), id("CHA"), hash("A"), "z@example.org", "") },
			"phone without +":              func() error { return f.user(id("U4"), "", "081234567890") },
			"phone with letters":           func() error { return f.user(id("U5"), "", "+62abc4567890") },
			"phone too short":              func() error { return f.user(id("U6"), "", "+62812") },
			"user without email and phone": func() error { return f.user(id("U7"), "", "") },
			"expiry not after creation": func() error {
				return f.exec(`INSERT INTO sessions (token_hash, user_id, user_agent, created_at, last_seen_at, expires_at)
					VALUES (?, ?, '', ?, ?, ?)`, hash("b"), id("U1"), f.ts(0), f.ts(0), f.ts(0))
			},
			"settings not an object": func() error {
				return f.exec(`UPDATE churches SET settings = '[]' WHERE id = ?`, id("CHA"))
			},
			"unknown role origin": func() error { return f.role(id("R9"), id("CHA"), "nine", "pastor") },
			"negative failures": func() error {
				return f.exec(`UPDATE auth_throttle SET failures = -1`)
			},
			"unknown throttle kind": func() error {
				return f.exec(`INSERT INTO auth_throttle (key, kind, failures, window_started_at) VALUES ('x', 'user', 1, ?)`, f.ts(0))
			},
		}
		for name, write := range invalid {
			if err := write(); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", name, err)
			}
		}

		if err := f.exec(`INSERT INTO churches (id, name, default_ui_language, default_language, default_translation_id,
			time_zone, settings, created_at, updated_at) VALUES (?, 'c', 'en', 'id', NULL, 'Asia/Jakarta', '{}', ?, ?)`,
			id("CHC"), f.ts(0), f.ts(0)); err == nil {
			t.Error("church without a translation must be rejected")
		}
		if err := f.exec(`INSERT INTO churches (id, name, default_ui_language, default_language, default_translation_id,
			time_zone, settings, created_at, updated_at) VALUES (?, 'c', 'en', 'id', ?, 'Asia/Jakarta', '{}', ?, ?)`,
			id("CHD"), id("NOPE"), f.ts(0), f.ts(0)); !errors.Is(err, app.ErrReferenced) {
			t.Errorf("unknown translation: want ErrReferenced, got %v", err)
		}
		// A used reset link no longer blocks a new one.
		f.must(`UPDATE password_resets SET used_at = ? WHERE id = ?`, f.ts(time.Minute), id("P1"))
		if err := f.reset(id("P5"), id("U1"), hash("c")); err != nil {
			t.Errorf("new link after using the old one: %v", err)
		}
		// A cancelled invite no longer blocks a new one for the same email.
		f.must(`UPDATE invites SET cancelled_at = ? WHERE id = ?`, f.ts(time.Minute), id("I1"))
		if err := f.invite(id("I6"), id("CHA"), hash("d"), "inv@example.org", ""); err != nil {
			t.Errorf("new invite after cancelling: %v", err)
		}
	})
}

// IT-P-002: rows can't point at another church's rows.
func TestCompositeForeignKeys(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		cross := map[string]func() error{
			"membership role with church B's role": func() error {
				return f.exec(`INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("M1"), id("RB"))
			},
			"membership role claiming church B for A's membership": func() error {
				return f.exec(`INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)`, id("CHB"), id("M1"), id("RB"))
			},
			"invite role with church B's role": func() error {
				return f.exec(`INSERT INTO invite_roles (church_id, invite_id, role_id) VALUES (?, ?, ?)`, id("CHA"), id("I1"), id("RB"))
			},
			"role scope for church B's role under church A": func() error {
				return f.exec(`INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, 'x')`, id("CHA"), id("RB"))
			},
		}
		for name, write := range cross {
			if err := write(); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("%s: want ErrReferenced, got %v", name, err)
			}
		}
		// Deleting a role removes it from memberships and open invites.
		f.must(`DELETE FROM roles WHERE id = ?`, id("RA"))
		var n int
		mustRead(t, db, func(s app.Store) error {
			return sqlstore.RawTx(s).QueryRowContext(ctx,
				`SELECT (SELECT count(*) FROM membership_roles) + (SELECT count(*) FROM invite_roles) + (SELECT count(*) FROM role_scopes)`).Scan(&n)
		})
		if n != 0 {
			t.Errorf("cascade left %d rows", n)
		}
	})
}

// IT-P-003: each lock serialises "count, then insert if zero".
func TestLocksSerialise(t *testing.T) {
	type lockFn func(s app.Store) error
	locks := map[string]lockFn{
		"LockChurch": func(s app.Store) error {
			cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			return cs.LockChurch(ctx)
		},
		"LockUser":    func(s app.Store) error { return s.LockUser(ctx, domain.UserID(id("U1"))) },
		"LockInstall": func(s app.Store) error { return s.LockInstall(ctx) },
		"no lock":     func(app.Store) error { return nil },
	}
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		base(t, db)
		for name, lock := range locks {
			if err := db.ExecForTest(ctx, "DROP TABLE IF EXISTS t_lock"); err != nil {
				t.Fatal(err)
			}
			if err := db.ExecForTest(ctx, "CREATE TABLE t_lock (n INTEGER)"); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					err := db.Write(ctx, func(s app.Store) error {
						if err := lock(s); err != nil {
							return err
						}
						tx := sqlstore.RawTx(s)
						var count int
						if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM t_lock").Scan(&count); err != nil {
							return err
						}
						time.Sleep(50 * time.Millisecond)
						if count == 0 {
							_, err := tx.ExecContext(ctx, "INSERT INTO t_lock (n) VALUES (1)")
							return err
						}
						return nil
					})
					if err != nil {
						t.Errorf("%s: %v", name, err)
					}
				}()
			}
			close(start)
			wg.Wait()
			var rows int
			mustRead(t, db, func(s app.Store) error {
				return sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT count(*) FROM t_lock").Scan(&rows)
			})
			want := 1
			if name == "no lock" && db.Dialect().Name() == "postgres" {
				want = 2 // control: without a lock PostgreSQL lets both transactions pass the check
			}
			if rows != want {
				t.Errorf("%s: %d rows, want %d", name, rows, want)
			}
		}
	})
}

const readingInsert = `INSERT INTO readings (id, church_id, reference, reference_display, translation_id, text, attribution,
	source_provider, search_fold, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// IT-R-007: the database itself rejects invalid reading rows.
func TestReadingConstraints(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		insert := func(rid, church, ref, translation string, version int) error {
			return f.exec(readingInsert, id(rid), id(church), ref, ref, translation, "x", "", "manual", "x", version, f.ts(0), f.ts(0))
		}
		f.must(readingInsert, id("RD1"), id("CHA"), "JHN 3:16", "JHN 3:16", tb, "x", "", "manual", "x", 1, f.ts(0), f.ts(0))

		for name, write := range map[string]func() error{
			"version 0": func() error { return insert("RD2", "CHA", "PSA 1", string(tb), 0) },
			"short ID": func() error {
				return f.exec(readingInsert, "short", id("CHA"), "PSA 1", "PSA 1", tb, "x", "", "manual", "x", 1, f.ts(0), f.ts(0))
			},
		} {
			if err := write(); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", name, err)
			}
		}
		for name, write := range map[string]func() error{
			"unknown translation": func() error { return insert("RD3", "CHA", "PSA 1", "01M3XY2HBEKN8PETK6KXXXXXXX", 1) },
			"unknown church":      func() error { return insert("RD4", "NOPE", "PSA 1", string(tb), 1) },
		} {
			if err := write(); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("%s: want ErrReferenced, got %v", name, err)
			}
		}
		// The same passage in another church, and in another translation, is fine.
		if err := insert("RD5", "CHB", "JHN 3:16", string(tb), 1); err != nil {
			t.Errorf("another church: %v", err)
		}
		if err := insert("RD6", "CHA", "JHN 3:16", "01M3XY2HBEKN8PETK6KHCT82HX", 1); err != nil {
			t.Errorf("another translation: %v", err)
		}
		// Deleting a church removes its readings.
		f.must(`DELETE FROM churches WHERE id = ?`, id("CHB"))
		if n := count(t, db, "SELECT count(*) FROM readings WHERE church_id = ?", id("CHB")); n != 0 {
			t.Errorf("%d readings left after deleting their church", n)
		}
	})
}

const (
	batchInsert = `INSERT INTO import_batches (id, church_id, source_format, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	candInsert = `INSERT INTO import_candidates (id, church_id, batch_id, position, kind, draft, decision, merge_into,
		merge_target_version, remove_unmatched, warnings, outcome, applied_song_id, error_code)
		VALUES (?, ?, ?, ?, 'song', '{}', ?, ?, ?, ?, '[]', ?, ?, ?)`
)

// IT-I constraints: the import tables reject rows the application never writes (08 §2.1, schema.md).
func TestImportConstraints(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		f.must(batchInsert, id("BT1"), id("CHA"), "paste", "open", id("U1"), f.ts(0), f.ts(0))
		n := 0
		cand := func(decision string, mergeInto any, version any, outcome, song, code any) error {
			n++
			return f.exec(candInsert, id("CN"+strconv.Itoa(n)), id("CHA"), id("BT1"), n, decision, mergeInto, version, false, outcome, song, code)
		}
		for name, write := range map[string]func() error{
			"unknown format": func() error {
				return f.exec(batchInsert, id("BT2"), id("CHA"), "docx", "open", id("U1"), f.ts(0), f.ts(0))
			},
			"unknown status": func() error {
				return f.exec(batchInsert, id("BT2"), id("CHA"), "paste", "done", id("U1"), f.ts(0), f.ts(0))
			},
			"short ID": func() error {
				return f.exec(batchInsert, "short", id("CHA"), "paste", "open", id("U1"), f.ts(0), f.ts(0))
			},
			"unknown decision":               func() error { return cand("maybe", nil, nil, nil, nil, nil) },
			"merge without target":           func() error { return cand("merge", nil, 1, nil, nil, nil) },
			"merge without version":          func() error { return cand("merge", id("S1"), nil, nil, nil, nil) },
			"target without merge":           func() error { return cand("accept", id("S1"), 1, nil, nil, nil) },
			"unknown outcome":                func() error { return cand("accept", nil, nil, "done", nil, nil) },
			"outcome of a pending candidate": func() error { return cand("pending", nil, nil, "failed", nil, "x") },
			"outcome of a skipped candidate": func() error { return cand("skip", nil, nil, "applied", id("S1"), nil) },
			"applied without a song":         func() error { return cand("accept", nil, nil, "applied", nil, nil) },
			"song without applied":           func() error { return cand("accept", nil, nil, nil, id("S1"), nil) },
			"failed without a code":          func() error { return cand("accept", nil, nil, "failed", nil, nil) },
			"code without failed":            func() error { return cand("accept", nil, nil, nil, nil, "x") },
			"negative position": func() error {
				return f.exec(candInsert, id("CNZ"), id("CHA"), id("BT1"), -1, "pending", nil, nil, false, nil, nil, nil)
			},
		} {
			if err := write(); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", name, err)
			}
		}
		for name, write := range map[string]func() error{
			"unknown church": func() error {
				return f.exec(batchInsert, id("BT3"), "NOPE", "paste", "open", id("U1"), f.ts(0), f.ts(0))
			},
			"unknown user": func() error {
				return f.exec(batchInsert, id("BT3"), id("CHA"), "paste", "open", "NOPE", f.ts(0), f.ts(0))
			},
			"batch of another church": func() error {
				return f.exec(candInsert, id("CNQ"), id("CHB"), id("BT1"), 0, "pending", nil, nil, false, nil, nil, nil)
			},
		} {
			if err := write(); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("%s: want ErrReferenced, got %v", name, err)
			}
		}
		// Rows the application writes are accepted.
		for name, write := range map[string]func() error{
			"pending":       func() error { return cand("pending", nil, nil, nil, nil, nil) },
			"merge":         func() error { return cand("merge", id("S1"), 2, nil, nil, nil) },
			"failed accept": func() error { return cand("accept", nil, nil, "failed", nil, "validation_failed") },
			"applied merge": func() error { return cand("merge", id("S1"), 2, "applied", id("S1"), nil) },
			"skip":          func() error { return cand("skip", nil, nil, nil, nil, nil) },
		} {
			if err := write(); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		// Deleting a church or a batch removes the candidates.
		f.must(`DELETE FROM import_batches WHERE id = ?`, id("BT1"))
		if c := count(t, db, "SELECT count(*) FROM import_candidates WHERE church_id = ?", id("CHA")); c != 0 {
			t.Errorf("%d candidates left after deleting their batch", c)
		}
		f.must(batchInsert, id("BT4"), id("CHB"), "chordpro", "closed", id("U1"), f.ts(0), f.ts(0))
		f.must(`DELETE FROM churches WHERE id = ?`, id("CHB"))
		if c := count(t, db, "SELECT count(*) FROM import_batches WHERE church_id = ?", id("CHB")); c != 0 {
			t.Errorf("%d batches left after deleting their church", c)
		}
	})
}
