// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// bis and cuv are seeded translations other than TB.
var (
	bis = domain.Translation{ID: "01M3XY2HBEKN8PETK6KHCT82HX", Code: "BIS", Language: "id"}
	cuv = domain.Translation{ID: "01M3XY2HBEKN8PETK6KHKD8V8T", Code: "CUV", Language: "zh-Hans"}
)

// The reading repository behaves the same on both databases (IT-R-002, IT-R-004, IT-R-005).
func TestReadingRepo(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		mk := func(rid, ref string, tr domain.Translation, text string, age time.Duration) domain.Reading {
			r := testReading(id(rid), ref, f.now.Add(age))
			r.Translation, r.Text = tr, text
			r.Attribution = tr.Code + " " + rid
			return r
		}
		f.churchRepo2(func(r app.ReadingRepo) error {
			for _, rd := range []domain.Reading{
				mk("RD1", "JHN 3:16", domain.Translation{ID: tb, Code: "TB", Language: "id"}, "Karena begitu besar", time.Minute),
				mk("RD2", "JHN 3:16", bis, "Allah sangat mengasihi", 2*time.Minute),
				mk("RD3", "PSA 23", domain.Translation{ID: tb, Code: "TB", Language: "id"}, "TUHAN adalah gembalaku", 3*time.Minute),
				mk("RD4", "JHN 3:16", cuv, "神爱世人", 4*time.Minute),
			} {
				if err := r.Create(ctx, rd); err != nil {
					return err
				}
			}

			// By ID and by reference; the translation comes with it.
			got, err := r.ByID(ctx, domain.ReadingID(id("RD1")))
			if err != nil || got.Reference != "JHN 3:16" || got.Translation.Code != "TB" || got.Translation.Name == "" ||
				got.Text != "Karena begitu besar" || got.Version != 1 || !got.CreatedAt.Equal(f.now.Add(time.Minute)) {
				t.Errorf("ByID: %+v %v", got, err)
			}
			if got, err := r.ByReference(ctx, "JHN 3:16", bis.ID); err != nil || got.ID != domain.ReadingID(id("RD2")) {
				t.Errorf("ByReference: %+v %v", got, err)
			}
			if _, err := r.ByReference(ctx, "PSA 23", bis.ID); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("ByReference of another translation: %v", err)
			}
			if _, err := r.ByID(ctx, domain.ReadingID(id("NONE"))); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("unknown ID: %v", err)
			}
			return nil
		})

		// A second reading for the same reference and translation is a unique violation.
		var u *app.UniqueError
		f.churchRepo2(func(r app.ReadingRepo) error {
			dup := mk("RD9", "JHN 3:16", bis, "again", 0)
			if err := r.Create(ctx, dup); !errors.As(err, &u) || u.Constraint != "readings_church_ref_key" {
				t.Errorf("duplicate: %v", err)
			}
			return errRollbackFixture
		})

		f.churchRepo2(func(r app.ReadingRepo) error {
			ids := func(q app.ReadingSearch) []string {
				rows, err := r.List(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
				var out []string
				for _, row := range rows {
					out = append(out, string(row.ID))
				}
				slices.Sort(out)
				return out
			}
			want := func(names ...string) []string {
				var out []string
				for _, n := range names {
					out = append(out, id(n))
				}
				slices.Sort(out)
				return out
			}
			for name, c := range map[string]struct {
				q    app.ReadingSearch
				want []string
			}{
				"all":                 {app.ReadingSearch{}, want("RD1", "RD2", "RD3", "RD4")},
				"by translation":      {app.ReadingSearch{Translation: "BIS"}, want("RD2")},
				"text":                {app.ReadingSearch{Fold: "gembalaku", FoldZh: "gembalaku"}, want("RD3")},
				"canonical book name": {app.ReadingSearch{Fold: "yohanes", FoldZh: "yohanes"}, want("RD1", "RD2", "RD4")},
				"two words":           {app.ReadingSearch{Fold: "allah sangat", FoldZh: "allahsangat"}, want("RD2")},
				"chinese, no spaces":  {app.ReadingSearch{Fold: "爱 世人", FoldZh: "爱世人"}, want("RD4")},
				"chinese with spaces": {app.ReadingSearch{Fold: "爱世人", FoldZh: "爱世人"}, want("RD4")},
				"nothing":             {app.ReadingSearch{Fold: "zzz", FoldZh: "zzz"}, want()},
				"filter and text":     {app.ReadingSearch{Fold: "jhn", FoldZh: "jhn", Translation: "TB"}, want("RD1")},
			} {
				if got := ids(c.q); !slices.Equal(got, c.want) && (len(got) > 0 || len(c.want) > 0) {
					t.Errorf("%s: got %v, want %v", name, got, c.want)
				}
			}
			rows, _ := r.List(ctx, app.ReadingSearch{Fold: "gembalaku", FoldZh: "gembalaku"})
			if len(rows) != 1 || rows[0].TextStart != "TUHAN adalah gembalaku" || rows[0].Translation.Code != "TB" {
				t.Errorf("row: %+v", rows)
			}

			// The most recently updated reading's attribution, per translation.
			for tr, want := range map[domain.TranslationID]string{tb: "TB RD3", bis.ID: "BIS RD2", cuv.ID: "CUV RD4", "01M3XY2HBEKN8PETK6KDMMG2WG": ""} { // the last is TB2, which has no readings
				if got, err := r.LatestAttribution(ctx, tr); err != nil || got != want {
					t.Errorf("LatestAttribution(%s) = %q (%v), want %q", tr, got, err, want)
				}
			}
			return nil
		})

		// Conditional update: only the expected version changes the row.
		f.churchRepo2(func(r app.ReadingRepo) error {
			rd, _ := r.ByID(ctx, domain.ReadingID(id("RD1")))
			rd.Text, rd.Attribution, rd.Version, rd.UpdatedAt = "Baru", "TB baru", 2, f.now.Add(time.Hour)
			if ok, err := r.Update(ctx, rd, 5); err != nil || ok {
				t.Errorf("update with a wrong version: %v %v", ok, err)
			}
			if ok, err := r.Update(ctx, rd, 1); err != nil || !ok {
				t.Errorf("update: %v %v", ok, err)
			}
			got, _ := r.ByID(ctx, rd.ID)
			if got.Text != "Baru" || got.Version != 2 || got.Attribution != "TB baru" {
				t.Errorf("after update: %+v", got)
			}
			if a, _ := r.LatestAttribution(ctx, tb); a != "TB baru" {
				t.Errorf("latest attribution after update: %q", a)
			}
			// The search text follows the new text.
			rows, _ := r.List(ctx, app.ReadingSearch{Fold: "baru", FoldZh: "baru"})
			if len(rows) != 1 || rows[0].ID != rd.ID {
				t.Errorf("search after update: %+v", rows)
			}
			if err := r.Delete(ctx, rd.ID); err != nil {
				t.Error(err)
			}
			if err := r.Delete(ctx, rd.ID); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("second delete: %v", err)
			}
			return errRollbackFixture
		})
	})
}

var errRollbackFixture = errors.New("rollback the test transaction")

// churchRepo2 runs fn on church A's reading repository in one transaction;
// returning errRollbackFixture rolls it back without failing the test.
func (f fixture) churchRepo2(fn func(r app.ReadingRepo) error) {
	f.t.Helper()
	err := f.db.Write(ctx, func(s app.Store) error {
		cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
		if err != nil {
			return err
		}
		return fn(cs.Readings())
	})
	if err != nil && !errors.Is(err, errRollbackFixture) {
		f.t.Fatal(err)
	}
}
