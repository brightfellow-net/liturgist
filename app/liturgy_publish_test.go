// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

type storedVersion struct {
	Number  int    `db:"number"`
	Content string `db:"content"`
}

func (r renv) versions() []storedVersion {
	r.t.Helper()
	var out []storedVersion
	if err := r.db.SelectForTest(r.ctx, &out, "SELECT number, content FROM published_versions WHERE liturgy_id = ? ORDER BY number", string(r.lid)); err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r renv) content(v storedVersion) domain.PublishedContent {
	r.t.Helper()
	var c domain.PublishedContent
	if err := json.Unmarshal([]byte(v.Content), &c); err != nil {
		r.t.Fatal(err)
	}
	return c
}

func (r renv) approve() {
	r.t.Helper()
	r.must(r.a, domain.ActionSubmit, nil, "")
	r.must(r.a, domain.ActionApprove, r.seq(), "")
}

func (r renv) publish() (app.LiturgyView, error) {
	return r.review(r.a, domain.ActionPublish, r.seq(), "")
}

func (r renv) mustPublish() app.LiturgyView {
	r.t.Helper()
	v, err := r.publish()
	if err != nil {
		r.t.Fatalf("publish: %v", err)
	}
	return v
}

// rich fills the liturgy: a song item whose sequence repeats the chorus, a
// reading, a prayer with a duty and a member and a free-text assignee.
func (r renv) rich() (song app.SongView, reading app.ReadingView) {
	r.t.Helper()
	song = r.newSong("Cinta Tuhan", "id")
	r.useSections(r.lid, song.Song.ID, song.Song.Sections[0].ID, song.Song.Sections[1].ID, song.Song.Sections[2].ID, song.Song.Sections[1].ID)
	var err error
	if reading, err = r.readings.Create(r.ctx, r.a, app.ReadingInput{Reference: "Yoh 3:16", Translation: "TB", Text: "Karena begitu besar kasih Allah", Attribution: "LAI"}); err != nil {
		r.t.Fatal(err)
	}
	r.useReading(r.lid, reading.Reading.ID)
	duty := r.duty()
	r.useDuty(r.lid, string(duty))
	for _, in := range []app.AssignmentInput{{DutyID: duty, UserID: r.a.UserID}, {DutyID: duty, Name: "  Pak Budi "}} {
		if _, err := r.liturgies.AddAssignment(r.ctx, r.a, r.lid, in); err != nil {
			r.t.Fatal(err)
		}
	}
	return song, reading
}

// IT-P-001, TC-P-002: the whole cycle, the content of a version, republishing.
func TestPublishCycle(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		song, _ := r.rich()
		r.approve()
		v := r.mustPublish()
		if v.Liturgy.State != domain.StatePublished || v.Published == nil || v.Published.Number != 1 || !v.Actions.Reopen ||
			v.Actions.Edit || v.Actions.Delete || v.Actions.Publish || !v.Actions.Archive || v.Actions.Unarchive {
			t.Fatalf("published: %+v %+v %+v", v.Liturgy.State, v.Published, v.Actions)
		}
		vs := r.versions()
		if len(vs) != 1 {
			t.Fatalf("%d versions", len(vs))
		}
		c := r.content(vs[0])
		if c.Format != 1 || c.Liturgy.ChurchName != "GKY Citragarden" || c.Liturgy.Language != "id" || c.Liturgy.Date != "2026-10-11" {
			t.Errorf("header: %+v", c.Liturgy)
		}
		if len(c.Items) != 4 {
			t.Fatalf("%d items", len(c.Items))
		}
		sg := c.Items[1].Songs
		if len(sg) != 1 || len(sg[0].Sections) != 3 || len(sg[0].Entries) != 4 || sg[0].Title != "Cinta Tuhan" ||
			sg[0].Sections[0].Label != "Bait 1" || sg[0].Sections[1].Label != "Refren" || sg[0].Sections[0].Text != "bait satu" ||
			sg[0].Entries[3].SectionID != sg[0].Entries[1].SectionID {
			t.Errorf("song: %+v", sg)
		}
		if rd := c.Items[2].Reading; rd == nil || rd.TranslationCode != "TB" || rd.Attribution != "LAI" || !strings.HasPrefix(rd.Text, "Karena") {
			t.Errorf("reading: %+v", c.Items[2].Reading)
		}
		if d := c.Items[3].Duty; d == nil || d.ID != string(r.duty()) || d.Name == "" {
			t.Errorf("duty: %+v", c.Items[3].Duty)
		}
		if len(c.Assignments) != 2 || c.Assignments[0].UserID != r.a.UserID || c.Assignments[0].Name != "Admin" ||
			c.Assignments[1].UserID != "" || c.Assignments[1].Name != "Pak Budi" {
			t.Errorf("assignments: %+v", c.Assignments)
		}

		// Editing the song afterwards does not change what was published.
		if _, err := r.songs.Update(r.ctx, r.a, song.Song.ID, app.SongChange{Version: song.Song.Version, Title: ptr("Cinta Tuhan 2")}); err != nil {
			t.Fatalf("edit the song: %v", err)
		}
		// Reopen, change, go through review again: version 2, version 1 untouched.
		v = r.must(r.a, domain.ActionReopen, nil, "typo")
		if v.Liturgy.State != domain.StateDraft || v.Published == nil || v.Published.Number != 1 || v.Actions.Delete {
			t.Fatalf("reopened: %+v %+v %+v", v.Liturgy.State, v.Published, v.Actions)
		}
		r.pray("Berkat")
		r.approve()
		if v = r.mustPublish(); v.Published.Number != 2 {
			t.Fatalf("second publication: %+v", v.Published)
		}
		vs = r.versions()
		if len(vs) != 2 || r.content(vs[0]).Items[1].Songs[0].Title != "Cinta Tuhan" ||
			r.content(vs[1]).Items[1].Songs[0].Title != "Cinta Tuhan 2" || len(r.content(vs[1]).Items) != 5 {
			t.Errorf("versions: %d", len(vs))
		}
		ch := r.changes(r.a)
		if ch[0].Change.To != domain.StatePublished || ch[0].Change.From != domain.StateApproved {
			t.Errorf("history: %+v", ch[0].Change)
		}
	})
}

// TC-P-001 and the checks of 13 §2: scope, state, edit_seq, in that order.
func TestPublishChecks(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.rich()
		if _, err := r.review(r.team, domain.ActionPublish, ptr(0), ""); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member: %v", err)
		}
		if _, err := r.review(r.b, domain.ActionPublish, ptr(0), ""); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("editor: %v", err)
		}
		if _, err := r.review(r.a, domain.ActionPublish, nil, ""); invalidField(err) != "edit_seq" {
			t.Errorf("no edit_seq: %v", err)
		}
		if _, err := r.publish(); !isInvalidTransition(err, domain.StateDraft) {
			t.Errorf("publish a draft: %v", err)
		}
		r.must(r.a, domain.ActionSubmit, nil, "")
		if _, err := r.publish(); !isInvalidTransition(err, domain.StateInReview) {
			t.Errorf("publish in review: %v", err)
		}
		seq := *r.seq()
		r.must(r.a, domain.ActionApprove, r.seq(), "")
		// An approved liturgy reopened, edited and approved again: a stale tab
		// cannot publish content its user never read.
		r.must(r.a, domain.ActionReopen, nil, "")
		r.pray("More")
		r.approve()
		if _, err := r.review(r.a, domain.ActionPublish, ptr(seq), ""); !errors.Is(err, app.ErrReviewStale) {
			t.Errorf("stale edit_seq: %v", err)
		}
		if got := len(r.versions()); got != 0 {
			t.Errorf("a refused publish left %d versions", got)
		}
		// The state wins over a stale edit_seq.
		if _, err := r.review(r.a, domain.ActionPublish, ptr(seq), ""); errors.Is(err, app.ErrReviewStale) == false {
			t.Errorf("still stale: %v", err)
		}
		r.mustPublish()
		if _, err := r.publish(); !isInvalidTransition(err, domain.StatePublished) {
			t.Errorf("publish twice: %v", err)
		}
	})
}

// IT-P-002: two simultaneous publishes, one version.
func TestPublishSimultaneous(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.rich()
		r.approve()
		seq := r.seq()
		errs := race(4, func(int) error { _, err := r.review(r.a, domain.ActionPublish, seq, ""); return err })
		ok, others := succeeded(errs)
		if ok != 1 {
			t.Fatalf("%d publishes won: %v", ok, others)
		}
		for _, err := range others {
			if !isInvalidTransition(err, domain.StatePublished) {
				t.Errorf("loser: %v", err)
			}
		}
		if got := len(r.versions()); got != 1 {
			t.Errorf("%d versions", got)
		}
	})
}

// IT-P-003: a failure while the copy is made leaves the liturgy approved, with no version.
func TestPublishRollsBack(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.rich()
		r.approve()
		// A reference that vanished after the approval: only a direct write can do it.
		r.sql("UPDATE liturgy_item_songs SET song_id = NULL WHERE item_id IN (SELECT id FROM liturgy_items WHERE liturgy_id = ?)", string(r.lid))
		_, err := r.publish()
		var hp *app.HasProblemsError
		if !errors.As(err, &hp) || len(hp.Problems) == 0 {
			t.Fatalf("publish with a problem: %v", err)
		}
		if r.state() != domain.StateApproved || len(r.versions()) != 0 {
			t.Errorf("state %s, %d versions", r.state(), len(r.versions()))
		}
		if ch := r.changes(r.a); ch[0].Change.To != domain.StateApproved {
			t.Errorf("a state change was kept: %+v", ch[0].Change)
		}
	})
}

// IT-P-013, TC-P-007: the cap is measured on the stored bytes.
func TestPublishSize(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		// Characters HTML escaping would triple are stored as they are.
		x := r.pray("Text")
		r.sql("UPDATE liturgy_items SET text = ? WHERE id = ?", strings.Repeat("<&>", 5000), string(x))
		r.approve()
		r.mustPublish()
		vs := r.versions()
		if n := len(vs[0].Content); n > 16000 || !strings.Contains(vs[0].Content, "<&><&>") {
			t.Errorf("content of %d bytes, or escaped", n)
		}
		r.must(r.a, domain.ActionReopen, nil, "")

		// 60 items of 20,000 four-byte characters are over 4 MiB.
		for i := range 58 {
			r.pray("P" + string(rune('A'+i%26)))
		}
		r.sql("UPDATE liturgy_items SET text = ? WHERE liturgy_id = ?", strings.Repeat("😀", 20000), string(r.lid))
		r.approve()
		_, err := r.publish()
		var big *app.PublishTooLargeError
		if !errors.As(err, &big) || big.LargestItem == "" {
			t.Fatalf("oversized copy: %v", err)
		}
		if r.state() != domain.StateApproved || len(r.versions()) != 1 {
			t.Errorf("state %s, %d versions", r.state(), len(r.versions()))
		}
	})
}

// IT-P-015, IT-P-004: a liturgy that has a version is never deleted.
func TestDeleteNeedsNoVersion(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.rich()
		r.approve()
		r.mustPublish()
		del := func() error { return r.liturgies.Delete(r.ctx, r.a, r.lid) }
		if err := del(); !errors.Is(err, app.ErrLiturgyNotDeletable) {
			t.Fatalf("delete published: %v", err)
		}
		r.must(r.a, domain.ActionReopen, nil, "")
		if v := r.liturgy(r.lid); v.Actions.Delete {
			t.Error("a reopened liturgy that was published offers Delete")
		}
		if err := del(); !errors.Is(err, app.ErrLiturgyNotDeletable) {
			t.Fatalf("delete reopened: %v", err)
		}
		// The statement itself refuses, whatever the early checks saw.
		r.write(func(s app.Store) error {
			cs, err := s.ForChurch(r.ctx, r.church)
			if err != nil {
				return err
			}
			if err := cs.Liturgies().Delete(r.ctx, r.lid); !errors.Is(err, app.ErrNotFound) {
				t.Errorf("repository delete of a liturgy with a version: %v", err)
			}
			return nil
		})
		if len(r.versions()) != 1 || r.state() != domain.StateDraft {
			t.Error("the liturgy or its version is gone")
		}
		// A draft that was never published is deleted as before.
		other := r.draft()
		if err := r.liturgies.Delete(r.ctx, r.a, other); err != nil {
			t.Errorf("delete an unpublished draft: %v", err)
		}
	})
}

// IT-P-004: a delete racing a publish never destroys a published liturgy.
func TestPublishAndDeleteRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		base := newReviewEnv(t, db)
		for round := range 30 {
			r := base
			r.lid = r.draft()
			r.pray("Doa")
			r.approve()
			seq := r.seq()
			errs := race(2, func(i int) error {
				if i == 0 {
					_, err := r.review(r.a, domain.ActionPublish, seq, "")
					return err
				}
				return r.liturgies.Delete(r.ctx, r.a, r.lid)
			})
			pubErr, delErr := errs[0], errs[1]
			_, getErr := r.liturgies.Get(r.ctx, r.a, r.lid)
			versions := 0
			if getErr == nil {
				versions = len(r.versions())
			}
			switch {
			case pubErr == nil && (getErr != nil || versions != 1):
				t.Fatalf("round %d: published but gone or without a version: get %v, %d versions, delete %v", round, getErr, versions, delErr)
			case pubErr == nil && !errors.Is(delErr, app.ErrLiturgyNotDeletable):
				t.Fatalf("round %d: delete beside a publish: %v", round, delErr)
			case pubErr != nil && delErr != nil:
				t.Fatalf("round %d: both lost: %v %v", round, pubErr, delErr)
			}
		}
	})
}

// IT-P-005: library deletions that only a published liturgy depends on; the
// reopened liturgy shows problems and cannot be submitted until they are fixed.
func TestReopenWithClearedReferences(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		song, reading := r.rich()
		r.approve()
		r.mustPublish()
		if err := r.songs.Delete(r.ctx, r.a, song.Song.ID); err != nil {
			t.Fatalf("delete a song only a published liturgy uses: %v", err)
		}
		if err := r.readings.Delete(r.ctx, r.a, reading.Reading.ID); err != nil {
			t.Fatalf("delete a reading only a published liturgy uses: %v", err)
		}
		v := r.must(r.a, domain.ActionReopen, nil, "")
		codes := map[string]bool{}
		for _, p := range v.Problems {
			codes[p.Code] = true
		}
		if !codes["song_removed"] || !codes["reading_removed"] {
			t.Fatalf("problems after the reopen: %+v", v.Problems)
		}
		if _, err := r.review(r.a, domain.ActionSubmit, nil, ""); err == nil {
			t.Error("submitted with removed references")
		}
		if len(r.versions()) != 1 || !strings.Contains(r.versions()[0].Content, "Cinta Tuhan") {
			t.Error("the published copy changed")
		}
	})
}

// publishedLiturgy makes a one-off liturgy on the date and publishes it.
func publishedLiturgy(t *testing.T, e cenv, date string) domain.LiturgyID {
	t.Helper()
	v, err := e.liturgies.Create(e.ctx, e.admin, app.CreateInput{Date: date, ServiceName: "Old", TemplateID: ptr(domain.TemplateID(""))})
	if err != nil {
		t.Fatal(err)
	}
	e.addItem(v.Liturgy.ID, domain.ItemPrayer, "Doa")
	for _, a := range []domain.ReviewAction{domain.ActionSubmit, domain.ActionApprove, domain.ActionPublish} {
		cur := e.liturgy(v.Liturgy.ID)
		if _, err := e.liturgies.Review(e.ctx, e.admin, v.Liturgy.ID, app.ReviewInput{Action: a, EditSeq: ptr(cur.Liturgy.EditSeq)}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	return v.Liturgy.ID
}

// IT-P-006: reopening a published liturgy takes an unpublished place; reopening
// an approved one does not.
func TestReopenLimit(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, namedLimits{app.LimitMaxUnpublishedLiturgies: 1})
		L := e.liturgies
		lid := publishedLiturgy(t, e, "2026-10-11")
		other, err := L.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-18", ServiceName: "Other", TemplateID: ptr(domain.TemplateID(""))})
		if err != nil {
			t.Fatalf("the published liturgy's place is free: %v", err)
		}
		reopen := func() error {
			_, err := L.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: domain.ActionReopen})
			return err
		}
		if n, used, max := limitErr(reopen()); n != app.LimitMaxUnpublishedLiturgies || used != 1 || max != 1 {
			t.Errorf("reopen at the limit: %v %d %d", n, used, max)
		}
		if e.liturgy(lid).Liturgy.State != domain.StatePublished {
			t.Error("a refused reopen changed the state")
		}
		if err := L.Delete(e.ctx, e.admin, other.Liturgy.ID); err != nil {
			t.Fatal(err)
		}
		if err := reopen(); err != nil {
			t.Fatalf("reopen with a free place: %v", err)
		}
		// An approved liturgy is already unpublished: reopening it takes no place.
		for _, a := range []domain.ReviewAction{domain.ActionSubmit, domain.ActionApprove} {
			if _, err := L.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: a, EditSeq: ptr(e.liturgy(lid).Liturgy.EditSeq)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := reopen(); err != nil {
			t.Errorf("reopen an approved liturgy at the limit: %v", err)
		}
	})
}

// IT-P-006: simultaneous reopens of published liturgies take the last place once.
func TestReopenLimitRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, namedLimits{app.LimitMaxUnpublishedLiturgies: 1})
		var ids []domain.LiturgyID
		for i := range 4 {
			ids = append(ids, publishedLiturgy(t, e, "2026-11-0"+string(rune('1'+i))))
		}
		errs := race(len(ids), func(i int) error {
			_, err := e.liturgies.Review(e.ctx, e.admin, ids[i], app.ReviewInput{Action: domain.ActionReopen})
			return err
		})
		ok, others := succeeded(errs)
		if ok != 1 {
			t.Errorf("%d reopens won: %v", ok, others)
		}
		for _, err := range others {
			if n, _, _ := limitErr(err); n != app.LimitMaxUnpublishedLiturgies {
				t.Errorf("loser: %v", err)
			}
		}
	})
}

// IT-P-007: archive and unarchive.
func TestArchiveUnarchive(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, namedLimits{app.LimitMaxActiveLiturgies: 1})
		editor, _ := e.member("ed@example.org", e.role(domain.OriginEditor).ID)
		team, _ := e.member("team@example.org")
		L := e.liturgies
		lid := publishedLiturgy(t, e, "2026-10-11")
		if _, err := L.Archive(e.ctx, editor, lid); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("editor archives: %v", err)
		}
		if _, err := L.Archive(e.ctx, team, lid); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member archives: %v", err)
		}
		if _, err := L.Unarchive(e.ctx, e.admin, lid); !errors.Is(err, app.ErrNotArchived) {
			t.Errorf("unarchive an active liturgy: %v", err)
		}
		v, err := L.Archive(e.ctx, e.admin, lid)
		if err != nil || v.Liturgy.ArchivedAt == nil || v.Liturgy.ArchivedBy != e.admin.UserID || v.Actions.Archive || !v.Actions.Unarchive ||
			v.Actions.Reopen || v.Liturgy.State != domain.StatePublished {
			t.Fatalf("archive: %+v %+v %v", v.Liturgy, v.Actions, err)
		}
		if _, err := L.Archive(e.ctx, e.admin, lid); !errors.Is(err, app.ErrLiturgyArchived) {
			t.Errorf("archive twice: %v", err)
		}
		if _, err := L.Review(e.ctx, e.admin, lid, app.ReviewInput{Action: domain.ActionReopen}); !errors.Is(err, app.ErrLiturgyArchived) {
			t.Errorf("reopen an archived liturgy: %v", err)
		}
		if err := L.Delete(e.ctx, e.admin, lid); !errors.Is(err, app.ErrLiturgyNotDeletable) {
			t.Errorf("delete an archived liturgy: %v", err)
		}
		if ch := e.liturgy(lid); ch.Published == nil || ch.Published.Number != 1 {
			t.Errorf("an archived liturgy keeps its version: %+v", ch.Published)
		}
		list := func(f app.ArchivedFilter) int {
			p, err := L.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50, Archived: f})
			if err != nil {
				t.Fatal(err)
			}
			return p.Total
		}
		if list(app.ArchivedExclude) != 0 || list(app.ArchivedOnly) != 1 || list(app.ArchivedAll) != 1 {
			t.Errorf("list filters: %d %d %d", list(app.ArchivedExclude), list(app.ArchivedOnly), list(app.ArchivedAll))
		}
		// Archived liturgies do not count: the place is free for another one, and
		// unarchiving then needs a place of its own.
		if _, err := L.Create(e.ctx, e.admin, app.CreateInput{Date: "2026-10-18", ServiceName: "X", TemplateID: ptr(domain.TemplateID(""))}); err != nil {
			t.Fatalf("a place freed by archiving: %v", err)
		}
		if _, err := L.Unarchive(e.ctx, editor, lid); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("editor unarchives: %v", err)
		}
		if n, used, max := limitErr(errOf(L.Unarchive(e.ctx, e.admin, lid))); n != app.LimitMaxActiveLiturgies || used != 1 || max != 1 {
			t.Errorf("unarchive at the limit: %v %d %d", n, used, max)
		}
		if err := L.Delete(e.ctx, e.admin, e.firstLiturgyExcept(lid)); err != nil {
			t.Fatal(err)
		}
		if v, err := L.Unarchive(e.ctx, e.admin, lid); err != nil || v.Liturgy.ArchivedAt != nil || !v.Actions.Archive {
			t.Errorf("unarchive: %+v %v", v.Actions, err)
		}
	})
}

func (e cenv) firstLiturgyExcept(skip domain.LiturgyID) domain.LiturgyID {
	e.t.Helper()
	p, err := e.liturgies.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, it := range p.Items {
		if it.Liturgy.ID != skip {
			return it.Liturgy.ID
		}
	}
	e.t.Fatal("no other liturgy")
	return ""
}

// IT-P-011: archive and create in one request, or nothing.
func TestPrepareArchiving(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, namedLimits{app.LimitMaxActiveLiturgies: 3})
		editor, _ := e.member("ed@example.org", e.role(domain.OriginEditor).ID)
		L := e.liturgies
		old1, old2, recent := publishedLiturgy(t, e, "2026-10-04"), publishedLiturgy(t, e, "2026-10-11"), publishedLiturgy(t, e, "2026-10-18")
		tpl, _ := e.seededTemplate()
		svc := e.newService("Pagi", "id", tpl, sunday("07:00"), domain.ServiceTime{Weekday: 3, Time: "19:00"})
		one := []app.PrepareEntry{{ServiceID: svc, Date: "2026-10-18", Time: "07:00"}}
		two := append([]app.PrepareEntry{{ServiceID: svc, Date: "2026-10-14", Time: "19:00"}}, one...)
		archived := func(id domain.LiturgyID) bool { return e.liturgy(id).Liturgy.ArchivedAt != nil }
		count := func() int {
			p, _ := L.List(e.ctx, e.admin, app.LiturgyFilter{Limit: 50, Archived: app.ArchivedAll})
			return p.Total
		}

		if _, err := L.PrepareCreate(e.ctx, e.admin, one); errOfName(err) != app.LimitMaxActiveLiturgies {
			t.Fatalf("at the limit: %v", err)
		}
		// Scopes: liturgy.edit alone may not archive.
		if _, _, err := L.PrepareCreateArchiving(e.ctx, editor, one, []domain.LiturgyID{old1}); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("editor with archive_ids: %v", err)
		}
		// Refusals change nothing: unknown, listed twice, not before the week, still over the limit.
		if _, _, err := L.PrepareCreateArchiving(e.ctx, e.admin, one, []domain.LiturgyID{"01M3XY2HBEKN8PETK6KXXXXXXX"}); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("unknown ID: %v", err)
		}
		if _, _, err := L.PrepareCreateArchiving(e.ctx, e.admin, one, []domain.LiturgyID{old1, old1}); invalidField(err) != "archive_ids.1" {
			t.Errorf("duplicate: %v", err)
		}
		if _, _, err := L.PrepareCreateArchiving(e.ctx, e.admin, one, []domain.LiturgyID{old2, recent}); !isInvalidTransition(err, domain.StatePublished) {
			t.Errorf("a liturgy of the week or later: %v", err)
		}
		if _, _, err := L.PrepareCreateArchiving(e.ctx, e.admin, two, []domain.LiturgyID{old1}); errOfName(err) != app.LimitMaxActiveLiturgies {
			t.Errorf("room for one of two: %v", err)
		}
		if archived(old1) || archived(old2) || count() != 3 {
			t.Fatal("a refused request archived or created something")
		}
		made, ids, err := L.PrepareCreateArchiving(e.ctx, e.admin, one, []domain.LiturgyID{old1})
		if err != nil || len(made) != 1 || len(ids) != 1 || !archived(old1) || archived(old2) || count() != 4 {
			t.Fatalf("archive and create: %v %v %v", made, ids, err)
		}
		// Already archived: refused.
		if _, _, err := L.PrepareCreateArchiving(e.ctx, e.admin, two[:1], []domain.LiturgyID{old1}); !errors.Is(err, app.ErrLiturgyArchived) {
			t.Errorf("archived again: %v", err)
		}
	})
}

func errOfName(err error) app.LimitName {
	n, _, _ := limitErr(err)
	return n
}

// IT-P-006 (forced): a second reopen that runs between the first one's limit
// check and its update must wait for the church lock, so the limit holds. On
// PostgreSQL a reopen without the lock lets both in.
func TestReopenLimitForced(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		postgresOnly(t, db)
		e := newChurch(t, db, namedLimits{app.LimitMaxUnpublishedLiturgies: 1})
		a, b := publishedLiturgy(t, e, "2026-11-01"), publishedLiturgy(t, e, "2026-11-08")
		var second error
		rc := &raceClock{clock: e.clock}
		l := &app.Liturgies{Tx: e.db, Clock: rc, IDs: e.ids, Entitlements: namedLimits{app.LimitMaxUnpublishedLiturgies: 1}}
		rc.hook = func() {
			done := make(chan error, 1)
			go func() {
				_, err := e.liturgies.Review(e.ctx, e.admin, b, app.ReviewInput{Action: domain.ActionReopen})
				done <- err
			}()
			select {
			case second = <-done: // not held back: it did not wait for the lock
			case <-time.After(time.Second): // waiting for the church lock, as it should
			}
		}
		if _, err := l.Review(e.ctx, e.admin, a, app.ReviewInput{Action: domain.ActionReopen}); err != nil {
			t.Fatalf("first reopen: %v", err)
		}
		unpublished := 0
		for _, id := range []domain.LiturgyID{a, b} {
			if e.liturgy(id).Liturgy.State != domain.StatePublished {
				unpublished++
			}
		}
		if unpublished != 1 {
			t.Errorf("%d liturgies reopened with a limit of 1 (the second ended with %v)", unpublished, second)
		}
	})
}
