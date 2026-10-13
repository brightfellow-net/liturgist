// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// ReviewInput is a request to move a liturgy through the review workflow (12 §2).
type ReviewInput struct {
	Action domain.ReviewAction
	// EditSeq is the edit_seq the reviewer saw; required by approve and request
	// changes (P-70), nil when the request had none.
	EditSeq *int
	Note    string
}

// openReviewable loads a liturgy whose review data the actor may read: a
// liturgy scope is needed whatever the state (12 §2, P-74).
func (sc churchScope) openReviewable(ctx context.Context, id domain.LiturgyID) (domain.Liturgy, error) {
	l, err := sc.cs.Liturgies().ByID(ctx, id)
	if err != nil {
		return domain.Liturgy{}, missing(err)
	}
	if !canSeeLiturgies(sc.actor) {
		return domain.Liturgy{}, notFound(ReasonNotVisible)
	}
	return l, nil
}

func (sc churchScope) stateChangeView(ctx context.Context, c domain.StateChange) (StateChangeView, error) {
	v := StateChangeView{Change: c}
	u, err := sc.users.ByID(ctx, c.UserID)
	switch {
	case err == nil:
		v.UserName = u.Name
	case !errors.Is(err, ErrNotFound):
		return StateChangeView{}, err
	}
	return v, nil
}

// Review runs one transition of the review workflow (12 §2, 13 §2). The order
// of the checks is part of the contract: 404, 403, 422, invalid_transition,
// liturgy_archived, the submit checks or the reopen limit, then one conditional
// update. A reopen takes LockChurch first, because reopening a published
// liturgy counts the unpublished liturgies (13 §4).
func (u *Liturgies) Review(ctx context.Context, sess *domain.Session, id domain.LiturgyID, in ReviewInput) (LiturgyView, error) {
	var res LiturgyView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, in.Action == domain.ActionReopen)
		if err != nil {
			return err
		}
		l, err := sc.openLiturgy(ctx, id)
		if err != nil {
			return err
		}
		rule, note, expect, err := u.reviewAllowed(ctx, sc, l, in)
		if err != nil {
			return err
		}
		if in.Action == domain.ActionSubmit {
			if err := reviewSubmitChecks(ctx, sc, id); err != nil {
				return err
			}
		}
		now := u.Clock.Now()
		seq, ok, err := sc.cs.Liturgies().Transition(ctx, id, l.State, rule.To, expect, now)
		if err != nil {
			return err
		}
		if !ok {
			return explainNoTransition(ctx, sc, id, l.State)
		}
		if err := sc.cs.StateChanges().Append(ctx, domain.StateChange{ID: domain.StateChangeID(u.IDs.NewID()), LiturgyID: id,
			From: l.State, To: rule.To, UserID: sc.actor.UserID, Note: note, EditSeq: seq, CreatedAt: now}); err != nil {
			return err
		}
		if in.Action == domain.ActionPublish {
			// The copy is made in the same transaction as the state change: a
			// liturgy is never published without a version (13 §2).
			content, assignees, err := buildPublished(ctx, sc, l)
			if err != nil {
				return err
			}
			if _, err := sc.cs.Published().Create(ctx, domain.PublishedVersion{ID: domain.PublishedVersionID(u.IDs.NewID()), LiturgyID: id,
				Content: content, PublishedBy: sc.actor.UserID, PublishedAt: now}, assignees); err != nil {
				return err
			}
		}
		l, err = sc.cs.Liturgies().ByID(ctx, id)
		if err != nil {
			return missing(err)
		}
		res, err = u.view(ctx, sc, l)
		return err
	})
	return res, err
}

// reviewAllowed checks everything that can be known before the state is
// changed, in this order: the action, the caller's scope, the note, the
// edit_seq, the current state, archiving and, for reopening a published
// liturgy, the limit. It returns the rule, the trimmed note and the edit_seq
// the conditional update must still see.
func (u *Liturgies) reviewAllowed(ctx context.Context, sc churchScope, l domain.Liturgy, in ReviewInput) (rule domain.ReviewRule, note string, expect int, err error) {
	rule, ok := in.Action.Rule()
	if !ok {
		return rule, "", 0, &domain.InvalidInputError{Field: "action", Message: "Unknown action."}
	}
	if err := sc.actor.Require(rule.Scope); err != nil {
		return rule, "", 0, err
	}
	note = in.Note
	if err := domain.ValidateNote(&note); err != nil {
		return rule, "", 0, err
	}
	if rule.NeedsSeq && in.EditSeq == nil {
		return rule, "", 0, &domain.InvalidInputError{Field: "edit_seq", Message: "Required."}
	}
	if !rule.Allowed(l.State) {
		return rule, "", 0, &InvalidTransitionError{State: l.State}
	}
	if l.ArchivedAt != nil {
		return rule, "", 0, ErrLiturgyArchived
	}
	if in.Action == domain.ActionReopen && l.State == domain.StatePublished {
		lim, err := u.limits(ctx)
		if err != nil {
			return rule, "", 0, err
		}
		if err := lim.check(ctx, sc.cs, 1, LimitMaxUnpublishedLiturgies); err != nil {
			return rule, "", 0, err
		}
	}
	expect = l.EditSeq
	if rule.NeedsSeq {
		expect = *in.EditSeq
	}
	return rule, note, expect, nil
}

// reviewSubmitChecks refuses to submit a liturgy with no items or with
// problems. The check and the update see the same edit_seq: an edit between
// them leaves the update without a row (12 §2, P-69).
func reviewSubmitChecks(ctx context.Context, sc churchScope, id domain.LiturgyID) error {
	items, err := sc.cs.LiturgyItems().ByLiturgy(ctx, id)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return ErrEmptyLiturgy
	}
	if p := domain.Problems(items); len(p) > 0 {
		return &HasProblemsError{Problems: p}
	}
	return nil
}

// explainNoTransition says why the conditional update found no row: the
// liturgy is gone, in another state or archived, or it changed since the
// reviewer saw it. The state wins (12 §2).
func explainNoTransition(ctx context.Context, sc churchScope, id domain.LiturgyID, was domain.LiturgyState) error {
	cur, err := sc.cs.Liturgies().ByID(ctx, id)
	if err != nil {
		return missing(err)
	}
	switch {
	case cur.State != was:
		return &InvalidTransitionError{State: cur.State}
	case cur.ArchivedAt != nil:
		return ErrLiturgyArchived
	}
	return ErrReviewStale
}

// StateChangePage is one page of the review history.
type StateChangePage struct {
	Items []StateChangeView
	Total int
}

// StateChanges lists the review history of a liturgy, newest first (12 §3).
func (u *Liturgies) StateChanges(ctx context.Context, sess *domain.Session, id domain.LiturgyID, limit, offset int) (StateChangePage, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var res StateChangePage
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if _, err := sc.openReviewable(ctx, id); err != nil {
			return err
		}
		rows, total, err := sc.cs.StateChanges().List(ctx, id, limit, offset)
		if err != nil {
			return err
		}
		res = StateChangePage{Total: total, Items: make([]StateChangeView, len(rows))}
		for i, c := range rows {
			if res.Items[i], err = sc.stateChangeView(ctx, c); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}
