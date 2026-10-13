// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// 16C.1, pinned before Review is split: an action the server does not know is
// invalid input on "action", and nothing changes.
func TestReviewUnknownAction(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		before := r.state()
		_, err := r.review(r.a, domain.ReviewAction("bless"), nil, "")
		var in *domain.InvalidInputError
		if !errors.As(err, &in) || in.Field != "action" {
			t.Fatalf("unknown action: %v", err)
		}
		if r.state() != before {
			t.Errorf("state %s -> %s", before, r.state())
		}
	})
}

// 16C.1, pinned before buildPublished is split: when the copy is too large the
// error names the largest item, also when that is not the first one.
func TestPublishSizeNamesLargestItem(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		r.pray("Small")
		last := r.pray("The largest")
		// Over 4 MiB in the last item only; the first (the opening prayer) and "Small" stay small.
		r.sql("UPDATE liturgy_items SET text = ? WHERE id = ?", strings.Repeat("a", domain.MaxPublishedBytes+1), string(last))
		r.approve()
		_, err := r.publish()
		var big *app.PublishTooLargeError
		if !errors.As(err, &big) || big.LargestItem != "The largest" {
			t.Fatalf("oversized copy: %v", err)
		}
		if r.state() != domain.StateApproved || len(r.versions()) != 0 {
			t.Errorf("state %s, %d versions", r.state(), len(r.versions()))
		}
	})
}

// 16C.1, pinned before the team list of a copy is split off: a member who
// holds two duties is on the copy twice (once per duty) and in the "my
// assignments" index once.
func TestPublishMemberWithTwoDuties(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		member, _ := r.member("two@example.org", r.role(domain.OriginEditor).ID)
		duties, err := r.vocab.List(r.ctx, r.a, domain.KindDuty)
		if err != nil || len(duties) < 2 {
			t.Fatalf("duties: %v (%d)", err, len(duties))
		}
		for _, d := range duties[:2] {
			if _, err := r.liturgies.AddAssignment(r.ctx, r.a, r.lid, app.AssignmentInput{DutyID: domain.DutyID(d.Entry.ID), UserID: member.UserID}); err != nil {
				t.Fatal(err)
			}
		}
		r.approve()
		r.mustPublish()
		c := r.content(r.versions()[0])
		if len(c.Assignments) != 2 || c.Assignments[0].UserID != member.UserID || c.Assignments[1].UserID != member.UserID {
			t.Errorf("assignments on the copy: %+v", c.Assignments)
		}
		mine, err := r.liturgies.MyAssignments(r.ctx, member)
		if err != nil {
			t.Fatal(err)
		}
		if len(mine.Items) != 1 {
			t.Errorf("my assignments: %d entries, want 1", len(mine.Items))
		}
	})
}
