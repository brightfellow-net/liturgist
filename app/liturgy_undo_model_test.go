// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// TC-U-007: the model of the 2026-10-03 review (P-65) as an oracle. Random
// histories of two people over the full command set, with undo and redo, run
// through the real use cases. After every step the state of the liturgy must
// equal the semantic replay of the history rows whose status is `done`
// (skipped ones included): undo and redo must leave exactly the state the
// remaining edits would have produced. Versions of an item must never go
// back, and seq must have no gap.

// mstate is the part of a liturgy the oracle compares.
type mstate struct {
	Order []domain.ItemID
	Items map[domain.ItemID]mitem
	Name  string
	Asg   []string // sorted duty|name
}

type mitem struct {
	Text  string
	Songs []msong
}

type msong struct {
	ID      string  `json:"id"`
	SongID  string  `json:"song_id"`
	Key     string  `json:"key"`
	Note    string  `json:"note"`
	Entries []mentr `json:"entries"`
}

type mentr struct {
	ID        string `json:"id"`
	SectionID string `json:"section_id"`
	PartID    string `json:"singing_part_id"`
	KeyChange string `json:"key_change"`
	Note      string `json:"note"`
	Label     string `json:"section_label"`
}

func (s mstate) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func msongsOf(songs []domain.LiturgySong) []msong {
	out := make([]msong, len(songs))
	for i, s := range songs {
		out[i] = msong{ID: string(s.ID), SongID: string(s.SongID), Key: s.Key, Note: s.Note, Entries: []mentr{}}
		for _, e := range s.Entries {
			out[i].Entries = append(out[i].Entries, mentr{ID: string(e.ID), SectionID: string(e.SectionID), PartID: string(e.SingingPartID),
				KeyChange: e.KeyChange, Note: e.Note, Label: e.SectionLabel})
		}
	}
	return out
}

// actual reads the state through the liturgy view.
func (u uenv) actual() mstate {
	u.t.Helper()
	v := u.liturgy(u.lid)
	s := mstate{Order: []domain.ItemID{}, Items: map[domain.ItemID]mitem{}, Name: v.Liturgy.ServiceName, Asg: []string{}}
	for _, it := range v.Items {
		s.Order = append(s.Order, it.Item.ID)
		s.Items[it.Item.ID] = mitem{Text: it.Item.Text, Songs: msongsOf(it.Item.Songs)}
	}
	for _, a := range v.Assignments {
		s.Asg = append(s.Asg, string(a.Assignment.DutyID)+"|"+a.Assignment.Name)
	}
	slices.Sort(s.Asg)
	return s
}

func (s mstate) clone() mstate {
	c := mstate{Order: slices.Clone(s.Order), Items: map[domain.ItemID]mitem{}, Name: s.Name, Asg: slices.Clone(s.Asg)}
	for k, v := range s.Items {
		c.Items[k] = v
	}
	return c
}

// replay applies the editing rows with status done, oldest first, to base.
func replay(base mstate, baseSeq int, rows []domain.Edit) (mstate, error) {
	s := base.clone()
	asg := map[string]bool{}
	for _, k := range s.Asg {
		asg[k] = true
	}
	for _, r := range rows {
		if r.Seq <= baseSeq || !domain.Editing(r.Command) || r.Status != domain.EditDone {
			continue
		}
		switch r.Command {
		case domain.CmdItemUpdate:
			var f struct {
				Text *string `json:"text"`
			}
			if err := json.Unmarshal(r.After, &f); err != nil {
				return s, err
			}
			if it, ok := s.Items[r.ItemID]; ok && f.Text != nil {
				it.Text = *f.Text
				s.Items[r.ItemID] = it
			}
		case domain.CmdItemSongs:
			var songs []msong
			if err := json.Unmarshal(r.After, &songs); err != nil {
				return s, err
			}
			if songs == nil {
				songs = []msong{}
			}
			for i := range songs {
				if songs[i].Entries == nil {
					songs[i].Entries = []mentr{}
				}
			}
			if it, ok := s.Items[r.ItemID]; ok {
				it.Songs = songs
				s.Items[r.ItemID] = it
			}
		case domain.CmdItemAdd:
			var img struct {
				Position int `json:"position"`
			}
			if err := json.Unmarshal(r.After, &img); err != nil {
				return s, err
			}
			s.Items[r.ItemID] = mitem{Songs: []msong{}}
			at := min(max(img.Position, 0), len(s.Order))
			s.Order = slices.Insert(s.Order, at, r.ItemID)
		case domain.CmdItemRemove:
			if _, ok := s.Items[r.ItemID]; ok {
				delete(s.Items, r.ItemID)
				s.Order = slices.DeleteFunc(s.Order, func(id domain.ItemID) bool { return id == r.ItemID })
			}
		case domain.CmdItemsReorder:
			var ids []domain.ItemID
			if err := json.Unmarshal(r.After, &ids); err != nil {
				return s, err
			}
			next := []domain.ItemID{}
			for _, id := range ids {
				if _, ok := s.Items[id]; ok {
					next = append(next, id)
				}
			}
			for _, id := range s.Order {
				if !slices.Contains(ids, id) {
					next = append(next, id)
				}
			}
			s.Order = next
		case domain.CmdLiturgyUpdate:
			var f struct {
				Name string `json:"service_name"`
			}
			if err := json.Unmarshal(r.After, &f); err != nil {
				return s, err
			}
			s.Name = f.Name
		case domain.CmdAssignmentAdd, domain.CmdAssignmentRemove:
			img := r.After
			if r.Command == domain.CmdAssignmentRemove {
				img = r.Before
			}
			var a struct {
				DutyID string `json:"duty_id"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(img, &a); err != nil {
				return s, err
			}
			asg[a.DutyID+"|"+a.Name] = r.Command == domain.CmdAssignmentAdd
		}
	}
	s.Asg = []string{}
	for k, on := range asg {
		if on {
			s.Asg = append(s.Asg, k)
		}
	}
	slices.Sort(s.Asg)
	return s, nil
}

// check compares the liturgy with the replay of its done rows.
func (u uenv) check(base mstate, baseSeq int) error {
	want, err := replay(base, baseSeq, u.history())
	if err != nil {
		return err
	}
	if got := u.actual(); got.String() != want.String() {
		return fmt.Errorf("state differs from the replay of the done edits\n actual: %s\n replay: %s", got, want)
	}
	return nil
}

// historyRun drives one random history; it returns the log and the first violation.
type historyRun struct {
	u        uenv
	rnd      *rand.Rand
	base     mstate
	baseSeq  int
	songs    []app.SongView
	duties   []domain.DutyID
	versions map[domain.ItemID]int
	log      []string
}

func newHistoryRun(t *testing.T, db *sqlstore.DB, seed int64) *historyRun {
	t.Helper()
	u := newUndoEnv(t, db)
	r := &historyRun{u: u, rnd: rand.New(rand.NewSource(seed)), versions: map[domain.ItemID]int{}}
	r.songs = []app.SongView{u.newSong("Lagu satu", "id"), u.newSong("Lagu dua", "id")}
	duties, err := u.vocab.List(u.ctx, u.a, domain.KindDuty)
	if err != nil || len(duties) < 2 {
		t.Fatal("duties", err)
	}
	r.duties = []domain.DutyID{domain.DutyID(duties[0].Entry.ID), domain.DutyID(duties[1].Entry.ID)}
	u.addItem(u.lid, domain.ItemPrayer, "p1")
	u.addItem(u.lid, domain.ItemPrayer, "p2")
	u.addItem(u.lid, domain.ItemSong, "s1")
	u.fresh() // the setup is below the floor: the base state of the replay
	r.base = u.actual()
	r.baseSeq = len(u.history())
	return r
}

func (r *historyRun) pick(typ domain.ItemType) (domain.ItemID, bool) {
	var ids []domain.ItemID
	for _, v := range r.u.liturgy(r.u.lid).Items {
		if v.Item.Type == typ {
			ids = append(ids, v.Item.ID)
		}
	}
	if len(ids) == 0 {
		return "", false
	}
	return ids[r.rnd.Intn(len(ids))], true
}

// step does one random operation as a random person; it returns an error only for a real failure.
func (r *historyRun) step() error {
	u := r.u
	sess := []*domain.Session{u.a, u.b}[r.rnd.Intn(2)]
	who := map[*domain.Session]string{u.a: "A", u.b: "B"}[sess]
	L := u.liturgies
	ver := func() int { return u.liturgy(u.lid).Liturgy.Version }
	var err error
	op := r.rnd.Intn(11)
	switch op {
	case 0, 1:
		id, ok := r.pick(domain.ItemPrayer)
		if !ok {
			return nil
		}
		text := fmt.Sprint("t", r.rnd.Intn(9))
		r.log = append(r.log, who+" text "+string(id)[20:]+"="+text)
		_, err = L.UpdateItem(u.ctx, sess, u.lid, id, app.ItemChange{Version: u.mustItem(id).Version, Text: &text})
	case 2:
		id, ok := r.pick(domain.ItemSong)
		if !ok {
			return nil
		}
		var in []app.ItemSongInput
		for _, k := range r.rnd.Perm(2)[:r.rnd.Intn(3)] {
			song := r.songs[k].Song
			si := app.ItemSongInput{SongID: song.ID}
			for _, j := range r.rnd.Perm(len(song.Sections))[:r.rnd.Intn(len(song.Sections)+1)] {
				si.Entries = append(si.Entries, app.EntryInput{SectionID: song.Sections[j].ID})
			}
			in = append(in, si)
		}
		r.log = append(r.log, who+" songs "+string(id)[20:])
		_, err = L.SetSongs(u.ctx, sess, u.lid, id, u.mustItem(id).Version, in)
	case 3:
		typ := []domain.ItemType{domain.ItemPrayer, domain.ItemSong}[r.rnd.Intn(2)]
		at := r.rnd.Intn(len(u.order()) + 1)
		r.log = append(r.log, fmt.Sprintf("%s add %s at %d", who, typ, at))
		_, err = L.AddItem(u.ctx, sess, u.lid, app.ItemInput{LiturgyVersion: ver(), Title: "new", Type: typ, Position: &at})
	case 4:
		ids := u.order()
		if len(ids) == 0 {
			return nil
		}
		id := ids[r.rnd.Intn(len(ids))]
		r.log = append(r.log, who+" remove "+string(id)[20:])
		_, err = L.RemoveItem(u.ctx, sess, u.lid, id, ver())
	case 5:
		ids := u.order()
		if len(ids) < 2 {
			return nil
		}
		r.rnd.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		r.log = append(r.log, who+" reorder")
		_, err = L.ReorderItems(u.ctx, sess, u.lid, ver(), ids)
	case 6:
		name := fmt.Sprint("N", r.rnd.Intn(9))
		r.log = append(r.log, who+" rename "+name)
		_, err = L.Update(u.ctx, sess, u.lid, app.LiturgyChange{Version: ver(), ServiceName: &name})
	case 7:
		duty, name := r.duties[r.rnd.Intn(2)], []string{"p", "q"}[r.rnd.Intn(2)]
		for _, a := range u.liturgy(u.lid).Assignments {
			if a.Assignment.DutyID == duty && a.Assignment.Name == name {
				r.log = append(r.log, who+" unassign "+name)
				return r.fail(L.RemoveAssignment(u.ctx, sess, u.lid, a.Assignment.ID))
			}
		}
		r.log = append(r.log, who+" assign "+name)
		_, err = L.AddAssignment(u.ctx, sess, u.lid, app.AssignmentInput{DutyID: duty, Name: name})
	case 8, 9:
		r.log = append(r.log, who+" undo")
		_, err = L.Undo(u.ctx, sess, u.lid)
	default:
		r.log = append(r.log, who+" redo")
		_, err = L.Redo(u.ctx, sess, u.lid)
	}
	return r.fail(err)
}

// fail lets refusals through; anything else is a failure.
func (r *historyRun) fail(err error) error {
	var ref *app.UndoRefusedError
	if err == nil || errors.As(err, &ref) {
		if ref != nil {
			r.log = append(r.log, "  refused: "+ref.Reason)
		}
		return nil
	}
	return err
}

// after checks the invariants once a step is done.
func (r *historyRun) after() error {
	if err := r.u.check(r.base, r.baseSeq); err != nil {
		return err
	}
	for _, v := range r.u.liturgy(r.u.lid).Items {
		if old, ok := r.versions[v.Item.ID]; ok && v.Item.Version < old {
			return fmt.Errorf("the version of item %s went back from %d to %d", v.Item.ID, old, v.Item.Version)
		}
		r.versions[v.Item.ID] = v.Item.Version
	}
	for i, e := range r.u.history() {
		if e.Seq != i+1 {
			return fmt.Errorf("seq %d at position %d: a gap in the history", e.Seq, i+1)
		}
	}
	return nil
}

func runModel(t *testing.T, newDB func(testing.TB) *sqlstore.DB, histories, steps int) {
	t.Helper()
	// One church per history: the setup of a church is the cost, not the steps.
	for seed := range int64(histories) {
		r := newHistoryRun(t, newDB(t), seed)
		for i := range steps {
			if err := r.step(); err != nil {
				t.Fatalf("seed %d step %d: %v\nlog: %s", seed, i, err, strings.Join(r.log, "; "))
			}
			if err := r.after(); err != nil {
				t.Fatalf("seed %d step %d: %v\nlog: %s", seed, i, err, strings.Join(r.log, "; "))
			}
		}
	}
}

// histories is how many random histories a test runs; LITURGIST_MODEL_HISTORIES raises it for a longer sweep.
func histories(def int) int {
	if n, err := strconv.Atoi(os.Getenv("LITURGIST_MODEL_HISTORIES")); err == nil && n > 0 {
		return n
	}
	return def
}

// The model tests run one history after another, so the race detector finds
// nothing in them and only slows them down: fewer histories under -race.
func TestUndoModelSQLite(t *testing.T) {
	n := 150
	if raceEnabled {
		n = 10
	}
	runModel(t, sqlstoretest.NewSQLite, histories(n), 40)
}

func TestUndoModelPostgres(t *testing.T) { runModel(t, sqlstoretest.NewPostgres, histories(10), 40) }

// The oracle can fail: a tampered state or a tampered row is caught.
func TestUndoModelCatchesTampering(t *testing.T) {
	r := newHistoryRun(t, sqlstoretest.NewSQLite(t), 1)
	for range 25 {
		if err := r.step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.after(); err != nil {
		t.Fatalf("an untouched history must pass: %v", err)
	}
	u := r.u

	// A change of state that no row accounts for.
	id := u.pray("tampered")
	if err := r.after(); err != nil {
		t.Fatalf("an untouched history must pass: %v", err)
	}
	u.sql("UPDATE liturgy_items SET text = 'tampered' WHERE id = ?", string(id))
	if err := u.check(r.base, r.baseSeq); err == nil {
		t.Error("a tampered item text went unnoticed")
	}

	// A row whose status says something the state does not.
	r2 := newHistoryRun(t, sqlstoretest.NewSQLite(t), 2)
	for len(r2.u.history()) < 6 {
		if err := r2.step(); err != nil {
			t.Fatal(err)
		}
	}
	var target domain.Edit
	for _, e := range r2.u.history() {
		if e.Status == domain.EditDone && (e.Command == domain.CmdItemUpdate || e.Command == domain.CmdItemAdd || e.Command == domain.CmdLiturgyUpdate) {
			target = e
		}
	}
	if target.ID == "" {
		t.Skip("history without a suitable row")
	}
	r2.u.sql("UPDATE liturgy_edits SET status = 'dropped' WHERE id = ?", string(target.ID))
	if err := r2.u.check(r2.base, r2.baseSeq); err == nil {
		t.Errorf("a done row marked dropped went unnoticed (%s)", target.Command)
	}
}
