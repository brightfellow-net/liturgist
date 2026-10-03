// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain"
)

// Songs holds the song library use cases (06). Every member can view; the
// library.edit scope changes, deletes and links songs.
type Songs struct {
	Tx    Tx
	Clock Clock
	IDs   IDGenerator
}

// SongActions are the advisory actions on a song (04 §5).
type SongActions struct {
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

// SongSummary is a song in a list; it has no lyrics.
type SongSummary struct {
	SongRow
	HasGroup bool
	Actions  SongActions
}

// SongListView is one page of songs.
type SongListView struct {
	Items []SongSummary
	Total int
}

// SongView is a whole song with the other-language versions linked to it.
type SongView struct {
	Song     domain.Song
	Versions []SongRef
	Actions  SongActions
}

// SectionInput is a section in a create or update request (06 §2.4): an
// existing section carries its ID; a new one may carry a request-local Key
// that the arrangement can refer to.
type SectionInput struct {
	ID     domain.SectionID
	Key    string
	Kind   domain.SectionKind
	Number int
	Label  string
	Text   string
}

// SongInput creates a song. Arrangement entries are the Keys of its sections.
type SongInput struct {
	Language, Title                                string
	AltTitles                                      []string
	HymnalSource, HymnalNumber                     string
	Lyricist, Composer, Translator, DefaultKey     string
	CopyrightHolder, CopyrightLine, CCLISongNumber string
	LicenceStatus                                  domain.LicenceStatus
	LicenceNotes                                   string
	Sections                                       []SectionInput
	Arrangement                                    []string
}

// SongChange is a PATCH: nil fields are unchanged. Version is the version the
// client loaded.
type SongChange struct {
	Version                                    int
	Language, Title                            *string
	AltTitles                                  *[]string
	HymnalSource, HymnalNumber                 *string
	Lyricist, Composer, Translator, DefaultKey *string
	CopyrightHolder, CopyrightLine             *string
	CCLISongNumber                             *string
	LicenceStatus                              *domain.LicenceStatus
	LicenceNotes                               *string
	Sections                                   *[]SectionInput
	Arrangement                                *[]string // entries: section IDs or Keys of new sections
}

// SongQuery is a list or search request (06 §3).
type SongQuery struct {
	Q                          string
	Language, LicenceStatus    string
	HymnalSource, HymnalNumber string
	Limit, Offset              int
}

const (
	defaultSongLimit = 50
	maxSongLimit     = 100
)

// List returns a page of songs, searched and filtered (any member).
func (u *Songs) List(ctx context.Context, sess *domain.Session, q SongQuery) (SongListView, error) {
	search, err := prepareSearch(q)
	if err != nil {
		return SongListView{}, err
	}
	var res SongListView
	err = u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		page, err := sc.cs.Songs().Search(ctx, search)
		if err != nil {
			return err
		}
		actions := songActions(sc.actor)
		res = SongListView{Total: page.Total}
		for _, r := range page.Items {
			res.Items = append(res.Items, SongSummary{SongRow: r, HasGroup: r.GroupID != "", Actions: actions})
		}
		return nil
	})
	return res, err
}

// prepareSearch folds and checks a list request (06 §5.2).
func prepareSearch(q SongQuery) (SongSearch, error) {
	if utf8.RuneCountInString(q.Q) > domain.MaxSongQuery {
		return SongSearch{}, &domain.InvalidInputError{Field: "q", Message: "At most 200 characters."}
	}
	terms := domain.SearchTerms(q.Q)
	if len(terms) > domain.MaxSongTerms {
		return SongSearch{}, &domain.InvalidInputError{Field: "q", Message: "At most 10 words."}
	}
	out := SongSearch{Terms: terms, Language: q.Language, LicenceStatus: domain.LicenceStatus(q.LicenceStatus),
		Limit: q.Limit, Offset: q.Offset}
	if key, ok := domain.ParseHymnalQuery(q.Q); ok {
		out.HymnalQueryKey = key
	}
	if q.Language != "" && !slices.Contains(domain.ContentLanguages, q.Language) {
		return SongSearch{}, &domain.InvalidInputError{Field: "language", Message: "Unknown language."}
	}
	if q.LicenceStatus != "" && !slices.Contains(domain.LicenceStatuses, domain.LicenceStatus(q.LicenceStatus)) {
		return SongSearch{}, &domain.InvalidInputError{Field: "licence_status", Message: "Unknown licence status."}
	}
	if q.HymnalNumber != "" && q.HymnalSource == "" {
		return SongSearch{}, &domain.InvalidInputError{Field: "hymnal_source", Message: "Give the hymnal with its number."}
	}
	if q.HymnalSource != "" {
		out.HymnalSourceKey = domain.HymnalSourceKey(q.HymnalSource)
		out.HymnalKey = domain.HymnalKey(q.HymnalSource, q.HymnalNumber)
		if out.HymnalSourceKey == "" {
			return SongSearch{}, &domain.InvalidInputError{Field: "hymnal_source", Message: "Unknown hymnal."}
		}
	}
	if out.Limit < 1 {
		out.Limit = defaultSongLimit
	}
	out.Limit = min(out.Limit, maxSongLimit)
	out.Offset = max(out.Offset, 0)
	return out, nil
}

// Get returns a song with its sections (any member).
func (u *Songs) Get(ctx context.Context, sess *domain.Session, id domain.SongID) (SongView, error) {
	var res SongView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		res, err = songView(ctx, sc, id)
		return err
	})
	return res, err
}

// Create adds a song with its sections (library.edit).
func (u *Songs) Create(ctx context.Context, sess *domain.Session, in SongInput) (SongView, error) {
	song := domain.Song{Language: in.Language, Title: in.Title, AltTitles: slices.Clone(in.AltTitles),
		HymnalSource: in.HymnalSource, HymnalNumber: in.HymnalNumber, Lyricist: in.Lyricist, Composer: in.Composer,
		Translator: in.Translator, DefaultKey: in.DefaultKey, CopyrightHolder: in.CopyrightHolder,
		CopyrightLine: in.CopyrightLine, CCLISongNumber: in.CCLISongNumber, LicenceStatus: in.LicenceStatus,
		LicenceNotes: in.LicenceNotes}
	if song.AltTitles == nil {
		song.AltTitles = []string{}
	}
	if err := domain.ValidateSong(&song); err != nil {
		return SongView{}, err
	}
	var res SongView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		res, err = u.createIn(ctx, sc, song, in)
		return err
	})
	return res, err
}

// createIn saves a validated song inside the caller's transaction; the
// caller has checked library.edit.
func (u *Songs) createIn(ctx context.Context, sc churchScope, song domain.Song, in SongInput) (SongView, error) {
	song.ID = domain.SongID(u.IDs.NewID())
	secs, keys, _, err := resolveSections(in.Sections, nil, u.IDs)
	if err != nil {
		return SongView{}, err
	}
	arr, err := resolveArrangement(in.Arrangement, keys, secs)
	if err != nil {
		return SongView{}, err
	}
	song.Sections, song.DefaultArrangement = secs, arr
	if err := domain.ValidateSections(song.Sections); err != nil {
		return SongView{}, err
	}
	if err := domain.ValidateArrangement(song.DefaultArrangement, song.Sections); err != nil {
		return SongView{}, err
	}
	now := u.Clock.Now()
	song.Version, song.CreatedAt, song.UpdatedAt = 1, now, now
	if err := sc.cs.Songs().Create(ctx, song); err != nil {
		return SongView{}, err
	}
	return songView(ctx, sc, song.ID)
}

// Update changes a song, its sections and its arrangement (library.edit). The
// request must carry the version the client loaded (06 §2.4).
func (u *Songs) Update(ctx context.Context, sess *domain.Session, id domain.SongID, ch SongChange) (SongView, error) {
	var res SongView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, ch.Sections != nil)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		res, err = u.updateIn(ctx, sc, id, ch)
		return err
	})
	return res, err
}

// updateIn changes a song inside the caller's transaction; the caller has
// checked library.edit.
func (u *Songs) updateIn(ctx context.Context, sc churchScope, id domain.SongID, ch SongChange) (SongView, error) {
	cur, err := sc.cs.Songs().ByID(ctx, id)
	if err != nil {
		return SongView{}, missing(err)
	}
	if cur.Version != ch.Version {
		return SongView{}, ErrVersionConflict
	}
	next := cur
	next.AltTitles = slices.Clone(cur.AltTitles)
	applyChange(&next, ch)
	if next.AltTitles == nil {
		next.AltTitles = []string{}
	}
	if err := domain.ValidateSong(&next); err != nil {
		return SongView{}, err
	}
	if ch.Sections != nil {
		secs, keys, removed, err := resolveSections(*ch.Sections, cur.Sections, u.IDs)
		if err != nil {
			return SongView{}, err
		}
		if len(removed) > 0 {
			busy, err := sc.cs.Usage().SectionsInUse(ctx, id, removed)
			if err != nil {
				return SongView{}, err
			}
			if len(busy) > 0 {
				return SongView{}, &SectionInUseError{IDs: busy}
			}
		}
		next.Sections = secs
		if ch.Arrangement != nil {
			if next.DefaultArrangement, err = resolveArrangement(*ch.Arrangement, keys, secs); err != nil {
				return SongView{}, err
			}
		} else {
			next.DefaultArrangement = keepArrangement(cur.DefaultArrangement, secs)
		}
	} else if ch.Arrangement != nil {
		var err error
		if next.DefaultArrangement, err = resolveArrangement(*ch.Arrangement, nil, next.Sections); err != nil {
			return SongView{}, err
		}
	}
	if err := domain.ValidateSections(next.Sections); err != nil {
		return SongView{}, err
	}
	if err := domain.ValidateArrangement(next.DefaultArrangement, next.Sections); err != nil {
		return SongView{}, err
	}
	next.Version, next.UpdatedAt = cur.Version+1, u.Clock.Now()
	ok, err := sc.cs.Songs().Update(ctx, next, cur.Version)
	if err != nil {
		return SongView{}, mapGroupLanguage(err)
	}
	if !ok {
		return SongView{}, ErrVersionConflict
	}
	return songView(ctx, sc, id)
}

// Delete removes a song for good (library.edit). A group left with one song is dissolved.
func (u *Songs) Delete(ctx context.Context, sess *domain.Session, id domain.SongID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true) // group dissolution under the church lock
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		cur, err := sc.cs.Songs().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		inUse, err := sc.cs.Usage().SongInUse(ctx, id)
		if err != nil {
			return err
		}
		if inUse {
			return ErrSongInUse
		}
		if err := sc.cs.Songs().Delete(ctx, id); err != nil {
			return err
		}
		if cur.GroupID == "" {
			return nil
		}
		return u.dissolveIfAlone(ctx, sc.cs.Songs(), cur.GroupID)
	})
}

// Link joins two songs into one group of language versions (library.edit, 06 §2.3).
func (u *Songs) Link(ctx context.Context, sess *domain.Session, id, other domain.SongID) (SongView, error) {
	var res SongView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		if id == other {
			return &domain.InvalidInputError{Field: "other_song_id", Message: "Choose a different song."}
		}
		repo := sc.cs.Songs()
		a, err := repo.ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		b, err := repo.ByID(ctx, other)
		if err != nil {
			return missing(err)
		}
		now := u.Clock.Now()
		switch {
		case a.GroupID != "" && a.GroupID == b.GroupID:
			// already linked: no change
		case a.GroupID != "" && b.GroupID != "":
			return &GroupConflictError{Reason: ReasonAlreadyGrouped}
		case a.GroupID == "" && b.GroupID == "":
			if a.Language == b.Language {
				return &GroupConflictError{Reason: ReasonLanguageTaken}
			}
			g := domain.SongGroupID(u.IDs.NewID())
			if err := repo.CreateGroup(ctx, g, now); err != nil {
				return err
			}
			if err := repo.SetGroup(ctx, []domain.SongID{a.ID, b.ID}, g, now); err != nil {
				return mapGroupLanguage(err)
			}
		default: // exactly one is grouped: the other joins its group
			joiner, group := b, a.GroupID
			if a.GroupID == "" {
				joiner, group = a, b.GroupID
			}
			if err := repo.SetGroup(ctx, []domain.SongID{joiner.ID}, group, now); err != nil {
				return mapGroupLanguage(err)
			}
		}
		res, err = songView(ctx, sc, id)
		return err
	})
	return res, err
}

// Unlink removes a song from its group (library.edit); a group left with one song is dissolved.
func (u *Songs) Unlink(ctx context.Context, sess *domain.Session, id domain.SongID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLibraryEdit); err != nil {
			return err
		}
		repo := sc.cs.Songs()
		cur, err := repo.ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		if cur.GroupID == "" {
			return nil
		}
		if err := repo.SetGroup(ctx, []domain.SongID{id}, "", u.Clock.Now()); err != nil {
			return err
		}
		return u.dissolveIfAlone(ctx, repo, cur.GroupID)
	})
}

// dissolveIfAlone deletes a group that has fewer than two songs left.
func (u *Songs) dissolveIfAlone(ctx context.Context, repo SongRepo, group domain.SongGroupID) error {
	members, err := repo.GroupMembers(ctx, group)
	if err != nil {
		return err
	}
	if len(members) >= 2 {
		return nil
	}
	if len(members) == 1 {
		if err := repo.SetGroup(ctx, []domain.SongID{members[0].ID}, "", u.Clock.Now()); err != nil {
			return err
		}
	}
	return repo.DeleteGroup(ctx, group)
}

func songActions(a Actor) SongActions {
	edit := a.Scopes.Has(domain.ScopeLibraryEdit)
	return SongActions{Edit: edit, Delete: edit}
}

// songView reads a song and its linked versions inside the transaction.
func songView(ctx context.Context, sc churchScope, id domain.SongID) (SongView, error) {
	song, err := sc.cs.Songs().ByID(ctx, id)
	if err != nil {
		return SongView{}, missing(err)
	}
	v := SongView{Song: song, Actions: songActions(sc.actor)}
	if song.GroupID != "" {
		members, err := sc.cs.Songs().GroupMembers(ctx, song.GroupID)
		if err != nil {
			return SongView{}, err
		}
		for _, m := range members {
			if m.ID != id {
				v.Versions = append(v.Versions, m)
			}
		}
	}
	return v, nil
}

// missing turns the repository's ErrNotFound into a 404 whose reason is logged.
func missing(err error) error {
	if errors.Is(err, ErrNotFound) {
		return notFound(ReasonMissing)
	}
	return err
}

func mapGroupLanguage(err error) error {
	var u *UniqueError
	if errors.As(err, &u) && u.Constraint == "songs_group_language_key" {
		return &GroupConflictError{Reason: ReasonLanguageTaken}
	}
	return err
}

// applyChange copies the fields a PATCH sets onto song.
func applyChange(song *domain.Song, ch SongChange) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&song.Language, ch.Language)
	set(&song.Title, ch.Title)
	set(&song.HymnalSource, ch.HymnalSource)
	set(&song.HymnalNumber, ch.HymnalNumber)
	set(&song.Lyricist, ch.Lyricist)
	set(&song.Composer, ch.Composer)
	set(&song.Translator, ch.Translator)
	set(&song.DefaultKey, ch.DefaultKey)
	set(&song.CopyrightHolder, ch.CopyrightHolder)
	set(&song.CopyrightLine, ch.CopyrightLine)
	set(&song.CCLISongNumber, ch.CCLISongNumber)
	set(&song.LicenceNotes, ch.LicenceNotes)
	if ch.AltTitles != nil {
		song.AltTitles = slices.Clone(*ch.AltTitles)
	}
	if ch.LicenceStatus != nil {
		song.LicenceStatus = *ch.LicenceStatus
	}
}

// resolveSections builds the final ordered section list from a request
// (06 §2.4). An entry with an ID must be one of existing and keeps it; an
// entry without one is new and gets an ID. keys maps request-local keys to
// the IDs of new sections; removed lists existing sections the request drops.
func resolveSections(in []SectionInput, existing []domain.Section, ids IDGenerator) (secs []domain.Section, keys map[string]domain.SectionID, removed []domain.SectionID, err error) {
	have := make(map[domain.SectionID]bool, len(existing))
	for _, e := range existing {
		have[e.ID] = true
	}
	kept := map[domain.SectionID]bool{}
	keys = map[string]domain.SectionID{}
	secs = make([]domain.Section, 0, len(in))
	for i, si := range in {
		field := "sections." + strconv.Itoa(i)
		sec := domain.Section{ID: si.ID, Kind: si.Kind, Number: si.Number, Label: si.Label, Text: si.Text}
		switch {
		case si.ID != "":
			if !have[si.ID] {
				return nil, nil, nil, &domain.InvalidInputError{Field: field + ".id", Message: "This is not a section of the song."}
			}
			if kept[si.ID] {
				return nil, nil, nil, &domain.InvalidInputError{Field: field + ".id", Message: "This section appears twice."}
			}
			kept[si.ID] = true
		default:
			sec.ID = domain.SectionID(ids.NewID())
		}
		if si.Key != "" {
			if si.ID != "" || !domain.ValidSectionRequestKey(si.Key) {
				return nil, nil, nil, &domain.InvalidInputError{Field: field + ".key", Message: "Only a new section has a key of 1 to 40 characters."}
			}
			if _, dup := keys[si.Key]; dup {
				return nil, nil, nil, &domain.InvalidInputError{Field: field + ".key", Message: "This key is used twice."}
			}
			keys[si.Key] = sec.ID
		}
		secs = append(secs, sec)
	}
	for _, e := range existing {
		if !kept[e.ID] {
			removed = append(removed, e.ID)
		}
	}
	return secs, keys, removed, nil
}

// resolveArrangement turns request entries (section IDs or keys of new
// sections) into section IDs of the final list (06 §2.4).
func resolveArrangement(entries []string, keys map[string]domain.SectionID, secs []domain.Section) ([]domain.SectionID, error) {
	out := make([]domain.SectionID, 0, len(entries))
	for i, e := range entries {
		if id, ok := keys[e]; ok {
			out = append(out, id)
			continue
		}
		id := domain.SectionID(e)
		if !slices.ContainsFunc(secs, func(s domain.Section) bool { return s.ID == id }) {
			return nil, &domain.InvalidInputError{Field: "default_arrangement." + strconv.Itoa(i), Message: "This is not a section of the song."}
		}
		out = append(out, id)
	}
	return out, nil
}

// keepArrangement drops entries of sections that no longer exist.
func keepArrangement(arr []domain.SectionID, secs []domain.Section) []domain.SectionID {
	out := make([]domain.SectionID, 0, len(arr))
	for _, id := range arr {
		if slices.ContainsFunc(secs, func(s domain.Section) bool { return s.ID == id }) {
			out = append(out, id)
		}
	}
	return out
}
