// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// JSON shapes of the church, member, role and invite resources (04 §6).

// ChurchView is the church.
type ChurchView struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	DefaultUILanguage      string            `json:"default_ui_language" enum:"en,id"`
	DefaultLanguage        string            `json:"default_language" enum:"id,en,zh-Hans,zh-Hant"`
	DefaultTranslationCode string            `json:"default_translation_code"`
	TimeZone               string            `json:"time_zone"`
	KeyDisplay             string            `json:"key_display" enum:"do,letter"`
	FeedbackURL            *string           `json:"feedback_url"`
	PrivacyContact         *string           `json:"privacy_contact"`
	ShowCredits            bool              `json:"show_credits"`
	LicenceFooter          string            `json:"licence_footer"`
	Print                  PrintDefaultsView `json:"print"`
	Actions                app.ChurchActions `json:"actions"`
}

// PrintDefaultsView is the church's print options with their defaults filled in (13 §6).
type PrintDefaultsView struct {
	Paper       string `json:"paper" enum:"a4,f4"`
	Lyrics      string `json:"lyrics" enum:"full,first_lines"`
	Readings    bool   `json:"readings"`
	Assignments bool   `json:"assignments"`
	Keys        bool   `json:"keys"`
	Notes       bool   `json:"notes"`
	Size        string `json:"size" enum:"normal,large"`
}

func printDefaultsView(p domain.PrintDefaults) PrintDefaultsView {
	return PrintDefaultsView{Paper: p.Paper, Lyrics: p.Lyrics, Readings: p.Readings, Assignments: p.Assignments, Keys: p.Keys, Notes: p.Notes, Size: p.Size}
}

func churchView(r app.ChurchResult) ChurchView {
	c := r.Church
	return ChurchView{ID: string(c.ID), Name: c.Name, DefaultUILanguage: c.DefaultUILanguage,
		DefaultLanguage: c.DefaultLanguage, DefaultTranslationCode: r.TranslationCode, TimeZone: c.TimeZone,
		KeyDisplay: c.Settings.KeyDisplay, FeedbackURL: optional(c.Settings.FeedbackURL),
		PrivacyContact: optional(c.Settings.PrivacyContact), ShowCredits: c.Settings.CreditsShown(),
		LicenceFooter: c.Settings.LicenceFooter, Print: printDefaultsView(c.Settings.PrintOrDefault()), Actions: r.Actions}
}

// RoleRef names a role.
type RoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func roleRefs(roles []domain.Role) []RoleRef {
	out := make([]RoleRef, len(roles))
	for i, r := range roles {
		out[i] = RoleRef{ID: string(r.ID), Name: r.Name}
	}
	return out
}

// ResetView is a member's latest reset link (never the link).
type ResetView struct {
	CreatedByName *string    `json:"created_by_name" doc:"null: created on the server's command line"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at"`
}

// MemberView is one member.
type MemberView struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Name      string            `json:"name"`
	Email     *string           `json:"email"`
	Phone     *string           `json:"phone"`
	Roles     []RoleRef         `json:"roles"`
	JoinedAt  time.Time         `json:"joined_at"`
	LastReset *ResetView        `json:"last_reset,omitempty" doc:"Only for viewers with members.manage"`
	Actions   app.MemberActions `json:"actions"`
}

func memberView(v app.MemberView) MemberView {
	u := v.Member.User
	out := MemberView{ID: string(v.Member.ID), UserID: string(u.ID), Name: u.Name, Email: optional(u.Email),
		Phone: optional(u.Phone), Roles: roleRefs(v.Roles), JoinedAt: v.Member.CreatedAt, Actions: v.Actions}
	if r := v.LastReset; r != nil {
		out.LastReset = &ResetView{CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, UsedAt: r.UsedAt}
	}
	return out
}

// MembershipView is the logged-in user's membership in GET /me.
type MembershipView struct {
	ID      string            `json:"id"`
	Roles   []RoleRef         `json:"roles"`
	Scopes  []string          `json:"scopes"`
	Actions app.MemberActions `json:"actions"`
}

// RoleView is one role.
type RoleView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Origin      *string         `json:"origin" doc:"Ready-made role this started as; null for custom roles"`
	Scopes      []string        `json:"scopes"`
	MemberCount int             `json:"member_count"`
	Actions     app.RoleActions `json:"actions"`
}

func roleView(v app.RoleView) RoleView {
	return RoleView{ID: string(v.Role.ID), Name: v.Role.Name, Description: v.Role.Description,
		Origin: optional(string(v.Role.Origin)), Scopes: scopeStrings(v.Role.Scopes),
		MemberCount: v.MemberCount, Actions: v.Actions}
}

func scopeStrings(s domain.ScopeSet) []string {
	out := []string{}
	for _, sc := range s.Sorted() {
		out = append(out, string(sc))
	}
	return out
}

// InviteView is one invite (never the link).
type InviteView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Email         *string           `json:"email"`
	Phone         *string           `json:"phone"`
	Roles         []RoleRef         `json:"roles"`
	Status        string            `json:"status" enum:"pending,expired"`
	CreatedByName *string           `json:"created_by_name"`
	CreatedAt     time.Time         `json:"created_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	Actions       app.InviteActions `json:"actions"`
}

func inviteView(v app.InviteView) InviteView {
	i := v.Invite
	return InviteView{ID: string(i.ID), Name: i.Name, Email: optional(i.Email), Phone: optional(i.Phone),
		Roles: roleRefs(v.Roles), Status: string(v.Status), CreatedByName: v.CreatedByName,
		CreatedAt: i.CreatedAt, ExpiresAt: i.ExpiresAt, Actions: v.Actions}
}

// LinkView is a one-time link.
type LinkView struct {
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
