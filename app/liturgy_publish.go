// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// PublishedInfo is the newest published version of a liturgy, without content.
type PublishedInfo struct {
	Number      int
	PublishedAt time.Time
}

// encodeJSON marshals v without HTML escaping: the size cap counts the bytes
// that are stored (13 §2).
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// copyBuilder holds what buildPublished looks up while it makes a copy.
type copyBuilder struct {
	ctx    context.Context
	sc     churchScope
	l      domain.Liturgy
	duties map[string]string // id → name
	parts  map[string]string // singing part id → name
	songs  map[domain.SongID]domain.Song
}

// ref is the id and the name of a duty or singing part now, for the copy.
func (b *copyBuilder) ref(m map[string]string, id string) *domain.PublishedRef {
	if id == "" {
		return nil
	}
	return &domain.PublishedRef{ID: id, Name: m[id]}
}

// buildPublished makes the self-contained copy of a liturgy (13 §3) inside the
// publishing transaction. It refuses a liturgy with problems (defence in depth:
// step 4 refuses them at submit) and a copy over the size cap.
func buildPublished(ctx context.Context, sc churchScope, l domain.Liturgy) (content []byte, assignees []domain.UserID, err error) {
	church, err := sc.cs.Church().Get(ctx)
	if err != nil {
		return nil, nil, err
	}
	items, err := sc.cs.LiturgyItems().ByLiturgy(ctx, l.ID)
	if err != nil {
		return nil, nil, err
	}
	if p := domain.Problems(items); len(p) > 0 {
		return nil, nil, &HasProblemsError{Problems: p}
	}
	b := &copyBuilder{ctx: ctx, sc: sc, l: l, songs: map[domain.SongID]domain.Song{}}
	if b.duties, err = nameMap(ctx, sc.cs.Duties()); err != nil {
		return nil, nil, err
	}
	if b.parts, err = nameMap(ctx, sc.cs.SingingParts()); err != nil {
		return nil, nil, err
	}
	c := domain.PublishedContent{Format: domain.PublishedFormat, Items: make([]domain.PublishedItem, 0, len(items)),
		Liturgy:       domain.PublishedLiturgy{Date: l.Date, Time: l.Time, ServiceName: l.ServiceName, Language: l.Language, ChurchName: church.Name},
		LicenceFooter: church.Settings.LicenceFooter}
	sizes := make([]int, len(items))
	for i, it := range items {
		pi, err := b.item(it)
		if err != nil {
			return nil, nil, err
		}
		enc, err := encodeJSON(pi)
		if err != nil {
			return nil, nil, err
		}
		sizes[i] = len(enc)
		c.Items = append(c.Items, pi)
	}
	if c.Assignments, assignees, err = b.assignments(); err != nil {
		return nil, nil, err
	}
	content, err = encodeJSON(c)
	if err != nil {
		return nil, nil, err
	}
	if len(content) > domain.MaxPublishedBytes {
		return nil, nil, tooLarge(items, sizes)
	}
	return content, assignees, nil
}

// item makes the copy of one item, with its reading and its songs.
func (b *copyBuilder) item(it domain.Item) (domain.PublishedItem, error) {
	pi := domain.PublishedItem{ID: it.ID, Position: it.Position, Type: it.Type, Title: it.Title, Text: it.Text,
		Duty: b.ref(b.duties, string(it.DutyID))}
	if it.ReadingID != "" {
		r, err := b.sc.cs.Readings().ByID(b.ctx, it.ReadingID)
		if err != nil {
			return pi, err
		}
		pi.Reading = &domain.PublishedReading{ReferenceDisplay: r.ReferenceDisplay, TranslationCode: r.Translation.Code,
			Text: r.Text, Attribution: r.Attribution}
	}
	for _, s := range it.Songs {
		ps, err := b.song(s)
		if err != nil {
			return pi, err
		}
		pi.Songs = append(pi.Songs, ps)
	}
	return pi, nil
}

// song makes the copy of one song of an item: the song as it is now, the
// sections its sequence uses (once each, in first-use order) and the sequence.
func (b *copyBuilder) song(s domain.LiturgySong) (domain.PublishedSong, error) {
	song, ok := b.songs[s.SongID]
	if !ok {
		var err error
		if song, err = b.sc.cs.Songs().ByID(b.ctx, s.SongID); err != nil {
			return domain.PublishedSong{}, err
		}
		b.songs[s.SongID] = song
	}
	ps := domain.PublishedSong{SongID: song.ID, Title: song.Title, HymnalSource: song.HymnalSource, HymnalNumber: song.HymnalNumber,
		Key: s.Key, Note: s.Note, CopyrightHolder: song.CopyrightHolder, CopyrightLine: song.CopyrightLine,
		CCLISongNumber: song.CCLISongNumber, Sections: []domain.PublishedSection{}, Entries: make([]domain.PublishedEntry, 0, len(s.Entries))}
	used := map[domain.SectionID]bool{}
	for _, e := range s.Entries {
		if !used[e.SectionID] {
			idx := -1
			for k := range song.Sections {
				if song.Sections[k].ID == e.SectionID {
					idx = k
				}
			}
			if idx < 0 {
				return domain.PublishedSong{}, fmt.Errorf("section %s is not a section of song %s", e.SectionID, song.ID)
			}
			sec := song.Sections[idx]
			ps.Sections = append(ps.Sections, domain.PublishedSection{ID: sec.ID, Kind: sec.Kind, Number: sec.Number,
				Label: domain.SectionLabelIn(sec, b.l.Language), Text: sec.Text})
			used[e.SectionID] = true
		}
		ps.Entries = append(ps.Entries, domain.PublishedEntry{SectionID: e.SectionID, Part: b.ref(b.parts, string(e.SingingPartID)),
			KeyChange: e.KeyChange, Note: e.Note})
	}
	return ps, nil
}

// assignments makes the team list of the copy, and the members on it (once
// each, in order) for the "my assignments" index.
func (b *copyBuilder) assignments() ([]domain.PublishedAssignment, []domain.UserID, error) {
	as, err := b.sc.cs.Assignments().ByLiturgy(b.ctx, b.l.ID)
	if err != nil {
		return nil, nil, err
	}
	var assignees []domain.UserID
	seen := map[domain.UserID]bool{}
	out := make([]domain.PublishedAssignment, 0, len(as))
	for _, a := range as {
		pa := domain.PublishedAssignment{Duty: domain.PublishedRef{ID: string(a.DutyID), Name: b.duties[string(a.DutyID)]}, Name: a.Name}
		if a.UserID != "" {
			pa.UserID = a.UserID
			if u, err := b.sc.users.ByID(b.ctx, a.UserID); err == nil {
				pa.Name = u.Name
			} else if !errors.Is(err, ErrNotFound) {
				return nil, nil, err
			}
			if !seen[a.UserID] {
				seen[a.UserID] = true
				assignees = append(assignees, a.UserID)
			}
		}
		out = append(out, pa)
	}
	return out, assignees, nil
}

// tooLarge is the error of a copy over the cap; it names the largest item.
func tooLarge(items []domain.Item, sizes []int) error {
	big := 0
	for i := range sizes {
		if sizes[i] > sizes[big] {
			big = i
		}
	}
	title := ""
	if len(items) > 0 {
		title = items[big].Title
	}
	return &PublishTooLargeError{LargestItem: title}
}

func nameMap(ctx context.Context, r NameListRepo) (map[string]string, error) {
	list, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(list))
	for _, e := range list {
		m[e.ID] = e.Name
	}
	return m, nil
}

// Archive archives a published liturgy (liturgy.manage, 13 §4). It is not a
// state change: the state stays published, and no history row is written.
func (u *Liturgies) Archive(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (LiturgyView, error) {
	var res LiturgyView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		l, err := sc.openLiturgy(ctx, id)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyManage); err != nil {
			return err
		}
		if l.State != domain.StatePublished {
			return &InvalidTransitionError{State: l.State}
		}
		if l.ArchivedAt != nil {
			return ErrLiturgyArchived
		}
		ok, err := sc.cs.Liturgies().Archive(ctx, id, sc.actor.UserID, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			cur, err := sc.cs.Liturgies().ByID(ctx, id)
			if err != nil {
				return missing(err)
			}
			if cur.State != domain.StatePublished {
				return &InvalidTransitionError{State: cur.State}
			}
			return ErrLiturgyArchived
		}
		if l, err = sc.cs.Liturgies().ByID(ctx, id); err != nil {
			return missing(err)
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}

// Unarchive makes an archived liturgy active again (liturgy.manage). It takes
// a slot of max_active_liturgies, so it counts under LockChurch (13 §4).
func (u *Liturgies) Unarchive(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (LiturgyView, error) {
	var res LiturgyView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		l, err := sc.openLiturgy(ctx, id)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyManage); err != nil {
			return err
		}
		if l.ArchivedAt == nil {
			return ErrNotArchived
		}
		lim, err := u.limits(ctx)
		if err != nil {
			return err
		}
		if err := lim.check(ctx, sc.cs, 1, LimitMaxActiveLiturgies); err != nil {
			return err
		}
		ok, err := sc.cs.Liturgies().Unarchive(ctx, id, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			if _, err := sc.cs.Liturgies().ByID(ctx, id); err != nil {
				return missing(err)
			}
			return ErrNotArchived
		}
		if l, err = sc.cs.Liturgies().ByID(ctx, id); err != nil {
			return missing(err)
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}
