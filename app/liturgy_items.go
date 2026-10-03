// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/brightfellow-net/liturgist/domain"
)

// ItemInput adds an item (10 §4). Position nil appends.
type ItemInput struct {
	LiturgyVersion int
	Title          string
	Type           domain.ItemType
	DutyID         domain.DutyID
	Text           string
	Position       *int
}

// ItemChange is a PATCH of an item: nil fields are unchanged; a DutyID or
// ReadingID of "" clears it.
type ItemChange struct {
	Version   int
	Title     *string
	DutyID    *domain.DutyID
	Text      *string
	ReadingID *domain.ReadingID
}

// EntryInput is one entry of a sequence in PUT …/songs.
type EntryInput struct {
	SectionID     domain.SectionID
	SingingPartID domain.SingingPartID
	KeyChange     string
	Note          string
}

// ItemSongInput is a song in a PUT …/songs list.
type ItemSongInput struct {
	SongID  domain.SongID
	Key     string
	Note    string
	Entries []EntryInput
}

// ItemResult is the answer of an item write: the item and the liturgy's version.
type ItemResult struct {
	Item           ItemView
	LiturgyVersion int
}

// OrderResult is the answer of a structure write that has no item to show.
type OrderResult struct {
	LiturgyVersion int
	ItemIDs        []domain.ItemID
}

func (u *Liturgies) itemResult(ctx context.Context, sc churchScope, liturgy domain.LiturgyID, id domain.ItemID, liturgyVersion int) (ItemResult, error) {
	it, err := sc.cs.LiturgyItems().ByID(ctx, liturgy, id)
	if err != nil {
		return ItemResult{}, err
	}
	v, err := itemViews(ctx, sc.cs, []domain.Item{it})
	if err != nil {
		return ItemResult{}, err
	}
	return ItemResult{Item: v[0], LiturgyVersion: liturgyVersion}, nil
}

// checkDuty rejects a duty the church does not have.
func checkDuty(ctx context.Context, cs ChurchStore, id domain.DutyID, field string) error {
	if id == "" {
		return nil
	}
	if _, err := cs.Duties().ByID(ctx, string(id)); err != nil {
		if errors.Is(err, ErrNotFound) {
			return &domain.InvalidInputError{Field: field, Message: "Unknown duty."}
		}
		return err
	}
	return nil
}

// AddItem inserts an item (liturgy.edit). Structural: it claims the liturgy's version.
func (u *Liturgies) AddItem(ctx context.Context, sess *domain.Session, id domain.LiturgyID, in ItemInput) (ItemResult, error) {
	var res ItemResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true) // adds a reference to a duty
		if err != nil {
			return err
		}
		if _, err := sc.openEdit(ctx, id); err != nil {
			return err
		}
		now := u.Clock.Now()
		it := domain.Item{ID: domain.ItemID(u.IDs.NewID()), LiturgyID: id, Title: in.Title, Type: in.Type, DutyID: in.DutyID,
			Text: in.Text, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := domain.ValidateItemFields(&it, ""); err != nil {
			return err
		}
		if err := checkDuty(ctx, sc.cs, it.DutyID, "duty_id"); err != nil {
			return err
		}
		order, err := sc.cs.LiturgyItems().IDs(ctx, id)
		if err != nil {
			return err
		}
		if len(order) >= domain.MaxLiturgyItems {
			return domain.LimitError("items", domain.MaxLiturgyItems, len(order))
		}
		at := len(order)
		if in.Position != nil {
			if *in.Position < 0 || *in.Position > len(order) {
				return &domain.InvalidInputError{Field: "position", Message: "0 to " + strconv.Itoa(len(order)) + "."}
			}
			at = *in.Position
		}
		ok, err := sc.cs.Liturgies().Bump(ctx, id, in.LiturgyVersion, now)
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleLiturgy(ctx, id)
		}
		it.Position = len(order)
		if err := sc.cs.LiturgyItems().Insert(ctx, it); err != nil {
			return err
		}
		order = slices.Insert(order, at, it.ID)
		if err := sc.cs.LiturgyItems().SetPositions(ctx, id, order); err != nil {
			return err
		}
		it.Position = at
		if err := u.record(ctx, sc, id, domain.CmdItemAdd, it.ID, in.LiturgyVersion+1, 1, nil, imageOfItem(it)); err != nil {
			return err
		}
		res, err = u.itemResult(ctx, sc, id, it.ID, in.LiturgyVersion+1)
		return err
	})
	return res, err
}

// UpdateItem changes an item's own fields (liturgy.edit). The item's version
// is checked; the liturgy's is not touched.
func (u *Liturgies) UpdateItem(ctx context.Context, sess *domain.Session, id domain.LiturgyID, item domain.ItemID, ch ItemChange) (ItemResult, error) {
	adds := (ch.DutyID != nil && *ch.DutyID != "") || (ch.ReadingID != nil && *ch.ReadingID != "")
	var res ItemResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, adds)
		if err != nil {
			return err
		}
		l, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		cur, err := sc.cs.LiturgyItems().ByID(ctx, id, item)
		if err != nil {
			return missing(err)
		}
		next := cur
		before, after := map[string]any{}, map[string]any{}
		if ch.Title != nil {
			next.Title = *ch.Title
		}
		if ch.Text != nil {
			next.Text = *ch.Text
		}
		if ch.DutyID != nil {
			next.DutyID = *ch.DutyID
		}
		if err := domain.ValidateItemFields(&next, ""); err != nil {
			return err
		}
		if ch.DutyID != nil {
			if err := checkDuty(ctx, sc.cs, next.DutyID, "duty_id"); err != nil {
				return err
			}
		}
		if ch.ReadingID != nil {
			if cur.Type != domain.ItemReading {
				return &domain.InvalidInputError{Field: "reading_id", Message: "Only reading items have a reading."}
			}
			next.ReadingID, next.ReadingLabel = "", ""
			if *ch.ReadingID != "" {
				r, err := sc.cs.Readings().ByID(ctx, *ch.ReadingID)
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						return &domain.InvalidInputError{Field: "reading_id", Message: "Unknown reading."}
					}
					return err
				}
				next.ReadingID, next.ReadingLabel = r.ID, domain.ReadingLabel(r)
			}
		}
		for _, f := range []struct {
			name     string
			old, new string
		}{{"title", cur.Title, next.Title}, {"duty_id", string(cur.DutyID), string(next.DutyID)}, {"text", cur.Text, next.Text},
			{"reading_id", string(cur.ReadingID), string(next.ReadingID)}, {"reading_label", cur.ReadingLabel, next.ReadingLabel}} {
			if f.old != f.new {
				before[f.name], after[f.name] = f.old, f.new
			}
		}
		next.UpdatedAt = u.Clock.Now()
		ok, err := sc.cs.LiturgyItems().Update(ctx, next, ch.Version)
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleItem(ctx, id, item)
		}
		if err := u.record(ctx, sc, id, domain.CmdItemUpdate, item, l.Version, ch.Version+1, before, after); err != nil {
			return err
		}
		res, err = u.itemResult(ctx, sc, id, item, l.Version)
		return err
	})
	return res, err
}

// RemoveItem deletes an item with its songs (liturgy.edit). Structural.
func (u *Liturgies) RemoveItem(ctx context.Context, sess *domain.Session, id domain.LiturgyID, item domain.ItemID, liturgyVersion int) (OrderResult, error) {
	var res OrderResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if _, err := sc.openEdit(ctx, id); err != nil {
			return err
		}
		cur, err := sc.cs.LiturgyItems().ByID(ctx, id, item)
		if err != nil {
			return missing(err)
		}
		ok, err := sc.cs.Liturgies().Bump(ctx, id, liturgyVersion, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleLiturgy(ctx, id)
		}
		if err := sc.cs.LiturgyItems().Delete(ctx, id, item); err != nil {
			return missing(err)
		}
		order, err := sc.cs.LiturgyItems().IDs(ctx, id)
		if err != nil {
			return err
		}
		if err := sc.cs.LiturgyItems().SetPositions(ctx, id, order); err != nil {
			return err
		}
		if err := u.record(ctx, sc, id, domain.CmdItemRemove, item, liturgyVersion+1, 0, imageOfItem(cur), nil); err != nil {
			return err
		}
		res = OrderResult{LiturgyVersion: liturgyVersion + 1, ItemIDs: order}
		return nil
	})
	return res, err
}

// ReorderItems sets the order of the items (liturgy.edit). The list must hold
// exactly the current IDs once each, else the liturgy changed meanwhile.
func (u *Liturgies) ReorderItems(ctx context.Context, sess *domain.Session, id domain.LiturgyID, liturgyVersion int, ids []domain.ItemID) (OrderResult, error) {
	var res OrderResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if _, err := sc.openEdit(ctx, id); err != nil {
			return err
		}
		cur, err := sc.cs.LiturgyItems().IDs(ctx, id)
		if err != nil {
			return err
		}
		a, b := slices.Clone(cur), slices.Clone(ids)
		slices.Sort(a)
		slices.Sort(b)
		if !slices.Equal(a, b) {
			return &VersionConflictError{Scope: ScopeLiturgy}
		}
		ok, err := sc.cs.Liturgies().Bump(ctx, id, liturgyVersion, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleLiturgy(ctx, id)
		}
		if err := sc.cs.LiturgyItems().SetPositions(ctx, id, ids); err != nil {
			return err
		}
		if err := u.record(ctx, sc, id, domain.CmdItemsReorder, "", liturgyVersion+1, 0, cur, ids); err != nil {
			return err
		}
		res = OrderResult{LiturgyVersion: liturgyVersion + 1, ItemIDs: ids}
		return nil
	})
	return res, err
}

// songItem opens the item for a write to its songs: it must be a song item.
func (sc churchScope) songItem(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) (domain.Item, error) {
	it, err := sc.cs.LiturgyItems().ByID(ctx, liturgy, id)
	if err != nil {
		return domain.Item{}, missing(err)
	}
	if it.Type != domain.ItemSong {
		return domain.Item{}, &domain.InvalidInputError{Field: "item", Message: "Only song items have songs."}
	}
	return it, nil
}

func songByID(ctx context.Context, cs ChurchStore, id domain.SongID, field string) (domain.Song, error) {
	song, err := cs.Songs().ByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return domain.Song{}, &domain.InvalidInputError{Field: field, Message: "Unknown song."}
	}
	return song, err
}

// AddSong adds a song with its sequence filled from the song's default
// arrangement, else all its sections in order (liturgy.edit, 10 §2.3 P-62).
func (u *Liturgies) AddSong(ctx context.Context, sess *domain.Session, id domain.LiturgyID, item domain.ItemID, version int,
	songID domain.SongID, position *int) (ItemResult, error) {
	var res ItemResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true) // adds a reference to a song
		if err != nil {
			return err
		}
		l, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		cur, err := sc.songItem(ctx, id, item)
		if err != nil {
			return err
		}
		if len(cur.Songs) >= domain.MaxItemSongs {
			return domain.LimitError("songs", domain.MaxItemSongs, len(cur.Songs))
		}
		song, err := songByID(ctx, sc.cs, songID, "song_id")
		if err != nil {
			return err
		}
		at := len(cur.Songs)
		if position != nil {
			if *position < 0 || *position > at {
				return &domain.InvalidInputError{Field: "position", Message: "0 to " + strconv.Itoa(at) + "."}
			}
			at = *position
		}
		ok, err := sc.cs.LiturgyItems().Bump(ctx, item, version, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleItem(ctx, id, item)
		}
		ns := domain.LiturgySong{ID: domain.ItemSongID(u.IDs.NewID()), Position: len(cur.Songs), SongID: song.ID,
			SongTitle: song.Title, Key: song.DefaultKey, Entries: u.fillSequence(song)}
		if err := sc.cs.LiturgyItems().InsertSong(ctx, item, ns); err != nil {
			return err
		}
		order := make([]domain.ItemSongID, 0, len(cur.Songs)+1)
		for _, e := range cur.Songs {
			order = append(order, e.ID)
		}
		order = slices.Insert(order, at, ns.ID)
		if err := sc.cs.LiturgyItems().SetSongPositions(ctx, item, order); err != nil {
			return err
		}
		after := slices.Insert(slices.Clone(cur.Songs), at, ns)
		if err := u.record(ctx, sc, id, domain.CmdItemSongs, item, l.Version, version+1, songsImage(cur.Songs), songsImage(after)); err != nil {
			return err
		}
		res, err = u.itemResult(ctx, sc, id, item, l.Version)
		return err
	})
	return res, err
}

// fillSequence makes the entries of a newly added song (10 §2.3 P-62).
func (u *Liturgies) fillSequence(song domain.Song) []domain.Entry {
	byID := make(map[domain.SectionID]domain.Section, len(song.Sections))
	for _, sec := range song.Sections {
		byID[sec.ID] = sec
	}
	ids := song.DefaultArrangement
	if len(ids) == 0 {
		for _, sec := range song.Sections {
			ids = append(ids, sec.ID)
		}
	}
	out := make([]domain.Entry, 0, len(ids))
	for i, sid := range ids {
		sec, ok := byID[sid]
		if !ok {
			continue
		}
		out = append(out, domain.Entry{ID: domain.EntryID(u.IDs.NewID()), Position: i, SectionID: sid, SectionLabel: domain.SectionDisplay(sec)})
	}
	return out
}

// SetSongs replaces the complete list of songs of a song item, with every
// ID new (liturgy.edit, 10 §4).
func (u *Liturgies) SetSongs(ctx context.Context, sess *domain.Session, id domain.LiturgyID, item domain.ItemID, version int,
	in []ItemSongInput) (ItemResult, error) {
	var res ItemResult
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true) // adds references to songs, sections and parts
		if err != nil {
			return err
		}
		l, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		cur, err := sc.songItem(ctx, id, item)
		if err != nil {
			return err
		}
		if len(in) > domain.MaxItemSongs {
			return domain.LimitError("songs", domain.MaxItemSongs, len(in))
		}
		cache := map[domain.SongID]domain.Song{}
		songs := make([]domain.LiturgySong, len(in))
		for i, si := range in {
			field := "songs." + strconv.Itoa(i) + "."
			song, ok := cache[si.SongID]
			if !ok {
				if song, err = songByID(ctx, sc.cs, si.SongID, field+"song_id"); err != nil {
					return err
				}
				cache[si.SongID] = song
			}
			ls := domain.LiturgySong{ID: domain.ItemSongID(u.IDs.NewID()), Position: i, SongID: song.ID, SongTitle: song.Title,
				Key: si.Key, Note: si.Note, Entries: make([]domain.Entry, len(si.Entries))}
			sections := make(map[domain.SectionID]domain.Section, len(song.Sections))
			for _, sec := range song.Sections {
				sections[sec.ID] = sec
			}
			for j, ei := range si.Entries {
				ef := field + "entries." + strconv.Itoa(j) + "."
				sec, ok := sections[ei.SectionID]
				if !ok {
					return &domain.InvalidInputError{Field: ef + "section_id", Message: "This section is not a section of the song."}
				}
				if ei.SingingPartID != "" {
					if _, err := sc.cs.SingingParts().ByID(ctx, string(ei.SingingPartID)); err != nil {
						if errors.Is(err, ErrNotFound) {
							return &domain.InvalidInputError{Field: ef + "singing_part_id", Message: "Unknown singing part."}
						}
						return err
					}
				}
				ls.Entries[j] = domain.Entry{ID: domain.EntryID(u.IDs.NewID()), Position: j, SectionID: sec.ID,
					SingingPartID: ei.SingingPartID, KeyChange: ei.KeyChange, Note: ei.Note, SectionLabel: domain.SectionDisplay(sec)}
			}
			if err := domain.ValidateItemSong(&ls, field); err != nil {
				return err
			}
			songs[i] = ls
		}
		ok, err := sc.cs.LiturgyItems().Bump(ctx, item, version, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return sc.staleItem(ctx, id, item)
		}
		if err := sc.cs.LiturgyItems().ReplaceSongs(ctx, item, songs); err != nil {
			return err
		}
		if err := u.record(ctx, sc, id, domain.CmdItemSongs, item, l.Version, version+1, songsImage(cur.Songs), songsImage(songs)); err != nil {
			return err
		}
		res, err = u.itemResult(ctx, sc, id, item, l.Version)
		return err
	})
	return res, err
}
