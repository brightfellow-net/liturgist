// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func testEntry(eid, name string, pos int, at time.Time) domain.NameEntry {
	return domain.NameEntry{ID: id(eid), Name: name, NameKey: domain.Fold(name), Position: pos, CreatedAt: at}
}

func testTemplate(tid, name string, at time.Time, dutyID string) domain.Template {
	t := domain.Template{ID: domain.TemplateID(id(tid)), Name: name, NameKey: domain.Fold(name), Language: "id", Version: 1,
		CreatedAt: at, UpdatedAt: at, Items: []domain.TemplateItem{
			{ID: id(tid + "I1"), Title: "Votum", Type: domain.ItemFreeText, DefaultText: "text"},
			{ID: id(tid + "I2"), Title: "Pujian", Type: domain.ItemSong},
		}}
	if dutyID != "" {
		t.Items[0].DefaultDutyID = domain.DutyID(id(dutyID))
	}
	return t
}

func testService(sid, name string, at time.Time, tpl string) domain.Service {
	s := domain.Service{ID: domain.ServiceID(id(sid)), Name: name, NameKey: domain.Fold(name), Language: "id", Version: 1,
		CreatedAt: at, UpdatedAt: at, Times: []domain.ServiceTime{
			{ID: id(sid + "T1"), Weekday: 7, Time: "07:00"}, {ID: id(sid + "T2"), Weekday: 7, Time: "09:00"}}}
	if tpl != "" {
		s.DefaultTemplateID = domain.TemplateID(id(tpl))
	}
	return s
}

// seedPlanning gives churches A and B their planning rows: A has one duty,
// one part, one template with a default duty and one service; B has two duties,
// a part, a template, a service and a seed marker. The IDs end in A or B.
func seedPlanning(a, b app.ChurchStore, now time.Time) error {
	steps := []error{
		a.Duties().Create(ctx, testEntry("DUA1", "Liturgis", 0, now)),
		a.SingingParts().Create(ctx, testEntry("PTA1", "Semua", 0, now)),
		a.Templates().Create(ctx, testTemplate("TPA1", "Ibadah", now, "DUA1")),
		a.Services().Create(ctx, testService("SVA1", "Ibadah Umum", now, "TPA1")),
		b.Duties().Create(ctx, testEntry("DUB1", "Liturgis", 0, now)),
		b.Duties().Create(ctx, testEntry("DUB2", "Pemusik", 1, now)),
		b.SingingParts().Create(ctx, testEntry("PTB1", "Semua", 0, now)),
		b.Templates().Create(ctx, testTemplate("TPB1", "Ibadah", now, "DUB1")),
		b.Services().Create(ctx, testService("SVB1", "Ibadah Umum", now, "TPB1")),
		b.Seeds().Mark(ctx, domain.SeedKey, now),
	}
	return errors.Join(steps...)
}

// planningHarness are the IT-P-007 entries of the planning repositories: each
// method called through church A sees and changes only A's rows.
func planningHarness(errRollback error, notFound func(error) error, unchanged func(bool, error) error) map[string]func(app.ChurchStore, time.Time) error {
	h := map[string]func(app.ChurchStore, time.Time) error{}
	lists := map[string]struct {
		repo     func(app.ChurchStore) app.NameListRepo
		own, oth string
	}{
		"Duties":       {func(cs app.ChurchStore) app.NameListRepo { return cs.Duties() }, "DUA1", "DUB1"},
		"SingingParts": {func(cs app.ChurchStore) app.NameListRepo { return cs.SingingParts() }, "PTA1", "PTB1"},
	}
	for name, l := range lists {
		h[name+".List"] = func(cs app.ChurchStore, _ time.Time) error {
			got, err := l.repo(cs).List(ctx)
			if err != nil || len(got) != 1 || got[0].ID != id(l.own) {
				return fmt.Errorf("list: %+v %w", got, err)
			}
			return nil
		}
		h[name+".ByID"] = func(cs app.ChurchStore, _ time.Time) error {
			if _, err := l.repo(cs).ByID(ctx, id(l.own)); err != nil {
				return err
			}
			_, err := l.repo(cs).ByID(ctx, id(l.oth))
			return notFound(err)
		}
		h[name+".Count"] = func(cs app.ChurchStore, _ time.Time) error {
			n, err := l.repo(cs).Count(ctx)
			if err != nil || n != 1 {
				return fmt.Errorf("count %d: %w", n, err)
			}
			return nil
		}
		h[name+".Create"] = func(cs app.ChurchStore, now time.Time) error {
			// "Pemusik" exists in church B only (duties) and is free here; the row is A's.
			if err := l.repo(cs).Create(ctx, testEntry("NEWX", "Pemusik", 1, now)); err != nil {
				return err
			}
			if got, err := l.repo(cs).ByID(ctx, id("NEWX")); err != nil || got.Position != 1 {
				return fmt.Errorf("created entry not visible to A: %w", err)
			}
			return errRollback
		}
		h[name+".Rename"] = func(cs app.ChurchStore, _ time.Time) error {
			return notFound(l.repo(cs).Rename(ctx, id(l.oth), "Hacked", "hacked"))
		}
		h[name+".SetOrder"] = func(cs app.ChurchStore, _ time.Time) error {
			return notFound(l.repo(cs).SetOrder(ctx, []string{id(l.oth)}))
		}
		h[name+".Delete"] = func(cs app.ChurchStore, _ time.Time) error {
			return notFound(l.repo(cs).Delete(ctx, id(l.oth)))
		}
	}
	h["Templates.List"] = func(cs app.ChurchStore, _ time.Time) error {
		rows, err := cs.Templates().List(ctx)
		if err != nil || len(rows) != 1 || rows[0].ID != domain.TemplateID(id("TPA1")) || rows[0].ItemCount != 2 {
			return fmt.Errorf("list: %+v %w", rows, err)
		}
		return nil
	}
	h["Templates.ByID"] = func(cs app.ChurchStore, _ time.Time) error {
		got, err := cs.Templates().ByID(ctx, domain.TemplateID(id("TPA1")))
		if err != nil || len(got.Items) != 2 || got.Items[0].DefaultDutyID != domain.DutyID(id("DUA1")) {
			return fmt.Errorf("own template: %+v %w", got, err)
		}
		_, err = cs.Templates().ByID(ctx, domain.TemplateID(id("TPB1")))
		return notFound(err)
	}
	h["Templates.Count"] = func(cs app.ChurchStore, _ time.Time) error {
		n, err := cs.Templates().Count(ctx)
		if err != nil || n != 1 {
			return fmt.Errorf("count %d: %w", n, err)
		}
		return nil
	}
	h["Templates.Create"] = func(cs app.ChurchStore, now time.Time) error {
		// A template that names church B's duty is refused by the composite foreign key.
		err := cs.Templates().Create(ctx, testTemplate("TPX", "Lain", now, "DUB1"))
		if !errors.Is(err, app.ErrReferenced) {
			return fmt.Errorf("church B's duty through A: %w", err)
		}
		return errRollback
	}
	h["Templates.Update"] = func(cs app.ChurchStore, now time.Time) error {
		t := testTemplate("TPB1", "Hacked", now, "")
		t.Version = 2
		return unchanged(cs.Templates().Update(ctx, t, 1))
	}
	h["Templates.Delete"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.Templates().Delete(ctx, domain.TemplateID(id("TPB1"))))
	}
	h["Templates.ServicesUsing"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Templates().ServicesUsing(ctx, domain.TemplateID(id("TPA1")))
		if err != nil || len(own) != 1 || own[0] != domain.ServiceID(id("SVA1")) {
			return fmt.Errorf("own: %v %w", own, err)
		}
		other, err := cs.Templates().ServicesUsing(ctx, domain.TemplateID(id("TPB1")))
		if err != nil || len(other) != 0 {
			return fmt.Errorf("church B's services: %v %w", other, err)
		}
		return nil
	}
	h["Services.List"] = func(cs app.ChurchStore, _ time.Time) error {
		got, err := cs.Services().List(ctx)
		if err != nil || len(got) != 1 || got[0].ID != domain.ServiceID(id("SVA1")) || len(got[0].Times) != 2 {
			return fmt.Errorf("list: %+v %w", got, err)
		}
		return nil
	}
	h["Services.ByID"] = func(cs app.ChurchStore, _ time.Time) error {
		got, err := cs.Services().ByID(ctx, domain.ServiceID(id("SVA1")))
		if err != nil || len(got.Times) != 2 {
			return fmt.Errorf("own service: %+v %w", got, err)
		}
		_, err = cs.Services().ByID(ctx, domain.ServiceID(id("SVB1")))
		return notFound(err)
	}
	h["Services.Count"] = func(cs app.ChurchStore, _ time.Time) error {
		n, err := cs.Services().Count(ctx)
		if err != nil || n != 1 {
			return fmt.Errorf("count %d: %w", n, err)
		}
		return nil
	}
	h["Services.Create"] = func(cs app.ChurchStore, now time.Time) error {
		err := cs.Services().Create(ctx, testService("SVX", "Lain", now, "TPB1"))
		if !errors.Is(err, app.ErrReferenced) {
			return fmt.Errorf("church B's template through A: %w", err)
		}
		return errRollback
	}
	h["Services.Update"] = func(cs app.ChurchStore, now time.Time) error {
		s := testService("SVB1", "Hacked", now, "")
		s.Version = 2
		return unchanged(cs.Services().Update(ctx, s, 1))
	}
	h["Services.Delete"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.Services().Delete(ctx, domain.ServiceID(id("SVB1"))))
	}
	h["Seeds.Applied"] = func(cs app.ChurchStore, _ time.Time) error {
		ok, err := cs.Seeds().Applied(ctx, domain.SeedKey) // only church B has the marker
		if err != nil || ok {
			return fmt.Errorf("church B's marker seen by A: %v %w", ok, err)
		}
		return nil
	}
	h["Seeds.Mark"] = func(cs app.ChurchStore, now time.Time) error {
		if err := cs.Seeds().Mark(ctx, domain.SeedKey, now); err != nil {
			return err
		}
		if ok, err := cs.Seeds().Applied(ctx, domain.SeedKey); err != nil || !ok {
			return fmt.Errorf("own marker: %v %w", ok, err)
		}
		return errRollback
	}
	return h
}

const (
	listInsert = `INSERT INTO %s (id, church_id, name, name_key, position, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	tplInsert  = `INSERT INTO templates (id, church_id, name, name_key, language, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	itemInsert = `INSERT INTO template_items (id, church_id, template_id, position, title, item_type, default_text, default_duty_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	svcInsert = `INSERT INTO services (id, church_id, name, name_key, language, default_template_id, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	timeInsert = `INSERT INTO service_times (id, church_id, service_id, weekday, time) VALUES (?, ?, ?, ?, ?)`
	seedInsert = `INSERT INTO church_seeds (church_id, seed_key, applied_at) VALUES (?, ?, ?)`
)

func (f fixture) duty(did, church, name string, pos int) error {
	return f.exec(fmt.Sprintf(listInsert, "duties"), id(did), id(church), name, domain.Fold(name), pos, f.ts(0))
}

func (f fixture) part(pid, church, name string, pos int) error {
	return f.exec(fmt.Sprintf(listInsert, "singing_parts"), id(pid), id(church), name, domain.Fold(name), pos, f.ts(0))
}

func (f fixture) template(tid, church, name, lang string, version int) error {
	return f.exec(tplInsert, id(tid), id(church), name, domain.Fold(name), lang, version, f.ts(0), f.ts(0))
}

func (f fixture) item(iid, church, tpl string, pos int, typ, text string, duty any) error {
	return f.exec(itemInsert, id(iid), id(church), id(tpl), pos, "Title", typ, text, duty)
}

func (f fixture) service(sid, church, name string, tpl any, version int) error {
	return f.exec(svcInsert, id(sid), id(church), name, domain.Fold(name), "id", tpl, version, f.ts(0), f.ts(0))
}

func (f fixture) svcTime(tid, church, svc string, weekday int, clock string) error {
	return f.exec(timeInsert, id(tid), id(church), id(svc), weekday, clock)
}

// IT-P-009 for the planning tables: the database rejects rows the application never writes.
func TestPlanningConstraints(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		f := base(t, db)
		f.must(fmt.Sprintf(listInsert, "duties"), id("DU1"), id("CHA"), "Liturgis", "liturgis", 0, f.ts(0))
		f.must(tplInsert, id("TP1"), id("CHA"), "Ibadah", "ibadah", "id", 1, f.ts(0), f.ts(0))
		f.must(svcInsert, id("SV1"), id("CHA"), "Umum", "umum", "id", nil, 1, f.ts(0), f.ts(0))

		invalid := map[string]func() error{
			"duty position -1": func() error { return f.duty("DU2", "CHA", "Pemusik", -1) },
			"short duty ID": func() error {
				return f.exec(fmt.Sprintf(listInsert, "duties"), "short", id("CHA"), "x", "x", 0, f.ts(0))
			},
			"template language":    func() error { return f.template("TP2", "CHA", "Lain", "fr", 1) },
			"template version 0":   func() error { return f.template("TP2", "CHA", "Lain", "id", 0) },
			"item type":            func() error { return f.item("IT1", "CHA", "TP1", 0, "dance", "", nil) },
			"item position 60":     func() error { return f.item("IT1", "CHA", "TP1", 60, "prayer", "", nil) },
			"item position -1":     func() error { return f.item("IT1", "CHA", "TP1", -1, "prayer", "", nil) },
			"text on a song item":  func() error { return f.item("IT1", "CHA", "TP1", 0, "song", "words", nil) },
			"text on reading item": func() error { return f.item("IT1", "CHA", "TP1", 0, "reading", "words", nil) },
			"service language": func() error {
				return f.exec(svcInsert, id("SV2"), id("CHA"), "Lain", "lain", "fr", nil, 1, f.ts(0), f.ts(0))
			},
			"service version 0": func() error { return f.service("SV2", "CHA", "Lain", nil, 0) },
			"weekday 0":         func() error { return f.svcTime("TM1", "CHA", "SV1", 0, "07:00") },
			"weekday 8":         func() error { return f.svcTime("TM1", "CHA", "SV1", 8, "07:00") },
			"time 7:00":         func() error { return f.svcTime("TM1", "CHA", "SV1", 7, "7:00") },
			"time 07-00":        func() error { return f.svcTime("TM1", "CHA", "SV1", 7, "07-00") },
			"time with seconds": func() error { return f.svcTime("TM1", "CHA", "SV1", 7, "07:00:00") },
			"empty seed key":    func() error { return f.exec(seedInsert, id("CHA"), "", f.ts(0)) },
		}
		for name, write := range invalid {
			if err := write(); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: want ErrInvalid, got %v", name, err)
			}
		}
		// Rows of another church can't be referenced (composite foreign keys).
		f.must(fmt.Sprintf(listInsert, "duties"), id("DUB"), id("CHB"), "Liturgis", "liturgis", 0, f.ts(0))
		f.must(tplInsert, id("TPB"), id("CHB"), "Ibadah", "ibadah", "id", 1, f.ts(0), f.ts(0))
		referenced := map[string]func() error{
			"item names another church's duty":     func() error { return f.item("IT1", "CHA", "TP1", 0, "prayer", "", id("DUB")) },
			"item in another church's template":    func() error { return f.item("IT1", "CHA", "TPB", 0, "prayer", "", nil) },
			"service names another church's tmpl":  func() error { return f.service("SV2", "CHA", "Lain", id("TPB"), 1) },
			"time for another church's service":    func() error { return f.svcTime("TM1", "CHB", "SV1", 7, "07:00") },
			"unknown church":                       func() error { return f.duty("DU3", "NOPE", "x", 0) },
			"seed marker of an unknown church":     func() error { return f.exec(seedInsert, id("NOPE"), "step3", f.ts(0)) },
			"item names a duty that doesn't exist": func() error { return f.item("IT1", "CHA", "TP1", 0, "prayer", "", id("NODUTY")) },
		}
		for name, write := range referenced {
			if err := write(); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("%s: want ErrReferenced, got %v", name, err)
			}
		}
		// The same name in another church, and valid rows, are fine.
		f.must(`INSERT INTO template_items (id, church_id, template_id, position, title, item_type, default_text, default_duty_id)
			VALUES (?, ?, ?, 0, 'Doa', 'prayer', 'x', ?)`, id("IT0"), id("CHA"), id("TP1"), id("DU1"))
		f.must(timeInsert, id("TM0"), id("CHA"), id("SV1"), 7, "07:00")

		// A used duty or template can't be deleted in SQL: the keys are RESTRICT, so
		// the application must clear the references first (schema "Clearing references").
		if err := f.exec(`DELETE FROM duties WHERE id = ?`, id("DU1")); !errors.Is(err, app.ErrReferenced) {
			t.Errorf("raw delete of a used duty: %v", err)
		}
		f.must(`UPDATE services SET default_template_id = ? WHERE id = ?`, id("TP1"), id("SV1"))
		if err := f.exec(`DELETE FROM templates WHERE id = ?`, id("TP1")); !errors.Is(err, app.ErrReferenced) {
			t.Errorf("raw delete of a template that is a service default: %v", err)
		}
		// The repository method clears the references and deletes in one transaction.
		mustWrite(t, db, func(s app.Store) error {
			cs, err := s.ForChurch(ctx, domain.ChurchID(id("CHA")))
			if err != nil {
				return err
			}
			if err := cs.Duties().Delete(ctx, id("DU1")); err != nil {
				return err
			}
			tp, err := cs.Templates().ByID(ctx, domain.TemplateID(id("TP1")))
			if err != nil || len(tp.Items) != 1 || tp.Items[0].DefaultDutyID != "" {
				return fmt.Errorf("default duty not cleared: %+v %w", tp.Items, err)
			}
			return nil
		})
		// Deleting a template removes its items; deleting a service its times; a church everything.
		f.must(`UPDATE services SET default_template_id = NULL WHERE id = ?`, id("SV1"))
		f.must(`DELETE FROM templates WHERE id = ?`, id("TP1"))
		if n := count(t, db, "SELECT count(*) FROM template_items WHERE church_id = ?", id("CHA")); n != 0 {
			t.Errorf("%d template items left", n)
		}
		f.must(`DELETE FROM services WHERE id = ?`, id("SV1"))
		if n := count(t, db, "SELECT count(*) FROM service_times WHERE church_id = ?", id("CHA")); n != 0 {
			t.Errorf("%d service times left", n)
		}
		f.must(`DELETE FROM churches WHERE id = ?`, id("CHB"))
		if n := count(t, db, "SELECT count(*) FROM duties WHERE church_id = ?", id("CHB")); n != 0 {
			t.Errorf("%d duties left after deleting their church", n)
		}
	})
}
