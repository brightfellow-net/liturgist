// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "time"

// Preferences are a user's settings (03 §4, 04 §6).
type Preferences struct {
	TextSize   string `json:"text_size,omitempty"`   // normal | large | larger
	UILanguage string `json:"ui_language,omitempty"` // en | id | "" (church default)
}

// User is a platform-wide account (SPEC §7).
type User struct {
	ID           UserID
	Name         string
	Email        string // "" when none
	Phone        string // "" when none
	PasswordHash string
	Preferences  Preferences
	LastSeenAt   time.Time // zero when never seen
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is a login session; only the token hash is stored (03 §4).
type Session struct {
	TokenHash  string
	UserID     UserID
	UserAgent  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// SessionExtendAfter is how stale last_seen_at must be before a request extends the session.
const SessionExtendAfter = time.Hour

// SessionExpiry is min(lastUse + ttl, created + maxAge), truncated to µs (03 §4, 02 §4).
func SessionExpiry(created, lastUse time.Time, ttl, maxAge time.Duration) time.Time {
	exp := lastUse.Add(ttl)
	if limit := created.Add(maxAge); limit.Before(exp) {
		exp = limit
	}
	return exp.Truncate(time.Microsecond)
}

// TruncateUserAgent keeps the first 200 characters.
func TruncateUserAgent(ua string) string {
	if r := []rune(ua); len(r) > 200 {
		return string(r[:200])
	}
	return ua
}
