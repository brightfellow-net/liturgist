// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// planUsageStub stands in for slice 3B's check of liturgies.
type planUsageStub struct{ duty, part bool }

func (u *planUsageStub) DutyInUse(context.Context, domain.ChurchID, string) (bool, error) {
	return u.duty, nil
}

func (u *planUsageStub) SingingPartInUse(context.Context, domain.ChurchID, string) (bool, error) {
	return u.part, nil
}

// planner is a member who holds templates.edit (and nothing else).
func (e cenv) planner() *domain.Session {
	e.t.Helper()
	role, err := e.roles.Create(e.ctx, e.admin, app.RoleInput{Name: "Planner", Scopes: []domain.Scope{domain.ScopeTemplatesEdit}})
	if err != nil {
		e.t.Fatal(err)
	}
	sess, _ := e.member("planner@example.org", role.Role.ID)
	return sess
}

func names(v []app.NameEntryView) []string {
	out := make([]string, len(v))
	for i, e := range v {
		out[i] = e.Entry.Name
	}
	return out
}

func limitReason(err error) (reason string, max, used int) {
	var in *domain.InvalidInputError
	if errors.As(err, &in) {
		return in.Reason, in.Max, in.Used
	}
	return "", 0, 0
}

// IT-P-001: who can read and who can change the planning setup.
func TestPlanningPermissions(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	team, _ := e.member("team@example.org")                                     // no role
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID) // liturgy.edit, no templates.edit
	planner := e.planner()

	// Duties and singing parts: any member reads; templates.edit changes.
	if v, err := e.vocab.List(e.ctx, team, domain.KindDuty); err != nil || len(v) != 7 || v[0].Actions.Edit {
		t.Errorf("team lists duties: %v %v", names(v), err)
	}
	if _, err := e.vocab.Create(e.ctx, team, domain.KindDuty, "X"); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team creates a duty: %v", err)
	}
	if _, err := e.vocab.Create(e.ctx, editor, domain.KindSingingPart, "X"); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor creates a part: %v", err)
	}
	if v, err := e.vocab.Create(e.ctx, planner, domain.KindDuty, "Penerima Tamu"); err != nil || !v.Actions.Edit || v.Entry.Position != 7 {
		t.Errorf("planner creates a duty: %+v %v", v, err)
	}

	// Templates and services: templates.edit or liturgy.edit read; only templates.edit changes.
	if _, err := e.tpls.List(e.ctx, team); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team lists templates: %v", err)
	}
	if _, err := e.svcs.List(e.ctx, team); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team lists services: %v", err)
	}
	list, err := e.tpls.List(e.ctx, editor)
	if err != nil || len(list) != 1 || list[0].Actions.Edit || list[0].Row.ItemCount != 7 {
		t.Fatalf("editor lists templates: %+v %v", list, err)
	}
	if _, err := e.tpls.Get(e.ctx, editor, list[0].Row.ID); err != nil {
		t.Errorf("editor reads a template: %v", err)
	}
	if _, err := e.tpls.Create(e.ctx, editor, app.TemplateInput{Name: "X"}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor creates a template: %v", err)
	}
	if _, err := e.svcs.Create(e.ctx, editor, app.ServiceInput{Name: "X", Times: []domain.ServiceTime{{Weekday: 7, Time: "07:00"}}}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor creates a service: %v", err)
	}
	if err := e.tpls.Delete(e.ctx, editor, list[0].Row.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor deletes a template: %v", err)
	}
	if _, err := e.tpls.List(e.ctx, planner); err != nil {
		t.Errorf("planner lists templates: %v", err)
	}
	// Someone who is not a member of the church gets the same 404 as anywhere (04 §5).
	if _, err := e.vocab.List(e.ctx, &domain.Session{UserID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}, domain.KindDuty); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	if _, err := e.vocab.List(e.ctx, nil, domain.KindDuty); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no session: %v", err)
	}
}

// TC-P-004, IT-P-003: names, order and limits of the two lists.
func TestNameLists(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		p := e.planner()
		for _, kind := range []domain.ListKind{domain.KindDuty, domain.KindSingingPart} {
			// Start from nothing: delete the seeded entries.
			cur, err := e.vocab.List(e.ctx, p, kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, en := range cur {
				if err := e.vocab.Delete(e.ctx, p, kind, en.Entry.ID); err != nil {
					t.Fatal(err)
				}
			}
			a, err := e.vocab.Create(e.ctx, p, kind, "  Pemandu ")
			if err != nil || a.Entry.Name != "Pemandu" || a.Entry.Position != 0 {
				t.Fatalf("%s create: %+v %v", kind, a, err)
			}
			b, _ := e.vocab.Create(e.ctx, p, kind, "Jemaat")
			c, _ := e.vocab.Create(e.ctx, p, kind, "Wanita")

			// A name equal after folding is taken.
			for _, dup := range []string{"pemandu", "PEMANDU", " Pemandu  "} {
				var taken *app.NameTakenError
				if _, err := e.vocab.Create(e.ctx, p, kind, dup); !errors.As(err, &taken) || taken.Reason != string(kind) {
					t.Errorf("%s duplicate %q: %v", kind, dup, err)
				}
			}
			// Renaming to an own folded name is fine (a change of case), to another's is not.
			if v, err := e.vocab.Rename(e.ctx, p, kind, a.Entry.ID, "PEMANDU"); err != nil || v.Entry.Name != "PEMANDU" {
				t.Errorf("%s change of case: %+v %v", kind, v, err)
			}
			var taken *app.NameTakenError
			if _, err := e.vocab.Rename(e.ctx, p, kind, b.Entry.ID, "pemandu"); !errors.As(err, &taken) {
				t.Errorf("%s rename onto another: %v", kind, err)
			}
			if _, err := e.vocab.Rename(e.ctx, p, kind, "01ARZ3NDEKTSV4RRFFQ69G5FAV", "X"); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("%s rename unknown: %v", kind, err)
			}
			if _, err := e.vocab.Create(e.ctx, p, kind, strings.Repeat("x", kind.MaxLen()+1)); invalidField(err) != "name" {
				t.Errorf("%s too long: %v", kind, err)
			}

			// Reorder: exactly the current IDs, once each.
			ids := []string{c.Entry.ID, a.Entry.ID, b.Entry.ID}
			got, err := e.vocab.Reorder(e.ctx, p, kind, ids)
			if err != nil || !slices.Equal(names(got), []string{"Wanita", "PEMANDU", "Jemaat"}) || got[2].Entry.Position != 2 {
				t.Errorf("%s reorder: %v %v", kind, names(got), err)
			}
			for name, bad := range map[string][]string{
				"missing": {a.Entry.ID, b.Entry.ID}, "extra": append(slices.Clone(ids), "01ARZ3NDEKTSV4RRFFQ69G5FAV"),
				"duplicate": {a.Entry.ID, a.Entry.ID, b.Entry.ID}, "foreign": {a.Entry.ID, b.Entry.ID, "01ARZ3NDEKTSV4RRFFQ69G5FAV"}, "empty": {},
			} {
				if _, err := e.vocab.Reorder(e.ctx, p, kind, bad); !errors.Is(err, app.ErrVersionConflict) {
					t.Errorf("%s reorder %s: %v", kind, name, err)
				}
			}
			// Deleting closes the gap.
			if err := e.vocab.Delete(e.ctx, p, kind, c.Entry.ID); err != nil {
				t.Fatal(err)
			}
			left, _ := e.vocab.List(e.ctx, p, kind)
			if !slices.Equal(names(left), []string{"PEMANDU", "Jemaat"}) || left[0].Entry.Position != 0 || left[1].Entry.Position != 1 {
				t.Errorf("%s after delete: %+v", kind, left)
			}
			// A used entry cannot be deleted.
			if kind == domain.KindDuty {
				e.planUse.duty = true
			} else {
				e.planUse.part = true
			}
			want := app.ErrDutyInUse
			if kind == domain.KindSingingPart {
				want = app.ErrSingingPartInUse
			}
			if err := e.vocab.Delete(e.ctx, p, kind, a.Entry.ID); !errors.Is(err, want) {
				t.Errorf("%s delete in use: %v", kind, err)
			}
			e.planUse.duty, e.planUse.part = false, false

			// The limit: fill up to it, the next one reports it with max and used.
			for i := len(left); i < kind.Limit(); i++ {
				if _, err := e.vocab.Create(e.ctx, p, kind, fmt.Sprintf("Name %d", i)); err != nil {
					t.Fatalf("%s fill %d: %v", kind, i, err)
				}
			}
			_, err = e.vocab.Create(e.ctx, p, kind, "One too many")
			if r, max, used := limitReason(err); r != domain.ReasonLimit || max != kind.Limit() || used != kind.Limit() {
				t.Errorf("%s over the limit: %v", kind, err)
			}
		}
	})
}

// IT-P-003: of two simultaneous creates for the last place, exactly one succeeds.
func TestNameListLimitRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		p := e.planner()
		cur, _ := e.vocab.List(e.ctx, p, domain.KindSingingPart) // 6 seeded
		for i := len(cur); i < domain.MaxSingingParts-1; i++ {
			if _, err := e.vocab.Create(e.ctx, p, domain.KindSingingPart, fmt.Sprintf("Part %d", i)); err != nil {
				t.Fatal(err)
			}
		}
		errs := race(4, func(i int) error {
			_, err := e.vocab.Create(e.ctx, p, domain.KindSingingPart, fmt.Sprintf("Racer %d", i))
			return err
		})
		ok, others := succeeded(errs)
		if ok != 1 || len(others) != 3 {
			t.Fatalf("last place: %d succeeded, others %v", ok, others)
		}
		for _, err := range others {
			if r, _, _ := limitReason(err); r != domain.ReasonLimit {
				t.Errorf("loser: %v", err)
			}
		}
		all, _ := e.vocab.List(e.ctx, p, domain.KindSingingPart)
		if len(all) != domain.MaxSingingParts {
			t.Errorf("%d parts", len(all))
		}
		for i, en := range all {
			if en.Entry.Position != i {
				t.Errorf("position %d at index %d", en.Entry.Position, i)
			}
		}
	})
}

func item(title string, typ domain.ItemType, text string, duty domain.DutyID) domain.TemplateItem {
	return domain.TemplateItem{Title: title, Type: typ, DefaultText: text, DefaultDutyID: duty}
}

// TC-P-002, IT-P-002, IT-P-005: templates and services.
func TestTemplatesAndServices(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		p := e.planner()
		duties, _ := e.vocab.List(e.ctx, p, domain.KindDuty)
		liturgis := domain.DutyID(duties[0].Entry.ID)

		// The seeded template is there, with its duties.
		seeded, err := e.tpls.List(e.ctx, p)
		if err != nil || len(seeded) != 1 || seeded[0].Row.Name != "Ibadah Minggu" || seeded[0].Row.Language != "id" {
			t.Fatalf("seeded: %+v %v", seeded, err)
		}
		st, _ := e.tpls.Get(e.ctx, p, seeded[0].Row.ID)
		if len(st.Template.Items) != 7 || st.Template.Items[0].DefaultDutyID != liturgis || st.Template.Items[1].Type != domain.ItemSong {
			t.Errorf("seeded items: %+v", st.Template.Items)
		}

		// Create; the language defaults to the church's; items keep their order.
		v, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: " Doa Malam ", Items: []domain.TemplateItem{
			item("Panggilan", domain.ItemFreeText, "a\r\nb", liturgis), item("Lagu", domain.ItemSong, "", ""), item("Doa", domain.ItemPrayer, "", "")}})
		if err != nil || v.Template.Name != "Doa Malam" || v.Template.Language != "id" || v.Template.Version != 1 ||
			len(v.Template.Items) != 3 || v.Template.Items[0].DefaultText != "a\nb" {
			t.Fatalf("create: %+v %v", v, err)
		}
		tpl := v.Template.ID
		var taken *app.NameTakenError
		if _, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: "doa  malam"}); !errors.As(err, &taken) || taken.Reason != "template" {
			t.Errorf("duplicate name: %v", err)
		}
		if _, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: "Lain", Items: []domain.TemplateItem{item("X", domain.ItemPrayer, "", "01ARZ3NDEKTSV4RRFFQ69G5FAV")}}); invalidField(err) != "items.0.default_duty_id" {
			t.Errorf("unknown duty: %v", err)
		}
		if _, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: "Lain", Language: "fr"}); invalidField(err) != "language" {
			t.Errorf("language: %v", err)
		}

		// Update replaces the items as a whole and checks the version.
		newItems := []domain.TemplateItem{item("Satu", domain.ItemSermon, "", "")}
		u, err := e.tpls.Update(e.ctx, p, tpl, app.TemplateChange{Version: 1, Items: &newItems})
		if err != nil || u.Template.Version != 2 || len(u.Template.Items) != 1 || u.Template.Name != "Doa Malam" {
			t.Fatalf("update: %+v %v", u, err)
		}
		if _, err := e.tpls.Update(e.ctx, p, tpl, app.TemplateChange{Version: 1, Items: &newItems}); !errors.Is(err, app.ErrVersionConflict) {
			t.Errorf("stale version: %v", err)
		}
		name := "Ibadah Minggu"
		if _, err := e.tpls.Update(e.ctx, p, tpl, app.TemplateChange{Version: 2, Name: &name}); !errors.As(err, &taken) {
			t.Errorf("rename onto the seeded one: %v", err)
		}
		if got, _ := e.tpls.Get(e.ctx, p, tpl); got.Template.Version != 2 || got.Template.Name != "Doa Malam" {
			t.Errorf("a refused update changed the template: %+v", got.Template)
		}
		many := make([]domain.TemplateItem, 61)
		for i := range many {
			many[i] = item("x", domain.ItemOther, "", "")
		}
		if _, err := e.tpls.Update(e.ctx, p, tpl, app.TemplateChange{Version: 2, Items: &many}); func() bool { r, mx, us := limitReason(err); return r != domain.ReasonLimit || mx != 60 || us != 61 }() {
			t.Errorf("61 items: %v", err)
		}

		// Services: times sorted, template checked, version checked.
		sv, err := e.svcs.Create(e.ctx, p, app.ServiceInput{Name: "Ibadah Umum", DefaultTemplateID: tpl,
			Times: []domain.ServiceTime{{Weekday: 7, Time: "09:00"}, {Weekday: 7, Time: "07:00"}, {Weekday: 3, Time: "19:00"}}})
		if err != nil || sv.DefaultTemplateName != "Doa Malam" || sv.Service.Language != "id" || len(sv.Service.Times) != 3 ||
			sv.Service.Times[0].Weekday != 3 || sv.Service.Times[1].Time != "07:00" {
			t.Fatalf("service: %+v %v", sv, err)
		}
		svc := sv.Service.ID
		if _, err := e.svcs.Create(e.ctx, p, app.ServiceInput{Name: "Lain", DefaultTemplateID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			Times: []domain.ServiceTime{{Weekday: 1, Time: "07:00"}}}); invalidField(err) != "default_template_id" {
			t.Errorf("unknown template: %v", err)
		}
		if _, err := e.svcs.Create(e.ctx, p, app.ServiceInput{Name: "ibadah umum", Times: []domain.ServiceTime{{Weekday: 1, Time: "07:00"}}}); !errors.As(err, &taken) || taken.Reason != "service" {
			t.Errorf("duplicate service: %v", err)
		}
		// Omitted times keep the list; an empty list is refused with "required".
		nm := "Ibadah Raya"
		up, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 1, Name: &nm})
		if err != nil || up.Service.Version != 2 || len(up.Service.Times) != 3 || up.Service.Name != "Ibadah Raya" {
			t.Fatalf("rename: %+v %v", up, err)
		}
		none := []domain.ServiceTime{}
		if _, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 2, Times: &none}); invalidField(err) != "times" {
			t.Errorf("empty times: %v", err)
		}
		var in *domain.InvalidInputError
		if _, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 2, Times: &none}); !errors.As(err, &in) || in.Reason != domain.ReasonRequired {
			t.Errorf("empty times reason: %v", err)
		}
		if _, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 1, Name: &nm}); !errors.Is(err, app.ErrVersionConflict) {
			t.Errorf("stale service: %v", err)
		}
		clear := domain.TemplateID("")
		cl, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 2, DefaultTemplateID: &clear})
		if err != nil || cl.Service.DefaultTemplateID != "" || cl.DefaultTemplateName != "" {
			t.Errorf("clear template: %+v %v", cl, err)
		}
		if _, err := e.svcs.Update(e.ctx, p, svc, app.ServiceChange{Version: 3, DefaultTemplateID: &tpl}); err != nil {
			t.Errorf("set template again: %v", err)
		}

		// A template that is a service default can't be deleted; its services are named.
		var inUse *app.TemplateInUseError
		if err := e.tpls.Delete(e.ctx, p, tpl); !errors.As(err, &inUse) || len(inUse.ServiceIDs) != 1 || inUse.ServiceIDs[0] != svc {
			t.Errorf("template in use: %v", err)
		}
		// Deleting the service frees it; a deleted duty is cleared from template items.
		if err := e.svcs.Delete(e.ctx, p, svc); err != nil {
			t.Errorf("delete service: %v", err)
		}
		if err := e.svcs.Delete(e.ctx, p, svc); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("delete service twice: %v", err)
		}
		if err := e.vocab.Delete(e.ctx, p, domain.KindDuty, string(liturgis)); err != nil {
			t.Fatal(err)
		}
		st, _ = e.tpls.Get(e.ctx, p, seeded[0].Row.ID)
		if st.Template.Items[0].DefaultDutyID != "" || st.Template.Items[1].DefaultDutyID == "" {
			t.Errorf("the deleted duty is cleared from template items only: %+v", st.Template.Items[:2])
		}
		if err := e.tpls.Delete(e.ctx, p, tpl); err != nil {
			t.Errorf("delete template: %v", err)
		}
		if _, err := e.tpls.Get(e.ctx, p, tpl); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("deleted template: %v", err)
		}
	})
}

// IT-P-003: the limits of templates (50) and services (30).
func TestTemplateAndServiceLimits(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	p := e.planner()
	for i := 1; i < domain.MaxTemplates; i++ { // one is seeded
		if _, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: fmt.Sprintf("T %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := e.tpls.Create(e.ctx, p, app.TemplateInput{Name: "One too many"})
	if r, mx, us := limitReason(err); r != domain.ReasonLimit || mx != 50 || us != 50 {
		t.Errorf("templates: %v", err)
	}
	times := []domain.ServiceTime{{Weekday: 7, Time: "07:00"}}
	for i := 0; i < domain.MaxServices; i++ {
		if _, err := e.svcs.Create(e.ctx, p, app.ServiceInput{Name: fmt.Sprintf("S %d", i), Times: times}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svcs.Create(e.ctx, p, app.ServiceInput{Name: "One too many", Times: times})
	if r, mx, us := limitReason(err); r != domain.ReasonLimit || mx != 30 || us != 30 {
		t.Errorf("services: %v", err)
	}
}

// TC-P-005, IT-P-004, IT-P-006: seeding at setup and at start-up.
func TestSeeding(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		counts := func() (duties, parts, templates, marks int) {
			e.write(func(s app.Store) error {
				cs, err := s.ForChurch(ctx, e.church)
				if err != nil {
					return err
				}
				duties, _ = cs.Duties().Count(ctx)
				parts, _ = cs.SingingParts().Count(ctx)
				templates, _ = cs.Templates().Count(ctx)
				if ok, _ := cs.Seeds().Applied(ctx, domain.SeedKey); ok {
					marks = 1
				}
				return nil
			})
			return
		}
		// Setup seeded the church in its content language (Indonesian).
		if d, p, tp, m := counts(); d != 7 || p != 6 || tp != 1 || m != 1 {
			t.Fatalf("after setup: %d %d %d %d", d, p, tp, m)
		}
		// Start-up seeding does nothing for a church that has the marker, even
		// if it deleted a default meanwhile.
		p := e.planner()
		duties, _ := e.vocab.List(e.ctx, p, domain.KindDuty)
		if err := e.vocab.Delete(e.ctx, p, domain.KindDuty, duties[2].Entry.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.seed.Run(ctx); err != nil {
			t.Fatal(err)
		}
		if d, _, _, _ := counts(); d != 6 {
			t.Errorf("a deleted default came back: %d duties", d)
		}
		// A church from before step 3 has no marker and no rows: start-up seeds it.
		old := e.oldChurch("id")
		if err := e.seed.Run(ctx); err != nil {
			t.Fatal(err)
		}
		if d, pa, tp, m := e.countsOf(old); d != 7 || pa != 6 || tp != 1 || m != 1 {
			t.Errorf("old church after start-up: %d %d %d %d", d, pa, tp, m)
		}
		// Running again changes nothing (the marker), also when two servers start together.
		errs := race(3, func(int) error { return e.seed.Run(ctx) })
		if ok, others := succeeded(errs); ok != 3 {
			t.Errorf("seed race: %v", others)
		}
		if d, pa, tp, m := e.countsOf(old); d != 7 || pa != 6 || tp != 1 || m != 1 {
			t.Errorf("old church after repeated start-ups: %d %d %d %d", d, pa, tp, m)
		}

		// IT-P-006: a seed that fails half-way leaves no rows and no marker.
		blocked := e.oldChurch("id")
		e.write(func(s app.Store) error {
			cs, err := s.ForChurch(ctx, blocked)
			if err != nil {
				return err
			}
			return cs.Duties().Create(ctx, domain.NameEntry{ID: e.ids.NewID(), Name: "Pemusik", NameKey: "pemusik", CreatedAt: e.clock.Now()})
		})
		if err := e.seed.Run(ctx); err == nil || !strings.Contains(err.Error(), string(blocked)) {
			t.Fatalf("a failing seed is reported with its church: %v", err)
		}
		if d, pa, tp, m := e.countsOf(blocked); d != 1 || pa != 0 || tp != 0 || m != 0 {
			t.Errorf("failed seed left rows: %d duties %d parts %d templates marker %d", d, pa, tp, m)
		}
		if d, _, _, _ := e.countsOf(old); d != 7 {
			t.Errorf("the failing church stopped the others: %d", d)
		}
		// After the operator removes the clash, the next start-up seeds it fully.
		e.write(func(s app.Store) error {
			cs, err := s.ForChurch(ctx, blocked)
			if err != nil {
				return err
			}
			list, err := cs.Duties().List(ctx)
			if err != nil {
				return err
			}
			return cs.Duties().Delete(ctx, list[0].ID)
		})
		if err := e.seed.Run(ctx); err != nil {
			t.Fatal(err)
		}
		if d, _, tp, m := e.countsOf(blocked); d != 7 || tp != 1 || m != 1 {
			t.Errorf("recovered: %d duties %d templates marker %d", d, tp, m)
		}
	})
}

// oldChurch stores a church that has no seed marker and no rows, as one made
// before step 3 would be.
func (e cenv) oldChurch(lang string) domain.ChurchID {
	e.t.Helper()
	id := domain.ChurchID(e.ids.NewID())
	e.write(func(s app.Store) error {
		tr, err := s.Translations().ByCode(ctx, "TB")
		if err != nil {
			return err
		}
		now := e.clock.Now()
		return s.Churches().Create(ctx, domain.Church{ID: id, Name: "Old " + string(id), DefaultUILanguage: "en", DefaultLanguage: lang,
			DefaultTranslationID: tr.ID, TimeZone: "Asia/Jakarta", Settings: domain.ChurchSettings{KeyDisplay: "do"}, CreatedAt: now, UpdatedAt: now})
	})
	return id
}

func (e cenv) countsOf(church domain.ChurchID) (duties, parts, templates, marks int) {
	e.write(func(s app.Store) error {
		cs, err := s.ForChurch(ctx, church)
		if err != nil {
			return err
		}
		duties, _ = cs.Duties().Count(ctx)
		parts, _ = cs.SingingParts().Count(ctx)
		templates, _ = cs.Templates().Count(ctx)
		if ok, _ := cs.Seeds().Applied(ctx, domain.SeedKey); ok {
			marks = 1
		}
		return nil
	})
	return
}

// TC-P-005: each content language is seeded in its own words.
func TestSeededLanguages(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	for lang, want := range map[string][2]string{"en": {"Liturgist", "Sunday service"}, "zh-Hans": {"主礼", "主日崇拜"}, "zh-Hant": {"主禮", "主日崇拜"}} {
		id := e.oldChurch(lang)
		if err := e.seed.Run(ctx); err != nil {
			t.Fatal(err)
		}
		e.write(func(s app.Store) error {
			cs, err := s.ForChurch(ctx, id)
			if err != nil {
				return err
			}
			duties, _ := cs.Duties().List(ctx)
			tpls, _ := cs.Templates().List(ctx)
			if len(duties) != 7 || duties[0].Name != want[0] || len(tpls) != 1 || tpls[0].Name != want[1] || tpls[0].Language != lang || tpls[0].ItemCount != 7 {
				t.Errorf("%s: %v %+v", lang, duties, tpls)
			}
			positions := make([]int, len(duties))
			for i, d := range duties {
				positions[i] = d.Position
			}
			if !slices.Equal(positions, []int{0, 1, 2, 3, 4, 5, 6}) {
				t.Errorf("%s: positions %v", lang, positions)
			}
			return nil
		})
	}
}
