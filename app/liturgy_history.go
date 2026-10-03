// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"

	"github.com/brightfellow-net/liturgist/domain"
)

// The images of the history (10 §7): IDs and field values, never lyrics, and
// no size cap.
type (
	liturgyImage struct {
		Date        string      `json:"date"`
		Time        string      `json:"time"`
		ServiceID   string      `json:"service_id,omitempty"`
		ServiceName string      `json:"service_name"`
		Language    string      `json:"language"`
		TemplateID  string      `json:"template_id,omitempty"`
		Items       []itemImage `json:"items,omitempty"`
	}
	liturgyFieldsImage struct {
		Date        string `json:"date"`
		Time        string `json:"time"`
		ServiceName string `json:"service_name"`
	}
	itemImage struct {
		ID           string      `json:"id"`
		Position     int         `json:"position"`
		Title        string      `json:"title"`
		Type         string      `json:"item_type"`
		DutyID       string      `json:"duty_id,omitempty"`
		Text         string      `json:"text,omitempty"`
		ReadingID    string      `json:"reading_id,omitempty"`
		ReadingLabel string      `json:"reading_label,omitempty"`
		Version      int         `json:"version"`
		Songs        []songImage `json:"songs,omitempty"`
	}
	songImage struct {
		ID        string       `json:"id"`
		SongID    string       `json:"song_id,omitempty"`
		SongTitle string       `json:"song_title"`
		Key       string       `json:"key,omitempty"`
		Note      string       `json:"note,omitempty"`
		Entries   []entryImage `json:"entries,omitempty"`
	}
	entryImage struct {
		ID            string `json:"id"`
		SectionID     string `json:"section_id,omitempty"`
		SingingPartID string `json:"singing_part_id,omitempty"`
		KeyChange     string `json:"key_change,omitempty"`
		Note          string `json:"note,omitempty"`
		SectionLabel  string `json:"section_label,omitempty"`
	}
	assignmentImage struct {
		ID     string `json:"id"`
		DutyID string `json:"duty_id"`
		UserID string `json:"user_id,omitempty"`
		Name   string `json:"name,omitempty"`
	}
)

func songsImage(songs []domain.LiturgySong) []songImage {
	out := make([]songImage, len(songs))
	for i, s := range songs {
		si := songImage{ID: string(s.ID), SongID: string(s.SongID), SongTitle: s.SongTitle, Key: s.Key, Note: s.Note}
		for _, e := range s.Entries {
			si.Entries = append(si.Entries, entryImage{ID: string(e.ID), SectionID: string(e.SectionID),
				SingingPartID: string(e.SingingPartID), KeyChange: e.KeyChange, Note: e.Note, SectionLabel: e.SectionLabel})
		}
		out[i] = si
	}
	return out
}

func imageOfItem(it domain.Item) itemImage {
	return itemImage{ID: string(it.ID), Position: it.Position, Title: it.Title, Type: string(it.Type), DutyID: string(it.DutyID),
		Text: it.Text, ReadingID: string(it.ReadingID), ReadingLabel: it.ReadingLabel, Version: it.Version, Songs: songsImage(it.Songs)}
}

func imageOfItems(items []domain.Item) []itemImage {
	out := make([]itemImage, len(items))
	for i, it := range items {
		out[i] = imageOfItem(it)
	}
	return out
}

func imageOfLiturgy(l domain.Liturgy, items []domain.Item) liturgyImage {
	return liturgyImage{Date: l.Date, Time: l.Time, ServiceID: string(l.ServiceID), ServiceName: l.ServiceName,
		Language: l.Language, TemplateID: string(l.TemplateID), Items: imageOfItems(items)}
}

func imageOfAssignment(a domain.Assignment) assignmentImage {
	return assignmentImage{ID: string(a.ID), DutyID: string(a.DutyID), UserID: string(a.UserID), Name: a.Name}
}

// record writes the history row of a change (10 §7): the number from
// edit_seq, taken last so the row lock of the liturgy is held to the commit
// (10 §5), then the row, in the caller's transaction. before and after are
// nil or a value that marshals to a JSON image. itemVersion is 0 when the
// change is not about one item.
func (u *Liturgies) record(ctx context.Context, sc churchScope, id domain.LiturgyID, cmd string, item domain.ItemID,
	liturgyVersion, itemVersion int, before, after any) error {
	seq, err := sc.cs.Liturgies().NextSeq(ctx, id)
	if err != nil {
		return err
	}
	e := domain.Edit{ID: domain.EditID(u.IDs.NewID()), LiturgyID: id, Seq: seq, UserID: sc.actor.UserID, Command: cmd,
		ItemID: item, LiturgyVersionAfter: liturgyVersion, ItemVersionAfter: itemVersion, Status: domain.EditDone,
		CreatedAt: u.Clock.Now()}
	if before != nil {
		if e.Before, err = json.Marshal(before); err != nil {
			return err
		}
	}
	if after != nil {
		if e.After, err = json.Marshal(after); err != nil {
			return err
		}
	}
	return sc.cs.Edits().Append(ctx, e)
}
