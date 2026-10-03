// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func ptr[T any](v T) *T { return &v }

// liturgy reads a liturgy as the admin.
func (e cenv) liturgy(id domain.LiturgyID) app.LiturgyView {
	e.t.Helper()
	v, err := e.liturgies.Get(e.ctx, e.admin, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

// draft makes an empty one-off liturgy.
func (e cenv) draft() domain.LiturgyID {
	e.t.Helper()
	v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", ServiceName: "Test", TemplateID: ptr(domain.TemplateID(""))})
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Liturgy.ID
}

// addItem appends an item of the type to the liturgy.
func (e cenv) addItem(lid domain.LiturgyID, typ domain.ItemType, title string) domain.ItemID {
	e.t.Helper()
	res, err := e.liturgies.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: e.liturgy(lid).Liturgy.Version, Title: title, Type: typ})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Item.Item.ID
}

// useSong adds a song item holding the song with its default sequence.
func (e cenv) useSong(lid domain.LiturgyID, song domain.SongID) domain.ItemID {
	e.t.Helper()
	item := e.addItem(lid, domain.ItemSong, "Song")
	if _, err := e.liturgies.AddSong(e.ctx, e.admin, lid, item, 1, song, nil); err != nil {
		e.t.Fatal(err)
	}
	return item
}

// useSections adds a song item whose sequence holds exactly the sections.
func (e cenv) useSections(lid domain.LiturgyID, song domain.SongID, sections ...domain.SectionID) domain.ItemID {
	e.t.Helper()
	item := e.addItem(lid, domain.ItemSong, "Song")
	in := app.ItemSongInput{SongID: song}
	for _, s := range sections {
		in.Entries = append(in.Entries, app.EntryInput{SectionID: s})
	}
	if _, err := e.liturgies.SetSongs(e.ctx, e.admin, lid, item, 1, []app.ItemSongInput{in}); err != nil {
		e.t.Fatal(err)
	}
	return item
}

// useReading adds a reading item holding the reading.
func (e cenv) useReading(lid domain.LiturgyID, reading domain.ReadingID) domain.ItemID {
	e.t.Helper()
	item := e.addItem(lid, domain.ItemReading, "Reading")
	if _, err := e.liturgies.UpdateItem(e.ctx, e.admin, lid, item, app.ItemChange{Version: 1, ReadingID: &reading}); err != nil {
		e.t.Fatal(err)
	}
	return item
}

// useDuty adds a prayer item done by the duty.
func (e cenv) useDuty(lid domain.LiturgyID, duty string) domain.ItemID {
	e.t.Helper()
	d := domain.DutyID(duty)
	res, err := e.liturgies.AddItem(e.ctx, e.admin, lid, app.ItemInput{LiturgyVersion: e.liturgy(lid).Liturgy.Version,
		Title: "Prayer", Type: domain.ItemPrayer, DutyID: d})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Item.Item.ID
}

// usePart adds a song item with one entry sung by the singing part.
func (e cenv) usePart(lid domain.LiturgyID, part string) domain.ItemID {
	e.t.Helper()
	song := e.newSong("Part song", "id")
	item := e.addItem(lid, domain.ItemSong, "Song")
	in := app.ItemSongInput{SongID: song.Song.ID, Entries: []app.EntryInput{{SectionID: song.Song.Sections[0].ID, SingingPartID: domain.SingingPartID(part)}}}
	if _, err := e.liturgies.SetSongs(e.ctx, e.admin, lid, item, 1, []app.ItemSongInput{in}); err != nil {
		e.t.Fatal(err)
	}
	return item
}

// seededTemplate returns the template every church starts with and its items.
func (e cenv) seededTemplate() (domain.TemplateID, []domain.TemplateItem) {
	e.t.Helper()
	list, err := e.tpls.List(e.ctx, e.admin)
	if err != nil || len(list) != 1 {
		e.t.Fatalf("seeded templates: %v %v", list, err)
	}
	v, err := e.tpls.Get(e.ctx, e.admin, list[0].Row.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Template.ID, v.Template.Items
}

// newService saves a service with its default template.
func (e cenv) newService(name, lang string, tpl domain.TemplateID, times ...domain.ServiceTime) domain.ServiceID {
	e.t.Helper()
	v, err := e.svcs.Create(e.ctx, e.admin, app.ServiceInput{Name: name, Language: lang, DefaultTemplateID: tpl, Times: times})
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Service.ID
}

func sunday(clock string) domain.ServiceTime { return domain.ServiceTime{Weekday: 7, Time: clock} }

func existsID(err error) domain.LiturgyID {
	var x *app.LiturgyExistsError
	if errors.As(err, &x) {
		return x.ID
	}
	return ""
}

// TC-L-002, TC-L-011, IT-L-002: creating a liturgy from a template.
func TestLiturgyCreate(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		tpl, tplItems := e.seededTemplate()
		svc := e.newService("Ibadah Minggu 1", "id", tpl, sunday("07:00"), sunday("09:00"))

		v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", Time: "07:00", ServiceID: svc})
		if err != nil {
			t.Fatal(err)
		}
		l := v.Liturgy
		if l.State != domain.StateDraft || l.Version != 1 || l.Language != "id" || l.ServiceName != "Ibadah Minggu 1" ||
			l.TemplateID != tpl || l.ServiceID != svc || l.EditSeq != 1 {
			t.Fatalf("liturgy: %+v", l)
		}
		if len(v.Items) != len(tplItems) || len(v.Items) != 7 {
			t.Fatalf("%d items, want %d", len(v.Items), len(tplItems))
		}
		for i, it := range v.Items {
			ti := tplItems[i]
			if it.Item.Title != ti.Title || it.Item.Type != ti.Type || it.Item.Text != ti.DefaultText || it.Item.DutyID != ti.DefaultDutyID ||
				it.Item.Position != i || it.Item.Version != 1 {
				t.Errorf("item %d: %+v from %+v", i, it.Item, ti)
			}
		}
		if !v.Actions.Edit || !v.Actions.Delete {
			t.Errorf("actions: %+v", v.Actions)
		}

		// A later change of the template does not reach the liturgy.
		tv, _ := e.tpls.Get(e.ctx, e.admin, tpl)
		renamed := append([]domain.TemplateItem{{Title: "Baru", Type: domain.ItemOther}}, tv.Template.Items...)
		if _, err := e.tpls.Update(e.ctx, e.admin, tpl, app.TemplateChange{Version: tv.Template.Version, Items: &renamed}); err != nil {
			t.Fatal(err)
		}
		if got := e.liturgy(l.ID); len(got.Items) != 7 || got.Items[0].Item.Title != tplItems[0].Title {
			t.Errorf("the liturgy changed with its template: %d items", len(got.Items))
		}

		// One liturgy per slot; another time of the service is another slot.
		_, err = e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", Time: "07:00", ServiceID: svc})
		if existsID(err) != l.ID {
			t.Errorf("same slot: %v", err)
		}
		if _, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", Time: "09:00", ServiceID: svc}); err != nil {
			t.Errorf("same date, other time: %v", err)
		}
		if _, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", Time: "10:30", ServiceID: svc}); err != nil {
			t.Errorf("special start: %v", err)
		}
		for i := range 2 { // one-offs are not limited
			if _, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-11", ServiceName: "Doa Malam"}); err != nil {
				t.Errorf("one-off %d: %v", i, err)
			}
		}

		// Field rules.
		for name, in := range map[string]app.CreateInput{
			"bad date":             {Date: "2026-02-30", ServiceName: "X"},
			"service without time": {Date: "2026-10-18", ServiceID: svc},
			"bad time":             {Date: "2026-10-18", Time: "24:00", ServiceID: svc},
			"one-off without name": {Date: "2026-10-18"},
			"unknown service":      {Date: "2026-10-18", Time: "07:00", ServiceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			"unknown template":     {Date: "2026-10-18", ServiceName: "X", TemplateID: ptr(domain.TemplateID("01ARZ3NDEKTSV4RRFFQ69G5FAV"))},
			"unknown language":     {Date: "2026-10-18", ServiceName: "X", Language: "fr"},
			"language mismatch":    {Date: "2026-10-18", ServiceName: "X", Language: "en", TemplateID: &tpl},
		} {
			if _, err := e.liturgies.Create(e.ctx, e.admin, in); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}

		// TC-L-011: a template of another language than the liturgy.
		en, err := e.tpls.Create(e.ctx, e.admin, app.TemplateInput{Name: "Sunday", Language: "en", Items: []domain.TemplateItem{{Title: "Welcome", Type: domain.ItemOther}}})
		if err != nil {
			t.Fatal(err)
		}
		enSvc := e.newService("English Service", "en", en.Template.ID, sunday("11:00"))
		_, err = e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-18", Time: "11:00", ServiceID: enSvc, Language: "id"})
		if in := new(domain.InvalidInputError); !errors.As(err, &in) || in.Field != "template_id" || in.Reason != domain.ReasonLanguageMismatch {
			t.Errorf("id liturgy with the en template: %v", err)
		}
		empty, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-18", Time: "11:00", ServiceID: enSvc, Language: "id", TemplateID: ptr(domain.TemplateID(""))})
		if err != nil || empty.Liturgy.Language != "id" || len(empty.Items) != 0 || empty.Liturgy.TemplateID != "" {
			t.Errorf("empty liturgy in id: %+v %v", empty.Liturgy, err)
		}
		// The template alone decides the language of a one-off; no template, the church's.
		if v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-19", ServiceName: "X", TemplateID: &en.Template.ID}); err != nil || v.Liturgy.Language != "en" {
			t.Errorf("one-off from the en template: %+v %v", v.Liturgy, err)
		}
		if v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-19", ServiceName: "Y", TemplateID: ptr(domain.TemplateID(""))}); err != nil || v.Liturgy.Language != "id" {
			t.Errorf("one-off without template: %+v %v", v.Liturgy, err)
		}

		// The service's name can be overridden for one liturgy.
		if v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-25", Time: "07:00", ServiceID: svc, ServiceName: "Natal"}); err != nil || v.Liturgy.ServiceName != "Natal" {
			t.Errorf("name override: %+v %v", v.Liturgy, err)
		}

		// Deleting the service or the template clears the reference and keeps the copy.
		if err := e.svcs.Delete(e.ctx, e.admin, svc); err != nil {
			t.Fatal(err)
		}
		got := e.liturgy(l.ID)
		if got.Liturgy.ServiceID != "" || got.Liturgy.ServiceName != "Ibadah Minggu 1" || got.Liturgy.Time != "07:00" {
			t.Errorf("after the service is deleted: %+v", got.Liturgy)
		}
		if err := e.tpls.Delete(e.ctx, e.admin, tpl); err != nil {
			t.Fatal(err)
		}
		if got := e.liturgy(l.ID); got.Liturgy.TemplateID != "" || len(got.Items) != 7 {
			t.Errorf("after the template is deleted: %+v", got.Liturgy)
		}
	})
}

// TC-L-003, IT-L-004: the weeks and occurrences of "Prepare next week".
func TestPrepareWeek(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		tpl, _ := e.seededTemplate()
		s1 := e.newService("Ibadah Pagi", "id", tpl, sunday("07:00"))
		s2 := e.newService("Ibadah Siang", "id", tpl, sunday("09:00"), domain.ServiceTime{Weekday: 3, Time: "19:00"})

		want := func(week string, wantWeek string, n int) app.PrepareWeek {
			t.Helper()
			w, err := e.liturgies.Prepare(e.ctx, e.admin, week)
			if err != nil || w.Week != wantWeek || len(w.Occurrences) != n {
				t.Fatalf("week %q: %+v %v", week, w, err)
			}
			return w
		}
		// A Wednesday, a Sunday and a Monday are in the same week.
		for _, day := range []string{"2026-10-14", "2026-10-18", "2026-10-12"} {
			w := want(day, "2026-10-12", 3)
			got := []string{w.Occurrences[0].Date + " " + w.Occurrences[0].Time + " " + w.Occurrences[0].ServiceName,
				w.Occurrences[1].Date + " " + w.Occurrences[1].Time + " " + w.Occurrences[1].ServiceName,
				w.Occurrences[2].Date + " " + w.Occurrences[2].Time + " " + w.Occurrences[2].ServiceName}
			if got[0] != "2026-10-14 19:00 Ibadah Siang" || got[1] != "2026-10-18 07:00 Ibadah Pagi" || got[2] != "2026-10-18 09:00 Ibadah Siang" {
				t.Errorf("order: %v", got)
			}
			if w.Occurrences[0].TemplateName != "Ibadah Minggu" || w.Occurrences[0].Language != "id" {
				t.Errorf("occurrence: %+v", w.Occurrences[0])
			}
		}
		if _, err := e.liturgies.Prepare(e.ctx, e.admin, "2026-13-01"); invalidField(err) != "week" {
			t.Errorf("bad week: %v", err)
		}

		// Without a week: next week in the church's time zone. The clock is a Friday.
		want("", "2026-10-05", 3)
		// Sunday 17:30 UTC is already Monday 00:30 in Jakarta: that week is the current one.
		e.clock.add(time.Date(2026, 10, 4, 17, 30, 0, 0, time.UTC).Sub(e.clock.Now()))
		want("", "2026-10-12", 3)

		// Existing slots are flagged.
		made, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-18", Time: "07:00", ServiceID: s1})
		if err != nil {
			t.Fatal(err)
		}
		w := want("2026-10-12", "2026-10-12", 3)
		if w.Occurrences[1].LiturgyID != made.Liturgy.ID || w.Occurrences[0].LiturgyID != "" || w.Occurrences[2].LiturgyID != "" {
			t.Errorf("flags: %+v", w.Occurrences)
		}
		if w.Active.Used != 1 || w.Unpublished.Used != 1 || !w.Active.Unlimited {
			t.Errorf("limits: %+v %+v", w.Active, w.Unpublished)
		}

		// A mix of a real and an invented occurrence creates nothing.
		_, err = e.liturgies.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{
			{ServiceID: s2, Date: "2026-10-14", Time: "19:00"}, {ServiceID: s2, Date: "2026-10-15", Time: "19:00"}})
		if invalidField(err) != "occurrences.1.time" {
			t.Errorf("invented occurrence: %v", err)
		}
		// An existing slot names the liturgy.
		_, err = e.liturgies.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{
			{ServiceID: s2, Date: "2026-10-14", Time: "19:00"}, {ServiceID: s1, Date: "2026-10-18", Time: "07:00"}})
		if existsID(err) != made.Liturgy.ID {
			t.Errorf("existing slot: %v", err)
		}
		if page, _ := e.liturgies.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50}); page.Total != 1 {
			t.Fatalf("a failed batch created liturgies: %d", page.Total)
		}
		// Two entries for one slot, and the size of a batch.
		if _, err := e.liturgies.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{
			{ServiceID: s2, Date: "2026-10-14", Time: "19:00"}, {ServiceID: s2, Date: "2026-10-14", Time: "19:00"}}); err == nil {
			t.Error("a slot listed twice")
		}
		if _, err := e.liturgies.PrepareCreate(e.ctx, e.admin, nil); invalidField(err) != "occurrences" {
			t.Errorf("empty batch: %v", err)
		}

		// The rest of the week in one go; each liturgy has its history row.
		made2, err := e.liturgies.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{
			{ServiceID: s2, Date: "2026-10-14", Time: "19:00"}, {ServiceID: s2, Date: "2026-10-18", Time: "09:00"}})
		if err != nil || len(made2) != 2 || made2[0].TemplateName != "Ibadah Minggu" || made2[1].Language != "id" {
			t.Fatalf("batch: %+v %v", made2, err)
		}
		if h, _ := e.liturgies.Edits(e.ctx, e.admin, made2[1].LiturgyID, 10); len(h) != 1 || h[0].Edit.Command != domain.CmdLiturgyCreate {
			t.Errorf("history of a prepared liturgy: %+v", h)
		}
		// A service whose template has another language fails the batch.
		en, _ := e.tpls.Create(e.ctx, e.admin, app.TemplateInput{Name: "Sunday", Language: "en"})
		sv, _ := e.svcs.Create(e.ctx, e.admin, app.ServiceInput{Name: "English", Language: "id", DefaultTemplateID: en.Template.ID, Times: []domain.ServiceTime{sunday("11:00")}})
		_, err = e.liturgies.PrepareCreate(e.ctx, e.admin, []app.PrepareEntry{{ServiceID: sv.Service.ID, Date: "2026-10-18", Time: "11:00"}})
		if in := new(domain.InvalidInputError); !errors.As(err, &in) || in.Reason != domain.ReasonLanguageMismatch {
			t.Errorf("language mismatch in a batch: %v", err)
		}
	})
}

// firstLiturgy returns the ID of the oldest liturgy by date.
func (e cenv) firstLiturgy() domain.LiturgyID {
	e.t.Helper()
	page, err := e.liturgies.List(e.ctx, e.admin, app.LiturgyFilter{Ascending: true, Limit: 1})
	if err != nil || len(page.Items) == 0 {
		e.t.Fatalf("list: %v", err)
	}
	return page.Items[0].Liturgy.ID
}
