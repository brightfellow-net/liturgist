// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "time"

// InviteLifetime and ResetLifetime are how long links stay valid (03 §7, §9).
const (
	InviteLifetime = 7 * 24 * time.Hour
	ResetLifetime  = 24 * time.Hour
	SetupLifetime  = 24 * time.Hour
)

// Invite invites a person to a church (03 §7).
type Invite struct {
	ID             InviteID
	ChurchID       ChurchID
	TokenHash      string
	Name           string
	Email          string // "" when none
	Phone          string // "" when none
	RoleIDs        []RoleID
	CreatedBy      UserID // "" when the creator was deleted
	CreatedAt      time.Time
	ExpiresAt      time.Time
	AcceptedAt     time.Time // zero when not accepted
	AcceptedUserID UserID
	CancelledAt    time.Time // zero when not cancelled
}

// InviteStatus is an invite's state at a moment.
type InviteStatus string

// Invite states.
const (
	InvitePending   InviteStatus = "pending"
	InviteExpired   InviteStatus = "expired"
	InviteAccepted  InviteStatus = "accepted"
	InviteCancelled InviteStatus = "cancelled"
)

// Status returns the invite's state at now.
func (i Invite) Status(now time.Time) InviteStatus {
	switch {
	case !i.AcceptedAt.IsZero():
		return InviteAccepted
	case !i.CancelledAt.IsZero():
		return InviteCancelled
	case !i.ExpiresAt.After(now):
		return InviteExpired
	}
	return InvitePending
}

// TokenReason says why a link token can't be used (400 invalid_token).
type TokenReason string

// Token reasons (01 §10).
const (
	TokenUnknown   TokenReason = "unknown"
	TokenExpired   TokenReason = "expired"
	TokenUsed      TokenReason = "used"
	TokenCancelled TokenReason = "cancelled"
)

// Reason maps a non-pending status to the invalid_token reason.
func (s InviteStatus) Reason() TokenReason {
	switch s {
	case InviteAccepted:
		return TokenUsed
	case InviteCancelled:
		return TokenCancelled
	case InviteExpired:
		return TokenExpired
	}
	return TokenUnknown
}

// PasswordReset is a single-use reset link (03 §9).
type PasswordReset struct {
	ID        string
	UserID    UserID
	TokenHash string
	CreatedBy UserID // "" for the command line (or a deleted creator)
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time // zero when unused
}

// Reason returns why the reset can't be used at now, or "" if it can.
func (r PasswordReset) Reason(now time.Time) TokenReason {
	switch {
	case !r.UsedAt.IsZero():
		return TokenUsed
	case !r.ExpiresAt.After(now):
		return TokenExpired
	}
	return ""
}
