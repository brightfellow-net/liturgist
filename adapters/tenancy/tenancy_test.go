// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package tenancy_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func TestMain(m *testing.M) { os.Exit(sqlstoretest.Main(m)) }

var ctx = context.Background()

// TC-T-003
func TestNoPrefix(t *testing.T) {
	for base, want := range map[string]string{
		"https://x.org":  "https://x.org/invite",
		"https://x.org/": "https://x.org/invite",
	} {
		u, _ := url.Parse(base)
		b := tenancy.NoPrefix{BaseURL: u}
		if got := b.AppURL(ctx, "/invite"); got != want {
			t.Errorf("%s: %s", base, got)
		}
		if got := b.AppPath(ctx, "/members"); got != "/members" {
			t.Errorf("AppPath: %s", got)
		}
	}
}

// countingTx counts transactions, to show the cached ID needs no database access.
type countingTx struct {
	app.Tx
	n int
}

func (c *countingTx) Read(ctx context.Context, fn func(app.Store) error) error {
	c.n++
	return c.Tx.Read(ctx, fn)
}

// TC-T-004
func TestSingleChurch(t *testing.T) {
	db := sqlstoretest.NewSQLite(t)
	tx := &countingTx{Tx: db}
	r := &tenancy.SingleChurch{Tx: tx}
	if _, err := r.ChurchID(ctx); !errors.Is(err, app.ErrNotSetUp) {
		t.Fatalf("no church: %v", err)
	}
	if err := r.Check(ctx); err != nil {
		t.Errorf("check before setup: %v", err)
	}
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	create := func(id string) {
		err := db.Write(ctx, func(s app.Store) error {
			return s.Churches().Create(ctx, domain.Church{ID: domain.ChurchID(id), Name: "C", DefaultUILanguage: "en",
				DefaultLanguage: "id", DefaultTranslationID: "01M3XY2HBEKN8PETK6KCN370RJ", TimeZone: "Asia/Jakarta",
				Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: now, UpdatedAt: now})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	create("01JCHURCHA0000000000000000")
	if err := r.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	before := tx.n
	for range 3 {
		if id, err := r.ChurchID(ctx); err != nil || id != "01JCHURCHA0000000000000000" {
			t.Fatalf("resolved %q %v", id, err)
		}
	}
	if tx.n != before {
		t.Errorf("cached ID read the database %d times", tx.n-before)
	}

	create("01JCHURCHB0000000000000000")
	err := (&tenancy.SingleChurch{Tx: db}).Check(ctx)
	var many *tenancy.TooManyChurchesError
	if !errors.As(err, &many) || many.N != 2 || !errors.Is(err, app.ErrTooManyChurches) {
		t.Errorf("two churches: %v", err)
	}
}
