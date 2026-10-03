// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Seed creates the editable defaults (duties, singing parts, a starter
// template) for churches that do not have them yet (09 §3).
type Seed struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// Run seeds every church that has no marker, once, at start-up after the
// migrations. Each church is one transaction: the church lock, the marker
// check, the rows, and the marker last (09 §3). A church whose seed fails is
// rolled back whole and the others still run; the errors are returned
// together, so the caller can log them and still start.
func (u *Seed) Run(ctx context.Context) error {
	var churches []domain.ChurchID
	if err := u.Tx.Read(ctx, func(s Store) error {
		var err error
		churches, err = s.Churches().IDs(ctx, 1<<30)
		return err
	}); err != nil {
		return err
	}
	var errs []error
	for _, id := range churches {
		err := u.Tx.Write(ctx, func(s Store) error {
			cs, err := s.ForChurch(ctx, id)
			if err != nil {
				return err
			}
			return seedChurch(ctx, cs, u.IDs, u.Clock.Now())
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("church %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// seedChurch is the body of the seed: Setup calls it in the transaction that
// creates the church. It takes the church lock, does nothing when the marker
// exists, and otherwise creates the rows in the church's default content
// language and writes the marker last.
func seedChurch(ctx context.Context, cs ChurchStore, ids IDGenerator, now time.Time) error {
	if err := cs.LockChurch(ctx); err != nil {
		return err
	}
	done, err := cs.Seeds().Applied(ctx, domain.SeedKey)
	if err != nil || done {
		return err
	}
	ch, err := cs.Church().Get(ctx)
	if err != nil {
		return err
	}
	set := domain.SeedFor(ch.DefaultLanguage)
	dutyIDs := make([]string, len(set.Duties))
	for i, name := range set.Duties {
		dutyIDs[i] = ids.NewID()
		if err := seedEntry(ctx, cs.Duties(), domain.KindDuty, dutyIDs[i], name, i, now); err != nil {
			return err
		}
	}
	for i, name := range set.SingingParts {
		if err := seedEntry(ctx, cs.SingingParts(), domain.KindSingingPart, ids.NewID(), name, i, now); err != nil {
			return err
		}
	}
	t := domain.Template{ID: domain.TemplateID(ids.NewID()), Name: set.TemplateName, Language: ch.DefaultLanguage,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	for _, it := range set.Items {
		t.Items = append(t.Items, domain.TemplateItem{Title: it.Title, Type: it.Type})
	}
	if err := domain.ValidateTemplate(&t); err != nil {
		return err
	}
	for i := range t.Items {
		t.Items[i].ID = ids.NewID()
		t.Items[i].DefaultDutyID = domain.DutyID(dutyIDs[set.Items[i].Duty])
	}
	if err := cs.Templates().Create(ctx, t); err != nil {
		return err
	}
	return cs.Seeds().Mark(ctx, domain.SeedKey, now)
}

func seedEntry(ctx context.Context, repo NameListRepo, kind domain.ListKind, id, name string, pos int, now time.Time) error {
	key, err := domain.ValidateEntryName(kind, &name)
	if err != nil {
		return err
	}
	return repo.Create(ctx, domain.NameEntry{ID: id, Name: name, NameKey: key, Position: pos, CreatedAt: now})
}
