// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// TC-A-010
func TestEffectiveScopes(t *testing.T) {
	roles := map[RoleID]Role{
		"ed":  {ID: "ed", Scopes: NewScopeSet(ScopeMembersView, ScopeLibraryEdit)},
		"set": {ID: "set", Scopes: NewScopeSet(ScopeChurchSettings)},
	}
	got := EffectiveScopes([]RoleID{"ed", "set", "deleted"}, roles)
	if !slices.Equal(got.Sorted(), []Scope{ScopeChurchSettings, ScopeMembersView, ScopeLibraryEdit}) {
		t.Errorf("union: %v", got.Sorted())
	}
	if len(EffectiveScopes(nil, roles)) != 0 {
		t.Error("no roles must give no scopes")
	}
}

// TC-A-007 (rule), TC-A-009 (missing scopes)
func TestLockoutAndMissing(t *testing.T) {
	roles := map[RoleID]Role{
		"admin": {ID: "admin", Scopes: NewScopeSet(ScopeRolesManage, ScopeMembersManage)},
		"roles": {ID: "roles", Scopes: NewScopeSet(ScopeRolesManage)},
		"mem":   {ID: "mem", Scopes: NewScopeSet(ScopeMembersManage)},
	}
	cases := []struct {
		name    string
		members []Membership
		want    bool
	}{
		{"one admin", []Membership{{RoleIDs: []RoleID{"admin"}}}, true},
		{"both scopes through two roles", []Membership{{RoleIDs: []RoleID{"roles", "mem"}}}, true},
		{"scopes split over two members", []Membership{{RoleIDs: []RoleID{"roles"}}, {RoleIDs: []RoleID{"mem"}}}, false},
		{"nobody", nil, false},
	}
	for _, c := range cases {
		if got := SomeoneCanAdminister(c.members, roles); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	lit, _ := ReadyMade(OriginLiturgist)
	admin, _ := ReadyMade(OriginChurchAdmin)
	missing := NewScopeSet(admin.Scopes...).Missing(NewScopeSet(lit.Scopes...))
	if !slices.Equal(missing, []Scope{ScopeLiturgyApprove, ScopeLiturgyComment, ScopeLiturgyEdit}) {
		t.Errorf("admin lacks: %v", missing)
	}
}

func TestReadyMadeRoles(t *testing.T) {
	if len(ReadyMadeRoles) != 3 {
		t.Fatal("three ready-made roles")
	}
	for _, r := range ReadyMadeRoles {
		for _, s := range r.Scopes {
			if !ValidScope(s) {
				t.Errorf("%s: unknown scope %s", r.Origin, s)
			}
		}
		if r.Name("id") == "" || r.Name("en") == "" || r.Name("fr") != r.Name("en") {
			t.Errorf("%s names: %v", r.Origin, r.Names)
		}
	}
	for _, s := range AllScopes {
		if ScopeDescription(s, "en") == "" || ScopeDescription(s, "id") == "" {
			t.Errorf("%s: missing description", s)
		}
	}
	if RoleNameKey("  Church Admin ") != "church admin" {
		t.Error("name key")
	}
}

func TestChurchSettingsKeepUnknownKeys(t *testing.T) {
	var s ChurchSettings
	if err := json.Unmarshal([]byte(`{"key_display":"letter","feedback_url":5,"theme":{"dark":true}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.KeyDisplay != "letter" || s.FeedbackURL != "" || string(s.Extra["theme"]) != `{"dark":true}` {
		t.Errorf("read: %+v", s)
	}
	s.PrivacyContact = "office"
	b, _ := json.Marshal(s)
	if string(b) != `{"key_display":"letter","privacy_contact":"office","theme":{"dark":true}}` {
		t.Errorf("written: %s", b)
	}
}

func TestValidateChurch(t *testing.T) {
	ok := Church{Name: " GKY ", DefaultUILanguage: "id", DefaultLanguage: "zh-Hans", Settings: ChurchSettings{KeyDisplay: "do"}}
	if err := ValidateChurch(&ok, ""); err != nil || ok.Name != "GKY" {
		t.Errorf("valid: %v %q", err, ok.Name)
	}
	for field, mutate := range map[string]func(c *Church){
		"name":                func(c *Church) { c.Name = strings.Repeat("x", 121) },
		"default_ui_language": func(c *Church) { c.DefaultUILanguage = "zh-Hans" },
		"default_language":    func(c *Church) { c.DefaultLanguage = "fr" },
		"key_display":         func(c *Church) { c.Settings.KeyDisplay = "C" },
		"feedback_url":        func(c *Church) { c.Settings.FeedbackURL = "http://x.org" },
		"privacy_contact":     func(c *Church) { c.Settings.PrivacyContact = strings.Repeat("x", 501) },
	} {
		c := ok
		mutate(&c)
		var e *InvalidInputError
		if err := ValidateChurch(&c, "church."); !errors.As(err, &e) || e.Field != "church."+field {
			t.Errorf("%s: %v", field, err)
		}
	}
}
