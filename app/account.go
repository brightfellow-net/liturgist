// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain"
)

// Account holds the "my account" use cases (04 §6).
type Account struct {
	Tx     Tx
	Hasher PasswordHasher
	Clock  Clock
	Auth   *Auth // shares login throttling for wrong current passwords
}

// ProfileChange is a PATCH: nil fields are unchanged (04 §6).
type ProfileChange struct {
	Name          *string
	TextSize      *string // normal | large | larger
	UILanguage    *string // en | id
	ClearLanguage bool    // ui_language: null → follow the church default
}

// Me returns the logged-in user.
func (a *Account) Me(ctx context.Context, sess *domain.Session) (domain.User, error) {
	if sess == nil {
		return domain.User{}, ErrUnauthenticated
	}
	var u domain.User
	err := a.Tx.Read(ctx, func(s Store) (err error) { u, err = s.Users().ByID(ctx, sess.UserID); return })
	return u, err
}

// UpdateProfile changes the user's name and preferences.
func (a *Account) UpdateProfile(ctx context.Context, sess *domain.Session, ch ProfileChange) (domain.User, error) {
	if sess == nil {
		return domain.User{}, ErrUnauthenticated
	}
	var name string
	if ch.Name != nil {
		name = strings.TrimSpace(*ch.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 120 {
			return domain.User{}, &domain.InvalidInputError{Field: "name", Message: "Name must be 1 to 120 characters."}
		}
	}
	if ch.TextSize != nil && *ch.TextSize != "normal" && *ch.TextSize != "large" && *ch.TextSize != "larger" {
		return domain.User{}, &domain.InvalidInputError{Field: "preferences.text_size", Message: "Text size must be normal, large or larger."}
	}
	if ch.UILanguage != nil && *ch.UILanguage != "en" && *ch.UILanguage != "id" {
		return domain.User{}, &domain.InvalidInputError{Field: "preferences.ui_language", Message: "Language must be en or id."}
	}
	var u domain.User
	err := a.Tx.Write(ctx, func(s Store) error {
		var err error
		if u, err = s.Users().ByID(ctx, sess.UserID); err != nil {
			return err
		}
		if ch.Name != nil {
			u.Name = name
		}
		if ch.TextSize != nil {
			u.Preferences.TextSize = *ch.TextSize
		}
		if ch.UILanguage != nil {
			u.Preferences.UILanguage = *ch.UILanguage
		}
		if ch.ClearLanguage {
			u.Preferences.UILanguage = ""
		}
		return s.Users().UpdateProfile(ctx, u.ID, u.Name, u.Preferences, a.Clock.Now())
	})
	return u, err
}

// ChangePassword verifies the current password (a failure counts toward the
// login throttle), then saves the new hash and ends all other sessions (03 §9).
func (a *Account) ChangePassword(ctx context.Context, sess *domain.Session, clientAddr, current, next string) error {
	user, err := a.Me(ctx, sess)
	if err != nil {
		return err
	}
	if err := a.Auth.CheckPassword(ctx, user, current, clientAddr); err != nil {
		return err
	}
	// The church name joins this check when churches exist (slice 4).
	if err := domain.CheckPassword(next, domain.PasswordIdentity{Name: user.Name, Email: user.Email, Phone: user.Phone}); err != nil {
		return err
	}
	hash, err := a.Hasher.Hash(ctx, next)
	if err != nil {
		return err
	}
	return a.Tx.Write(ctx, func(s Store) error {
		if err := s.Users().SetPasswordHash(ctx, user.ID, hash, a.Clock.Now()); err != nil {
			return err
		}
		return s.Sessions().DeleteOthers(ctx, user.ID, sess.TokenHash)
	})
}
