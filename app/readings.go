// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain"
)

// defaultProviderTimeout is the deadline of every BibleTextProvider lookup (07 §3.1).
const defaultProviderTimeout = 5 * time.Second

// Readings holds the readings use cases (07). Every member can view;
// library.edit saves, changes and deletes readings.
type Readings struct {
	Tx        Tx
	Clock     Clock
	IDs       IDGenerator
	Usage     ReadingUsage
	Providers []BibleTextProvider
	Log       *slog.Logger
	// ProviderTimeout is the deadline of every provider lookup; zero means
	// five seconds (07 §3.1).
	ProviderTimeout time.Duration
}

// ReadingActions are the advisory actions on a reading (04 §5).
type ReadingActions struct {
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

func readingActions(a Actor) ReadingActions {
	edit := a.Scopes.Has(domain.ScopeLibraryEdit)
	return ReadingActions{Edit: edit, Delete: edit}
}

// ReadingView is a whole reading.
type ReadingView struct {
	Reading domain.Reading
	Actions ReadingActions
}

// ReadingSummary is a reading in a list.
type ReadingSummary struct {
	ID               domain.ReadingID
	Reference        string
	Canonical        string
	ReferenceDisplay string
	Translation      domain.Translation
	Snippet          string
	Actions          ReadingActions
}

// ReadingListView is one page of readings.
type ReadingListView struct {
	Items []ReadingSummary
	Total int
}

// ReadingQuery is a list request (07 §4).
type ReadingQuery struct {
	Q, Translation string
	Limit, Offset  int
}

// ParsedReference is the result of Parse.
type ParsedReference struct {
	Reference domain.Reference
	Display   string
}

// ReadingInput creates a reading from text a member typed or pasted.
type ReadingInput struct {
	Reference, Translation, Text, Attribution string
}

// ReadingChange is a PATCH: nil fields are unchanged.
type ReadingChange struct {
	Version                             int
	Text, Attribution, ReferenceDisplay *string
}

// LookupResult is what GET /readings/lookup returns (07 §3.1, §4).
type LookupResult struct {
	Reference            domain.Reference
	Display              string
	Translation          domain.Translation
	Reading              *domain.Reading // a stored reading, which wins
	Provider             *BibleText      // text a provider offers
	ProviderError        bool            // a provider failed and nothing was found
	SuggestedAttribution string
	Actions              ReadingActions
}

const (
	defaultReadingLimit = 50
	maxReadingLimit     = 100
)

// Parse reads a typed reference (any member); the web app's live preview
// uses it, so there is one parser (07 §2.3).
func (u *Readings) Parse(ctx context.Context, sess *domain.Session, input string) (ParsedReference, error) {
	ref, err := domain.ParseReference(input)
	if err != nil {
		return ParsedReference{}, err
	}
	err = u.Tx.Read(ctx, func(s Store) error {
		_, err := actorIn(ctx, s, sess, false)
		return err
	})
	return ParsedReference{Reference: ref, Display: domain.CollapseSpaces(input)}, err
}

// translationFor finds the translation by code; "" means the church's default.
func translationFor(ctx context.Context, s Store, sc churchScope, code string) (domain.Translation, error) {
	if code == "" {
		ch, err := sc.cs.Church().Get(ctx)
		if err != nil {
			return domain.Translation{}, err
		}
		return s.Translations().ByID(ctx, ch.DefaultTranslationID)
	}
	tr, err := s.Translations().ByCode(ctx, code)
	if errors.Is(err, ErrNotFound) {
		return domain.Translation{}, &domain.InvalidInputError{Field: "translation", Message: "Unknown translation."}
	}
	return tr, err
}

// Lookup finds the text for a reference: a stored reading first, then each
// registered provider in turn (07 §3.1). The database is not held while a
// provider is asked.
func (u *Readings) Lookup(ctx context.Context, sess *domain.Session, input, translation string) (LookupResult, error) {
	ref, err := domain.ParseReference(input)
	if err != nil {
		return LookupResult{}, err
	}
	res := LookupResult{Reference: ref, Display: domain.CollapseSpaces(input)}
	err = u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		res.Actions = readingActions(sc.actor)
		if res.Translation, err = translationFor(ctx, s, sc, translation); err != nil {
			return err
		}
		stored, err := sc.cs.Readings().ByReference(ctx, ref.String(), res.Translation.ID)
		switch {
		case err == nil:
			res.Reading = &stored
		case !errors.Is(err, ErrNotFound):
			return err
		}
		res.SuggestedAttribution, err = sc.cs.Readings().LatestAttribution(ctx, res.Translation.ID)
		return err
	})
	if err != nil || res.Reading != nil {
		return res, err
	}
	for _, p := range u.Providers {
		text, ok, failed := u.ask(ctx, p, ref, res.Translation.Code)
		if failed {
			res.ProviderError = true
		}
		if ok {
			res.Provider, res.ProviderError = &text, false
			break
		}
	}
	return res, nil
}

// ask runs one provider lookup with its deadline. A result that breaks the
// limits, an error and a timeout are logged without text and count as "not
// available" (failed is true then); ErrNotAvailable is not a failure.
func (u *Readings) ask(ctx context.Context, p BibleTextProvider, ref domain.Reference, translation string) (text BibleText, ok, failed bool) {
	timeout := u.ProviderTimeout
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type answer struct {
		text BibleText
		err  error
	}
	got := make(chan answer, 1) // buffered: a provider that ignores the deadline does not block here
	go func() {
		t, err := p.Lookup(ctx, ref, translation)
		got <- answer{t, err}
	}()
	var err error
	select {
	case a := <-got:
		text, err = a.text, a.err
	case <-ctx.Done():
		err = ctx.Err()
	}
	switch {
	case errors.Is(err, ErrNotAvailable):
		return BibleText{}, false, false
	case err != nil:
		class := "error"
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			class = "timeout"
		}
		u.warn("bible_provider_failed", p.ID(), class)
		return BibleText{}, false, true
	case text.Source != p.ID():
		u.warn("bible_provider_bad_result", p.ID(), "wrong_source")
		return BibleText{}, false, true
	case utf8.RuneCountInString(text.Text) > domain.MaxReadingText || utf8.RuneCountInString(text.Attribution) > domain.MaxReadingAttribution:
		u.warn("bible_provider_bad_result", p.ID(), "too_long")
		return BibleText{}, false, true
	case strings.TrimSpace(text.Text) == "":
		return BibleText{}, false, false
	}
	return text, true, false
}

func (u *Readings) warn(msg, provider, class string) {
	if u.Log != nil {
		u.Log.Warn(msg, "provider", provider, "class", class)
	}
}

// List returns a page of readings in book, chapter, verse order (any member).
func (u *Readings) List(ctx context.Context, sess *domain.Session, q ReadingQuery) (ReadingListView, error) {
	if utf8.RuneCountInString(q.Q) > domain.MaxReadingQuery {
		return ReadingListView{}, &domain.InvalidInputError{Field: "q", Message: "At most 200 characters."}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultReadingLimit
	}
	limit = min(limit, maxReadingLimit)
	search := ReadingSearch{Fold: domain.Fold(q.Q), FoldZh: domain.FoldZh(q.Q), Translation: q.Translation}
	var res ReadingListView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		rows, err := sc.cs.Readings().List(ctx, search)
		if err != nil {
			return err
		}
		type keyed struct {
			row  ReadingRow
			ref  domain.Reference
			book int
		}
		items := make([]keyed, len(rows))
		for i, r := range rows {
			ref, _ := domain.ParseReference(r.Reference) // stored in standard form
			items[i] = keyed{r, ref, ref.BookOrdinal()}
		}
		slices.SortStableFunc(items, func(a, b keyed) int {
			return cmp.Or(cmp.Compare(a.book, b.book), cmp.Compare(a.ref.Chapter, b.ref.Chapter),
				cmp.Compare(a.ref.Verse, b.ref.Verse), strings.Compare(a.row.Translation.Code, b.row.Translation.Code),
				strings.Compare(a.row.Reference, b.row.Reference), strings.Compare(string(a.row.ID), string(b.row.ID)))
		})
		res.Total = len(items)
		from := min(max(q.Offset, 0), len(items))
		to := min(from+limit, len(items))
		actions := readingActions(sc.actor)
		for _, it := range items[from:to] {
			res.Items = append(res.Items, ReadingSummary{ID: it.row.ID, Reference: it.row.Reference,
				Canonical: it.ref.Canonical("id"), ReferenceDisplay: it.row.ReferenceDisplay,
				Translation: it.row.Translation, Snippet: domain.Snippet(it.row.TextStart, 120), Actions: actions})
		}
		return nil
	})
	return res, err
}

// Get returns one reading (any member).
func (u *Readings) Get(ctx context.Context, sess *domain.Session, id domain.ReadingID) (ReadingView, error) {
	var res ReadingView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		r, err := sc.cs.Readings().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		res = ReadingView{Reading: r, Actions: readingActions(sc.actor)}
		return nil
	})
	return res, err
}

// Create saves text a member typed or pasted; the source is "manual" (library.edit).
func (u *Readings) Create(ctx context.Context, sess *domain.Session, in ReadingInput) (ReadingView, error) {
	ref, err := domain.ParseReference(in.Reference)
	if err != nil {
		return ReadingView{}, err
	}
	return u.save(ctx, sess, ref, domain.CollapseSpaces(in.Reference), in.Translation, in.Text, in.Attribution, domain.SourceManual, nil)
}

// FromProvider saves the text a registered provider returns, when it allows
// storing it (library.edit). The text comes from the provider, never from the
// request (07 §3.1).
func (u *Readings) FromProvider(ctx context.Context, sess *domain.Session, input, translation, provider string) (ReadingView, error) {
	ref, err := domain.ParseReference(input)
	if err != nil {
		return ReadingView{}, err
	}
	var p BibleTextProvider
	for _, c := range u.Providers {
		if c.ID() == provider {
			p = c
		}
	}
	if p == nil {
		return ReadingView{}, &domain.InvalidInputError{Field: "provider", Message: "Unknown provider."}
	}
	// Check the permission, the translation and any existing reading before
	// asking the provider.
	var tr domain.Translation
	err = u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		if tr, err = translationFor(ctx, s, sc, translation); err != nil {
			return err
		}
		return existing(ctx, sc, ref, tr.ID)
	})
	if err != nil {
		return ReadingView{}, err
	}
	text, ok, _ := u.ask(ctx, p, ref, tr.Code)
	if !ok || !text.MayStore {
		return ReadingView{}, &domain.InvalidInputError{Field: "provider", Message: "This provider has no text it allows saving."}
	}
	return u.save(ctx, sess, ref, domain.CollapseSpaces(input), tr.Code, text.Text, text.Attribution, provider, &tr)
}

// existing returns a ReadingExistsError when the reading is already stored.
func existing(ctx context.Context, sc churchScope, ref domain.Reference, tr domain.TranslationID) error {
	r, err := sc.cs.Readings().ByReference(ctx, ref.String(), tr)
	switch {
	case err == nil:
		return &ReadingExistsError{ID: r.ID}
	case errors.Is(err, ErrNotFound):
		return nil
	}
	return err
}

func (u *Readings) save(ctx context.Context, sess *domain.Session, ref domain.Reference, display, translation, text, attribution, source string, known *domain.Translation) (ReadingView, error) {
	var res ReadingView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		var tr domain.Translation
		if known != nil {
			tr = *known
		} else if tr, err = translationFor(ctx, s, sc, translation); err != nil {
			return err
		}
		now := u.Clock.Now()
		r := domain.Reading{ID: domain.ReadingID(u.IDs.NewID()), Reference: ref.String(), ReferenceDisplay: display,
			Translation: tr, Text: text, Attribution: attribution, SourceProvider: source, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := domain.ValidateReading(&r); err != nil {
			return err
		}
		if err := existing(ctx, sc, ref, tr.ID); err != nil {
			return err
		}
		if err := sc.cs.Readings().Create(ctx, r); err != nil {
			return mapReadingUnique(ctx, sc, ref, tr.ID, err)
		}
		res = ReadingView{Reading: r, Actions: readingActions(sc.actor)}
		return nil
	})
	return res, err
}

// mapReadingUnique turns a clash on the unique key into reading_exists.
func mapReadingUnique(ctx context.Context, sc churchScope, ref domain.Reference, tr domain.TranslationID, err error) error {
	var uq *UniqueError
	if errors.As(err, &uq) && uq.Constraint == "readings_church_ref_key" {
		if e := existing(ctx, sc, ref, tr); e != nil {
			return e
		}
	}
	return err
}

// Update changes the text, the attribution or the display of a reading
// (library.edit). The reference and the translation cannot change (07 §3).
func (u *Readings) Update(ctx context.Context, sess *domain.Session, id domain.ReadingID, ch ReadingChange) (ReadingView, error) {
	var res ReadingView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		cur, err := sc.cs.Readings().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		if cur.Version != ch.Version {
			return ErrVersionConflict
		}
		next := cur
		if ch.Text != nil {
			next.Text = *ch.Text
		}
		if ch.Attribution != nil {
			next.Attribution = *ch.Attribution
		}
		if ch.ReferenceDisplay != nil {
			next.ReferenceDisplay = *ch.ReferenceDisplay
		}
		if err := domain.ValidateReading(&next); err != nil {
			return err
		}
		if ref, err := domain.ParseReference(next.ReferenceDisplay); err != nil || ref.String() != cur.Reference {
			return &domain.InvalidInputError{Field: "reference_display", Message: "This does not name the reading's reference."}
		}
		next.Version, next.UpdatedAt = cur.Version+1, u.Clock.Now()
		ok, err := sc.cs.Readings().Update(ctx, next, cur.Version)
		if err != nil {
			return err
		}
		if !ok {
			return ErrVersionConflict
		}
		res = ReadingView{Reading: next, Actions: readingActions(sc.actor)}
		return nil
	})
	return res, err
}

// Delete removes a reading for good (library.edit).
func (u *Readings) Delete(ctx context.Context, sess *domain.Session, id domain.ReadingID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		if _, err := sc.cs.Readings().ByID(ctx, id); err != nil {
			return missing(err)
		}
		inUse, err := u.Usage.ReadingInUse(ctx, sc.actor.ChurchID, id)
		if err != nil {
			return err
		}
		if inUse {
			return ErrReadingInUse
		}
		return missing(sc.cs.Readings().Delete(ctx, id))
	})
}
