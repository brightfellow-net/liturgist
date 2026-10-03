// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// churchRows dumps every church-owned row of church cid, for before/after comparison.
func churchRows(t *testing.T, db *sqlstore.DB, cid string) string {
	t.Helper()
	var out string
	mustRead(t, db, func(s app.Store) error { out = churchRowsIn(t, s, db, cid); return nil })
	return out
}

// churchRowsIn dumps the rows inside an open transaction.
func churchRowsIn(t *testing.T, s app.Store, db *sqlstore.DB, cid string) string {
	t.Helper()
	var b strings.Builder
	err := func() error {
		for _, q := range []string{
			"SELECT * FROM churches WHERE id = ?",
			"SELECT * FROM memberships WHERE church_id = ? ORDER BY id",
			"SELECT * FROM roles WHERE church_id = ? ORDER BY id",
			"SELECT * FROM role_scopes WHERE church_id = ? ORDER BY role_id, scope",
			"SELECT * FROM membership_roles WHERE church_id = ? ORDER BY membership_id, role_id",
			"SELECT * FROM invites WHERE church_id = ? ORDER BY id",
			"SELECT * FROM invite_roles WHERE church_id = ? ORDER BY invite_id, role_id",
			"SELECT * FROM song_groups WHERE church_id = ? ORDER BY id",
			"SELECT * FROM songs WHERE church_id = ? ORDER BY id",
			"SELECT * FROM song_sections WHERE church_id = ? ORDER BY song_id, position",
			"SELECT * FROM song_arrangement_entries WHERE church_id = ? ORDER BY song_id, position",
			"SELECT * FROM song_search WHERE church_id = ? ORDER BY song_id",
		} {
			rows, err := sqlstore.RawTx(s).QueryxContext(ctx, db.Dialect().Rebind(q), cid)
			if err != nil {
				return err
			}
			for rows.Next() {
				m := map[string]any{}
				if err := rows.MapScan(m); err != nil {
					_ = rows.Close()
					return err
				}
				fmt.Fprintf(&b, "%v\n", m)
			}
			_ = rows.Close()
		}
		return nil
	}()
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// IT-P-007: every ChurchStore repository method, called through ForChurch(A),
// reads and changes only A's rows, even when given B's IDs.
func TestScopedRepositories(t *testing.T) {
	type call func(cs app.ChurchStore, now time.Time) error
	errRollback := errors.New("rollback")
	notFound := func(err error) error {
		if errors.Is(err, app.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("want ErrNotFound for church B's row, got %w", err)
	}
	unchanged := func(ok bool, err error) error {
		if err != nil || ok {
			return fmt.Errorf("church B's row changed through church A (ok=%v, err=%w)", ok, err)
		}
		return nil
	}
	harness := map[string]call{
		"Church.Get": func(cs app.ChurchStore, _ time.Time) error {
			c, err := cs.Church().Get(ctx)
			if err == nil && c.ID != domain.ChurchID(id("CHA")) {
				err = fmt.Errorf("got church %s", c.ID)
			}
			return err
		},
		"Church.Update": func(cs app.ChurchStore, now time.Time) error {
			c, err := cs.Church().Get(ctx)
			if err != nil {
				return err
			}
			c.Name, c.UpdatedAt = "Renamed", now
			return cs.Church().Update(ctx, c)
		},
		"Memberships.List": func(cs app.ChurchStore, _ time.Time) error {
			all, err := cs.Memberships().List(ctx)
			if err == nil && (len(all) != 1 || all[0].ID != domain.MembershipID(id("M1")) || len(all[0].RoleIDs) != 1) {
				err = fmt.Errorf("got %+v", all)
			}
			return err
		},
		"Memberships.ByID": func(cs app.ChurchStore, _ time.Time) error {
			if _, err := cs.Memberships().ByID(ctx, domain.MembershipID(id("M1"))); err != nil {
				return err
			}
			_, err := cs.Memberships().ByID(ctx, domain.MembershipID(id("MB")))
			return notFound(err)
		},
		"Memberships.ByUser": func(cs app.ChurchStore, _ time.Time) error {
			_, err := cs.Memberships().ByUser(ctx, domain.UserID(id("U2")))
			return notFound(err)
		},
		"Memberships.Create": func(cs app.ChurchStore, now time.Time) error {
			err := cs.Memberships().Create(ctx, domain.Membership{ID: domain.MembershipID(id("MX")), UserID: domain.UserID(id("U2")),
				RoleIDs: []domain.RoleID{domain.RoleID(id("RB"))}, CreatedAt: now})
			if !errors.Is(err, app.ErrReferenced) {
				return fmt.Errorf("assigning church B's role through A: %w", err)
			}
			return errRollback
		},
		"Memberships.SetRoles": func(cs app.ChurchStore, _ time.Time) error {
			return notFound(cs.Memberships().SetRoles(ctx, domain.MembershipID(id("MB")), nil))
		},
		"Memberships.Delete": func(cs app.ChurchStore, _ time.Time) error {
			return notFound(cs.Memberships().Delete(ctx, domain.MembershipID(id("MB"))))
		},
		"Memberships.Count": func(cs app.ChurchStore, _ time.Time) error {
			n, err := cs.Memberships().Count(ctx)
			if err == nil && n != 1 {
				err = fmt.Errorf("count %d", n)
			}
			return err
		},
		"Roles.List": func(cs app.ChurchStore, _ time.Time) error {
			all, err := cs.Roles().List(ctx)
			if err == nil && (len(all) != 1 || all[0].ID != domain.RoleID(id("RA")) || !all[0].Scopes.Has(domain.ScopeMembersView)) {
				err = fmt.Errorf("got %+v", all)
			}
			return err
		},
		"Roles.ByID": func(cs app.ChurchStore, _ time.Time) error {
			_, err := cs.Roles().ByID(ctx, domain.RoleID(id("RB")))
			return notFound(err)
		},
		"Roles.ByOrigin": func(cs app.ChurchStore, _ time.Time) error {
			r, err := cs.Roles().ByOrigin(ctx, domain.OriginChurchAdmin)
			if err == nil && r.ID != domain.RoleID(id("RA")) {
				err = fmt.Errorf("got %s", r.ID)
			}
			return err
		},
		"Roles.Create": func(cs app.ChurchStore, now time.Time) error {
			return cs.Roles().Create(ctx, domain.Role{ID: domain.RoleID(id("RX")), Name: "New", Scopes: domain.NewScopeSet(domain.ScopeLibraryEdit),
				CreatedAt: now, UpdatedAt: now})
		},
		"Roles.Update": func(cs app.ChurchStore, now time.Time) error {
			return notFound(cs.Roles().Update(ctx, domain.Role{ID: domain.RoleID(id("RB")), Name: "Hijack", Scopes: domain.ScopeSet{}, UpdatedAt: now}))
		},
		"Roles.Delete": func(cs app.ChurchStore, _ time.Time) error {
			return notFound(cs.Roles().Delete(ctx, domain.RoleID(id("RB"))))
		},
		"Roles.MemberCounts": func(cs app.ChurchStore, _ time.Time) error {
			n, err := cs.Roles().MemberCounts(ctx)
			if err == nil && (len(n) != 1 || n[domain.RoleID(id("RA"))] != 1) {
				err = fmt.Errorf("got %v", n)
			}
			return err
		},
		"Invites.List": func(cs app.ChurchStore, _ time.Time) error {
			all, err := cs.Invites().List(ctx)
			if err == nil && (len(all) != 1 || all[0].ID != domain.InviteID(id("I1"))) {
				err = fmt.Errorf("got %+v", all)
			}
			return err
		},
		"Invites.ByID": func(cs app.ChurchStore, _ time.Time) error {
			_, err := cs.Invites().ByID(ctx, domain.InviteID(id("IB")))
			return notFound(err)
		},
		"Invites.Create": func(cs app.ChurchStore, now time.Time) error {
			err := cs.Invites().Create(ctx, domain.Invite{ID: domain.InviteID(id("IX")), TokenHash: hash("9"), Name: "X",
				Email: "x@example.org", RoleIDs: []domain.RoleID{domain.RoleID(id("RB"))}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
			if !errors.Is(err, app.ErrReferenced) {
				return fmt.Errorf("inviting with church B's role through A: %w", err)
			}
			return errRollback
		},
		"Invites.CancelExpired": func(cs app.ChurchStore, now time.Time) error {
			return cs.Invites().CancelExpired(ctx, "invb@example.org", "", now.Add(48*time.Hour))
		},
		"Invites.CountPending": func(cs app.ChurchStore, now time.Time) error {
			n, err := cs.Invites().CountPending(ctx, now)
			if err == nil && n != 1 {
				err = fmt.Errorf("count %d", n)
			}
			return err
		},
		"Invites.Renew": func(cs app.ChurchStore, now time.Time) error {
			return unchanged(cs.Invites().Renew(ctx, domain.InviteID(id("IB")), hash("8"), now.Add(time.Hour)))
		},
		"Invites.Cancel": func(cs app.ChurchStore, now time.Time) error {
			return unchanged(cs.Invites().Cancel(ctx, domain.InviteID(id("IB")), now))
		},
		"Songs.ByID": func(cs app.ChurchStore, _ time.Time) error {
			s, err := cs.Songs().ByID(ctx, domain.SongID(id("SA1")))
			if err != nil || len(s.Sections) != 2 || len(s.DefaultArrangement) != 3 {
				return fmt.Errorf("own song: %+v %w", s, err)
			}
			_, err = cs.Songs().ByID(ctx, domain.SongID(id("SB1")))
			return notFound(err)
		},
		"Songs.Create": func(cs app.ChurchStore, now time.Time) error {
			s := testSong(id("SX"), "Hijack", "id", now)
			s.GroupID = domain.SongGroupID(id("GB"))
			if err := cs.Songs().Create(ctx, s); !errors.Is(err, app.ErrReferenced) {
				return fmt.Errorf("joining church B's group through A: %w", err)
			}
			return errRollback
		},
		"Songs.Update": func(cs app.ChurchStore, now time.Time) error {
			s := testSong(id("SB1"), "Hijack", "en", now)
			s.Version = 2
			return unchanged(cs.Songs().Update(ctx, s, 1))
		},
		"Songs.Delete": func(cs app.ChurchStore, _ time.Time) error {
			return notFound(cs.Songs().Delete(ctx, domain.SongID(id("SB1"))))
		},
		"Songs.Search": func(cs app.ChurchStore, _ time.Time) error {
			for _, q := range []app.SongSearch{{Limit: 50}, {Limit: 50, Terms: []string{"cinta"}}} {
				page, err := cs.Songs().Search(ctx, q)
				if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != domain.SongID(id("SA1")) {
					return fmt.Errorf("search %+v: %+v %w", q, page, err)
				}
			}
			return nil
		},
		"Songs.CreateGroup": func(cs app.ChurchStore, now time.Time) error {
			return cs.Songs().CreateGroup(ctx, domain.SongGroupID(id("GX")), now)
		},
		"Songs.DeleteGroup": func(cs app.ChurchStore, _ time.Time) error {
			return cs.Songs().DeleteGroup(ctx, domain.SongGroupID(id("GB"))) // matches no row of church A
		},
		"Songs.SetGroup": func(cs app.ChurchStore, now time.Time) error {
			return cs.Songs().SetGroup(ctx, []domain.SongID{domain.SongID(id("SB1"))}, "", now)
		},
		"Songs.GroupMembers": func(cs app.ChurchStore, _ time.Time) error {
			m, err := cs.Songs().GroupMembers(ctx, domain.SongGroupID(id("GB")))
			if err == nil && len(m) != 0 {
				err = fmt.Errorf("church B's group members through A: %+v", m)
			}
			return err
		},
		"Songs.Reindex": func(cs app.ChurchStore, _ time.Time) error {
			return cs.Songs().Reindex(ctx)
		},
	}

	// Reflection check: the harness covers every method of every repository.
	var want []string
	cst := reflect.TypeFor[app.ChurchStore]()
	for i := range cst.NumMethod() {
		m := cst.Method(i)
		if m.Type.NumIn() != 0 || m.Type.NumOut() != 1 || m.Type.Out(0).Kind() != reflect.Interface {
			continue // ChurchID, LockChurch
		}
		repo := m.Type.Out(0)
		for j := range repo.NumMethod() {
			want = append(want, m.Name+"."+repo.Method(j).Name)
		}
	}
	var have []string
	for k := range harness {
		have = append(have, k)
	}
	slices.Sort(want)
	slices.Sort(have)
	if !slices.Equal(want, have) {
		t.Fatalf("harness out of date:\nrepository methods %v\nharness            %v", want, have)
	}

	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		if err := f.user(id("U2"), "u2@example.org", ""); err != nil {
			t.Fatal(err)
		}
		f.must(`INSERT INTO memberships (id, church_id, user_id, created_at) VALUES (?, ?, ?, ?)`, id("MB"), id("CHB"), id("U2"), f.ts(0))
		f.must(`INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)`, id("CHB"), id("MB"), id("RB"))
		f.must(`INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, 'roles.manage')`, id("CHB"), id("RB"))
		if err := f.invite(id("IB"), id("CHB"), hash("5"), "invb@example.org", ""); err != nil {
			t.Fatal(err)
		}
		f.must(`INSERT INTO invite_roles (church_id, invite_id, role_id) VALUES (?, ?, ?)`, id("CHB"), id("IB"), id("RB"))
		mustWrite(t, db, func(s app.Store) error {
			a, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			if err := a.Songs().Create(ctx, testSong(id("SA1"), "Cinta Tuhan", "id", f.now)); err != nil {
				return err
			}
			b, err := s.ForChurch(ctx, domain.ChurchID(id("CHB")))
			if err != nil {
				return err
			}
			if err := b.Songs().CreateGroup(ctx, domain.SongGroupID(id("GB")), f.now); err != nil {
				return err
			}
			for sid, lang := range map[string]string{"SB1": "en", "SB2": "id"} {
				song := testSong(id(sid), "Cinta Tuhan B", lang, f.now)
				song.GroupID = domain.SongGroupID(id("GB"))
				if err := b.Songs().Create(ctx, song); err != nil {
					return err
				}
			}
			return nil
		})

		// Each call runs in its own transaction, rolled back afterwards, so
		// every call sees the same rows; changes to B would show up inside.
		before := churchRows(t, db, id("CHB"))
		for _, name := range have {
			err := db.Write(ctx, func(s app.Store) error {
				cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
				if err != nil {
					return err
				}
				if err := harness[name](cs, f.now); err != nil {
					return err
				}
				if after := churchRowsIn(t, s, db, id("CHB")); after != before {
					return fmt.Errorf("church B changed:\nbefore %s\nafter  %s", before, after)
				}
				return errRollback
			})
			if !errors.Is(err, errRollback) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if after := churchRows(t, db, id("CHB")); after != before {
			t.Errorf("church B changed:\nbefore %s\nafter  %s", before, after)
		}
	})
}

// Platform repositories added in slice 4 (both dialects).
func TestPlatformRepos(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		now := f.now
		mustRead(t, db, func(s app.Store) error {
			n, err := s.Churches().Count(ctx)
			ids, err2 := s.Churches().IDs(ctx, 1)
			if err != nil || err2 != nil || n != 2 || len(ids) != 1 || ids[0] != domain.ChurchID(id("CHA")) {
				t.Errorf("churches: %d %v %v %v", n, ids, err, err2)
			}
			c, err := s.Churches().ByID(ctx, domain.ChurchID(id("CHB")))
			if err != nil || c.TimeZone != "Asia/Jakarta" || !c.CreatedAt.Equal(now) {
				t.Errorf("church by ID: %+v %v", c, err)
			}
			list, err := s.Translations().List(ctx)
			if err != nil || len(list) != 6 || list[0].Code != "BIS" {
				t.Errorf("translations: %+v %v", list, err)
			}
			if tr, err := s.Translations().ByCode(ctx, "TB"); err != nil || tr.ID != tb {
				t.Errorf("TB: %+v %v", tr, err)
			}
			if _, err := s.Translations().ByID(ctx, "01M3XY2HBEKN8PETK6KXXXXXXX"); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("unknown translation: %v", err)
			}
			users, err := s.Users().List(ctx)
			if err != nil || len(users) != 1 {
				t.Errorf("users: %+v %v", users, err)
			}
			return nil
		})

		// Church settings keep unknown keys.
		f.must("UPDATE churches SET settings = ? WHERE id = ?", `{"key_display":"letter","future":{"x":1}}`, id("CHA"))
		mustWrite(t, db, func(s app.Store) error {
			cs, _ := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			c, err := cs.Church().Get(ctx)
			if err != nil || c.Settings.KeyDisplay != "letter" {
				return fmt.Errorf("settings: %+v %w", c.Settings, err)
			}
			c.Settings.FeedbackURL = "https://f.example.org"
			return cs.Church().Update(ctx, c)
		})
		mustRead(t, db, func(s app.Store) error {
			c, err := s.Churches().ByID(ctx, domain.ChurchID(id("CHA")))
			if err != nil || !strings.Contains(strings.ReplaceAll(string(c.Settings.Extra["future"]), " ", ""), `{"x":1}`) || c.Settings.FeedbackURL != "https://f.example.org" {
				t.Errorf("unknown settings key lost: %+v %v", c.Settings, err)
			}
			return nil
		})

		// Setup token: upsert, claim once, expiry.
		mustWrite(t, db, func(s app.Store) error { return s.SetupTokens().Put(ctx, hash("6"), now, now.Add(time.Hour)) })
		var claimed []bool
		for range 2 {
			mustWrite(t, db, func(s app.Store) error {
				ok, err := s.SetupTokens().Claim(ctx, hash("6"), now)
				claimed = append(claimed, ok)
				return err
			})
		}
		if !slices.Equal(claimed, []bool{true, false}) {
			t.Errorf("setup claim: %v", claimed)
		}

		// Invite tokens: claim once, then set the user.
		mustWrite(t, db, func(s app.Store) error {
			inv, err := s.InviteTokens().ByTokenHash(ctx, hash("1"))
			if err != nil || inv.ID != domain.InviteID(id("I1")) {
				return fmt.Errorf("by token: %+v %w", inv, err)
			}
			inv, ok, err := s.InviteTokens().Claim(ctx, hash("1"), "", now)
			if err != nil || !ok || inv.AcceptedAt.IsZero() || inv.ChurchID != domain.ChurchID(id("CHA")) {
				return fmt.Errorf("claim: %+v %v %w", inv, ok, err)
			}
			if _, ok, err := s.InviteTokens().Claim(ctx, hash("1"), "", now); ok || err != nil {
				return fmt.Errorf("second claim: %v %w", ok, err)
			}
			return s.InviteTokens().SetAcceptedUser(ctx, inv.ID, domain.UserID(id("U1")))
		})

		// Reset links: close open ones, claim once, latest per user, cleanup.
		mustWrite(t, db, func(s app.Store) error {
			if err := s.PasswordResets().CloseOpen(ctx, domain.UserID(id("U1")), now); err != nil {
				return err
			}
			if err := s.PasswordResets().Create(ctx, domain.PasswordReset{ID: id("P2"), UserID: domain.UserID(id("U1")),
				TokenHash: hash("7"), CreatedBy: domain.UserID(id("U1")), CreatedAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour)}); err != nil {
				return err
			}
			p, ok, err := s.PasswordResets().Claim(ctx, hash("7"), now)
			if err != nil || !ok || p.UsedAt.IsZero() {
				return fmt.Errorf("claim: %+v %v %w", p, ok, err)
			}
			if _, ok, err := s.PasswordResets().Claim(ctx, hash("2"), now); ok || err != nil {
				return fmt.Errorf("closed link claimed: %v %w", ok, err)
			}
			latest, err := s.PasswordResets().Latest(ctx, []domain.UserID{domain.UserID(id("U1"))})
			if err != nil || latest[domain.UserID(id("U1"))].ID != id("P2") {
				return fmt.Errorf("latest: %+v %w", latest, err)
			}
			return s.PasswordResets().DeleteOld(ctx, now.Add(2*time.Hour))
		})

		// Throttle deletions by identifier and address.
		mustWrite(t, db, func(s app.Store) error {
			keys := domain.ThrottleKeys("u1@example.org", "203.0.113.5")
			for kind, key := range keys {
				if err := s.AuthThrottle().RecordFailure(ctx, key, domain.ThrottleRules[kind], now); err != nil {
					return err
				}
			}
			if err := s.AuthThrottle().DeleteForIdentifier(ctx, "u1@example.org"); err != nil {
				return err
			}
			return s.AuthThrottle().DeleteForAddr(ctx, "203.0.113.5")
		})
		mustRead(t, db, func(s app.Store) error {
			var n int
			if err := sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT count(*) FROM auth_throttle").Scan(&n); err != nil || n != 0 {
				t.Errorf("throttle rows left: %d %v", n, err)
			}
			if err := sqlstore.RawTx(s).QueryRowContext(ctx, "SELECT count(*) FROM password_resets").Scan(&n); err != nil || n != 0 {
				t.Errorf("reset rows left: %d %v", n, err)
			}
			return nil
		})
	})
}

// testSong is a song with a verse, a chorus and the arrangement V1 C V1,
// whose section IDs are derived from the song ID.
func testSong(songID, title, language string, now time.Time) domain.Song {
	verse, chorus := domain.SectionID(songID[:24]+"V1"), domain.SectionID(songID[:24]+"C1")
	return domain.Song{
		ID: domain.SongID(songID), Language: language, Title: title, AltTitles: []string{}, LicenceStatus: domain.LicenceUnknown,
		Sections: []domain.Section{
			{ID: verse, Kind: domain.SectionVerse, Number: 1, Text: "cinta yang besar"},
			{ID: chorus, Kind: domain.SectionChorus, Text: "setia selamanya"},
		},
		DefaultArrangement: []domain.SectionID{verse, chorus, verse},
		Version:            1, CreatedAt: now, UpdatedAt: now,
	}
}
