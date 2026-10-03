// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// CreateInput creates one liturgy (10 §3): a service and a time, or a name
// for a one-off. A nil TemplateID takes the service's default; a pointer to ""
// makes an empty liturgy.
type CreateInput struct {
	Date, Time  string
	ServiceID   domain.ServiceID
	ServiceName string
	Language    string
	TemplateID  *domain.TemplateID
}

type plannedLiturgy struct {
	liturgy      domain.Liturgy
	items        []domain.Item
	templateName string
}

// plan builds the liturgy and its items from the input and the service and
// template as they are now (10 §3 rules 2 to 4). field prefixes the paths of
// errors ("occurrences.2." in a batch).
func (u *Liturgies) plan(ctx context.Context, sc churchScope, church domain.Church, in CreateInput, field string, now time.Time) (plannedLiturgy, error) {
	l := domain.Liturgy{ID: domain.LiturgyID(u.IDs.NewID()), Date: in.Date, Time: in.Time, ServiceName: in.ServiceName,
		State: domain.StateDraft, Version: 1, CreatedBy: sc.actor.UserID, CreatedAt: now, UpdatedAt: now}
	var svc domain.Service
	if in.ServiceID != "" {
		var err error
		if svc, err = sc.cs.Services().ByID(ctx, in.ServiceID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return plannedLiturgy{}, &domain.InvalidInputError{Field: field + "service_id", Message: "Unknown service."}
			}
			return plannedLiturgy{}, err
		}
		l.ServiceID = svc.ID
		if l.ServiceName == "" {
			l.ServiceName = svc.Name
		}
	}
	tplID := svc.DefaultTemplateID
	if in.TemplateID != nil {
		tplID = *in.TemplateID
	}
	var tpl *domain.Template
	if tplID != "" {
		t, err := sc.cs.Templates().ByID(ctx, tplID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return plannedLiturgy{}, &domain.InvalidInputError{Field: field + "template_id", Message: "Unknown template."}
			}
			return plannedLiturgy{}, err
		}
		tpl = &t
	}
	switch {
	case in.Language != "":
		l.Language = in.Language
	case svc.ID != "":
		l.Language = svc.Language
	case tpl != nil:
		l.Language = tpl.Language
	default:
		l.Language = church.DefaultLanguage
	}
	if !slices.Contains(domain.ContentLanguages, l.Language) {
		return plannedLiturgy{}, &domain.InvalidInputError{Field: field + "language", Message: "Must be id, en, zh-Hans or zh-Hant."}
	}
	if err := domain.ValidateLiturgyFields(&l); err != nil {
		return plannedLiturgy{}, err
	}
	p := plannedLiturgy{liturgy: l}
	if tpl != nil {
		if tpl.Language != l.Language {
			return plannedLiturgy{}, &domain.InvalidInputError{Field: field + "template_id",
				Message: "The template is in another language than the liturgy.", Reason: domain.ReasonLanguageMismatch}
		}
		p.liturgy.TemplateID, p.templateName = tpl.ID, tpl.Name
		for i, ti := range tpl.Items {
			p.items = append(p.items, domain.Item{ID: domain.ItemID(u.IDs.NewID()), LiturgyID: l.ID, Position: i, Title: ti.Title,
				Type: ti.Type, DutyID: ti.DefaultDutyID, Text: ti.DefaultText, Version: 1, CreatedAt: now, UpdatedAt: now})
		}
	}
	return p, nil
}

// store writes a planned liturgy with its items and its history row.
func (u *Liturgies) store(ctx context.Context, sc churchScope, p plannedLiturgy) error {
	if err := sc.cs.Liturgies().Create(ctx, p.liturgy); err != nil {
		return mapSlotTaken(err)
	}
	for _, it := range p.items {
		if err := sc.cs.LiturgyItems().Insert(ctx, it); err != nil {
			return err
		}
	}
	return u.record(ctx, sc, p.liturgy.ID, domain.CmdLiturgyCreate, "", 1, 0, nil, imageOfLiturgy(p.liturgy, p.items))
}

// mapSlotTaken turns the slot index's violation into liturgy_exists when the
// liturgy is not known (the checks under the lock normally name it).
func mapSlotTaken(err error) error {
	var uq *UniqueError
	if errors.As(err, &uq) && uq.Constraint == "liturgies_service_slot_key" {
		return &LiturgyExistsError{}
	}
	return err
}

// slotTaken is 409 liturgy_exists when another liturgy of the church holds
// the slot of l. Under LockChurch the answer cannot go stale.
func slotTaken(ctx context.Context, cs ChurchStore, l domain.Liturgy) error {
	if l.ServiceID == "" {
		return nil
	}
	slots, err := cs.Liturgies().Slots(ctx, l.Date, l.Date)
	if err != nil {
		return err
	}
	if id, ok := slots[Slot{ServiceID: l.ServiceID, Date: l.Date, Time: l.Time}]; ok && id != l.ID {
		return &LiturgyExistsError{ID: id}
	}
	return nil
}

// Create makes one liturgy (liturgy.edit), copying the items of its template.
func (u *Liturgies) Create(ctx context.Context, sess *domain.Session, in CreateInput) (LiturgyView, error) {
	if sess == nil {
		return LiturgyView{}, ErrUnauthenticated
	}
	lim, err := u.limits(ctx)
	if err != nil {
		return LiturgyView{}, err
	}
	var res LiturgyView
	err = u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyEdit); err != nil {
			return err
		}
		if err := lim.check(ctx, sc.cs, 1); err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		p, err := u.plan(ctx, sc, church, in, "", u.Clock.Now())
		if err != nil {
			return err
		}
		if err := slotTaken(ctx, sc.cs, p.liturgy); err != nil {
			return err
		}
		if err := u.store(ctx, sc, p); err != nil {
			return err
		}
		l, err := sc.cs.Liturgies().ByID(ctx, p.liturgy.ID)
		if err != nil {
			return err
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}

// Occurrence is one service time in a week (10 §3.1).
type Occurrence struct {
	ServiceID    domain.ServiceID
	ServiceName  string
	Language     string
	Date, Time   string
	TemplateID   domain.TemplateID
	TemplateName string
	LiturgyID    domain.LiturgyID // "" when no liturgy exists for the slot
}

// PrepareLimit is one limit with its use, for the prepare page.
type PrepareLimit struct {
	Unlimited bool
	Max, Used int
}

// PrepareWeek is the answer of GET /liturgies/prepare.
type PrepareWeek struct {
	Week        string // the Monday
	Occurrences []Occurrence
	Active      PrepareLimit
	Unpublished PrepareLimit
}

// Prepare lists the occurrences of the week containing week (YYYY-MM-DD), or of
// next week in the church's time zone when week is empty (liturgy.edit, 10 §3.1).
func (u *Liturgies) Prepare(ctx context.Context, sess *domain.Session, week string) (PrepareWeek, error) {
	if sess == nil {
		return PrepareWeek{}, ErrUnauthenticated
	}
	if week != "" && !domain.ValidDate(week) {
		return PrepareWeek{}, &domain.InvalidInputError{Field: "week", Message: "Use a real date as YYYY-MM-DD."}
	}
	lim, err := u.limits(ctx)
	if err != nil {
		return PrepareWeek{}, err
	}
	var res PrepareWeek
	err = u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyEdit); err != nil {
			return err
		}
		var day time.Time
		if week != "" {
			day, _ = time.Parse("2006-01-02", week)
		} else {
			church, err := sc.cs.Church().Get(ctx)
			if err != nil {
				return err
			}
			loc, err := time.LoadLocation(church.TimeZone)
			if err != nil {
				return err
			}
			n := u.Clock.Now().In(loc)
			day = time.Date(n.Year(), n.Month(), n.Day()+7, 0, 0, 0, 0, time.UTC)
		}
		start := domain.WeekStart(day)
		res = PrepareWeek{Week: start.Format("2006-01-02")}
		services, err := sc.cs.Services().List(ctx)
		if err != nil {
			return err
		}
		names, err := templateNames(ctx, sc.cs)
		if err != nil {
			return err
		}
		end := start.AddDate(0, 0, 6).Format("2006-01-02")
		slots, err := sc.cs.Liturgies().Slots(ctx, res.Week, end)
		if err != nil {
			return err
		}
		for _, sv := range services {
			for _, t := range sv.Times {
				date := start.AddDate(0, 0, t.Weekday-1).Format("2006-01-02")
				res.Occurrences = append(res.Occurrences, Occurrence{ServiceID: sv.ID, ServiceName: sv.Name, Language: sv.Language,
					Date: date, Time: t.Time, TemplateID: sv.DefaultTemplateID, TemplateName: names[sv.DefaultTemplateID],
					LiturgyID: slots[Slot{ServiceID: sv.ID, Date: date, Time: t.Time}]})
			}
		}
		slices.SortFunc(res.Occurrences, func(a, b Occurrence) int {
			switch {
			case a.Date != b.Date:
				return cmpString(a.Date, b.Date)
			case a.Time != b.Time:
				return cmpString(a.Time, b.Time)
			}
			return cmpString(a.ServiceName, b.ServiceName)
		})
		if res.Active, res.Unpublished, err = limitUse(ctx, sc.cs, lim); err != nil {
			return err
		}
		return nil
	})
	return res, err
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func limitUse(ctx context.Context, cs ChurchStore, lim liturgyLimits) (active, unpublished PrepareLimit, err error) {
	active = PrepareLimit{Unlimited: lim.active.Unlimited, Max: lim.active.Max}
	unpublished = PrepareLimit{Unlimited: lim.unpublished.Unlimited, Max: lim.unpublished.Max}
	if active.Used, err = cs.Liturgies().CountActive(ctx, nil); err != nil {
		return
	}
	unpublished.Used, err = cs.Liturgies().CountActive(ctx, unpublishedStates)
	return
}

// PrepareEntry is one occurrence to create.
type PrepareEntry struct {
	ServiceID  domain.ServiceID
	Date, Time string
}

// Prepared is a liturgy made by PrepareCreate.
type Prepared struct {
	LiturgyID    domain.LiturgyID
	ServiceName  string
	Date, Time   string
	Language     string
	TemplateID   domain.TemplateID
	TemplateName string
}

// PrepareCreate makes one liturgy per entry, all or nothing (liturgy.edit,
// 10 §3.1). Each entry must be a real occurrence of its service.
func (u *Liturgies) PrepareCreate(ctx context.Context, sess *domain.Session, entries []PrepareEntry) ([]Prepared, error) {
	if sess == nil {
		return nil, ErrUnauthenticated
	}
	if len(entries) < 1 || len(entries) > domain.MaxPrepareOccurence {
		return nil, &domain.InvalidInputError{Field: "occurrences", Message: "1 to 50 occurrences.", Reason: domain.ReasonLimit, Max: domain.MaxPrepareOccurence, Used: len(entries)}
	}
	lim, err := u.limits(ctx)
	if err != nil {
		return nil, err
	}
	var res []Prepared
	err = u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyEdit); err != nil {
			return err
		}
		if err := lim.check(ctx, sc.cs, len(entries)); err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		now := u.Clock.Now()
		res = make([]Prepared, 0, len(entries))
		plans := make([]plannedLiturgy, len(entries))
		seen := map[Slot]bool{}
		from, to := entries[0].Date, entries[0].Date
		for _, e := range entries {
			from, to = min(from, e.Date), max(to, e.Date)
		}
		slots, err := sc.cs.Liturgies().Slots(ctx, from, to)
		if err != nil {
			return err
		}
		for i, e := range entries {
			field := "occurrences." + strconv.Itoa(i) + "."
			if !domain.ValidDate(e.Date) {
				return &domain.InvalidInputError{Field: field + "date", Message: "Use a real date as YYYY-MM-DD."}
			}
			svc, err := sc.cs.Services().ByID(ctx, e.ServiceID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return &domain.InvalidInputError{Field: field + "service_id", Message: "Unknown service."}
				}
				return err
			}
			day, _ := time.Parse("2006-01-02", e.Date)
			if !slices.ContainsFunc(svc.Times, func(t domain.ServiceTime) bool { return t.Weekday == domain.ISOWeekday(day) && t.Time == e.Time }) {
				return &domain.InvalidInputError{Field: field + "time", Message: "The service has no occurrence at this date and time."}
			}
			slot := Slot{ServiceID: e.ServiceID, Date: e.Date, Time: e.Time}
			if id, ok := slots[slot]; ok {
				return &LiturgyExistsError{ID: id}
			}
			if seen[slot] {
				return &domain.InvalidInputError{Field: field + "date", Message: "This occurrence is listed twice."}
			}
			seen[slot] = true
			if plans[i], err = u.plan(ctx, sc, church, CreateInput{Date: e.Date, Time: e.Time, ServiceID: e.ServiceID}, field, now); err != nil {
				return err
			}
		}
		for _, p := range plans {
			if err := u.store(ctx, sc, p); err != nil {
				return err
			}
			res = append(res, Prepared{LiturgyID: p.liturgy.ID, ServiceName: p.liturgy.ServiceName, Date: p.liturgy.Date,
				Time: p.liturgy.Time, Language: p.liturgy.Language, TemplateID: p.liturgy.TemplateID, TemplateName: p.templateName})
		}
		return nil
	})
	return res, err
}

// LiturgyChange is a PATCH of the liturgy's own fields (structural, 10 §5).
type LiturgyChange struct {
	Version                 int
	Date, Time, ServiceName *string
}

// Update changes date, time and service name (liturgy.edit). A service
// liturgy that moves onto a taken slot is 409 liturgy_exists.
func (u *Liturgies) Update(ctx context.Context, sess *domain.Session, id domain.LiturgyID, ch LiturgyChange) (LiturgyView, error) {
	var res LiturgyView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true) // the slot check needs the lock
		if err != nil {
			return err
		}
		cur, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		next := cur
		if ch.Date != nil {
			next.Date = *ch.Date
		}
		if ch.Time != nil {
			next.Time = *ch.Time
		}
		if ch.ServiceName != nil {
			next.ServiceName = *ch.ServiceName
		}
		if err := domain.ValidateLiturgyFields(&next); err != nil {
			return err
		}
		if err := slotTaken(ctx, sc.cs, next); err != nil {
			return err
		}
		next.UpdatedAt = u.Clock.Now()
		ok, err := sc.cs.Liturgies().Update(ctx, next, ch.Version)
		if err != nil {
			return mapSlotTaken(err)
		}
		if !ok {
			return sc.staleLiturgy(ctx, id)
		}
		if err := u.record(ctx, sc, id, domain.CmdLiturgyUpdate, "", ch.Version+1, 0,
			liturgyFieldsImage{Date: cur.Date, Time: cur.Time, ServiceName: cur.ServiceName},
			liturgyFieldsImage{Date: next.Date, Time: next.Time, ServiceName: next.ServiceName}); err != nil {
			return err
		}
		l, err := sc.cs.Liturgies().ByID(ctx, id)
		if err != nil {
			return err
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}
