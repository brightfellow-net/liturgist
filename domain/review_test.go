// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strings"
	"testing"
)

// TC-R-001: exactly four (state, action) pairs plus the second source of submit.
func TestReviewTransitionTable(t *testing.T) {
	allowed := map[ReviewAction][]LiturgyState{
		ActionSubmit:         {StateDraft, StateNeedsRevision},
		ActionApprove:        {StateInReview},
		ActionRequestChanges: {StateInReview},
		ActionReopen:         {StateApproved},
	}
	n := 0
	for _, act := range ReviewActions {
		rule, ok := act.Rule()
		if !ok {
			t.Fatalf("no rule for %s", act)
		}
		for _, s := range LiturgyStates {
			want := false
			for _, a := range allowed[act] {
				want = want || a == s
			}
			if got := rule.Allowed(s); got != want {
				t.Errorf("%s from %s: allowed %v, want %v", act, s, got, want)
			}
			if want {
				n++
			}
		}
		if !rule.To.Valid() {
			t.Errorf("%s leads to %q", act, rule.To)
		}
	}
	if n != 5 {
		t.Errorf("%d allowed pairs, want 5", n)
	}
	if _, ok := ReviewAction("publish").Rule(); ok {
		t.Error("publish is step 5")
	}
	// Only approve and request changes state the edit_seq; the scopes are the SPEC's.
	for act, want := range map[ReviewAction]struct {
		seq   bool
		scope Scope
	}{
		ActionSubmit: {false, ScopeLiturgyEdit}, ActionApprove: {true, ScopeLiturgyApprove},
		ActionRequestChanges: {true, ScopeLiturgyApprove}, ActionReopen: {false, ScopeLiturgyApprove},
	} {
		if r, _ := act.Rule(); r.NeedsSeq != want.seq || r.Scope != want.scope {
			t.Errorf("%s: %+v", act, r)
		}
	}
}

// TC-R-006: the text rule of notes.
func TestValidateNote(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"  hi \r\n there\r ", "hi \n there", true},
		{"", "", true},
		{strings.Repeat("é", MaxNote), strings.Repeat("é", MaxNote), true},
		{strings.Repeat("é", MaxNote+1), "", false},
		{" " + strings.Repeat("a", MaxNote) + " ", strings.Repeat("a", MaxNote), true},
	} {
		got := tc.in
		err := ValidateNote(&got)
		if (err == nil) != tc.ok || (tc.ok && got != tc.want) {
			t.Errorf("%q: got %q, err %v", tc.in[:min(len(tc.in), 10)], got[:min(len(got), 10)], err)
		}
	}
}

func TestCanComment(t *testing.T) {
	want := map[LiturgyState]bool{StateDraft: true, StateInReview: true, StateNeedsRevision: true}
	for _, s := range LiturgyStates {
		if s.CanComment() != want[s] {
			t.Errorf("%s: %v", s, s.CanComment())
		}
	}
}

func TestValidateComment(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"  Ganti \r\nini ", "Ganti \nini", true},
		{"", "", false},
		{" \n\t ", "", false},
		{strings.Repeat("é", MaxComment), strings.Repeat("é", MaxComment), true},
		{strings.Repeat("é", MaxComment+1), "", false},
	} {
		got := tc.in
		err := ValidateComment(&got)
		if (err == nil) != tc.ok || (tc.ok && got != tc.want) {
			t.Errorf("%q: got %q, err %v", tc.in[:min(len(tc.in), 10)], got[:min(len(got), 10)], err)
		}
	}
}
