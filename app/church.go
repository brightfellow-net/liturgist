// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// Churches holds the church settings use cases (04 §6).
type Churches struct {
	Tx    Tx
	Clock Clock
}

// ChurchActions are the advisory actions on the church (04 §5).
type ChurchActions struct {
	Edit bool `json:"edit"`
}

// ChurchResult is the church with its translation code and actions.
type ChurchResult struct {
	Church          domain.Church
	TranslationCode string
	Actions         ChurchActions
}

// ChurchChange is a PATCH /church: nil fields are unchanged; for FeedbackURL
// and PrivacyContact a pointer to "" clears the value (04 §6).
type ChurchChange struct {
	Name, DefaultUILanguage, DefaultLanguage, DefaultTranslationCode, TimeZone, KeyDisplay *string
	FeedbackURL, PrivacyContact                                                            *string
}

// Get returns the tenant church (baseline: every member).
func (c *Churches) Get(ctx context.Context, sess *domain.Session) (ChurchResult, error) {
	var res ChurchResult
	err := c.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		res, err = churchResult(ctx, s, sc)
		return err
	})
	return res, err
}

func churchResult(ctx context.Context, s Store, sc churchScope) (ChurchResult, error) {
	ch, err := sc.cs.Church().Get(ctx)
	if err != nil {
		return ChurchResult{}, err
	}
	tr, err := s.Translations().ByID(ctx, ch.DefaultTranslationID)
	if err != nil {
		return ChurchResult{}, err
	}
	return ChurchResult{Church: ch, TranslationCode: tr.Code,
		Actions: ChurchActions{Edit: sc.actor.Scopes.Has(domain.ScopeChurchSettings)}}, nil
}

// Update changes the church settings (church.settings).
func (c *Churches) Update(ctx context.Context, sess *domain.Session, ch ChurchChange) (ChurchResult, error) {
	if ch.TimeZone != nil {
		if err := validateTimeZone(*ch.TimeZone, "time_zone"); err != nil {
			return ChurchResult{}, err
		}
	}
	for field, v := range map[string]*string{"name": ch.Name, "default_ui_language": ch.DefaultUILanguage,
		"default_language": ch.DefaultLanguage, "default_translation_code": ch.DefaultTranslationCode, "key_display": ch.KeyDisplay} {
		if v != nil && *v == "" {
			return ChurchResult{}, &domain.InvalidInputError{Field: field, Message: "Must not be empty."}
		}
	}
	var res ChurchResult
	err := c.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeChurchSettings); err != nil {
			return err
		}
		church, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		set := func(dst *string, v *string) {
			if v != nil {
				*dst = *v
			}
		}
		set(&church.Name, ch.Name)
		set(&church.DefaultUILanguage, ch.DefaultUILanguage)
		set(&church.DefaultLanguage, ch.DefaultLanguage)
		set(&church.TimeZone, ch.TimeZone)
		set(&church.Settings.KeyDisplay, ch.KeyDisplay)
		set(&church.Settings.FeedbackURL, ch.FeedbackURL)
		set(&church.Settings.PrivacyContact, ch.PrivacyContact)
		if err := domain.ValidateChurch(&church, ""); err != nil {
			return err
		}
		if ch.DefaultTranslationCode != nil {
			tr, err := s.Translations().ByCode(ctx, *ch.DefaultTranslationCode)
			if errors.Is(err, ErrNotFound) {
				return &domain.InvalidInputError{Field: "default_translation_code", Message: "Unknown translation."}
			}
			if err != nil {
				return err
			}
			church.DefaultTranslationID = tr.ID
		}
		church.UpdatedAt = c.Clock.Now()
		if err := sc.cs.Church().Update(ctx, church); err != nil {
			return err
		}
		res, err = churchResult(ctx, s, sc)
		return err
	})
	return res, err
}

// Translations lists the Bible translations (public).
func (c *Churches) Translations(ctx context.Context) ([]domain.Translation, error) {
	var list []domain.Translation
	err := c.Tx.Read(ctx, func(s Store) (err error) { list, err = s.Translations().List(ctx); return })
	return list, err
}

// MeResult is GET /me: the user, and the church and membership when the
// request has a tenant and the user is a member of it.
type MeResult struct {
	User       domain.User
	Church     *ChurchResult
	Membership *MemberView
}

// MeView returns the logged-in user with the church and membership.
func (a *Account) MeView(ctx context.Context, sess *domain.Session) (MeResult, error) {
	if sess == nil {
		return MeResult{}, ErrUnauthenticated
	}
	var res MeResult
	err := a.Tx.Read(ctx, func(s Store) error {
		u, err := s.Users().ByID(ctx, sess.UserID)
		if err != nil {
			return err
		}
		res = MeResult{User: u}
		if _, err := TenantFrom(ctx); err != nil {
			return nil // no church yet
		}
		sc, err := actorIn(ctx, s, sess, false)
		if errors.Is(err, ErrNotFound) {
			return nil // not a member: church and membership stay null
		}
		if err != nil {
			return err
		}
		ch, err := churchResult(ctx, s, sc)
		if err != nil {
			return err
		}
		members, err := sc.cs.Memberships().List(ctx)
		if err != nil {
			return err
		}
		for _, m := range members {
			if m.ID == sc.actor.MembershipID {
				v := sc.memberView(m, members)
				res.Church, res.Membership = &ch, &v
			}
		}
		return nil
	})
	return res, err
}
