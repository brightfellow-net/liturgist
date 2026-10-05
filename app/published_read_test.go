// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (e cenv) dutyID() domain.DutyID {
	e.t.Helper()
	duties, err := e.vocab.List(e.ctx, e.admin, domain.KindDuty)
	if err != nil || len(duties) == 0 {
		e.t.Fatal("no duties", err)
	}
	return domain.DutyID(duties[0].Entry.ID)
}

// publishWith creates a liturgy with a prayer item for the duty, assigns the
// people, and takes it through review to published.
func (e cenv) publishWith(in app.CreateInput, who ...app.AssignmentInput) domain.LiturgyID {
	e.t.Helper()
	in.TemplateID = ptr(domain.TemplateID(""))
	if in.ServiceName == "" {
		in.ServiceName = "Service"
	}
	v, err := e.liturgies.Create(e.ctx, e.admin, in)
	if err != nil {
		e.t.Fatal(err)
	}
	lid := v.Liturgy.ID
	e.useDuty(lid, string(e.dutyID()))
	for _, a := range who {
		a.DutyID = e.dutyID()
		if _, err := e.liturgies.AddAssignment(e.ctx, e.admin, lid, a); err != nil {
			e.t.Fatal(err)
		}
	}
	e.review(lid, domain.ActionSubmit, domain.ActionApprove, domain.ActionPublish)
	return lid
}

func (e cenv) review(lid domain.LiturgyID, actions ...domain.ReviewAction) {
	e.t.Helper()
	for _, a := range actions {
		cur := e.liturgy(lid)
		if _, err := e.liturgies.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: a, EditSeq: ptr(cur.Liturgy.EditSeq)}); err != nil {
			e.t.Fatalf("%s: %v", a, err)
		}
	}
}

// IT-P-008: every member reads the newest published version, whatever the
// state now; nobody reads a liturgy that has none.
func TestReadPublished(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		L := r.liturgies
		if _, err := L.ReadPublished(r.ctx, r.team, r.lid); !isNotFound(err, app.ReasonMissing) {
			t.Errorf("a liturgy with no version, team member: %v", err)
		}
		if _, err := L.ReadPublished(r.ctx, r.a, r.lid); !isNotFound(err, app.ReasonMissing) {
			t.Errorf("a liturgy with no version, admin: %v", err)
		}
		r.rich()
		r.approve()
		r.mustPublish()

		c, err := L.ReadPublished(r.ctx, r.team, r.lid)
		if err != nil {
			t.Fatal(err)
		}
		if c.Number != 1 || c.Revising || c.Archived || c.PublishedBy != "Admin" || c.PublishedByID != r.a.UserID ||
			c.Render.KeyDisplay != "do" || !c.Render.ShowCredits || c.URL != "https://liturgi.example.org/published/"+string(r.lid) {
			t.Errorf("copy: %+v", c)
		}
		if c.Content.Format != 1 || len(c.Content.Items) != 4 || c.Content.Items[1].Songs[0].Title != "Cinta Tuhan" ||
			c.Content.Liturgy.ChurchName != "GKY Citragarden" {
			t.Errorf("content: %+v", c.Content.Liturgy)
		}
		// The team member reaches the copy only: the editable liturgy stays closed.
		if _, err := L.Get(r.ctx, r.team, r.lid); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member, editable liturgy: %v", err)
		}
		// A user who is not a member of this church.
		other, _ := r.membersOfOtherChurch()
		if _, err := L.ReadPublished(r.ctx, other, r.lid); !isNotFound(err, app.ReasonNotMember) {
			t.Errorf("not a member: %v", err)
		}
		// Another church's context does not find the liturgy.
		bctx := app.WithTenant(ctx, r.oldChurch("id"))
		if _, err := L.ReadPublished(bctx, r.a, r.lid); err == nil {
			t.Error("a liturgy of church A read through church B")
		}

		// Reopened: the last copy stays, marked as being revised.
		r.must(r.a, domain.ActionReopen, nil, "typo")
		r.pray("Berkat")
		if c, err = L.ReadPublished(r.ctx, r.team, r.lid); err != nil || c.Number != 1 || !c.Revising || len(c.Content.Items) != 4 {
			t.Errorf("revising: %+v %v", c.Number, err)
		}
		r.approve()
		r.mustPublish()
		if c, err = L.ReadPublished(r.ctx, r.team, r.lid); err != nil || c.Number != 2 || c.Revising || len(c.Content.Items) != 5 {
			t.Errorf("second copy: %+v %v", c.Number, err)
		}
		if _, err := L.Archive(r.ctx, r.a, r.lid); err != nil {
			t.Fatal(err)
		}
		if c, err = L.ReadPublished(r.ctx, r.team, r.lid); err != nil || !c.Archived {
			t.Errorf("archived: %+v %v", c.Archived, err)
		}
	})
}

// membersOfOtherChurch is a user who belongs to no church of this test.
func (e cenv) membersOfOtherChurch() (*domain.Session, domain.UserID) {
	e.t.Helper()
	uid := domain.UserID(e.ids.NewID())
	now := e.clock.Now()
	e.write(func(s app.Store) error {
		return s.Users().Create(ctx, domain.User{ID: uid, Name: "Stranger", Email: "stranger@example.org", PasswordHash: "x", CreatedAt: now, UpdatedAt: now})
	})
	return &domain.Session{UserID: uid}, uid
}

// IT-P-014: the filters of the list, its order and its paging.
func TestListPublished(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		team, _ := e.member("team@example.org")
		L := e.liturgies
		list := func(f app.PublishedFilter) app.PublishedPage {
			t.Helper()
			p, err := L.ListPublished(e.ctx, team, f)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
		if p := list(app.PublishedFilter{}); p.Total != 0 || len(p.Items) != 0 {
			t.Fatalf("empty church: %+v", p)
		}
		e.draft() // never published: not in the list
		a := e.publishWith(app.CreateInput{Date: "2026-10-11"})
		b := e.publishWith(app.CreateInput{Date: "2026-10-18", Time: "09:00", ServiceName: "Morning"})
		c := e.publishWith(app.CreateInput{Date: "2026-10-18", Time: "17:00", ServiceName: "Evening"})
		d := e.publishWith(app.CreateInput{Date: "2026-10-18", ServiceName: "No time"})
		f := e.publishWith(app.CreateInput{Date: "2026-10-04"})
		ids := func(p app.PublishedPage) []domain.LiturgyID {
			var out []domain.LiturgyID
			for _, it := range p.Items {
				out = append(out, it.Liturgy.ID)
			}
			return out
		}
		same := func(got, want []domain.LiturgyID) bool {
			if len(got) != len(want) {
				return false
			}
			for i := range got {
				if got[i] != want[i] {
					return false
				}
			}
			return true
		}
		// Newest date first; on one date the later time first, no time last.
		if p := list(app.PublishedFilter{}); p.Total != 4+1 || !same(ids(p), []domain.LiturgyID{c, b, d, a, f}) {
			t.Errorf("order: %v of %d", ids(p), p.Total)
		}
		if p := list(app.PublishedFilter{From: "2026-10-11", To: "2026-10-11"}); p.Total != 1 || !same(ids(p), []domain.LiturgyID{a}) {
			t.Errorf("from/to: %v", ids(p))
		}
		if p := list(app.PublishedFilter{From: "2026-10-12"}); p.Total != 3 {
			t.Errorf("from: %d", p.Total)
		}
		if p := list(app.PublishedFilter{Limit: 2, Offset: 1}); p.Total != 5 || !same(ids(p), []domain.LiturgyID{b, d}) {
			t.Errorf("page: %v of %d", ids(p), p.Total)
		}
		// Archived: hidden by default.
		if _, err := L.Archive(e.ctx, e.admin, a); err != nil {
			t.Fatal(err)
		}
		if p := list(app.PublishedFilter{}); p.Total != 4 {
			t.Errorf("archived are hidden: %d", p.Total)
		}
		if p := list(app.PublishedFilter{Archived: app.ArchivedOnly}); p.Total != 1 || p.Items[0].Liturgy.ID != a || p.Items[0].Liturgy.ArchivedAt == nil {
			t.Errorf("archived only: %+v", p)
		}
		if p := list(app.PublishedFilter{Archived: app.ArchivedAll}); p.Total != 5 {
			t.Errorf("all: %d", p.Total)
		}
		// A reopened liturgy stays listed with its last number.
		e.review(c, domain.ActionReopen)
		p := list(app.PublishedFilter{})
		if p.Items[0].Liturgy.State != domain.StateDraft || p.Items[0].Number != 1 {
			t.Errorf("reopened: %+v", p.Items[0])
		}
		// Number and time of the newest version.
		e.review(c, domain.ActionSubmit, domain.ActionApprove, domain.ActionPublish)
		if p = list(app.PublishedFilter{}); p.Items[0].Number != 2 || !p.Items[0].PublishedAt.Equal(e.clock.Now()) {
			t.Errorf("republished: %+v", p.Items[0])
		}
	})
}

// IT-P-009: "my assignments".
func TestMyAssignments(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		me, _ := e.member("me@example.org")
		you, _ := e.member("you@example.org")
		L := e.liturgies
		mine := func(sess *domain.Session) app.MyAssignments {
			t.Helper()
			res, err := L.MyAssignments(e.ctx, sess)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		if res := mine(me); len(res.Items) != 0 || res.More || res.Items == nil {
			t.Fatalf("nothing yet: %+v", res)
		}
		// The clock is 2026-10-02 09:00 UTC, which is 16:00 in Jakarta.
		past := e.publishWith(app.CreateInput{Date: "2026-10-01"}, app.AssignmentInput{UserID: me.UserID})
		today := e.publishWith(app.CreateInput{Date: "2026-10-02", Time: "07:00", ServiceName: "Early"}, app.AssignmentInput{UserID: me.UserID})
		sunday := e.publishWith(app.CreateInput{Date: "2026-10-11", Time: "09:00", ServiceName: "Sunday"},
			app.AssignmentInput{UserID: me.UserID}, app.AssignmentInput{Name: "Pak Budi"})
		sundayLater := e.publishWith(app.CreateInput{Date: "2026-10-11", Time: "17:00", ServiceName: "Evening"}, app.AssignmentInput{UserID: you.UserID})
		archived := e.publishWith(app.CreateInput{Date: "2026-10-12"}, app.AssignmentInput{UserID: me.UserID})
		if _, err := L.Archive(e.ctx, e.admin, archived); err != nil {
			t.Fatal(err)
		}
		_ = past
		_ = sundayLater
		// A draft with the same person is not published.
		draft := e.draft()
		if _, err := L.AddAssignment(e.ctx, e.admin, draft, app.AssignmentInput{DutyID: e.dutyID(), UserID: me.UserID}); err != nil {
			t.Fatal(err)
		}

		res := mine(me)
		if len(res.Items) != 2 || res.More || res.Items[0].Liturgy.ID != today || res.Items[1].Liturgy.ID != sunday {
			t.Fatalf("cards: %+v", res)
		}
		c := res.Items[1]
		if c.Number != 1 || c.Revising || len(c.Duties) != 1 || c.Duties[0].ID != string(e.dutyID()) || len(c.Items) != 1 || c.Items[0].Title != "Prayer" {
			t.Errorf("sunday card: %+v", c)
		}
		if res := mine(you); len(res.Items) != 1 || res.Items[0].Liturgy.ID != sundayLater {
			t.Errorf("you: %+v", res)
		}
		if res := mine(e.admin); len(res.Items) != 0 {
			t.Errorf("admin has no duty: %+v", res)
		}
		stranger, _ := e.membersOfOtherChurch()
		if _, err := L.MyAssignments(e.ctx, stranger); !isNotFound(err, app.ReasonNotMember) {
			t.Errorf("not a member: %v", err)
		}

		// A liturgy dated today stays listed through the evening; the date flips at
		// midnight in Jakarta (17:00 UTC), not at midnight UTC.
		e.clock.add(8*time.Hour + 30*time.Minute) // 17:30 UTC = 00:30 on 2026-10-03 in Jakarta
		if res := mine(me); len(res.Items) != 1 || res.Items[0].Liturgy.ID != sunday {
			t.Errorf("after midnight in Jakarta: %+v", res)
		}
		e.clock.add(-9 * time.Hour) // 08:30 UTC
		if res := mine(me); len(res.Items) != 2 {
			t.Errorf("before: %+v", res)
		}

		// What is in the editor is not shown until it is published, and a reopened
		// liturgy is marked.
		e.review(sunday, domain.ActionReopen)
		for _, a := range e.liturgy(sunday).Assignments {
			if a.Assignment.UserID == me.UserID {
				if err := L.RemoveAssignment(e.ctx, e.admin, sunday, a.Assignment.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		if res := mine(me); len(res.Items) != 2 || !res.Items[1].Revising || len(res.Items[1].Duties) != 1 {
			t.Errorf("revising: %+v", res.Items)
		}
		// Republished without me: gone.
		e.review(sunday, domain.ActionSubmit, domain.ActionApprove, domain.ActionPublish)
		if res := mine(me); len(res.Items) != 1 {
			t.Errorf("removed from the new version: %+v", res.Items)
		}
	})
}

// IT-P-009: more than 50 upcoming liturgies are cut at 50.
func TestMyAssignmentsMore(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		me, _ := e.member("me@example.org")
		base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 51; i++ {
			e.publishWith(app.CreateInput{Date: base.AddDate(0, 0, i).Format("2006-01-02")}, app.AssignmentInput{UserID: me.UserID})
		}
		res, err := e.liturgies.MyAssignments(e.ctx, me)
		if err != nil || len(res.Items) != 50 || !res.More || res.Items[0].Liturgy.Date != "2026-11-01" || res.Items[49].Liturgy.Date != "2026-12-20" {
			t.Fatalf("%d cards, more=%v, %v", len(res.Items), res.More, err)
		}
	})
}
