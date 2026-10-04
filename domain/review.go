// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// StateChangeID identifies one row of a liturgy's review history (12 §5).
type StateChangeID string

// MaxNote is the longest note of a state change, in characters (12 §2).
const MaxNote = 500

// ReviewAction is one of the four transitions of the review workflow (12 §2).
type ReviewAction string

// The actions. Publishing is step 5.
const (
	ActionSubmit         ReviewAction = "submit"
	ActionApprove        ReviewAction = "approve"
	ActionRequestChanges ReviewAction = "request_changes"
	ActionReopen         ReviewAction = "reopen"
)

// ReviewActions lists every action.
var ReviewActions = []ReviewAction{ActionSubmit, ActionApprove, ActionRequestChanges, ActionReopen}

// ReviewRule is the row of the transition table of one action (12 §2).
type ReviewRule struct {
	From  []LiturgyState
	To    LiturgyState
	Scope Scope
	// NeedsSeq: the request must state the edit_seq the reviewer saw (P-70).
	NeedsSeq bool
}

var reviewRules = map[ReviewAction]ReviewRule{
	ActionSubmit:         {From: []LiturgyState{StateDraft, StateNeedsRevision}, To: StateInReview, Scope: ScopeLiturgyEdit},
	ActionApprove:        {From: []LiturgyState{StateInReview}, To: StateApproved, Scope: ScopeLiturgyApprove, NeedsSeq: true},
	ActionRequestChanges: {From: []LiturgyState{StateInReview}, To: StateNeedsRevision, Scope: ScopeLiturgyApprove, NeedsSeq: true},
	ActionReopen:         {From: []LiturgyState{StateApproved}, To: StateDraft, Scope: ScopeLiturgyApprove},
}

// Rule returns the rule of the action; false for an unknown one.
func (a ReviewAction) Rule() (ReviewRule, bool) {
	r, ok := reviewRules[a]
	return r, ok
}

// Allowed reports whether the action may start from the state.
func (r ReviewRule) Allowed(from LiturgyState) bool {
	for _, s := range r.From {
		if s == from {
			return true
		}
	}
	return false
}

// CanComment reports whether comments may be written in the state (12 §4).
func (s LiturgyState) CanComment() bool {
	return s == StateDraft || s == StateInReview || s == StateNeedsRevision
}

// StateChange is one row of the review history (12 §5).
type StateChange struct {
	ID        StateChangeID
	LiturgyID LiturgyID
	From, To  LiturgyState
	UserID    UserID
	Note      string
	EditSeq   int
	CreatedAt time.Time
}

// NormalizeNote is the text rule of notes and comments (12 §2, §4): line endings
// become "\n", the ends are trimmed. Lengths count runes.
func NormalizeNote(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.TrimSpace(s)
}

// ValidateNote normalises a note and checks its length (12 §2).
func ValidateNote(note *string) error {
	*note = NormalizeNote(*note)
	if utf8.RuneCountInString(*note) > MaxNote {
		return tooLong("note", MaxNote)
	}
	return nil
}
