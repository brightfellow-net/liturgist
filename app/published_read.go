// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Reading the published copy is open to every member of the church, whatever
// their scopes and whatever state the liturgy is in now (13 §5, P-79). These
// use cases never call openLiturgy: they are a resource of their own.

// MaxPublishedPage is the largest page of the published list.
const MaxPublishedPage = 100

// myAssignmentsMax is the number of cards of "my assignments" (13 §5, P-80).
const myAssignmentsMax = 50

// PublishedRender is how the church wants the copy shown now (13 §5): the
// key display, the credits switch and the print defaults.
type PublishedRender struct {
	KeyDisplay  string
	ShowCredits bool
	Print       domain.PrintDefaults
}

// PublishedCopy is the newest published version of a liturgy.
type PublishedCopy struct {
	Number        int
	PublishedAt   time.Time
	PublishedByID domain.UserID
	PublishedBy   string
	Revising      bool
	Archived      bool
	Content       domain.PublishedContent
	Render        PublishedRender
	URL           string
}

// PublishedListItem is a row of the published list.
type PublishedListItem struct {
	Liturgy     domain.Liturgy
	Number      int
	PublishedAt time.Time
}

// PublishedPage is a page of the published list.
type PublishedPage struct {
	Items []PublishedListItem
	Total int
}

// decodeContent reads the stored JSON of a version.
func decodeContent(v domain.PublishedVersion) (domain.PublishedContent, error) {
	var c domain.PublishedContent
	if err := json.Unmarshal(v.Content, &c); err != nil {
		return c, fmt.Errorf("published version %s: %w", v.ID, err)
	}
	return c, nil
}

// ReadPublished returns the newest published version of a liturgy. A liturgy
// without a version is a 404 for everybody.
func (u *Liturgies) ReadPublished(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (PublishedCopy, error) {
	var res PublishedCopy
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		v, err := sc.cs.Published().Latest(ctx, id)
		if err != nil {
			return missing(err)
		}
		l, err := sc.cs.Liturgies().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		content, err := decodeContent(v)
		if err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		res = PublishedCopy{
			Number: v.Number, PublishedAt: v.PublishedAt, PublishedByID: v.PublishedBy,
			Revising: l.State != domain.StatePublished, Archived: l.ArchivedAt != nil, Content: content,
			Render: PublishedRender{KeyDisplay: church.Settings.KeyDisplay, ShowCredits: church.Settings.CreditsShown(), Print: church.Settings.PrintOrDefault()},
		}
		switch by, err := sc.users.ByID(ctx, v.PublishedBy); {
		case err == nil:
			res.PublishedBy = by.Name
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if u.URLs != nil {
			res.URL = u.URLs.AppURL(ctx, "/published/"+string(id))
		}
		return nil
	})
	return res, err
}

// ListPublished lists the liturgies that have a published version, newest
// first. Limit 0 means 50; more than MaxPublishedPage is the caller's error.
func (u *Liturgies) ListPublished(ctx context.Context, sess *domain.Session, f PublishedFilter) (PublishedPage, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	var res PublishedPage
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		rows, total, err := sc.cs.Published().ListLatest(ctx, f)
		if err != nil {
			return err
		}
		res = PublishedPage{Total: total, Items: make([]PublishedListItem, len(rows))}
		for i, r := range rows {
			res.Items[i] = PublishedListItem{Liturgy: r.Liturgy, Number: r.Number, PublishedAt: r.PublishedAt}
		}
		return nil
	})
	return res, err
}

// MyAssignmentItem is an item of a card that belongs to one of the viewer's duties.
type MyAssignmentItem struct {
	ID    domain.ItemID
	Title string
	Type  domain.ItemType
}

// MyAssignment is a card of "my assignments".
type MyAssignment struct {
	Liturgy  domain.Liturgy
	Number   int
	Revising bool
	Duties   []domain.PublishedRef
	Items    []MyAssignmentItem
}

// MyAssignments is the answer of "my assignments".
type MyAssignments struct {
	Items []MyAssignment
	More  bool
}

// MyAssignments lists the viewer's duties in the newest published version of
// each upcoming liturgy (13 §5, P-80). "Today" is the church's calendar date in
// its time zone, compared as a string with the liturgy's date.
func (u *Liturgies) MyAssignments(ctx context.Context, sess *domain.Session) (MyAssignments, error) {
	var res MyAssignments
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(church.TimeZone)
		if err != nil {
			return err
		}
		today := u.Clock.Now().In(loc).Format("2006-01-02")
		rows, err := sc.cs.Published().Upcoming(ctx, sc.actor.UserID, today, myAssignmentsMax+1)
		if err != nil {
			return err
		}
		res = MyAssignments{Items: []MyAssignment{}}
		if len(rows) > myAssignmentsMax {
			res.More, rows = true, rows[:myAssignmentsMax]
		}
		for _, r := range rows {
			content, err := decodeContent(r.Version)
			if err != nil {
				return err
			}
			card := MyAssignment{Liturgy: r.Liturgy, Number: r.Version.Number, Revising: r.Liturgy.State != domain.StatePublished,
				Duties: []domain.PublishedRef{}, Items: []MyAssignmentItem{}}
			mine := map[string]bool{}
			for _, a := range content.Assignments {
				if a.UserID == sc.actor.UserID && !mine[a.Duty.ID] {
					mine[a.Duty.ID] = true
					card.Duties = append(card.Duties, a.Duty)
				}
			}
			for _, it := range content.Items {
				if it.Duty != nil && mine[it.Duty.ID] {
					card.Items = append(card.Items, MyAssignmentItem{ID: it.ID, Title: it.Title, Type: it.Type})
				}
			}
			res.Items = append(res.Items, card)
		}
		return nil
	})
	return res, err
}
