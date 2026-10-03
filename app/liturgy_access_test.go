// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// IT-L-001: who sees and who changes liturgies.
func TestLiturgyPermissions(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	L := e.liturgies
	lid := e.draft()
	item := e.addItem(lid, domain.ItemPrayer, "Doa")
	duties, _ := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
	duty := domain.DutyID(duties[0].Entry.ID)

	team, _ := e.member("team@example.org")
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID) // liturgy.edit, no liturgy.manage
	commenter, _ := e.member("commenter@example.org", func() domain.RoleID {
		r, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "Komentator", Scopes: []domain.Scope{domain.ScopeLiturgyComment}})
		if err != nil {
			t.Fatal(err)
		}
		return r.Role.ID
	}())
	var outsider *domain.Session
	e.write(func(s app.Store) error {
		now := e.clock.Now()
		id := domain.UserID(e.ids.NewID())
		outsider = &domain.Session{UserID: id}
		return s.Users().Create(ctx, domain.User{ID: id, Name: "Luar", Email: "luar@example.org", PasswordHash: "x", CreatedAt: now, UpdatedAt: now})
	})

	// No session: 401. Not a member: 404.
	if _, err := L.List(e.ctx, nil, app.LiturgyFilter{Limit: 50}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no session: %v", err)
	}
	if _, err := L.Get(e.ctx, outsider, lid); !isNotFound(err, app.ReasonNotMember) {
		t.Errorf("a user of no church: %v", err)
	}
	if _, err := L.Create(e.ctx, nil, app.CreateInput{}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("create without a session: %v", err)
	}

	// A team member sees nothing of an unpublished liturgy and cannot write.
	if page, err := L.List(e.ctx, team, app.LiturgyFilter{Limit: 50}); err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("team member's list: %+v %v", page, err)
	}
	if _, err := L.Get(e.ctx, team, lid); !isNotFound(err, app.ReasonNotVisible) {
		t.Errorf("team member reads: %v", err)
	}
	if _, err := L.Assignable(e.ctx, team); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member lists assignable: %v", err)
	}
	title := "x"
	for name, err := range map[string]error{
		"update": func() error { _, err := L.Update(e.ctx, team, lid, app.LiturgyChange{Version: 1}); return err }(),
		"delete": L.Delete(e.ctx, team, lid),
		"add item": func() error {
			_, err := L.AddItem(e.ctx, team, lid, app.ItemInput{LiturgyVersion: 1, Title: "x", Type: domain.ItemPrayer})
			return err
		}(),
		"item": func() error {
			_, err := L.UpdateItem(e.ctx, team, lid, item, app.ItemChange{Version: 1, Title: &title})
			return err
		}(),
		"assign": func() error {
			_, err := L.AddAssignment(e.ctx, team, lid, app.AssignmentInput{DutyID: duty, Name: "X"})
			return err
		}(),
	} {
		if !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member %s: %v (404 comes before 403)", name, err)
		}
	}
	if _, err := L.Create(e.ctx, team, app.CreateInput{Date: "2026-10-11", ServiceName: "X"}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member creates: %v", err)
	}
	if _, err := L.Prepare(e.ctx, team, ""); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team member prepares: %v", err)
	}

	// A commenter reads everything and changes nothing: 403, not 404.
	if v, err := L.Get(e.ctx, commenter, lid); err != nil || v.Actions.Edit || v.Actions.Delete || len(v.Items) != 1 {
		t.Errorf("commenter reads: %+v %v", v.Actions, err)
	}
	if page, _ := L.List(e.ctx, commenter, app.LiturgyFilter{Limit: 50}); page.Total != 1 || page.Items[0].Actions.Edit {
		t.Errorf("commenter's list: %+v", page)
	}
	if h, err := L.Edits(e.ctx, commenter, lid, 10); err != nil || len(h) == 0 {
		t.Errorf("commenter reads the history: %v", err)
	}
	if _, err := L.Assignable(e.ctx, commenter); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("commenter lists assignable: %v", err)
	}
	for name, err := range map[string]error{
		"update": func() error { _, err := L.Update(e.ctx, commenter, lid, app.LiturgyChange{Version: 1}); return err }(),
		"delete": L.Delete(e.ctx, commenter, lid),
		"add item": func() error {
			_, err := L.AddItem(e.ctx, commenter, lid, app.ItemInput{LiturgyVersion: 2, Title: "x", Type: domain.ItemPrayer})
			return err
		}(),
		"item": func() error {
			_, err := L.UpdateItem(e.ctx, commenter, lid, item, app.ItemChange{Version: 1, Title: &title})
			return err
		}(),
		"remove":  func() error { _, err := L.RemoveItem(e.ctx, commenter, lid, item, 2); return err }(),
		"reorder": func() error { _, err := L.ReorderItems(e.ctx, commenter, lid, 2, []domain.ItemID{item}); return err }(),
		"songs":   func() error { _, err := L.SetSongs(e.ctx, commenter, lid, item, 1, nil); return err }(),
		"assign": func() error {
			_, err := L.AddAssignment(e.ctx, commenter, lid, app.AssignmentInput{DutyID: duty, Name: "X"})
			return err
		}(),
		"unassign": L.RemoveAssignment(e.ctx, commenter, lid, "01ARZ3NDEKTSV4RRFFQ69G5FAV"),
	} {
		if !errors.Is(err, app.ErrForbidden) {
			t.Errorf("commenter %s: %v", name, err)
		}
	}
	if got := e.liturgy(lid); got.Liturgy.Version != 2 || got.Items[0].Item.Title != "Doa" {
		t.Errorf("refused writes changed the liturgy: %+v", got.Liturgy)
	}

	// An editor writes but cannot delete; delete needs liturgy.manage.
	made, err := L.Create(e.ctx, editor, app.CreateInput{Date: "2026-10-18", ServiceName: "Editor", TemplateID: ptr(domain.TemplateID(""))})
	if err != nil || !made.Actions.Edit || made.Actions.Delete {
		t.Fatalf("editor creates: %+v %v", made.Actions, err)
	}
	if _, err := L.AddItem(e.ctx, editor, made.Liturgy.ID, app.ItemInput{LiturgyVersion: 1, Title: "x", Type: domain.ItemPrayer}); err != nil {
		t.Errorf("editor adds an item: %v", err)
	}
	if _, err := L.AddAssignment(e.ctx, editor, made.Liturgy.ID, app.AssignmentInput{DutyID: duty, UserID: team.UserID}); err != nil {
		t.Errorf("editor assigns: %v", err)
	}
	if list, err := L.Assignable(e.ctx, editor); err != nil || len(list) != 4 {
		t.Errorf("assignable for an editor: %+v %v", list, err)
	}
	if err := L.Delete(e.ctx, editor, made.Liturgy.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor deletes: %v", err)
	}
	if err := L.Delete(e.ctx, e.admin, made.Liturgy.ID); err != nil {
		t.Errorf("manager deletes: %v", err)
	}
	if _, err := L.Get(e.ctx, e.admin, made.Liturgy.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	// Deleting takes the items, assignments and history with it.
	if err := L.Delete(e.ctx, e.admin, made.Liturgy.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

// namedLimits is an Entitlements with usage limits by name (a missing name is unlimited).
type namedLimits map[app.LimitName]int

func (namedLimits) Has(context.Context, domain.ChurchID, app.Feature) (bool, error) { return true, nil }
func (n namedLimits) Limit(_ context.Context, _ domain.ChurchID, name app.LimitName) (app.Limit, error) {
	if max, ok := n[name]; ok {
		return app.Limit{Max: max}, nil
	}
	return app.Limit{Unlimited: true}, nil
}

type brokenLimits struct{ namedLimits }

func (brokenLimits) Limit(context.Context, domain.ChurchID, app.LimitName) (app.Limit, error) {
	return app.Limit{}, errors.New("entitlements down")
}

func limitErr(err error) (name app.LimitName, used, max int) {
	var l *app.LimitReachedError
	if errors.As(err, &l) {
		return l.Limit, l.Used, l.Max
	}
	return "", 0, 0
}

// IT-L-005: the limits on liturgies.
func TestLiturgyLimits(t *testing.T) {
	for _, name := range []app.LimitName{app.LimitMaxUnpublishedLiturgies, app.LimitMaxActiveLiturgies} {
		t.Run(string(name), func(t *testing.T) {
			sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
				e := newChurch(t, db, namedLimits{name: 2})
				L := e.liturgies
				create := func(date string) (app.LiturgyView, error) {
					return L.Create(e.ctx, e.admin, app.CreateInput{Date: date, ServiceName: "X", TemplateID: ptr(domain.TemplateID(""))})
				}
				a, err := create("2026-10-11")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := create("2026-10-12"); err != nil {
					t.Fatal(err)
				}
				if n, used, max := limitErr(func() error { _, err := create("2026-10-13"); return err }()); n != name || used != 2 || max != 2 {
					t.Errorf("third liturgy: %v %d %d", n, used, max)
				}
				// Editing, reading and deleting are never limited; deleting frees a place.
				e.addItem(a.Liturgy.ID, domain.ItemPrayer, "Doa")
				e.liturgy(a.Liturgy.ID)
				if err := L.Delete(e.ctx, e.admin, a.Liturgy.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := create("2026-10-13"); err != nil {
					t.Errorf("after a delete: %v", err)
				}
				// A batch needs places for all of it, and creates none when it does not fit.
				tpl, _ := e.seededTemplate()
				svc := e.newService("Pagi", "id", tpl, sunday("07:00"), domain.ServiceTime{Weekday: 3, Time: "19:00"})
				if err := L.Delete(e.ctx, e.admin, e.firstLiturgy()); err != nil {
					t.Fatal(err)
				}
				_, err = L.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{{ServiceID: svc, Date: "2026-10-14", Time: "19:00"}, {ServiceID: svc, Date: "2026-10-18", Time: "07:00"}})
				if n, used, max := limitErr(err); n != name || used != 1 || max != 2 {
					t.Errorf("a batch of two with one place: %v", err)
				}
				if page, _ := L.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50}); page.Total != 1 {
					t.Errorf("a refused batch created liturgies: %d", page.Total)
				}
				w, _ := L.Prepare(e.ctx, e.admin, "2026-10-12")
				if w.Active.Used != 1 || w.Unpublished.Used != 1 {
					t.Errorf("use on the prepare page: %+v %+v", w.Active, w.Unpublished)
				}
				if name == app.LimitMaxActiveLiturgies && (w.Active.Unlimited || w.Active.Max != 2 || !w.Unpublished.Unlimited) ||
					name == app.LimitMaxUnpublishedLiturgies && (w.Unpublished.Unlimited || w.Unpublished.Max != 2 || !w.Active.Unlimited) {
					t.Errorf("limit values: %+v %+v", w.Active, w.Unpublished)
				}
				// Two requests for the last place: one wins (the race harness).
				errs := race(2, func(i int) error { _, err := create([]string{"2026-11-01", "2026-11-02"}[i]); return err })
				ok, others := succeeded(errs)
				if n, _, _ := limitErr(others[0]); ok != 1 || n != name {
					t.Errorf("last place: %d won, others %v", ok, others)
				}
			})
		})
	}
	t.Run("entitlements unavailable", func(t *testing.T) {
		e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
		l := &app.Liturgies{Tx: e.db, Clock: e.clock, IDs: e.ids, Entitlements: brokenLimits{}}
		if _, err := l.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", ServiceName: "X"}); !errors.Is(err, app.ErrUnavailable) {
			t.Errorf("an error is never read as allowed: %v", err)
		}
	})
}
