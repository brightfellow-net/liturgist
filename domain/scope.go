// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"slices"
	"strings"
)

// Scope is one fixed capability; the only permission vocabulary in code (03 §8).
type Scope string

// Scopes (SPEC §4).
const (
	ScopeChurchSettings Scope = "church.settings"
	ScopeMembersView    Scope = "members.view"
	ScopeMembersManage  Scope = "members.manage"
	ScopeRolesManage    Scope = "roles.manage"
	ScopeLibraryEdit    Scope = "library.edit"
	ScopeTemplatesEdit  Scope = "templates.edit"
	ScopeLiturgyEdit    Scope = "liturgy.edit"
	ScopeLiturgyComment Scope = "liturgy.comment"
	ScopeLiturgyApprove Scope = "liturgy.approve"
	ScopeLiturgyManage  Scope = "liturgy.manage"
)

// AllScopes lists every scope in display order.
var AllScopes = []Scope{
	ScopeChurchSettings, ScopeMembersView, ScopeMembersManage, ScopeRolesManage, ScopeLibraryEdit,
	ScopeTemplatesEdit, ScopeLiturgyEdit, ScopeLiturgyComment, ScopeLiturgyApprove, ScopeLiturgyManage,
}

// ValidScope reports whether s is a known scope.
func ValidScope(s Scope) bool { return slices.Contains(AllScopes, s) }

// scopeDescriptions are shown in the role editor (GET /scopes), per UI language.
var scopeDescriptions = map[string]map[Scope]string{
	"en": {
		ScopeChurchSettings: "Change church settings",
		ScopeMembersView:    "See the member list with contact details",
		ScopeMembersManage:  "Invite and remove members; create password-reset links",
		ScopeRolesManage:    "Create, edit and delete roles; assign roles to members",
		ScopeLibraryEdit:    "Maintain songs and readings; imports",
		ScopeTemplatesEdit:  "Maintain templates and regular services",
		ScopeLiturgyEdit:    "Create and edit liturgies, assign the team, submit for review",
		ScopeLiturgyComment: "Comment on liturgy items; resolve comments",
		ScopeLiturgyApprove: "Approve, request changes, publish, reopen",
		ScopeLiturgyManage:  "Archive and unarchive published liturgies; delete unpublished liturgies",
	},
	"id": {
		ScopeChurchSettings: "Mengubah pengaturan gereja",
		ScopeMembersView:    "Melihat daftar anggota beserta kontaknya",
		ScopeMembersManage:  "Mengundang dan mengeluarkan anggota; membuat tautan atur ulang kata sandi",
		ScopeRolesManage:    "Membuat, mengubah dan menghapus peran; memberikan peran kepada anggota",
		ScopeLibraryEdit:    "Mengelola lagu dan bacaan; impor",
		ScopeTemplatesEdit:  "Mengelola templat dan ibadah rutin",
		ScopeLiturgyEdit:    "Membuat dan mengubah liturgi, menugaskan tim, mengajukan untuk ditinjau",
		ScopeLiturgyComment: "Mengomentari butir liturgi; menyelesaikan komentar",
		ScopeLiturgyApprove: "Menyetujui, meminta perubahan, menerbitkan, membuka kembali",
		ScopeLiturgyManage:  "Mengarsipkan dan membatalkan arsip liturgi terbit; menghapus liturgi yang belum terbit",
	},
}

// ScopeDescription returns the description in lang ("en" | "id"; anything else → en).
func ScopeDescription(s Scope, lang string) string {
	if d, ok := scopeDescriptions[lang]; ok {
		return d[s]
	}
	return scopeDescriptions["en"][s]
}

// ScopeSet is a set of scopes.
type ScopeSet map[Scope]bool

// NewScopeSet builds a set from scopes.
func NewScopeSet(scopes ...Scope) ScopeSet {
	s := ScopeSet{}
	for _, sc := range scopes {
		s[sc] = true
	}
	return s
}

// Has reports whether the set holds s.
func (s ScopeSet) Has(sc Scope) bool { return s[sc] }

// Add adds every scope of o.
func (s ScopeSet) Add(o ScopeSet) {
	for sc := range o {
		s[sc] = true
	}
}

// Missing returns the scopes of want that s lacks, sorted.
func (s ScopeSet) Missing(want ScopeSet) []Scope {
	var m []Scope
	for sc := range want {
		if !s[sc] {
			m = append(m, sc)
		}
	}
	slices.Sort(m)
	return m
}

// Sorted returns the scopes in AllScopes order.
func (s ScopeSet) Sorted() []Scope {
	out := make([]Scope, 0, len(s))
	for _, sc := range AllScopes {
		if s[sc] {
			out = append(out, sc)
		}
	}
	return out
}

// CanAdminister reports whether the set holds both scopes needed to manage
// the church's people: the lock-out rule keeps one such member (03 §8 rule 1).
func (s ScopeSet) CanAdminister() bool { return s[ScopeRolesManage] && s[ScopeMembersManage] }

// RoleOrigin marks a ready-made role, kept when renamed (schema roles.origin).
type RoleOrigin string

// Ready-made role origins.
const (
	OriginChurchAdmin RoleOrigin = "church_admin"
	OriginLiturgist   RoleOrigin = "liturgist"
	OriginEditor      RoleOrigin = "editor"
)

// ReadyMadeRole is the default definition of a ready-made role (03 §8).
type ReadyMadeRole struct {
	Origin RoleOrigin
	Names  map[string]string // by UI language
	Scopes []Scope
}

// ReadyMadeRoles are created for every new church.
var ReadyMadeRoles = []ReadyMadeRole{
	// Church admin holds every scope, so it can give every role (03 §8 rule 2).
	{OriginChurchAdmin, map[string]string{"en": "Church admin", "id": "Admin gereja"}, AllScopes},
	{OriginLiturgist, map[string]string{"en": "Liturgist", "id": "Liturgis"},
		[]Scope{ScopeMembersView, ScopeTemplatesEdit, ScopeLiturgyEdit, ScopeLiturgyComment, ScopeLiturgyApprove, ScopeLiturgyManage}},
	{OriginEditor, map[string]string{"en": "Editor", "id": "Editor"},
		[]Scope{ScopeMembersView, ScopeLibraryEdit, ScopeLiturgyEdit, ScopeLiturgyComment}},
}

// ReadyMade returns the definition for origin.
func ReadyMade(o RoleOrigin) (ReadyMadeRole, bool) {
	for _, r := range ReadyMadeRoles {
		if r.Origin == o {
			return r, true
		}
	}
	return ReadyMadeRole{}, false
}

// Name returns the role's default name in lang (en if unknown).
func (r ReadyMadeRole) Name(lang string) string {
	if n, ok := r.Names[lang]; ok {
		return n
	}
	return r.Names["en"]
}

// RoleNameKey is the case-insensitive uniqueness key of a role name.
func RoleNameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
