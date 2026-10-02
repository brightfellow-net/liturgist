// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Setup creates the church and its first admin (03 §10).
type Setup struct {
	Tx     Tx
	Hasher PasswordHasher
	Clock  Clock
	IDs    IDGenerator
	Auth   *Auth  // session lifetimes
	OnDone func() // called after a church was created (refreshes the tenant resolver)
}

// ChurchInput is the church part of the setup form.
type ChurchInput struct {
	Name, DefaultUILanguage, DefaultLanguage, DefaultTranslationCode, TimeZone, KeyDisplay string
}

// SetupInput is one setup attempt. From the command line Token is empty and
// ViaCLI is set; no session is created then.
type SetupInput struct {
	Token             string
	ViaCLI            bool
	Church            ChurchInput
	AdminName         string
	AdminIdentifier   string
	AdminPassword     string
	UserAgent         string
	PreviousTokenHash string
}

// SetupResult is what setup created; Token/Session are empty for the CLI.
type SetupResult struct {
	Church  domain.Church
	User    domain.User
	Token   string
	Session domain.Session
}

// Status reports whether a church exists.
func (u *Setup) Status(ctx context.Context) (setUp bool, err error) {
	err = u.Tx.Read(ctx, func(s Store) error {
		n, err := s.Churches().Count(ctx)
		setUp = n > 0
		return err
	})
	return setUp, err
}

// IssueToken replaces the setup token with a new one valid for 24 hours, so
// the most recently issued link is the only valid one. ErrAlreadySetUp if a
// church exists.
func (u *Setup) IssueToken(ctx context.Context) (token string, expires time.Time, err error) {
	now := u.Clock.Now()
	token, hash := domain.NewToken()
	expires = now.Add(domain.SetupLifetime)
	err = u.Tx.Write(ctx, func(s Store) error {
		if err := s.LockInstall(ctx); err != nil {
			return err
		}
		n, err := s.Churches().Count(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadySetUp
		}
		return s.SetupTokens().Put(ctx, hash, now, expires)
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Run validates the form, hashes the password outside any transaction, then
// in one transaction under LockInstall: refuses if a church exists, claims the
// token (web only), and creates the church, the ready-made roles, the user, a
// membership with the Church admin role and (web only) a session.
func (u *Setup) Run(ctx context.Context, in SetupInput) (SetupResult, error) {
	church := domain.Church{
		Name: in.Church.Name, DefaultUILanguage: in.Church.DefaultUILanguage, DefaultLanguage: in.Church.DefaultLanguage,
		TimeZone: in.Church.TimeZone, Settings: domain.ChurchSettings{KeyDisplay: in.Church.KeyDisplay},
	}
	if err := domain.ValidateChurch(&church, "church."); err != nil {
		return SetupResult{}, err
	}
	if err := validateTimeZone(church.TimeZone, "church.time_zone"); err != nil {
		return SetupResult{}, err
	}
	name := in.AdminName
	if err := domain.ValidatePersonName(&name, "admin.name"); err != nil {
		return SetupResult{}, err
	}
	ident, err := domain.ParseIdentifier(in.AdminIdentifier)
	if err != nil {
		return SetupResult{}, err
	}
	user := domain.User{Name: name, Preferences: domain.Preferences{}}
	setIdentifier(&user, ident)
	if err := domain.CheckPassword(in.AdminPassword, domain.PasswordIdentity{
		Name: name, Email: user.Email, Phone: user.Phone, ChurchName: church.Name}); err != nil {
		return SetupResult{}, err
	}
	pwHash, err := u.Hasher.Hash(ctx, in.AdminPassword)
	if err != nil {
		return SetupResult{}, err
	}

	var res SetupResult
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		if err := s.LockInstall(ctx); err != nil {
			return err
		}
		n, err := s.Churches().Count(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadySetUp
		}
		if !in.ViaCLI {
			ok, err := s.SetupTokens().Claim(ctx, domain.HashToken(in.Token), now)
			if err != nil {
				return err
			}
			if !ok {
				return &InvalidTokenError{Reason: domain.TokenUnknown}
			}
		}
		tr, err := s.Translations().ByCode(ctx, in.Church.DefaultTranslationCode)
		if errors.Is(err, ErrNotFound) {
			return &domain.InvalidInputError{Field: "church.default_translation_code", Message: "Unknown translation."}
		}
		if err != nil {
			return err
		}
		c := church
		c.ID, c.DefaultTranslationID, c.CreatedAt, c.UpdatedAt = domain.ChurchID(u.IDs.NewID()), tr.ID, now, now
		if err := s.Churches().Create(ctx, c); err != nil {
			return err
		}
		cs, err := s.ForChurch(ctx, c.ID)
		if err != nil {
			return err
		}
		var adminRole domain.RoleID
		for _, rm := range domain.ReadyMadeRoles {
			r := readyMadeRole(rm, c.DefaultUILanguage, domain.RoleID(u.IDs.NewID()), now)
			if err := cs.Roles().Create(ctx, r); err != nil {
				return err
			}
			if rm.Origin == domain.OriginChurchAdmin {
				adminRole = r.ID
			}
		}
		usr := user
		usr.ID, usr.PasswordHash, usr.CreatedAt, usr.UpdatedAt, usr.LastSeenAt = domain.UserID(u.IDs.NewID()), pwHash, now, now, now
		if err := s.Users().Create(ctx, usr); err != nil {
			return mapIdentifierTaken(err)
		}
		if err := cs.Memberships().Create(ctx, domain.Membership{
			ID: domain.MembershipID(u.IDs.NewID()), UserID: usr.ID, RoleIDs: []domain.RoleID{adminRole}, CreatedAt: now}); err != nil {
			return err
		}
		res = SetupResult{Church: c, User: usr}
		if in.ViaCLI {
			return nil
		}
		res.Token, res.Session, err = u.Auth.newSession(ctx, s, usr.ID, in.UserAgent, in.PreviousTokenHash, now)
		return err
	})
	if err != nil {
		return SetupResult{}, err
	}
	if u.OnDone != nil {
		u.OnDone()
	}
	return res, nil
}

func readyMadeRole(rm domain.ReadyMadeRole, lang string, id domain.RoleID, now time.Time) domain.Role {
	return domain.Role{ID: id, Name: rm.Name(lang), Origin: rm.Origin,
		Scopes: domain.NewScopeSet(rm.Scopes...), CreatedAt: now, UpdatedAt: now}
}

func validateTimeZone(tz, field string) error {
	if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" {
		return &domain.InvalidInputError{Field: field, Message: "Unknown time zone."}
	}
	return nil
}

func setIdentifier(u *domain.User, id domain.Identifier) {
	if id.Kind == domain.Email {
		u.Email = id.Value
	} else {
		u.Phone = id.Value
	}
}

// mapIdentifierTaken turns a duplicate email or phone into ErrIdentifierTaken.
func mapIdentifierTaken(err error) error {
	var u *UniqueError
	if errors.As(err, &u) && (u.Constraint == "users_email_key" || u.Constraint == "users_phone_key") {
		return ErrIdentifierTaken
	}
	return err
}
