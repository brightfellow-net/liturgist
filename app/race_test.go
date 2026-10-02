// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// race runs fn n times at once and returns the errors.
func race(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func succeeded(errs []error) (ok int, others []error) {
	for _, err := range errs {
		if err == nil {
			ok++
		} else {
			others = append(others, err)
		}
	}
	return ok, others
}

func linkToken(link string) string { return link[strings.Index(link, "#t=")+3:] }

// IT-A-013: of two simultaneous uses of one invite, reset link or setup
// token, exactly one succeeds; the other gets invalid_token (both dialects).
func TestTokenRaces(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := wireChurch(t, db, nil)
		token, _, err := e.setup.IssueToken(ctx)
		if err != nil {
			t.Fatal(err)
		}
		errs := race(2, func(i int) error {
			_, err := e.setup.Run(ctx, app.SetupInput{Token: token, Church: churchInput(), AdminName: "Admin",
				AdminIdentifier: []string{"a@example.org", "b@example.org"}[i], AdminPassword: password})
			return err
		})
		if ok, others := succeeded(errs); ok != 1 || tokenReason(others[0]) != domain.TokenUnknown && !errors.Is(others[0], app.ErrAlreadySetUp) {
			t.Fatalf("setup race: %v", errs)
		}
		var churches int
		if err := db.Read(ctx, func(s app.Store) (err error) { churches, err = s.Churches().Count(ctx); return }); err != nil || churches != 1 {
			t.Fatalf("churches after setup race: %d %v", churches, err)
		}
		ids := make([]domain.ChurchID, 0, 1)
		_ = db.Read(ctx, func(s app.Store) (err error) { ids, err = s.Churches().IDs(ctx, 1); return })
		e.church, e.ctx = ids[0], app.WithTenant(ctx, ids[0])
		_ = db.Read(ctx, func(s app.Store) error {
			cs, _ := s.ForChurch(ctx, e.church)
			all, err := cs.Memberships().List(ctx)
			if err == nil {
				e.admin = &domain.Session{UserID: all[0].UserID}
			}
			return err
		})

		_, link, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "Sari", Email: "sari@example.org"})
		if err != nil {
			t.Fatal(err)
		}
		errs = race(2, func(i int) error {
			_, err := e.invites.Accept(ctx, app.AcceptInput{Token: linkToken(link.Link), Name: "Sari",
				Email: []string{"sari@example.org", "sari2@example.org"}[i], Password: password})
			return err
		})
		if ok, others := succeeded(errs); ok != 1 || tokenReason(others[0]) != domain.TokenUsed {
			t.Errorf("invite race: %v", errs)
		}
		list, _ := e.members.List(e.ctx, e.admin)
		if len(list.Members) != 2 {
			t.Errorf("members after invite race: %d", len(list.Members))
		}

		res, err := e.resets.CreateForUser(ctx, "sari@example.org")
		if err != nil {
			res, err = e.resets.CreateForUser(ctx, "sari2@example.org")
		}
		if err != nil {
			t.Fatal(err)
		}
		errs = race(2, func(int) error {
			_, err := e.resets.Use(ctx, app.ResetInput{Token: linkToken(res.Link), NewPassword: password})
			return err
		})
		if ok, others := succeeded(errs); ok != 1 || tokenReason(others[0]) != domain.TokenUsed {
			t.Errorf("reset race: %v", errs)
		}

		// Two simultaneous reset-link creations leave one open link.
		var adminEmail string
		_ = db.Read(ctx, func(s app.Store) error {
			u, err := s.Users().ByID(ctx, e.admin.UserID)
			adminEmail = u.Email
			return err
		})
		errs = race(2, func(int) error { _, err := e.resets.CreateForUser(ctx, adminEmail); return err })
		if ok, _ := succeeded(errs); ok != 2 {
			t.Errorf("reset creation: %v", errs)
		}
		var open int
		_ = db.Read(ctx, func(s app.Store) error {
			latest, err := s.PasswordResets().Latest(ctx, []domain.UserID{e.admin.UserID})
			if err == nil && latest[e.admin.UserID].UsedAt.IsZero() {
				open = 1
			}
			return err
		})
		if open != 1 {
			t.Error("the newest reset link should be open")
		}
	})
}

// IT-A-007: two invites at the limit at once; exactly one succeeds.
func TestConcurrentInvitesAtLimit(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, limitStub{max: 2})
		errs := race(2, func(i int) error {
			_, _, err := e.invites.Create(e.ctx, e.admin, app.InviteInput{Name: "X", Email: []string{"x@example.org", "y@example.org"}[i]})
			return err
		})
		var lim *app.LimitReachedError
		if ok, others := succeeded(errs); ok != 1 || !errors.As(others[0], &lim) {
			t.Errorf("invites at limit: %v", errs)
		}
	})
}
