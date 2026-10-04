// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// Liturgies holds the liturgy use cases (10). Members holding a liturgy.*
// scope see unpublished liturgies; liturgy.edit writes; liturgy.manage deletes.
type Liturgies struct {
	Tx           Tx
	Clock        Clock
	IDs          IDGenerator
	Entitlements Entitlements
}

// LiturgyActions are the advisory actions on a liturgy (10 §4): both depend only
// on the scope and the liturgy's own state.
type LiturgyActions struct {
	Edit           bool `json:"edit"`
	Delete         bool `json:"delete"`
	Submit         bool `json:"submit"`
	Approve        bool `json:"approve"`
	RequestChanges bool `json:"request_changes"`
	Reopen         bool `json:"reopen"`
	Comment        bool `json:"comment"`
}

func liturgyActions(a Actor, state domain.LiturgyState) LiturgyActions {
	can := func(act domain.ReviewAction) bool {
		r, _ := act.Rule()
		return a.Scopes.Has(r.Scope) && r.Allowed(state)
	}
	return LiturgyActions{
		Edit:           a.Scopes.Has(domain.ScopeLiturgyEdit) && state.Editable(),
		Delete:         a.Scopes.Has(domain.ScopeLiturgyManage) && state.Deletable(),
		Submit:         can(domain.ActionSubmit),
		Approve:        can(domain.ActionApprove),
		RequestChanges: can(domain.ActionRequestChanges),
		Reopen:         can(domain.ActionReopen),
		Comment:        a.Scopes.Has(domain.ScopeLiturgyComment) && state.CanComment(),
	}
}

// canSeeLiturgies reports whether the actor holds a liturgy scope, which shows
// unpublished liturgies (10 §2.1, P-64).
func canSeeLiturgies(a Actor) bool {
	return a.RequireAny(domain.ScopeLiturgyEdit, domain.ScopeLiturgyComment, domain.ScopeLiturgyApprove, domain.ScopeLiturgyManage) == nil
}

// openLiturgy loads a liturgy the actor may see: 404 when it is missing or
// unpublished and the actor holds no liturgy scope.
func (sc churchScope) openLiturgy(ctx context.Context, id domain.LiturgyID) (domain.Liturgy, error) {
	l, err := sc.cs.Liturgies().ByID(ctx, id)
	if err != nil {
		return domain.Liturgy{}, missing(err)
	}
	if l.State != domain.StatePublished && !canSeeLiturgies(sc.actor) {
		return domain.Liturgy{}, notFound(ReasonNotVisible)
	}
	return l, nil
}

// openEdit is openLiturgy for a write: 404, then 403, then 409 liturgy_locked
// (10 §4), so nothing is revealed to someone who may not see the liturgy.
func (sc churchScope) openEdit(ctx context.Context, id domain.LiturgyID) (domain.Liturgy, error) {
	l, err := sc.openLiturgy(ctx, id)
	if err != nil {
		return domain.Liturgy{}, err
	}
	if err := sc.actor.Require(domain.ScopeLiturgyEdit); err != nil {
		return domain.Liturgy{}, err
	}
	if !l.State.Editable() {
		return domain.Liturgy{}, ErrLiturgyLocked
	}
	return l, nil
}

// staleLiturgy is the error of a conditional update of the liturgy that
// changed no row: 404 when the liturgy is gone, else a conflict on its structure.
func (sc churchScope) staleLiturgy(ctx context.Context, id domain.LiturgyID) error {
	if _, err := sc.cs.Liturgies().ByID(ctx, id); err != nil {
		return missing(err)
	}
	return &VersionConflictError{Scope: ScopeLiturgy}
}

// staleItem is staleLiturgy for an item's content.
func (sc churchScope) staleItem(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) error {
	if _, err := sc.cs.LiturgyItems().ByID(ctx, liturgy, id); err != nil {
		return missing(err)
	}
	return &VersionConflictError{Scope: ScopeItem, ItemID: id}
}

// --- limits (10 §8) ---

type liturgyLimits struct{ active, unpublished Limit }

var unpublishedStates = []domain.LiturgyState{domain.StateDraft, domain.StateInReview, domain.StateNeedsRevision, domain.StateApproved}

func (u *Liturgies) limits(ctx context.Context) (liturgyLimits, error) {
	t, err := TenantFrom(ctx)
	if err != nil {
		return liturgyLimits{}, err
	}
	var lim liturgyLimits
	for _, x := range []struct {
		name LimitName
		dst  *Limit
	}{{LimitMaxActiveLiturgies, &lim.active}, {LimitMaxUnpublishedLiturgies, &lim.unpublished}} {
		l, err := u.Entitlements.Limit(ctx, t.ChurchID, x.name)
		if err != nil {
			return liturgyLimits{}, errors.Join(ErrUnavailable, err)
		}
		*x.dst = l
	}
	return lim, nil
}

// check refuses n more liturgies when they would pass a limit. It counts in
// the caller's transaction, under LockChurch.
func (lim liturgyLimits) check(ctx context.Context, cs ChurchStore, n int) error {
	for _, x := range []struct {
		name   LimitName
		limit  Limit
		states []domain.LiturgyState
	}{{LimitMaxActiveLiturgies, lim.active, nil}, {LimitMaxUnpublishedLiturgies, lim.unpublished, unpublishedStates}} {
		if x.limit.Unlimited {
			continue
		}
		used, err := cs.Liturgies().CountActive(ctx, x.states)
		if err != nil {
			return err
		}
		if used+n > x.limit.Max {
			return &LimitReachedError{Limit: x.name, Used: used, Max: x.limit.Max}
		}
	}
	return nil
}

// --- views ---

// SectionRef is a section without its text.
type SectionRef struct {
	ID     domain.SectionID
	Kind   domain.SectionKind
	Number int
	Label  string
}

// LiturgySongSummary is what the editor shows of an item song's song; it has no lyrics.
type LiturgySongSummary struct {
	Title, Language, HymnalSource, HymnalNumber, DefaultKey string
	Sections                                                []SectionRef
}

// ItemSongView is an item song with a summary of its song (nil when the song was deleted).
type ItemSongView struct {
	Song    domain.LiturgySong
	Summary *LiturgySongSummary
}

// LiturgyReading is the reading of a reading item, with its text.
type LiturgyReading struct {
	Reading domain.Reading
}

// ItemView is an item with its songs and reading summaries.
type ItemView struct {
	Item    domain.Item
	Songs   []ItemSongView
	Reading *LiturgyReading
}

// AssignmentView is an assignment with the member's name and whether they are still a member.
type AssignmentView struct {
	Assignment   domain.Assignment
	UserName     string
	FormerMember bool
}

// LiturgyView is a liturgy with everything the editor needs (10 §4).
type LiturgyView struct {
	Liturgy     domain.Liturgy
	Items       []ItemView
	Assignments []AssignmentView
	Problems    []domain.Problem
	Actions     LiturgyActions
	// Review is nil for a member without a liturgy scope (12 §2, P-74).
	Review *ReviewInfo
}

// ReviewInfo is what the liturgy view carries of the review workflow: the
// newest state change and the number of unresolved comments (12 §2).
type ReviewInfo struct {
	LastChange   *StateChangeView
	OpenComments int
}

// StateChangeView is a state change with the name of its author.
type StateChangeView struct {
	Change   domain.StateChange
	UserName string
}

// LiturgyListItem is one line of the list.
type LiturgyListItem struct {
	Liturgy   domain.Liturgy
	ItemCount int
	Actions   LiturgyActions
}

// LiturgyPage is one page of the list.
type LiturgyPage struct {
	Items []LiturgyListItem
	Total int
}

// itemViews attaches the summaries; songs and readings are read once each.
func itemViews(ctx context.Context, cs ChurchStore, items []domain.Item) ([]ItemView, error) {
	songs := map[domain.SongID]*LiturgySongSummary{}
	out := make([]ItemView, len(items))
	for i, it := range items {
		v := ItemView{Item: it, Songs: make([]ItemSongView, len(it.Songs))}
		for j, s := range it.Songs {
			v.Songs[j].Song = s
			if s.SongID == "" {
				continue
			}
			sum, ok := songs[s.SongID]
			if !ok {
				song, err := cs.Songs().ByID(ctx, s.SongID)
				if err != nil && !errors.Is(err, ErrNotFound) {
					return nil, err
				}
				if err == nil {
					sum = &LiturgySongSummary{Title: song.Title, Language: song.Language, HymnalSource: song.HymnalSource,
						HymnalNumber: song.HymnalNumber, DefaultKey: song.DefaultKey}
					for _, sec := range song.Sections {
						sum.Sections = append(sum.Sections, SectionRef{ID: sec.ID, Kind: sec.Kind, Number: sec.Number, Label: sec.Label})
					}
				}
				songs[s.SongID] = sum
			}
			v.Songs[j].Summary = sum
		}
		if it.ReadingID != "" {
			r, err := cs.Readings().ByID(ctx, it.ReadingID)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
			if err == nil {
				v.Reading = &LiturgyReading{Reading: r}
			}
		}
		out[i] = v
	}
	return out, nil
}

func assignmentViews(ctx context.Context, sc churchScope, list []domain.Assignment) ([]AssignmentView, error) {
	out := make([]AssignmentView, len(list))
	names := map[domain.UserID]AssignmentView{}
	for i, a := range list {
		v := AssignmentView{Assignment: a, UserName: a.Name}
		if a.UserID != "" {
			c, ok := names[a.UserID]
			if !ok {
				if u, err := sc.users.ByID(ctx, a.UserID); err == nil {
					c.UserName = u.Name
				} else if !errors.Is(err, ErrNotFound) {
					return nil, err
				}
				if _, err := sc.cs.Memberships().ByUser(ctx, a.UserID); errors.Is(err, ErrNotFound) {
					c.FormerMember = true
				} else if err != nil {
					return nil, err
				}
				names[a.UserID] = c
			}
			v.UserName, v.FormerMember = c.UserName, c.FormerMember
		}
		out[i] = v
	}
	return out, nil
}

// view reads everything of a liturgy inside the caller's transaction.
func (u *Liturgies) view(ctx context.Context, sc churchScope, l domain.Liturgy) (LiturgyView, error) {
	items, err := sc.cs.LiturgyItems().ByLiturgy(ctx, l.ID)
	if err != nil {
		return LiturgyView{}, err
	}
	iv, err := itemViews(ctx, sc.cs, items)
	if err != nil {
		return LiturgyView{}, err
	}
	as, err := sc.cs.Assignments().ByLiturgy(ctx, l.ID)
	if err != nil {
		return LiturgyView{}, err
	}
	av, err := assignmentViews(ctx, sc, as)
	if err != nil {
		return LiturgyView{}, err
	}
	v := LiturgyView{Liturgy: l, Items: iv, Assignments: av, Problems: domain.Problems(items),
		Actions: liturgyActions(sc.actor, l.State)}
	if canSeeLiturgies(sc.actor) {
		v.Review = &ReviewInfo{}
		if v.Review.OpenComments, err = sc.cs.Comments().Open(ctx, l.ID); err != nil {
			return LiturgyView{}, err
		}
		last, err := sc.cs.StateChanges().Last(ctx, l.ID)
		switch {
		case err == nil:
			cv, err := sc.stateChangeView(ctx, last)
			if err != nil {
				return LiturgyView{}, err
			}
			v.Review.LastChange = &cv
		case !errors.Is(err, ErrNotFound):
			return LiturgyView{}, err
		}
	}
	return v, nil
}

// List returns the liturgies the caller may see (10 §4). A member without a
// liturgy scope sees the published ones only.
func (u *Liturgies) List(ctx context.Context, sess *domain.Session, f LiturgyFilter) (LiturgyPage, error) {
	var res LiturgyPage
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if !canSeeLiturgies(sc.actor) {
			if f.State != "" && f.State != domain.StatePublished {
				res = LiturgyPage{}
				return nil
			}
			f.State = domain.StatePublished
		}
		rows, total, err := sc.cs.Liturgies().List(ctx, f)
		if err != nil {
			return err
		}
		res = LiturgyPage{Total: total, Items: make([]LiturgyListItem, len(rows))}
		for i, r := range rows {
			res.Items[i] = LiturgyListItem{Liturgy: r.Liturgy, ItemCount: r.ItemCount, Actions: liturgyActions(sc.actor, r.Liturgy.State)}
		}
		return nil
	})
	return res, err
}

// Get returns one liturgy with its items, assignments and problems.
func (u *Liturgies) Get(ctx context.Context, sess *domain.Session, id domain.LiturgyID) (LiturgyView, error) {
	var res LiturgyView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		l, err := sc.openLiturgy(ctx, id)
		if err != nil {
			return err
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}

// EditView is a history row with the name of its author.
type EditView struct {
	Edit     domain.Edit
	UserName string
}

// Edits returns the history of a liturgy, newest first (10 §7).
func (u *Liturgies) Edits(ctx context.Context, sess *domain.Session, id domain.LiturgyID, limit int) ([]EditView, error) {
	if limit < 1 || limit > domain.MaxLiturgyQuery {
		limit = 50
	}
	var res []EditView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if _, err := sc.openLiturgy(ctx, id); err != nil {
			return err
		}
		edits, err := sc.cs.Edits().List(ctx, id, limit)
		if err != nil {
			return err
		}
		names := map[domain.UserID]string{}
		res = make([]EditView, len(edits))
		for i, e := range edits {
			name, ok := names[e.UserID]
			if !ok {
				if usr, err := sc.users.ByID(ctx, e.UserID); err == nil {
					name = usr.Name
				} else if !errors.Is(err, ErrNotFound) {
					return err
				}
				names[e.UserID] = name
			}
			res[i] = EditView{Edit: e, UserName: name}
		}
		return nil
	})
	return res, err
}

// Assignable is one member who can be assigned.
type Assignable struct {
	UserID domain.UserID
	Name   string
}

// Assignable lists every member by name and nothing else, for liturgy.edit
// holders, who may lack members.view (10 §4, Q-3.8).
func (u *Liturgies) Assignable(ctx context.Context, sess *domain.Session) ([]Assignable, error) {
	var res []Assignable
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyEdit); err != nil {
			return err
		}
		members, err := sc.cs.Memberships().List(ctx)
		if err != nil {
			return err
		}
		res = make([]Assignable, len(members))
		for i, m := range members {
			res[i] = Assignable{UserID: m.UserID, Name: m.User.Name}
		}
		return nil
	})
	return res, err
}

// Delete removes a liturgy that is not published, with everything in it
// (liturgy.manage). Published liturgies can only be archived (10 §2.1).
func (u *Liturgies) Delete(ctx context.Context, sess *domain.Session, id domain.LiturgyID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		l, err := sc.openLiturgy(ctx, id)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeLiturgyManage); err != nil {
			return err
		}
		if !l.State.Deletable() {
			return ErrLiturgyNotDeletable
		}
		return missing(sc.cs.Liturgies().Delete(ctx, id))
	})
}
