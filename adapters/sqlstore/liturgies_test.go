// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/entitlements/unlimited"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func sectionOf(song, suffix string) domain.SectionID { return domain.SectionID(id(song)[:24] + suffix) }

// liturgyOf is a draft liturgy of the service SV?1 on 2026-10-18 at 07:00.
func liturgyOf(lid, service, by string, now time.Time) domain.Liturgy {
	return domain.Liturgy{ID: domain.LiturgyID(id(lid)), Date: "2026-10-18", Time: "07:00", ServiceID: domain.ServiceID(id(service)),
		ServiceName: "Ibadah Umum", Language: "id", State: domain.StateDraft, Version: 1, CreatedBy: domain.UserID(id(by)),
		CreatedAt: now, UpdatedAt: now}
}

func itemOf(iid, lid string, pos int, typ domain.ItemType, now time.Time) domain.Item {
	return domain.Item{ID: domain.ItemID(id(iid)), LiturgyID: domain.LiturgyID(id(lid)), Position: pos, Title: "Item " + iid,
		Type: typ, Version: 1, CreatedAt: now, UpdatedAt: now}
}

// songOf is an item song of the song with a two-entry sequence (verse, chorus).
func songOf(sid, song string, part string) domain.LiturgySong {
	s := domain.LiturgySong{ID: domain.ItemSongID(id(sid)), SongID: domain.SongID(id(song)), SongTitle: "Cinta Tuhan", Key: "G",
		Entries: []domain.Entry{
			{ID: domain.EntryID(id(sid + "E1")), SectionID: sectionOf(song, "V1"), SectionLabel: "Verse 1"},
			{ID: domain.EntryID(id(sid + "E2")), Position: 1, SectionID: sectionOf(song, "C1"), SectionLabel: "Chorus"}}}
	if part != "" {
		s.Entries[1].SingingPartID = domain.SingingPartID(id(part))
	}
	return s
}

// seedLiturgies gives churches A and B one liturgy each, built on the rows of
// seedPlanning and the songs and readings of the isolation test. A's holds a
// song item (with a part), a prayer item with a duty, a reading item, an
// assignment and one history row; B's the same with its own rows.
func seedLiturgies(a, b app.ChurchStore, now time.Time) error {
	var errs []error
	for _, c := range []struct {
		cs                                        app.ChurchStore
		l, svc, song, reading, duty, part, suffix string
	}{
		{a, "LGA1", "SVA1", "SA1", "RDA1", "DUA1", "PTA1", "A"},
		{b, "LGB1", "SVB1", "SB1", "RDB1", "DUB1", "PTB1", "B"},
	} {
		l := liturgyOf(c.l, c.svc, "U1", now)
		song := itemOf("ITS"+c.suffix, c.l, 0, domain.ItemSong, now)
		song.Songs = []domain.LiturgySong{songOf("IS"+c.suffix, c.song, c.part)}
		prayer := itemOf("ITP"+c.suffix, c.l, 1, domain.ItemPrayer, now)
		prayer.DutyID, prayer.Text = domain.DutyID(id(c.duty)), "Doa"
		reading := itemOf("ITR"+c.suffix, c.l, 2, domain.ItemReading, now)
		reading.ReadingID, reading.ReadingLabel = domain.ReadingID(id(c.reading)), "Yohanes 3:16 (TB)"
		errs = append(errs, c.cs.Liturgies().Create(ctx, l),
			c.cs.LiturgyItems().Insert(ctx, song), c.cs.LiturgyItems().Insert(ctx, prayer), c.cs.LiturgyItems().Insert(ctx, reading),
			c.cs.Assignments().Add(ctx, domain.Assignment{ID: domain.AssignmentID(id("AS" + c.suffix)), LiturgyID: l.ID,
				DutyID: domain.DutyID(id(c.duty)), UserID: domain.UserID(id("U1")), CreatedAt: now}))
		seq, err := c.cs.Liturgies().NextSeq(ctx, l.ID)
		errs = append(errs, err, c.cs.StateChanges().Append(ctx, domain.StateChange{ID: domain.StateChangeID(id("SC" + c.suffix)), LiturgyID: l.ID,
			From: domain.StateDraft, To: domain.StateInReview, UserID: domain.UserID(id("U1")), Note: "n", EditSeq: seq, CreatedAt: now}))
		errs = append(errs, err, c.cs.Edits().Append(ctx, domain.Edit{ID: domain.EditID(id("ED" + c.suffix)), LiturgyID: l.ID, Seq: seq,
			UserID: domain.UserID(id("U1")), Command: domain.CmdLiturgyCreate, After: []byte(`{}`), LiturgyVersionAfter: 1,
			Status: domain.EditDone, CreatedAt: now}))
		// An editing row in both churches, and in B an undone one, so that the
		// undo queries have something of B's to leak (IT-L-008).
		seq, err = c.cs.Liturgies().NextSeq(ctx, l.ID)
		errs = append(errs, err, c.cs.Edits().Append(ctx, domain.Edit{ID: domain.EditID(id("ED" + c.suffix + "2")), LiturgyID: l.ID, Seq: seq,
			UserID: domain.UserID(id("U1")), Command: domain.CmdItemUpdate, ItemID: domain.ItemID(id("ITP" + c.suffix)),
			Before: []byte(`{"text":""}`), After: []byte(`{"text":"Doa"}`), LiturgyVersionAfter: 1, ItemVersionAfter: 2,
			Status: domain.EditDone, CreatedAt: now}))
		if c.suffix == "B" {
			seq, err = c.cs.Liturgies().NextSeq(ctx, l.ID)
			errs = append(errs, err, c.cs.Edits().Append(ctx, domain.Edit{ID: domain.EditID(id("EDB3")), LiturgyID: l.ID, Seq: seq,
				UserID: domain.UserID(id("U1")), Command: domain.CmdItemUpdate, ItemID: domain.ItemID(id("ITPB")),
				Before: []byte(`{"text":"Doa"}`), After: []byte(`{"text":"Amin"}`), LiturgyVersionAfter: 1, ItemVersionAfter: 3,
				Status: domain.EditUndone, UndoSeq: seq + 1, CreatedAt: now}))
		}
	}
	return errors.Join(errs...)
}

// liturgyHarness are the IT-L-008 entries: each method called through church A
// sees and changes only A's rows.
func liturgyHarness(errRollback error, notFound func(error) error, unchanged func(bool, error) error) map[string]func(app.ChurchStore, time.Time) error {
	referenced := func(what string, err error) error {
		if !errors.Is(err, app.ErrReferenced) {
			return fmt.Errorf("%s through A: want ErrReferenced, got %w", what, err)
		}
		return errRollback
	}
	lgA, lgB := domain.LiturgyID(id("LGA1")), domain.LiturgyID(id("LGB1"))
	itB, isB := domain.ItemID(id("ITSB")), domain.ItemSongID(id("ISB"))
	h := map[string]func(app.ChurchStore, time.Time) error{}
	h["Liturgies.Create"] = func(cs app.ChurchStore, now time.Time) error {
		if err := cs.Liturgies().Create(ctx, liturgyOf("LGX", "SVB1", "U1", now)); !errors.Is(err, app.ErrReferenced) {
			return fmt.Errorf("church B's service through A: %w", err)
		}
		return errRollback
	}
	h["Liturgies.ByID"] = func(cs app.ChurchStore, _ time.Time) error {
		if got, err := cs.Liturgies().ByID(ctx, lgA); err != nil || got.ServiceID != domain.ServiceID(id("SVA1")) || got.EditSeq != 2 {
			return fmt.Errorf("own: %+v %w", got, err)
		}
		_, err := cs.Liturgies().ByID(ctx, lgB)
		return notFound(err)
	}
	h["Liturgies.List"] = func(cs app.ChurchStore, _ time.Time) error {
		rows, total, err := cs.Liturgies().List(ctx, app.LiturgyFilter{Limit: 50})
		if err != nil || total != 1 || len(rows) != 1 || rows[0].Liturgy.ID != lgA || rows[0].ItemCount != 3 {
			return fmt.Errorf("list: %+v %d %w", rows, total, err)
		}
		return nil
	}
	h["Liturgies.CountActive"] = func(cs app.ChurchStore, _ time.Time) error {
		n, err := cs.Liturgies().CountActive(ctx, nil)
		if err != nil || n != 1 {
			return fmt.Errorf("count %d: %w", n, err)
		}
		return nil
	}
	h["Liturgies.Slots"] = func(cs app.ChurchStore, _ time.Time) error {
		slots, err := cs.Liturgies().Slots(ctx, "2026-10-01", "2026-10-31")
		if err != nil || len(slots) != 1 || slots[app.Slot{ServiceID: domain.ServiceID(id("SVA1")), Date: "2026-10-18", Time: "07:00"}] != lgA {
			return fmt.Errorf("slots: %v %w", slots, err)
		}
		return nil
	}
	h["Liturgies.Update"] = func(cs app.ChurchStore, now time.Time) error {
		l := liturgyOf("LGB1", "SVB1", "U1", now)
		l.ServiceName = "Hacked"
		return unchanged(cs.Liturgies().Update(ctx, l, 1))
	}
	h["Liturgies.Bump"] = func(cs app.ChurchStore, now time.Time) error {
		return unchanged(cs.Liturgies().Bump(ctx, lgB, 1, now))
	}
	h["Liturgies.NextSeq"] = func(cs app.ChurchStore, _ time.Time) error {
		if _, err := cs.Liturgies().NextSeq(ctx, lgB); !errors.Is(err, app.ErrNoSeq) {
			return fmt.Errorf("church B's liturgy through A: want ErrNoSeq, got %v", err)
		}
		return nil
	}
	h["Liturgies.Transition"] = func(cs app.ChurchStore, now time.Time) error {
		_, ok, err := cs.Liturgies().Transition(ctx, lgB, domain.StateDraft, domain.StateInReview, 0, now)
		return unchanged(ok, err)
	}
	h["StateChanges.Append"] = func(cs app.ChurchStore, now time.Time) error {
		return referenced("a state change in church B's liturgy", cs.StateChanges().Append(ctx, domain.StateChange{ID: domain.StateChangeID(id("SCX")),
			LiturgyID: lgB, From: domain.StateDraft, To: domain.StateInReview, UserID: domain.UserID(id("U1")), CreatedAt: now}))
	}
	h["StateChanges.List"] = func(cs app.ChurchStore, _ time.Time) error {
		own, total, err := cs.StateChanges().List(ctx, lgA, 10, 0)
		other, total2, err2 := cs.StateChanges().List(ctx, lgB, 10, 0)
		if err != nil || err2 != nil || len(own) != 1 || total != 1 || len(other) != 0 || total2 != 0 {
			return fmt.Errorf("state changes: %+v %d %+v %d %w %w", own, total, other, total2, err, err2)
		}
		return nil
	}
	h["StateChanges.Last"] = func(cs app.ChurchStore, _ time.Time) error {
		if c, err := cs.StateChanges().Last(ctx, lgA); err != nil || c.ID != domain.StateChangeID(id("SCA")) {
			return fmt.Errorf("own: %+v %w", c, err)
		}
		_, err := cs.StateChanges().Last(ctx, lgB)
		return notFound(err)
	}
	h["Liturgies.Delete"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.Liturgies().Delete(ctx, lgB))
	}
	h["LiturgyItems.ByLiturgy"] = func(cs app.ChurchStore, _ time.Time) error {
		items, err := cs.LiturgyItems().ByLiturgy(ctx, lgA)
		if err != nil || len(items) != 3 || len(items[0].Songs) != 1 || len(items[0].Songs[0].Entries) != 2 ||
			items[0].Songs[0].Entries[1].SingingPartID != domain.SingingPartID(id("PTA1")) || items[1].DutyID != domain.DutyID(id("DUA1")) ||
			items[2].ReadingID != domain.ReadingID(id("RDA1")) {
			return fmt.Errorf("own: %+v %w", items, err)
		}
		other, err := cs.LiturgyItems().ByLiturgy(ctx, lgB)
		if err != nil || len(other) != 0 {
			return fmt.Errorf("church B's items: %+v %w", other, err)
		}
		return nil
	}
	h["LiturgyItems.ByID"] = func(cs app.ChurchStore, _ time.Time) error {
		if _, err := cs.LiturgyItems().ByID(ctx, lgA, domain.ItemID(id("ITSA"))); err != nil {
			return err
		}
		_, err := cs.LiturgyItems().ByID(ctx, lgB, itB)
		return notFound(err)
	}
	h["LiturgyItems.IDs"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.LiturgyItems().IDs(ctx, lgA)
		other, err2 := cs.LiturgyItems().IDs(ctx, lgB)
		if err != nil || err2 != nil || len(own) != 3 || len(other) != 0 {
			return fmt.Errorf("ids: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["LiturgyItems.Insert"] = func(cs app.ChurchStore, now time.Time) error {
		return referenced("an item in church B's liturgy", cs.LiturgyItems().Insert(ctx, itemOf("ITX", "LGB1", 5, domain.ItemPrayer, now)))
	}
	h["LiturgyItems.SetPositions"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.LiturgyItems().SetPositions(ctx, lgB, []domain.ItemID{itB}))
	}
	h["LiturgyItems.Delete"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.LiturgyItems().Delete(ctx, lgB, itB))
	}
	h["LiturgyItems.Update"] = func(cs app.ChurchStore, now time.Time) error {
		it := itemOf("ITSB", "LGB1", 0, domain.ItemSong, now)
		it.Title = "Hacked"
		return unchanged(cs.LiturgyItems().Update(ctx, it, 1))
	}
	h["LiturgyItems.Bump"] = func(cs app.ChurchStore, now time.Time) error {
		return unchanged(cs.LiturgyItems().Bump(ctx, itB, 1, now))
	}
	h["LiturgyItems.InsertSong"] = func(cs app.ChurchStore, _ time.Time) error {
		return referenced("a song in church B's item", cs.LiturgyItems().InsertSong(ctx, itB, songOf("ISX", "SA1", "")))
	}
	h["LiturgyItems.SetSongPositions"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.LiturgyItems().SetSongPositions(ctx, itB, []domain.ItemSongID{isB}))
	}
	h["LiturgyItems.ReplaceSongs"] = func(cs app.ChurchStore, _ time.Time) error {
		return referenced("songs in church B's item", cs.LiturgyItems().ReplaceSongs(ctx, itB, []domain.LiturgySong{songOf("ISX", "SA1", "")}))
	}
	h["Assignments.Add"] = func(cs app.ChurchStore, now time.Time) error {
		return referenced("an assignment in church B's liturgy", cs.Assignments().Add(ctx, domain.Assignment{ID: domain.AssignmentID(id("ASX")),
			LiturgyID: lgB, DutyID: domain.DutyID(id("DUA1")), Name: "X", NameKey: "x", CreatedAt: now}))
	}
	h["Assignments.ByID"] = func(cs app.ChurchStore, _ time.Time) error {
		if _, err := cs.Assignments().ByID(ctx, lgA, domain.AssignmentID(id("ASA"))); err != nil {
			return err
		}
		_, err := cs.Assignments().ByID(ctx, lgB, domain.AssignmentID(id("ASB")))
		return notFound(err)
	}
	h["Assignments.ByLiturgy"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Assignments().ByLiturgy(ctx, lgA)
		other, err2 := cs.Assignments().ByLiturgy(ctx, lgB)
		if err != nil || err2 != nil || len(own) != 1 || own[0].UserID != domain.UserID(id("U1")) || len(other) != 0 {
			return fmt.Errorf("assignments: %+v %+v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Assignments.Count"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Assignments().Count(ctx, lgA)
		other, err2 := cs.Assignments().Count(ctx, lgB)
		if err != nil || err2 != nil || own != 1 || other != 0 {
			return fmt.Errorf("counts: %d %d %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Assignments.Remove"] = func(cs app.ChurchStore, _ time.Time) error {
		return notFound(cs.Assignments().Remove(ctx, lgB, domain.AssignmentID(id("ASB"))))
	}
	h["Edits.Append"] = func(cs app.ChurchStore, now time.Time) error {
		return referenced("a history row in church B's liturgy", cs.Edits().Append(ctx, domain.Edit{ID: domain.EditID(id("EDX")), LiturgyID: lgB, Seq: 9,
			UserID: domain.UserID(id("U1")), Command: domain.CmdLiturgyUpdate, LiturgyVersionAfter: 2, Status: domain.EditDone, CreatedAt: now}))
	}
	h["Edits.List"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Edits().List(ctx, lgA, 10)
		other, err2 := cs.Edits().List(ctx, lgB, 10)
		if err != nil || err2 != nil || len(own) != 2 || own[0].Command != domain.CmdItemUpdate || len(other) != 0 {
			return fmt.Errorf("history: %+v %+v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Edits.BySeq"] = func(cs app.ChurchStore, _ time.Time) error {
		if e, err := cs.Edits().BySeq(ctx, lgA, 2); err != nil || e.ID != domain.EditID(id("EDA2")) {
			return fmt.Errorf("own: %+v %w", e, err)
		}
		_, err := cs.Edits().BySeq(ctx, lgB, 2)
		return notFound(err)
	}
	h["Edits.LastActing"] = func(cs app.ChurchStore, _ time.Time) error {
		_, err := cs.Edits().LastActing(ctx, lgB, domain.EditID(id("EDB2")))
		return notFound(err)
	}
	h["Edits.Newest"] = func(cs app.ChurchStore, _ time.Time) error {
		if e, err := cs.Edits().Newest(ctx, domain.UserID(id("U1")), lgA, 0, 50); err != nil || e.ID != domain.EditID(id("EDA2")) {
			return fmt.Errorf("own: %+v %w", e, err)
		}
		_, err := cs.Edits().Newest(ctx, domain.UserID(id("U1")), lgB, 0, 50)
		return notFound(err)
	}
	h["Edits.NewestUndone"] = func(cs app.ChurchStore, _ time.Time) error {
		_, err := cs.Edits().NewestUndone(ctx, domain.UserID(id("U1")), lgB, 0)
		return notFound(err)
	}
	h["Edits.Foreign"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Edits().Foreign(ctx, lgA, 0, domain.UserID(id("U2")))
		other, err2 := cs.Edits().Foreign(ctx, lgB, 0, domain.UserID(id("U2")))
		if err != nil || err2 != nil || len(own) != 2 || len(other) != 0 {
			return fmt.Errorf("foreign: %+v %+v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Edits.SetStatus"] = func(cs app.ChurchStore, _ time.Time) error {
		return unchanged(cs.Edits().SetStatus(ctx, domain.EditID(id("EDB2")), domain.EditDone, domain.EditUndone, 9))
	}
	h["Edits.MarkSkipped"] = func(cs app.ChurchStore, _ time.Time) error {
		return cs.Edits().MarkSkipped(ctx, domain.EditID(id("EDB2"))) // church B's row stays as it is, which the runner checks
	}
	h["Edits.DropUndone"] = func(cs app.ChurchStore, _ time.Time) error {
		return cs.Edits().DropUndone(ctx, domain.UserID(id("U1")), lgB)
	}
	h["Usage.SongInUse"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Usage().SongInUse(ctx, domain.SongID(id("SA1")))
		other, err2 := cs.Usage().SongInUse(ctx, domain.SongID(id("SB1")))
		if err != nil || err2 != nil || !own || other {
			return fmt.Errorf("song in use: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Usage.SectionsInUse"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Usage().SectionsInUse(ctx, domain.SongID(id("SA1")), []domain.SectionID{sectionOf("SA1", "V1"), "01ARZ3NDEKTSV4RRFFQ69G5FAV"})
		other, err2 := cs.Usage().SectionsInUse(ctx, domain.SongID(id("SB1")), []domain.SectionID{sectionOf("SB1", "V1")})
		if err != nil || err2 != nil || len(own) != 1 || own[0] != sectionOf("SA1", "V1") || len(other) != 0 {
			return fmt.Errorf("sections in use: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Usage.ReadingInUse"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Usage().ReadingInUse(ctx, domain.ReadingID(id("RDA1")))
		other, err2 := cs.Usage().ReadingInUse(ctx, domain.ReadingID(id("RDB1")))
		if err != nil || err2 != nil || !own || other {
			return fmt.Errorf("reading in use: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Usage.DutyInUse"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Usage().DutyInUse(ctx, id("DUA1"))
		other, err2 := cs.Usage().DutyInUse(ctx, id("DUB1"))
		if err != nil || err2 != nil || !own || other {
			return fmt.Errorf("duty in use: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	h["Usage.SingingPartInUse"] = func(cs app.ChurchStore, _ time.Time) error {
		own, err := cs.Usage().SingingPartInUse(ctx, id("PTA1"))
		other, err2 := cs.Usage().SingingPartInUse(ctx, id("PTB1"))
		if err != nil || err2 != nil || !own || other {
			return fmt.Errorf("part in use: %v %v %w %w", own, other, err, err2)
		}
		return nil
	}
	return h
}

// --- the states of a liturgy and the usage rules, through the use cases ---

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// lenv wires the use cases on church A of the base fixture, whose user U1
// holds every scope the tests need.
type lenv struct {
	t        *testing.T
	f        fixture
	ctx      context.Context
	sess     *domain.Session
	liturgy  *app.Liturgies
	songs    *app.Songs
	readings *app.Readings
	vocab    *app.Vocabulary
}

func newLenv(t *testing.T, db *sqlstore.DB) lenv {
	t.Helper()
	f := base(t, db)
	for _, s := range []string{"library.edit", "templates.edit", "liturgy.edit", "liturgy.comment", "liturgy.approve", "liturgy.manage"} {
		f.must(`INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, ?)`, id("CHA"), id("RA"), s)
	}
	c, ids := fixedClock{f.now}, ulidgen.New()
	return lenv{t: t, f: f, ctx: app.WithTenant(ctx, domain.ChurchID(id("CHA"))), sess: &domain.Session{UserID: domain.UserID(id("U1"))},
		liturgy:  &app.Liturgies{Tx: db, Clock: c, IDs: ids, Entitlements: unlimited.Entitlements{}},
		songs:    &app.Songs{Tx: db, Clock: c, IDs: ids},
		readings: &app.Readings{Tx: db, Clock: c, IDs: ids},
		vocab:    &app.Vocabulary{Tx: db, Clock: c, IDs: ids}}
}

func (e lenv) draft(name string) domain.LiturgyID {
	e.t.Helper()
	v, err := e.liturgy.Create(e.ctx, e.sess, app.CreateInput{Date: "2026-10-11", ServiceName: name, TemplateID: ptr(domain.TemplateID(""))})
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Liturgy.ID
}

func ptr[T any](v T) *T { return &v }

func (e lenv) setState(lid domain.LiturgyID, state domain.LiturgyState) {
	e.t.Helper()
	e.f.must(`UPDATE liturgies SET state = ? WHERE id = ?`, string(state), string(lid))
}

func (e lenv) get(lid domain.LiturgyID) app.LiturgyView {
	e.t.Helper()
	v, err := e.liturgy.Get(e.ctx, e.sess, lid)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e lenv) song(title string) domain.Song {
	e.t.Helper()
	v, err := e.songs.Create(e.ctx, e.sess, app.SongInput{Language: "id", Title: title, DefaultKey: "G",
		Sections: []app.SectionInput{{Key: "v1", Kind: domain.SectionVerse, Number: 1, Text: "bait satu"},
			{Key: "c", Kind: domain.SectionChorus, Text: "reff"}, {Key: "v2", Kind: domain.SectionVerse, Number: 2, Text: "bait dua"}},
		Arrangement: []string{"v1", "c", "v2"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return v.Song
}

func (e lenv) item(lid domain.LiturgyID, typ domain.ItemType) domain.ItemID {
	e.t.Helper()
	res, err := e.liturgy.AddItem(e.ctx, e.sess, lid, app.ItemInput{LiturgyVersion: e.get(lid).Liturgy.Version, Title: "Item", Type: typ})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Item.Item.ID
}

func invalidField(err error) string {
	var in *domain.InvalidInputError
	if errors.As(err, &in) {
		return in.Field
	}
	return ""
}

// IT-L-007: every write route of a liturgy that is not editable.
func TestLiturgyLockedStates(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newLenv(t, db)
		L := e.liturgy
		song := e.song("Besar Setia-Mu")
		duty, err := e.vocab.Create(e.ctx, e.sess, domain.KindDuty, "Liturgis")
		if err != nil {
			t.Fatal(err)
		}
		lid := e.draft("Ibadah")
		si := e.item(lid, domain.ItemSong)
		if _, err := L.AddSong(e.ctx, e.sess, lid, si, 1, song.ID, nil); err != nil {
			t.Fatal(err)
		}
		a, err := L.AddAssignment(e.ctx, e.sess, lid, app.AssignmentInput{DutyID: domain.DutyID(duty.Entry.ID), Name: "Pak Yan"})
		if err != nil {
			t.Fatal(err)
		}
		before := e.get(lid)
		hist := func() int {
			h, err := L.Edits(e.ctx, e.sess, lid, 100)
			if err != nil {
				t.Fatal(err)
			}
			return len(h)
		}
		rows := hist()

		for _, state := range []domain.LiturgyState{domain.StateInReview, domain.StateApproved, domain.StatePublished} {
			e.setState(lid, state)
			title, date := "x", "2026-10-18"
			v := before.Liturgy.Version
			// The state is checked before the version: a stale version is still "locked".
			stale := 99
			for name, err := range map[string]error{
				"update": func() error {
					_, err := L.Update(e.ctx, e.sess, lid, app.LiturgyChange{Version: v, Date: &date})
					return err
				}(),
				"update/stale": func() error {
					_, err := L.Update(e.ctx, e.sess, lid, app.LiturgyChange{Version: stale, Date: &date})
					return err
				}(),
				"add item": func() error {
					_, err := L.AddItem(e.ctx, e.sess, lid, app.ItemInput{LiturgyVersion: v, Title: "x", Type: domain.ItemPrayer})
					return err
				}(),
				"update item": func() error {
					_, err := L.UpdateItem(e.ctx, e.sess, lid, si, app.ItemChange{Version: before.Items[0].Item.Version, Title: &title})
					return err
				}(),
				"remove item": func() error { _, err := L.RemoveItem(e.ctx, e.sess, lid, si, v); return err }(),
				"reorder":     func() error { _, err := L.ReorderItems(e.ctx, e.sess, lid, v, []domain.ItemID{si}); return err }(),
				"add song": func() error {
					_, err := L.AddSong(e.ctx, e.sess, lid, si, before.Items[0].Item.Version, song.ID, nil)
					return err
				}(),
				"set songs": func() error {
					_, err := L.SetSongs(e.ctx, e.sess, lid, si, before.Items[0].Item.Version, nil)
					return err
				}(),
				"assign": func() error {
					_, err := L.AddAssignment(e.ctx, e.sess, lid, app.AssignmentInput{DutyID: domain.DutyID(duty.Entry.ID), Name: "Lain"})
					return err
				}(),
				"unassign": L.RemoveAssignment(e.ctx, e.sess, lid, a.Assignment.ID),
			} {
				if !errors.Is(err, app.ErrLiturgyLocked) {
					t.Errorf("%s in %s: %v", name, state, err)
				}
			}
			got := e.get(lid)
			if got.Liturgy.Version != v || len(got.Items) != 1 || len(got.Assignments) != 1 || hist() != rows || got.Actions.Edit || got.Items[0].Item.Version != before.Items[0].Item.Version {
				t.Errorf("%s: a refused write changed the liturgy: %+v", state, got.Liturgy)
			}
			if state == domain.StatePublished {
				if err := L.Delete(e.ctx, e.sess, lid); !errors.Is(err, app.ErrLiturgyNotDeletable) {
					t.Errorf("delete a published liturgy: %v", err)
				}
				if got.Actions.Delete {
					t.Error("a published liturgy offers delete")
				}
			}
			if page, err := L.List(e.ctx, e.sess, app.LiturgyFilter{State: state, Limit: 10}); err != nil || page.Total != 1 {
				t.Errorf("list in %s: %+v %v", state, page, err)
			}
		}
		// needs_revision is editable again; draft, in review and approved can be deleted.
		e.setState(lid, domain.StateNeedsRevision)
		if _, err := L.AddItem(e.ctx, e.sess, lid, app.ItemInput{LiturgyVersion: before.Liturgy.Version, Title: "x", Type: domain.ItemPrayer}); err != nil {
			t.Errorf("edit in needs_revision: %v", err)
		}
		for _, state := range []domain.LiturgyState{domain.StateDraft, domain.StateInReview, domain.StateNeedsRevision, domain.StateApproved} {
			other := e.draft("Hapus " + string(state))
			e.setState(other, state)
			if err := L.Delete(e.ctx, e.sess, other); err != nil {
				t.Errorf("delete in %s: %v", state, err)
			}
		}
	})
}

// IT-L-006, TC-L-012: usage rules, and what deleting a song, section or
// reading used only by a published liturgy leaves behind.
func TestUsageAndSnapshots(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newLenv(t, db)
		L := e.liturgy
		song := e.song("Besar Setia-Mu")
		other := e.song("Lagu lain")
		reading, err := e.readings.Create(e.ctx, e.sess, app.ReadingInput{Reference: "Yoh 3:16-21", Translation: "TB", Text: "Karena begitu besar"})
		if err != nil {
			t.Fatal(err)
		}
		duty, _ := e.vocab.Create(e.ctx, e.sess, domain.KindDuty, "Liturgis")
		part, _ := e.vocab.Create(e.ctx, e.sess, domain.KindSingingPart, "Semua")
		chorus := song.Sections[1].ID

		lid := e.draft("Ibadah")
		si := e.item(lid, domain.ItemSong)
		if _, err := L.SetSongs(e.ctx, e.sess, lid, si, 1, []app.ItemSongInput{{SongID: song.ID, Entries: []app.EntryInput{
			{SectionID: song.Sections[0].ID}, {SectionID: chorus, SingingPartID: domain.SingingPartID(part.Entry.ID)}}}}); err != nil {
			t.Fatal(err)
		}
		ri := e.item(lid, domain.ItemReading)
		if _, err := L.UpdateItem(e.ctx, e.sess, lid, ri, app.ItemChange{Version: 1, ReadingID: &reading.Reading.ID}); err != nil {
			t.Fatal(err)
		}
		pi, err := L.AddItem(e.ctx, e.sess, lid, app.ItemInput{LiturgyVersion: e.get(lid).Liturgy.Version, Title: "Doa", Type: domain.ItemPrayer, DutyID: domain.DutyID(duty.Entry.ID)})
		if err != nil {
			t.Fatal(err)
		}

		// In an unpublished liturgy everything is in use.
		if err := e.songs.Delete(e.ctx, e.sess, song.ID); !errors.Is(err, app.ErrSongInUse) {
			t.Errorf("song: %v", err)
		}
		without := []app.SectionInput{{ID: song.Sections[0].ID, Kind: domain.SectionVerse, Number: 1, Text: "bait satu"},
			{ID: song.Sections[2].ID, Kind: domain.SectionVerse, Number: 2, Text: "bait dua"}}
		var busy *app.SectionInUseError
		if _, err := e.songs.Update(e.ctx, e.sess, song.ID, app.SongChange{Version: 1, Sections: &without}); !errors.As(err, &busy) || len(busy.IDs) != 1 || busy.IDs[0] != chorus {
			t.Errorf("section: %v", err)
		}
		if err := e.readings.Delete(e.ctx, e.sess, reading.Reading.ID); !errors.Is(err, app.ErrReadingInUse) {
			t.Errorf("reading: %v", err)
		}
		if err := e.vocab.Delete(e.ctx, e.sess, domain.KindDuty, duty.Entry.ID); !errors.Is(err, app.ErrDutyInUse) {
			t.Errorf("duty: %v", err)
		}
		if err := e.vocab.Delete(e.ctx, e.sess, domain.KindSingingPart, part.Entry.ID); !errors.Is(err, app.ErrSingingPartInUse) {
			t.Errorf("part: %v", err)
		}
		// An unused song and a song in no liturgy go.
		if err := e.songs.Delete(e.ctx, e.sess, other.ID); err != nil {
			t.Errorf("unused song: %v", err)
		}

		// Published: the references go to null at delete, the snapshots stay.
		e.setState(lid, domain.StatePublished)
		if _, err := e.songs.Update(e.ctx, e.sess, song.ID, app.SongChange{Version: 1, Sections: &without}); err != nil {
			t.Fatalf("removing a section used only by a published liturgy: %v", err)
		}
		got := e.get(lid)
		entries := got.Items[0].Songs[0].Song.Entries
		if entries[0].SectionID == "" || entries[1].SectionID != "" || entries[1].SectionLabel != "Chorus" || entries[1].SingingPartID == "" {
			t.Errorf("entries after the section was removed: %+v", entries)
		}
		if err := e.readings.Delete(e.ctx, e.sess, reading.Reading.ID); err != nil {
			t.Fatalf("deleting a reading used only by a published liturgy: %v", err)
		}
		if err := e.songs.Delete(e.ctx, e.sess, song.ID); err != nil {
			t.Fatalf("deleting a song used only by a published liturgy: %v", err)
		}
		got = e.get(lid)
		s := got.Items[0].Songs[0]
		if s.Song.SongID != "" || s.Song.SongTitle != "Besar Setia-Mu" || s.Summary != nil || len(s.Song.Entries) != 2 ||
			s.Song.Entries[0].SectionID != "" || s.Song.Entries[0].SectionLabel != "Verse 1" {
			t.Errorf("song after delete: %+v", s)
		}
		if r := got.Items[1]; r.Item.ReadingID != "" || r.Item.ReadingLabel != "Yohanes 3:16-21 (TB)" || r.Reading != nil {
			t.Errorf("reading after delete: %+v", r)
		}
		codes := map[string]int{}
		for _, p := range got.Problems {
			codes[p.Code]++
		}
		if codes[domain.ProblemSongRemoved] != 1 || codes[domain.ProblemSectionRemoved] != 2 || codes[domain.ProblemReadingRemoved] != 1 || len(got.Problems) != 4 {
			t.Errorf("problems: %v", got.Problems)
		}
		// A used duty or part is never cleared: its delete stays refused.
		if err := e.vocab.Delete(e.ctx, e.sess, domain.KindDuty, duty.Entry.ID); !errors.Is(err, app.ErrDutyInUse) {
			t.Errorf("duty in a published liturgy: %v", err)
		}
		if err := e.vocab.Delete(e.ctx, e.sess, domain.KindSingingPart, part.Entry.ID); !errors.Is(err, app.ErrSingingPartInUse) {
			t.Errorf("part in a published liturgy: %v", err)
		}

		// The editor can replace each removed reference (the liturgy is reopened).
		e.setState(lid, domain.StateDraft)
		fresh := e.song("Pengganti")
		cur := e.get(lid)
		if _, err := L.SetSongs(e.ctx, e.sess, lid, si, cur.Items[0].Item.Version, []app.ItemSongInput{{SongID: fresh.ID,
			Entries: []app.EntryInput{{SectionID: fresh.Sections[0].ID}}}}); err != nil {
			t.Fatal(err)
		}
		again, err := e.readings.Create(e.ctx, e.sess, app.ReadingInput{Reference: "Mzm 23", Translation: "TB", Text: "TUHAN adalah gembalaku"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := L.UpdateItem(e.ctx, e.sess, lid, ri, app.ItemChange{Version: cur.Items[1].Item.Version, ReadingID: &again.Reading.ID}); err != nil {
			t.Fatal(err)
		}
		if p := e.get(lid).Problems; len(p) != 0 {
			t.Errorf("problems after replacing: %v", p)
		}
		_ = pi
	})
}

const (
	rawLiturgy = `INSERT INTO liturgies (id, church_id, date, time, service_id, service_name, language, state, version, created_by,
		edit_seq, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'Ibadah', ?, ?, ?, ?, 0, ?, ?)`
	rawItem = `INSERT INTO liturgy_items (id, church_id, liturgy_id, position, title, item_type, duty_id, text, reading_id,
		reading_label, version, created_at, updated_at) VALUES (?, ?, ?, ?, 'T', ?, ?, ?, ?, ?, 1, ?, ?)`
	rawItemSong = `INSERT INTO liturgy_item_songs (id, church_id, item_id, position, song_id, song_title, key, note) VALUES (?, ?, ?, ?, ?, 'T', '', '')`
	rawEntry    = `INSERT INTO sequence_entries (id, church_id, item_song_id, position, kind, song_section_id, singing_part_id, key_change, section_label, note)
		VALUES (?, ?, ?, ?, ?, ?, NULL, '', '', '')`
	rawAssign = `INSERT INTO assignments (id, church_id, liturgy_id, duty_id, user_id, name, name_key, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	rawEdit   = `INSERT INTO liturgy_edits (id, church_id, liturgy_id, user_id, seq, command, liturgy_version_after, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`
)

func (e lenv) count(table string) int {
	e.t.Helper()
	var n int
	err := e.f.db.Read(ctx, func(s app.Store) error {
		return sqlstore.RawTx(s).GetContext(ctx, &n, "SELECT COUNT(*) FROM "+table)
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// IT-L-009: the database itself rejects rows the application never writes, and
// deleting a liturgy removes its rows.
func TestLiturgyConstraints(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newLenv(t, db)
		f := e.f
		song := e.song("Besar Setia-Mu")
		reading, _ := e.readings.Create(e.ctx, e.sess, app.ReadingInput{Reference: "Yoh 3:16", Translation: "TB", Text: "x"})
		duty, _ := e.vocab.Create(e.ctx, e.sess, domain.KindDuty, "Liturgis")
		if err := f.template("TPL1", "CHA", "Ibadah", "id", 1); err != nil {
			t.Fatal(err)
		}
		if err := f.service("SVC1", "CHA", "Ibadah Umum", id("TPL1"), 1); err != nil {
			t.Fatal(err)
		}
		cha, chb, u1 := id("CHA"), id("CHB"), id("U1")
		lg := func(lid, church string, service any, date, clock, lang, state string, version int) error {
			return f.exec(rawLiturgy, id(lid), church, date, clock, service, lang, state, version, u1, f.ts(0), f.ts(0))
		}
		svc := id("SVC1")
		if err := lg("L1", cha, svc, "2026-10-11", "07:00", "id", "draft", 1); err != nil {
			t.Fatal(err)
		}
		if err := lg("L2", cha, nil, "2026-10-11", "", "id", "draft", 1); err != nil { // one-offs are not limited
			t.Fatal(err)
		}
		if err := lg("L3", cha, nil, "2026-10-11", "", "id", "draft", 1); err != nil {
			t.Fatal(err)
		}
		var uq *app.UniqueError
		if err := lg("L4", cha, svc, "2026-10-11", "07:00", "id", "draft", 1); !errors.As(err, &uq) || uq.Constraint != "liturgies_service_slot_key" {
			t.Errorf("two liturgies in a slot: %v", err)
		}
		if err := lg("L5", cha, svc, "2026-10-11", "09:00", "id", "draft", 1); err != nil {
			t.Errorf("another time of the service: %v", err)
		}
		for name, err := range map[string]error{
			"date shape":           lg("LX", cha, nil, "2026/10/11", "", "id", "draft", 1),
			"time shape":           lg("LX", cha, nil, "2026-10-11", "7:00", "id", "draft", 1),
			"service without time": lg("LX", cha, svc, "2026-10-12", "", "id", "draft", 1),
			"state":                lg("LX", cha, nil, "2026-10-11", "", "id", "archived", 1),
			"version":              lg("LX", cha, nil, "2026-10-11", "", "id", "draft", 0),
			"language":             lg("LX", cha, nil, "2026-10-11", "", "fr", "draft", 1),
			"archived without who": f.exec(`UPDATE liturgies SET archived_at = ? WHERE id = ?`, f.ts(0), id("L1")),
		} {
			if !errors.Is(err, app.ErrInvalid) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if err := lg("LX", chb, svc, "2026-10-13", "07:00", "id", "draft", 1); !errors.Is(err, app.ErrReferenced) {
			t.Errorf("church B's liturgy with church A's service: %v", err)
		}

		item := func(iid, church, lid string, pos int, typ string, duty any, text string, reading any, label string) error {
			return f.exec(rawItem, id(iid), church, id(lid), pos, typ, duty, text, reading, label, f.ts(0), f.ts(0))
		}
		if err := item("IT1", cha, "L1", 0, "song", nil, "", nil, ""); err != nil {
			t.Fatal(err)
		}
		if err := item("IT2", cha, "L1", 1, "prayer", duty.Entry.ID, "Doa", nil, ""); err != nil {
			t.Fatal(err)
		}
		if err := item("IT3", cha, "L1", 2, "reading", nil, "", string(reading.Reading.ID), "Yohanes 3:16 (TB)"); err != nil {
			t.Fatal(err)
		}
		for name, err := range map[string]error{
			"text on a song":           item("ITX", cha, "L1", 3, "song", nil, "text", nil, ""),
			"reading on a prayer":      item("ITX", cha, "L1", 3, "prayer", nil, "", string(reading.Reading.ID), ""),
			"label on a prayer":        item("ITX", cha, "L1", 3, "prayer", nil, "", nil, "label"),
			"position 60":              item("ITX", cha, "L1", 60, "prayer", nil, "", nil, ""),
			"unknown type":             item("ITX", cha, "L1", 3, "video", nil, "", nil, ""),
			"item of another church's": item("ITX", chb, "L1", 3, "prayer", nil, "", nil, ""),
			"unknown duty":             item("ITX", cha, "L1", 3, "prayer", id("NODUTY"), "", nil, ""),
		} {
			if err == nil || (!errors.Is(err, app.ErrInvalid) && !errors.Is(err, app.ErrReferenced)) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if err := f.exec(rawItemSong, id("S1"), cha, id("IT1"), 0, song.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.exec(rawEntry, id("E1"), cha, id("S1"), 0, "section", string(song.Sections[0].ID)); err != nil {
			t.Fatal(err)
		}
		for name, err := range map[string]error{
			"song position 10":   f.exec(rawItemSong, id("SX"), cha, id("IT1"), 10, song.ID),
			"entry kind":         f.exec(rawEntry, id("EX"), cha, id("S1"), 1, "instrumental", nil),
			"entry position 100": f.exec(rawEntry, id("EX"), cha, id("S1"), 100, "section", nil),
			"unknown song":       f.exec(rawItemSong, id("SX"), cha, id("IT1"), 1, id("NOSONG")),
			"unknown section":    f.exec(rawEntry, id("EX"), cha, id("S1"), 1, "section", id("NOSECT")),
		} {
			if err == nil || (!errors.Is(err, app.ErrInvalid) && !errors.Is(err, app.ErrReferenced)) {
				t.Errorf("%s: %v", name, err)
			}
		}
		// An entry of a section of another song is refused by the use case (tested in package app),
		// not by the database: the entry has no column for the song of its item song.

		as := func(aid string, duty, user, name, key any) error {
			return f.exec(rawAssign, id(aid), cha, id("L1"), duty, user, name, key, f.ts(0))
		}
		d := duty.Entry.ID
		if err := as("A1", d, u1, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := as("A2", d, nil, "Pak Yan", "pak yan"); err != nil {
			t.Fatal(err)
		}
		for name, err := range map[string]error{
			"both":             as("AX", d, u1, "X", "x"),
			"neither":          as("AX", d, nil, nil, nil),
			"name without key": as("AX", d, nil, "X", nil),
			"key without name": as("AX", d, u1, nil, "x"),
		} {
			if !errors.Is(err, app.ErrInvalid) {
				t.Errorf("assignment with %s: %v", name, err)
			}
		}
		if err := as("A3", d, u1, nil, nil); !errors.As(err, &uq) || uq.Constraint != "assignments_user_key" {
			t.Errorf("the same member twice: %v", err)
		}
		if err := as("A4", d, nil, "PAK YAN", "pak yan"); !errors.As(err, &uq) || uq.Constraint != "assignments_name_key" {
			t.Errorf("the same name twice: %v", err)
		}
		if err := f.exec(rawEdit, id("H1"), cha, id("L1"), u1, 1, "liturgy.create", "done", f.ts(0)); err != nil {
			t.Fatal(err)
		}
		if err := f.exec(rawEdit, id("H2"), cha, id("L1"), u1, 1, "liturgy.update", "done", f.ts(0)); !errors.As(err, &uq) || uq.Constraint != "liturgy_edits_seq_key" {
			t.Errorf("seq twice: %v", err)
		}
		if err := f.exec(rawEdit, id("H3"), cha, id("L1"), u1, 2, "bogus", "done", f.ts(0)); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("unknown command: %v", err)
		}
		if err := f.exec(rawEdit, id("H3"), cha, id("L1"), u1, 2, "undo", "bogus", f.ts(0)); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("unknown status: %v", err)
		}

		// Deleting a referenced row directly is refused: the application clears the
		// reference first, in the same transaction (schema "Clearing references").
		for name, q := range map[string]string{
			"song":     "DELETE FROM songs WHERE id = ?",
			"section":  "DELETE FROM song_sections WHERE id = ?",
			"reading":  "DELETE FROM readings WHERE id = ?",
			"duty":     "DELETE FROM duties WHERE id = ?",
			"service":  "DELETE FROM services WHERE id = ?",
			"template": "DELETE FROM templates WHERE id = ?",
		} {
			arg := map[string]string{"song": string(song.ID), "section": string(song.Sections[0].ID), "reading": string(reading.Reading.ID),
				"duty": d, "service": svc, "template": id("TPL1")}[name]
			if name == "template" { // the template is only the service's default, which is not a liturgy
				continue
			}
			if err := f.exec(q, arg); !errors.Is(err, app.ErrReferenced) {
				t.Errorf("raw delete of a %s in use: %v", name, err)
			}
		}

		// Deleting a liturgy removes everything under it.
		if e.count("liturgy_items") != 3 || e.count("liturgy_item_songs") != 1 || e.count("sequence_entries") != 1 ||
			e.count("assignments") != 2 || e.count("liturgy_edits") != 1 {
			t.Fatal("setup")
		}
		if err := f.exec(`DELETE FROM liturgies WHERE id = ?`, id("L1")); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"liturgy_items", "liturgy_item_songs", "sequence_entries", "assignments", "liturgy_edits"} {
			if n := e.count(table); n != 0 {
				t.Errorf("%s: %d rows left after the liturgy was deleted", table, n)
			}
		}
	})
}
