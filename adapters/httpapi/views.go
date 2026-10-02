// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// JSON shapes of the church, member and role resources (04 §6).

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
	Actions                app.ChurchActions `json:"actions"`
}

func churchView(r app.ChurchResult) ChurchView {
	c := r.Church
	return ChurchView{ID: string(c.ID), Name: c.Name, DefaultUILanguage: c.DefaultUILanguage,
		DefaultLanguage: c.DefaultLanguage, DefaultTranslationCode: r.TranslationCode, TimeZone: c.TimeZone,
		KeyDisplay: c.Settings.KeyDisplay, FeedbackURL: optional(c.Settings.FeedbackURL),
		PrivacyContact: optional(c.Settings.PrivacyContact), Actions: r.Actions}
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

// MemberView is one member.
type MemberView struct {
	ID       string            `json:"id"`
	UserID   string            `json:"user_id"`
	Name     string            `json:"name"`
	Email    *string           `json:"email"`
	Phone    *string           `json:"phone"`
	Roles    []RoleRef         `json:"roles"`
	JoinedAt time.Time         `json:"joined_at"`
	Actions  app.MemberActions `json:"actions"`
}

func memberView(v app.MemberView) MemberView {
	u := v.Member.User
	out := MemberView{ID: string(v.Member.ID), UserID: string(u.ID), Name: u.Name, Email: optional(u.Email),
		Phone: optional(u.Phone), Roles: roleRefs(v.Roles), JoinedAt: v.Member.CreatedAt, Actions: v.Actions}
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

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
