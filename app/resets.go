// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Resets holds the password-reset use cases (03 §9).
type Resets struct {
	Tx     Tx
	Hasher PasswordHasher
	Clock  Clock
	IDs    IDGenerator
	URLs   URLBuilder
	Auth   *Auth
}

// ResetLink is a new reset link for a user.
type ResetLink struct {
	User      domain.User
	Link      string
	ExpiresAt time.Time
}

// ResetInfo is what POST /auth/reset/inspect returns.
type ResetInfo struct {
	UserName      string
	CreatedByName *string // nil: created on the command line ("the server administrator")
	ExpiresAt     time.Time
}

// ResetInput uses a reset link.
type ResetInput struct {
	Token, NewPassword           string
	UserAgent, PreviousTokenHash string
}

// ResetResult is the new session after a reset, and whose password it was.
type ResetResult struct {
	LoginResult
	UserID domain.UserID
}

// CreateForMember creates a reset link for a member (members.manage plus
// the member's scopes); members of another church → reset_not_allowed.
func (u *Resets) CreateForMember(ctx context.Context, sess *domain.Session, id domain.MembershipID) (ResetLink, error) {
	token, hash := domain.NewToken()
	var res ResetLink
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		m, err := sc.cs.Memberships().ByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return notFound(ReasonMissing)
		}
		if err != nil {
			return err
		}
		if err := sc.actor.RequireHeld(sc.scopesOf(m.RoleIDs)); err != nil {
			return err
		}
		elsewhere, err := memberElsewhere(ctx, s, m.UserID, sc.actor.ChurchID)
		if err != nil {
			return err
		}
		if elsewhere {
			return ErrResetNotAllowed
		}
		res, err = u.create(ctx, s, m.UserID, sc.actor.UserID, hash)
		return err
	})
	if err != nil {
		return ResetLink{}, err
	}
	res.Link = u.URLs.AppURL(ctx, "/reset") + "#t=" + token
	return res, nil
}

// CreateForUser creates a reset link from the command line: no church or
// scope check (the operator controls the server). Unknown identifier → ErrNotFound.
func (u *Resets) CreateForUser(ctx context.Context, identifier string) (ResetLink, error) {
	ident, err := domain.ParseIdentifier(identifier)
	if err != nil {
		return ResetLink{}, err
	}
	token, hash := domain.NewToken()
	var res ResetLink
	err = u.Tx.Write(ctx, func(s Store) error {
		usr, err := s.Users().ByIdentifier(ctx, ident)
		if err != nil {
			return err
		}
		res, err = u.create(ctx, s, usr.ID, "", hash)
		return err
	})
	if err != nil {
		return ResetLink{}, err
	}
	res.Link = u.URLs.AppURL(ctx, "/reset") + "#t=" + token
	return res, nil
}

// create takes LockUser, closes the user's unused links and inserts the new one.
func (u *Resets) create(ctx context.Context, s Store, user, by domain.UserID, hash string) (ResetLink, error) {
	if err := s.LockUser(ctx, user); err != nil {
		return ResetLink{}, err
	}
	usr, err := s.Users().ByID(ctx, user)
	if err != nil {
		return ResetLink{}, err
	}
	now := u.Clock.Now()
	if err := s.PasswordResets().CloseOpen(ctx, user, now); err != nil {
		return ResetLink{}, err
	}
	r := domain.PasswordReset{ID: u.IDs.NewID(), UserID: user, TokenHash: hash, CreatedBy: by,
		CreatedAt: now, ExpiresAt: now.Add(domain.ResetLifetime)}
	if err := s.PasswordResets().Create(ctx, r); err != nil {
		return ResetLink{}, err
	}
	return ResetLink{User: usr, ExpiresAt: r.ExpiresAt}, nil
}

// Inspect describes a reset link (public).
func (u *Resets) Inspect(ctx context.Context, token string) (ResetInfo, error) {
	var info ResetInfo
	err := u.Tx.Read(ctx, func(s Store) error {
		r, usr, err := u.usable(ctx, s, token)
		if err != nil {
			return err
		}
		info = ResetInfo{UserName: usr.Name, ExpiresAt: r.ExpiresAt}
		sum, err := resetSummary(ctx, s, r)
		info.CreatedByName = sum.CreatedByName
		return err
	})
	return info, err
}

func (u *Resets) usable(ctx context.Context, s Store, token string) (domain.PasswordReset, domain.User, error) {
	r, err := s.PasswordResets().ByTokenHash(ctx, domain.HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return domain.PasswordReset{}, domain.User{}, &InvalidTokenError{Reason: domain.TokenUnknown}
	}
	if err != nil {
		return domain.PasswordReset{}, domain.User{}, err
	}
	if reason := r.Reason(u.Clock.Now()); reason != "" {
		return domain.PasswordReset{}, domain.User{}, &InvalidTokenError{Reason: reason}
	}
	usr, err := s.Users().ByID(ctx, r.UserID)
	return r, usr, err
}

// Use sets a new password with a reset link. The password is checked against
// the user's identity and hashed outside any transaction; then one
// transaction claims the link, saves the hash, ends all the user's sessions,
// clears their identifier counters and creates a new session.
func (u *Resets) Use(ctx context.Context, in ResetInput) (ResetResult, error) {
	var (
		usr      domain.User
		churches []string
	)
	err := u.Tx.Read(ctx, func(s Store) error {
		var err error
		if _, usr, err = u.usable(ctx, s, in.Token); err != nil {
			return err
		}
		churches, err = churchNames(ctx, s, usr.ID)
		return err
	})
	if err != nil {
		return ResetResult{}, err
	}
	for _, cn := range append(churches, "") {
		if err := domain.CheckPassword(in.NewPassword, domain.PasswordIdentity{Name: usr.Name, Email: usr.Email, Phone: usr.Phone, ChurchName: cn}); err != nil {
			return ResetResult{}, err
		}
	}
	pwHash, err := u.Hasher.Hash(ctx, in.NewPassword)
	if err != nil {
		return ResetResult{}, err
	}
	var res ResetResult
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		r, ok, err := s.PasswordResets().Claim(ctx, domain.HashToken(in.Token), now)
		if err != nil {
			return err
		}
		if !ok {
			if _, _, err := u.usable(ctx, s, in.Token); err != nil {
				return err
			}
			return &InvalidTokenError{Reason: domain.TokenUsed}
		}
		if err := s.Users().SetPasswordHash(ctx, r.UserID, pwHash, now); err != nil {
			return err
		}
		if err := s.Sessions().DeleteAllForUser(ctx, r.UserID); err != nil {
			return err
		}
		for _, ident := range []string{usr.Email, usr.Phone} {
			if ident != "" {
				if err := s.AuthThrottle().DeleteForIdentifier(ctx, ident); err != nil {
					return err
				}
			}
		}
		res.UserID = r.UserID
		res.Token, res.Session, err = u.Auth.newSession(ctx, s, r.UserID, in.UserAgent, in.PreviousTokenHash, now)
		return err
	})
	return res, err
}

// churchNames returns the names of the churches the user belongs to.
func churchNames(ctx context.Context, s Store, user domain.UserID) ([]string, error) {
	ids, err := s.Users().MembershipChurchIDs(ctx, user)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		c, err := s.Churches().ByID(ctx, id)
		if err != nil {
			return nil, err
		}
		names = append(names, c.Name)
	}
	return names, nil
}
