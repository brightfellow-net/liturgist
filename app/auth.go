// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Auth holds the login and session use cases (03 §4, §5).
type Auth struct {
	Tx            Tx
	Hasher        PasswordHasher
	Clock         Clock
	SessionTTL    time.Duration
	SessionMaxAge time.Duration
}

// LoginInput is one login attempt.
type LoginInput struct {
	Identifier, Password string
	ClientAddr           string // domain.ClientAddrKey of the client
	UserAgent            string
	PreviousTokenHash    string // session cookie the request carried, if any
}

// LoginResult carries the new session; Token goes into the cookie.
type LoginResult struct {
	Token   string
	Session domain.Session
}

// Login checks the throttle counters, verifies outside any transaction, then
// records the outcome in one write transaction (03 §5).
func (a *Auth) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	now := a.Clock.Now()
	ident, parseErr := domain.ParseIdentifier(in.Identifier)
	keys := domain.ThrottleKeys(domain.ThrottleIdentifierInput(in.Identifier, ident, parseErr), in.ClientAddr)
	allKeys := []string{keys[domain.ThrottleIP], keys[domain.ThrottleIdentifierIP], keys[domain.ThrottleIdentifier]}

	var user domain.User
	found := false
	err := a.Tx.Read(ctx, func(s Store) error {
		until, err := s.AuthThrottle().LockedUntil(ctx, allKeys, now)
		if err != nil {
			return err
		}
		if !until.IsZero() {
			return &TooManyAttemptsError{RetryAfter: until.Sub(now)}
		}
		if parseErr != nil {
			return nil
		}
		user, err = s.Users().ByIdentifier(ctx, ident)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}

	ok, rehash := false, false
	if found {
		if ok, rehash, err = a.Hasher.Verify(ctx, user.PasswordHash, in.Password); err != nil {
			return LoginResult{}, err
		}
	} else if err := a.Hasher.VerifyDummy(ctx, in.Password); err != nil {
		return LoginResult{}, err
	}
	var newHash string
	if ok && rehash {
		if newHash, err = a.Hasher.Hash(ctx, in.Password); err != nil {
			return LoginResult{}, err
		}
	}

	if !ok {
		err := a.Tx.Write(ctx, func(s Store) error {
			for kind, key := range keys {
				if err := s.AuthThrottle().RecordFailure(ctx, key, domain.ThrottleRules[kind], now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	token, hash := domain.NewToken()
	sess := domain.Session{
		TokenHash: hash, UserID: user.ID, UserAgent: domain.TruncateUserAgent(in.UserAgent),
		CreatedAt: now, LastSeenAt: now,
		ExpiresAt: domain.SessionExpiry(now, now, a.SessionTTL, a.SessionMaxAge),
	}
	err = a.Tx.Write(ctx, func(s Store) error {
		if err := s.AuthThrottle().Delete(ctx, keys[domain.ThrottleIdentifierIP]); err != nil {
			return err
		}
		if in.PreviousTokenHash != "" {
			if err := s.Sessions().Delete(ctx, in.PreviousTokenHash); err != nil {
				return err
			}
		}
		if newHash != "" {
			if err := s.Users().SetPasswordHash(ctx, user.ID, newHash, now); err != nil {
				return err
			}
		}
		if err := s.Users().Touch(ctx, user.ID, now); err != nil {
			return err
		}
		return s.Sessions().Create(ctx, sess)
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, Session: sess}, nil
}

// Authenticate validates a session token and extends the session when
// last_seen_at is older than an hour (03 §4). refreshed tells the caller to
// re-send the cookie; this call has committed by then.
func (a *Auth) Authenticate(ctx context.Context, token string) (sess domain.Session, refreshed bool, err error) {
	if token == "" {
		return domain.Session{}, false, ErrUnauthenticated
	}
	now := a.Clock.Now()
	hash := domain.HashToken(token)
	err = a.Tx.Read(ctx, func(s Store) error {
		sess, err = s.Sessions().ByTokenHash(ctx, hash)
		return err
	})
	if errors.Is(err, ErrNotFound) || (err == nil && !sess.ExpiresAt.After(now)) {
		return domain.Session{}, false, ErrUnauthenticated
	}
	if err != nil {
		return domain.Session{}, false, err
	}
	if now.Sub(sess.LastSeenAt) <= domain.SessionExtendAfter {
		return sess, false, nil
	}
	expires := domain.SessionExpiry(sess.CreatedAt, now, a.SessionTTL, a.SessionMaxAge)
	extended := false
	err = a.Tx.Write(ctx, func(s Store) error {
		var err error
		if extended, err = s.Sessions().Extend(ctx, hash, now, expires); err != nil || !extended {
			return err
		}
		return s.Users().Touch(ctx, sess.UserID, now)
	})
	if err != nil {
		return domain.Session{}, false, err
	}
	if !extended { // ended or expired meanwhile: deletion wins
		return domain.Session{}, false, ErrUnauthenticated
	}
	sess.LastSeenAt, sess.ExpiresAt = now, expires
	return sess, true, nil
}

// Logout deletes the session (idempotent).
func (a *Auth) Logout(ctx context.Context, tokenHash string) error {
	return a.Tx.Write(ctx, func(s Store) error { return s.Sessions().Delete(ctx, tokenHash) })
}

// EndOtherSessions deletes every session of the user except the current one.
func (a *Auth) EndOtherSessions(ctx context.Context, user domain.UserID, keepHash string) error {
	return a.Tx.Write(ctx, func(s Store) error { return s.Sessions().DeleteOthers(ctx, user, keepHash) })
}
