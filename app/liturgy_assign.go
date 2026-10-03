// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/brightfellow-net/liturgist/domain"
)

// AssignmentInput assigns a member (UserID) or a person by name (Name) to a duty.
type AssignmentInput struct {
	DutyID domain.DutyID
	UserID domain.UserID
	Name   string
}

// AddAssignment assigns a person to a duty (liturgy.edit). Not versioned
// (10 §2.4); it takes LockChurch, so the person is a member when it is written.
func (u *Liturgies) AddAssignment(ctx context.Context, sess *domain.Session, id domain.LiturgyID, in AssignmentInput) (AssignmentView, error) {
	var res AssignmentView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		l, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		a := domain.Assignment{ID: domain.AssignmentID(u.IDs.NewID()), LiturgyID: id, DutyID: in.DutyID, UserID: in.UserID,
			Name: in.Name, CreatedAt: u.Clock.Now()}
		switch {
		case a.UserID != "" && a.Name != "":
			return &domain.InvalidInputError{Field: "name", Message: "Give a member or a name, not both."}
		case a.UserID == "" && a.Name == "":
			return &domain.InvalidInputError{Field: "user_id", Message: "Give a member or a name.", Reason: domain.ReasonRequired}
		case a.Name != "":
			if a.NameKey, err = domain.ValidateAssignmentName(&a.Name); err != nil {
				return err
			}
		}
		if err := checkDuty(ctx, sc.cs, a.DutyID, "duty_id"); err != nil {
			return err
		}
		if a.DutyID == "" {
			return &domain.InvalidInputError{Field: "duty_id", Message: "Choose a duty.", Reason: domain.ReasonRequired}
		}
		if a.UserID != "" {
			if _, err := sc.cs.Memberships().ByUser(ctx, a.UserID); err != nil {
				if errors.Is(err, ErrNotFound) {
					return &domain.InvalidInputError{Field: "user_id", Message: "This person is not a member of the church."}
				}
				return err
			}
		}
		have, err := sc.cs.Assignments().ByLiturgy(ctx, id)
		if err != nil {
			return err
		}
		if len(have) >= domain.MaxAssignments {
			return domain.LimitError("assignments", domain.MaxAssignments, len(have))
		}
		for _, h := range have {
			if h.DutyID == a.DutyID && ((a.UserID != "" && h.UserID == a.UserID) || (a.Name != "" && h.NameKey == a.NameKey)) {
				return ErrAssignmentExists
			}
		}
		if err := sc.cs.Assignments().Add(ctx, a); err != nil {
			var uq *UniqueError
			if errors.As(err, &uq) && (uq.Constraint == "assignments_user_key" || uq.Constraint == "assignments_name_key") {
				return ErrAssignmentExists
			}
			return err
		}
		if err := u.record(ctx, sc, id, domain.CmdAssignmentAdd, "", l.Version, 0, nil, imageOfAssignment(a)); err != nil {
			return err
		}
		v, err := assignmentViews(ctx, sc, []domain.Assignment{a})
		if err != nil {
			return err
		}
		res = v[0]
		return nil
	})
	return res, err
}

// RemoveAssignment removes an assignment (liturgy.edit).
func (u *Liturgies) RemoveAssignment(ctx context.Context, sess *domain.Session, id domain.LiturgyID, assignment domain.AssignmentID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		l, err := sc.openEdit(ctx, id)
		if err != nil {
			return err
		}
		a, err := sc.cs.Assignments().ByID(ctx, id, assignment)
		if err != nil {
			return missing(err)
		}
		if err := sc.cs.Assignments().Remove(ctx, id, assignment); err != nil {
			return missing(err)
		}
		return u.record(ctx, sc, id, domain.CmdAssignmentRemove, "", l.Version, 0, imageOfAssignment(a), nil)
	})
}
