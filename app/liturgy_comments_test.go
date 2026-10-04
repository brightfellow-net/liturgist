// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (r renv) comment(sess *domain.Session, item domain.ItemID, body string) (app.CommentView, error) {
	return r.liturgies.AddComment(r.ctx, sess, r.lid, item, body)
}

func (r renv) mustComment(sess *domain.Session, item domain.ItemID, body string) app.CommentView {
	r.t.Helper()
	c, err := r.comment(sess, item, body)
	if err != nil {
		r.t.Fatalf("comment: %v", err)
	}
	return c
}

func (r renv) comments(sess *domain.Session, resolved *bool) app.CommentList {
	r.t.Helper()
	l, err := r.liturgies.Comments(r.ctx, sess, r.lid, resolved)
	if err != nil {
		r.t.Fatal(err)
	}
	return l
}

// commenterOnly is a member with liturgy.comment and nothing else.
func (r renv) commenterOnly() *domain.Session {
	r.t.Helper()
	role, err := r.roles.Create(r.ctx, r.a, app.RoleInput{Name: "Komentator", Scopes: []domain.Scope{domain.ScopeLiturgyComment}})
	if err != nil {
		r.t.Fatal(err)
	}
	sess, _ := r.member("kom@example.org", role.Role.ID)
	return sess
}

// IT-R-007: create, resolve, reopen, filter; the view counts what is open.
func TestComments(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		item := r.order()[0]
		c1 := r.mustComment(r.b, item, "  Ganti kata\r\nini  ")
		c2 := r.mustComment(r.a, "", "Urutan sudah baik")
		if c1.Comment.Body != "Ganti kata\nini" || c1.Comment.ItemTitle != "Opening prayer" || c1.AuthorName != "b" || c2.Comment.ItemID != "" || c2.Comment.ItemTitle != "" {
			t.Fatalf("comments: %+v %+v", c1, c2)
		}
		l := r.comments(r.a, nil)
		if len(l.Items) != 2 || l.Open != 2 || l.Items[0].Comment.ID != c1.Comment.ID {
			t.Fatalf("list: %+v", l)
		}
		if v := r.liturgy(r.lid); v.Review.OpenComments != 2 || !v.Actions.Comment {
			t.Errorf("view: open %d, actions %+v", v.Review.OpenComments, v.Actions)
		}

		// Anyone with liturgy.comment may resolve any comment; the resolver is recorded.
		got, err := r.liturgies.SetCommentResolved(r.ctx, r.b, r.lid, c2.Comment.ID, true)
		if err != nil || !got.Comment.Resolved() || got.ResolverName != "b" || got.Comment.AuthorID == got.Comment.ResolvedBy {
			t.Fatalf("resolve: %+v %v", got, err)
		}
		// Again: a no-op, the first resolver stays.
		again, err := r.liturgies.SetCommentResolved(r.ctx, r.a, r.lid, c2.Comment.ID, true)
		if err != nil || again.ResolverName != "b" {
			t.Fatalf("resolve twice: %+v %v", again, err)
		}
		yes, no := true, false
		if l := r.comments(r.a, &yes); len(l.Items) != 1 || l.Open != 1 {
			t.Errorf("resolved filter: %+v", l)
		}
		if l := r.comments(r.a, &no); len(l.Items) != 1 || l.Items[0].Comment.ID != c1.Comment.ID || l.Open != 1 {
			t.Errorf("open filter: %+v", l)
		}
		reopened, err := r.liturgies.SetCommentResolved(r.ctx, r.a, r.lid, c2.Comment.ID, false)
		if err != nil || reopened.Comment.Resolved() || r.comments(r.a, nil).Open != 2 {
			t.Fatalf("reopen: %+v %v", reopened, err)
		}

		// Comments work in review, and a transition counts them.
		r.must(r.b, domain.ActionSubmit, nil, "")
		r.mustComment(r.a, item, "Satu lagi")
		v := r.must(r.a, domain.ActionApprove, r.seq(), "")
		if v.Review.OpenComments != 3 {
			t.Errorf("open comments at approval: %d", v.Review.OpenComments)
		}
	})
}

// A comment survives its item's removal, and undo brings it back to the item.
func TestCommentOutlivesItem(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		item := r.order()[0]
		c := r.mustComment(r.b, item, "Perlu diubah")
		r.fresh()
		v := r.liturgy(r.lid)
		if _, err := r.liturgies.RemoveItem(r.ctx, r.b, r.lid, item, v.Liturgy.Version); err != nil {
			t.Fatal(err)
		}
		l := r.comments(r.a, nil)
		if len(l.Items) != 1 || l.Items[0].Comment.ID != c.Comment.ID || l.Items[0].Comment.ItemID != item || l.Items[0].Comment.ItemTitle != "Opening prayer" {
			t.Fatalf("after removal: %+v", l)
		}
		if _, ok := r.item(item); ok {
			t.Fatal("item still there")
		}
		r.mustUndo(r.b)
		if _, ok := r.item(item); !ok {
			t.Fatal("undo did not restore the item")
		}
		if l := r.comments(r.a, nil); l.Items[0].Comment.ItemID != item {
			t.Errorf("comment no longer points at the restored item: %+v", l.Items[0].Comment)
		}
	})
}

// TC-R-005, IT-R-008, IT-R-009: validation, states, scopes.
func TestCommentRules(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		item := r.order()[0]
		var inv *domain.InvalidInputError
		for _, tc := range []struct {
			item        domain.ItemID
			body, field string
		}{
			{"", "   ", "body"}, {"", strings.Repeat("é", 2001), "body"}, {"01ARZ3NDEKTSV4RRFFQ69G5FAV", "x", "item_id"},
		} {
			if _, err := r.comment(r.b, tc.item, tc.body); !errors.As(err, &inv) || inv.Field != tc.field {
				t.Errorf("%q/%s: %v", tc.body[:min(3, len(tc.body))], tc.item, err)
			}
		}
		if _, err := r.comment(r.b, item, strings.Repeat("é", 2000)); err != nil {
			t.Errorf("2000 runes: %v", err)
		}
		// Another liturgy's item is not an item of this one.
		other := r.draft()
		foreign := r.addItem(other, domain.ItemPrayer, "Elsewhere")
		if _, err := r.comment(r.b, foreign, "x"); !errors.As(err, &inv) || inv.Field != "item_id" {
			t.Errorf("foreign item: %v", err)
		}

		// Scopes: no scope 404, edit-only 403, a commenter without edit may comment.
		if _, err := r.comment(r.team, "", "x"); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member: %v", err)
		}
		if _, err := r.liturgies.Comments(r.ctx, r.team, r.lid, nil); !isNotFound(err, app.ReasonNotVisible) {
			t.Errorf("team member list: %v", err)
		}
		editOnly, err := r.roles.Create(r.ctx, r.a, app.RoleInput{Name: "Edit saja", Scopes: []domain.Scope{domain.ScopeLiturgyEdit}})
		if err != nil {
			t.Fatal(err)
		}
		eo, _ := r.member("eo@example.org", editOnly.Role.ID)
		if _, err := r.comment(eo, "", "x"); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("edit-only: %v", err)
		}
		kom := r.commenterOnly()
		c := r.mustComment(kom, "", "Dari komentator")
		if _, err := r.liturgies.AddItem(r.ctx, kom, r.lid, app.ItemInput{LiturgyVersion: 1, Title: "x", Type: domain.ItemPrayer}); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("commenter editing: %v", err)
		}

		// Approved and published: locked, for creating and resolving.
		r.must(r.a, domain.ActionSubmit, nil, "")
		r.must(r.a, domain.ActionApprove, r.seq(), "")
		for _, state := range []string{"approved", "published"} {
			r.sql("UPDATE liturgies SET state = ? WHERE id = ?", state, string(r.lid))
			if _, err := r.comment(r.a, "", "x"); !errors.Is(err, app.ErrLiturgyLocked) {
				t.Errorf("%s comment: %v", state, err)
			}
			if _, err := r.liturgies.SetCommentResolved(r.ctx, r.a, r.lid, c.Comment.ID, true); !errors.Is(err, app.ErrLiturgyLocked) {
				t.Errorf("%s resolve: %v", state, err)
			}
			if l := r.comments(r.a, nil); len(l.Items) != 2 {
				t.Errorf("%s: %d comments readable", state, len(l.Items))
			}
		}
	})
}

// IT-R-014: a comment id of another liturgy or church is a 404.
func TestCommentScoping(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		c := r.mustComment(r.b, "", "x")
		other := r.draft()
		if _, err := r.liturgies.SetCommentResolved(r.ctx, r.a, other, c.Comment.ID, true); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("comment of another liturgy: %v", err)
		}
		if _, err := r.liturgies.SetCommentResolved(r.ctx, r.a, r.lid, "01ARZ3NDEKTSV4RRFFQ69G5FAV", true); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("unknown comment: %v", err)
		}
		bctx := app.WithTenant(ctx, r.oldChurch("id"))
		if _, err := r.liturgies.Comments(bctx, r.a, r.lid, nil); err == nil {
			t.Error("comments of church A through church B")
		}
		if got := r.comments(r.a, nil).Items[0].Comment; got.Resolved() {
			t.Error("a refused resolve changed the comment")
		}
		// Two simultaneous resolves: the first is recorded.
		var wg sync.WaitGroup
		for _, s := range []*domain.Session{r.a, r.b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := r.liturgies.SetCommentResolved(r.ctx, s, r.lid, c.Comment.ID, true); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		got := r.comments(r.a, nil).Items[0]
		if !got.Comment.Resolved() || got.Comment.ResolvedBy == "" {
			t.Errorf("after two resolves: %+v", got)
		}
	})
}

// The cap of 500 comments, with simultaneous requests at the edge.
func TestCommentLimit(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		for i := 0; i < domain.MaxComments-8; i++ {
			r.sql(`INSERT INTO liturgy_comments (id, church_id, liturgy_id, item_title, author_id, body, created_at)
				SELECT ?, church_id, id, '', created_by, 'x', created_at FROM liturgies WHERE id = ?`, r.ids.NewID(), string(r.lid))
		}
		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			okay int
		)
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := r.comment(r.b, "", "x")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					okay++
				case errors.Is(err, app.ErrCommentLimit):
				default:
					t.Errorf("comment: %v", err)
				}
			}()
		}
		wg.Wait()
		if okay != 8 || len(r.comments(r.a, nil).Items) != domain.MaxComments {
			t.Fatalf("%d accepted, %d stored", okay, len(r.comments(r.a, nil).Items))
		}
		if _, err := r.comment(r.b, "", "x"); !errors.Is(err, app.ErrCommentLimit) {
			t.Errorf("501st: %v", err)
		}
	})
}

// IT-R-013: a comment racing an approval is counted by it or refused by it.
func TestCommentLosesToApproval(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		postgresOnly(t, db)
		r := newReviewEnv(t, db)
		r.must(r.b, domain.ActionSubmit, nil, "")
		slow := *r.liturgies
		slow.Clock = &raceClock{clock: r.clock, hook: func() { r.must(r.a, domain.ActionApprove, r.seq(), "") }}
		_, err := slow.AddComment(r.ctx, r.a, r.lid, "", "late")
		if !errors.Is(err, app.ErrLiturgyLocked) {
			t.Fatalf("comment after approval: %v", err)
		}
		if n := len(r.comments(r.a, nil).Items); n != 0 {
			t.Errorf("%d comments on an approved liturgy", n)
		}
	})
}

func TestCommentAndApproveRace(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		r := newReviewEnv(t, db)
		for round := 0; round < 40; round++ {
			lid := r.draft()
			r.addItem(lid, domain.ItemPrayer, "X")
			if _, err := r.liturgies.Review(r.ctx, r.b, lid, app.ReviewInput{Action: domain.ActionSubmit}); err != nil {
				t.Fatal(err)
			}
			seq := r.liturgy(lid).Liturgy.EditSeq
			var wg sync.WaitGroup
			var cErr, aErr error
			var approved app.LiturgyView
			wg.Add(2)
			go func() { defer wg.Done(); _, cErr = r.liturgies.AddComment(r.ctx, r.a, lid, "", "x") }()
			go func() {
				defer wg.Done()
				approved, aErr = r.liturgies.Review(r.ctx, r.a, lid, app.ReviewInput{Action: domain.ActionApprove, EditSeq: &seq})
			}()
			wg.Wait()
			if aErr != nil || (cErr != nil && !errors.Is(cErr, app.ErrLiturgyLocked)) {
				t.Fatalf("round %d: approve %v, comment %v", round, aErr, cErr)
			}
			list, err := r.liturgies.Comments(r.ctx, r.a, lid, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if cErr == nil {
				want = 1
			}
			if len(list.Items) != want {
				t.Fatalf("round %d: comment error %v but %d comments stored", round, cErr, len(list.Items))
			}
			// A comment that was accepted was committed before the approval read its
			// count (it held the row lock first), so the approval counted it.
			if approved.Review.OpenComments != want {
				t.Fatalf("round %d: approval counted %d open comments, %d stored", round, approved.Review.OpenComments, want)
			}
		}
	})
}
