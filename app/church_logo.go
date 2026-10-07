// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"

	"github.com/brightfellow-net/liturgist/domain"
)

// NormalizedImage is a logo ready to store: a PNG that fits in
// domain.LogoFitSide, with its size.
type NormalizedImage struct {
	PNG           []byte
	Width, Height int
}

// ImageNormalizer checks an uploaded image and turns it into the stored PNG
// (15 §2, L-2). Errors that the sender can fix are *domain.InvalidInputError
// on the field "image".
type ImageNormalizer interface {
	Normalize(data []byte) (NormalizedImage, error)
}

// ChurchLogos holds the church logo use cases (15 §2).
type ChurchLogos struct {
	Tx      Tx
	Clock   Clock
	Storage Storage
	Images  ImageNormalizer
	Log     *slog.Logger // optional
}

// LogoFile is the stored logo, to be read and closed by the caller.
type LogoFile struct {
	domain.ChurchLogo
	io.ReadCloser
}

// Set replaces the church's logo with data (church.settings). The file is
// written before the setting changes, so a failure leaves at most an unused
// file, never a setting that points at nothing (15 §2, L-3).
func (l *ChurchLogos) Set(ctx context.Context, sess *domain.Session, data []byte) (ChurchResult, error) {
	// The scope is checked first, so a sender without it does not cost an image decode.
	if err := l.Tx.Read(ctx, func(s Store) error { return l.require(ctx, s, sess, false) }); err != nil {
		return ChurchResult{}, err
	}
	switch {
	case len(data) == 0:
		return ChurchResult{}, &domain.InvalidInputError{Field: "image", Message: "Choose an image."}
	case len(data) > domain.LogoMaxBytes:
		return ChurchResult{}, &domain.InvalidInputError{Field: "image", Message: "Use an image under 2 MB."}
	}
	img, err := l.Images.Normalize(data)
	if err != nil {
		return ChurchResult{}, err
	}
	sum := sha256.Sum256(img.PNG)
	logo := domain.ChurchLogo{Version: hex.EncodeToString(sum[:8]), Width: img.Width, Height: img.Height}
	t, err := TenantFrom(ctx)
	if err != nil {
		return ChurchResult{}, err
	}
	if err := l.Storage.Put(ctx, domain.LogoKey(t.ChurchID, logo.Version), bytes.NewReader(img.PNG)); err != nil {
		return ChurchResult{}, err
	}
	var old *domain.ChurchLogo
	var res ChurchResult
	err = l.Tx.Write(ctx, func(s Store) error {
		return l.change(ctx, s, sess, &old, &res, func(ch *domain.Church) bool { ch.Settings.Logo = &logo; return true })
	})
	if err != nil {
		return ChurchResult{}, err
	}
	if old != nil && old.Version != logo.Version {
		l.discard(ctx, t.ChurchID, old.Version)
	}
	return res, nil
}

// Remove clears the logo (church.settings). Without a logo it changes nothing.
func (l *ChurchLogos) Remove(ctx context.Context, sess *domain.Session) (ChurchResult, error) {
	var old *domain.ChurchLogo
	var res ChurchResult
	err := l.Tx.Write(ctx, func(s Store) error {
		return l.change(ctx, s, sess, &old, &res, func(ch *domain.Church) bool { had := ch.Settings.Logo != nil; ch.Settings.Logo = nil; return had })
	})
	if err != nil {
		return ChurchResult{}, err
	}
	if old != nil {
		t, err := TenantFrom(ctx)
		if err != nil {
			return res, nil // cannot happen after a successful write; the file stays
		}
		l.discard(ctx, t.ChurchID, old.Version)
	}
	return res, nil
}

// Open returns the stored logo (baseline: every member). ErrNotFound when the
// church has none or the file is gone.
func (l *ChurchLogos) Open(ctx context.Context, sess *domain.Session) (LogoFile, error) {
	var logo *domain.ChurchLogo
	var church domain.ChurchID
	err := l.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		ch, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return err
		}
		logo, church = ch.Settings.Logo, ch.ID
		return nil
	})
	if err != nil {
		return LogoFile{}, err
	}
	if logo == nil {
		return LogoFile{}, ErrNotFound
	}
	f, err := l.Storage.Open(ctx, domain.LogoKey(church, logo.Version))
	if err != nil {
		return LogoFile{}, err
	}
	return LogoFile{ChurchLogo: *logo, ReadCloser: f}, nil
}

func (l *ChurchLogos) require(ctx context.Context, s Store, sess *domain.Session, lock bool) error {
	sc, err := actorIn(ctx, s, sess, lock)
	if err != nil {
		return err
	}
	return sc.actor.Require(domain.ScopeChurchSettings)
}

// change applies edit to the church under the lock; edit says whether it
// changed anything. It reports the old logo and the church as it is after.
func (l *ChurchLogos) change(ctx context.Context, s Store, sess *domain.Session, old **domain.ChurchLogo, res *ChurchResult, edit func(*domain.Church) bool) error {
	sc, err := actorIn(ctx, s, sess, true)
	if err != nil {
		return err
	}
	if err := sc.actor.Require(domain.ScopeChurchSettings); err != nil {
		return err
	}
	ch, err := sc.cs.Church().Get(ctx)
	if err != nil {
		return err
	}
	*old = ch.Settings.Logo
	if edit(&ch) {
		ch.UpdatedAt = l.Clock.Now()
		if err := sc.cs.Church().Update(ctx, ch); err != nil {
			return err
		}
	}
	*res, err = churchResult(ctx, s, sc)
	return err
}

// discard deletes an old logo file. A failure only leaves an unused file.
func (l *ChurchLogos) discard(ctx context.Context, church domain.ChurchID, version string) {
	if err := l.Storage.Delete(ctx, domain.LogoKey(church, version)); err != nil && l.Log != nil {
		l.Log.Warn("logo_file_not_deleted", "version", version, "error", err.Error())
	}
}
