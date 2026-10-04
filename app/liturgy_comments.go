// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// CommentView is a comment with the names of its author and resolver.
type CommentView struct {
	Comment      domain.Comment
	AuthorName   string
	ResolverName string
}

// CommentList is the comments of a liturgy, oldest first, and the number of
// unresolved ones whatever the filter (12 §4).
type CommentList struct {
	Items []CommentView
	Open  int
}

func (sc churchScope) commentView(ctx context.Context, c domain.Comment, names map[domain.UserID]string) (CommentView, error) {
	name := func(id domain.UserID) (string, error) {
		if id == "" {
			return "", nil
		}
		if n, ok := names[id]; ok {
			return n, nil
		}
		u, err := sc.users.ByID(ctx, id)
		switch {
		case err == nil:
			names[id] = u.Name
		case errors.Is(err, ErrNotFound):
			names[id] = ""
		default:
			return "", err
		}
		return names[id], nil
	}
	v := CommentView{Comment: c}
	var err error
	if v.AuthorName, err = name(c.AuthorID); err != nil {
		return CommentView{}, err
	}
	v.ResolverName, err = name(c.ResolvedBy)
	return v, err
}

// Comments lists the comments of a liturgy (12 §3); a liturgy scope is needed
// whatever the state (P-74).
func (u *Liturgies) Comments(ctx context.Context, sess *domain.Session, id domain.LiturgyID, resolved *bool) (CommentList, error) {
	var res CommentList
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if _, err := sc.openReviewable(ctx, id); err != nil {
			return err
		}
		rows, err := sc.cs.Comments().List(ctx, id, resolved)
		if err != nil {
			return err
		}
		if res.Open, err = sc.cs.Comments().Open(ctx, id); err != nil {
			return err
		}
		names := map[domain.UserID]string{}
		res.Items = make([]CommentView, len(rows))
		for i, c := range rows {
			if res.Items[i], err = sc.commentView(ctx, c, names); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

// lockForComment is the first statement of a comment write: it waits for a
// transition and re-checks the state under the liturgy's row lock (12 §4).
func (sc churchScope) lockForComment(ctx context.Context, id domain.LiturgyID) error {
	ok, err := sc.cs.Liturgies().LockForComment(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		if _, err := sc.cs.Liturgies().ByID(ctx, id); err != nil {
			return missing(err)
		}
		return ErrLiturgyLocked
	}
	return nil
}

// openComment is the common start of the two comment writes: 404, 403, 409.
func (sc churchScope) openComment(ctx context.Context, id domain.LiturgyID) error {
	l, err := sc.openReviewable(ctx, id)
	if err != nil {
		return err
	}
	if err := sc.actor.Require(domain.ScopeLiturgyComment); err != nil {
		return err
	}
	if !l.State.CanComment() {
		return ErrLiturgyLocked
	}
	return nil
}

// AddComment writes a comment on an item (item "" = the whole liturgy) (12 §4).
func (u *Liturgies) AddComment(ctx context.Context, sess *domain.Session, id domain.LiturgyID, item domain.ItemID, body string) (CommentView, error) {
	var res CommentView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.openComment(ctx, id); err != nil {
			return err
		}
		now := u.Clock.Now()
		if err := domain.ValidateComment(&body); err != nil {
			return err
		}
		if err := sc.lockForComment(ctx, id); err != nil {
			return err
		}
		n, err := sc.cs.Comments().Count(ctx, id)
		if err != nil {
			return err
		}
		if n >= domain.MaxComments {
			return ErrCommentLimit
		}
		c := domain.Comment{ID: domain.CommentID(u.IDs.NewID()), LiturgyID: id, ItemID: item, AuthorID: sc.actor.UserID, Body: body, CreatedAt: now}
		if item != "" {
			it, err := sc.cs.LiturgyItems().ByID(ctx, id, item)
			if errors.Is(err, ErrNotFound) {
				return &domain.InvalidInputError{Field: "item_id", Message: "This item is not in the liturgy."}
			}
			if err != nil {
				return err
			}
			c.ItemTitle = it.Title
		}
		if err := sc.cs.Comments().Create(ctx, c); err != nil {
			if errors.Is(err, ErrReferenced) { // the liturgy was deleted meanwhile
				return ErrNotFound
			}
			return err
		}
		res, err = sc.commentView(ctx, c, map[domain.UserID]string{})
		return err
	})
	return res, err
}

// SetCommentResolved resolves or reopens a comment (12 §4). Setting the state it
// already has changes nothing and succeeds.
func (u *Liturgies) SetCommentResolved(ctx context.Context, sess *domain.Session, id domain.LiturgyID, cid domain.CommentID, resolved bool) (CommentView, error) {
	var res CommentView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.openComment(ctx, id); err != nil {
			return err
		}
		now := u.Clock.Now()
		if err := sc.lockForComment(ctx, id); err != nil {
			return err
		}
		if _, err := sc.cs.Comments().SetResolved(ctx, id, cid, resolved, sc.actor.UserID, now); err != nil {
			return err
		}
		c, err := sc.cs.Comments().ByID(ctx, id, cid)
		if err != nil {
			return missing(err)
		}
		res, err = sc.commentView(ctx, c, map[domain.UserID]string{})
		return err
	})
	return res, err
}
