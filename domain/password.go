// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strings"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain/commonpw"
	"golang.org/x/text/unicode/norm"
)

// Password length limits in Unicode code points, after normalisation (03 §3).
const (
	MinPasswordLength = 10
	MaxPasswordLength = 128
)

// PasswordProblem is the reason of a 422 weak_password error (03 §3).
type PasswordProblem string

// Password problems.
const (
	PasswordTooShort        PasswordProblem = "too_short"
	PasswordTooLong         PasswordProblem = "too_long"
	PasswordCommon          PasswordProblem = "common"
	PasswordMatchesIdentity PasswordProblem = "matches_identity"
)

// WeakPasswordError reports why a password was rejected.
type WeakPasswordError struct{ Reason PasswordProblem }

func (e *WeakPasswordError) Error() string { return "weak password: " + string(e.Reason) }

// NormalizePassword applies Unicode NFKC. Spaces are kept (never trimmed).
// Hashing and verifying always use the normalised form.
func NormalizePassword(pw string) string { return norm.NFKC.String(pw) }

// PasswordIdentity is what a password must not equal.
type PasswordIdentity struct {
	Name, Email, Phone, ChurchName string
}

// CheckPassword applies the step-1 password rules (03 §3).
func CheckPassword(pw string, id PasswordIdentity) error {
	n := NormalizePassword(pw)
	switch l := utf8.RuneCountInString(n); {
	case l < MinPasswordLength:
		return &WeakPasswordError{PasswordTooShort}
	case l > MaxPasswordLength:
		return &WeakPasswordError{PasswordTooLong}
	}
	lower := strings.ToLower(n)
	squashed := squash(lower)
	if commonpw.Contains(lower) || commonpw.Contains(squashed) {
		return &WeakPasswordError{PasswordCommon}
	}
	for _, c := range identityForms(id) {
		if c != "" && squashed == c {
			return &WeakPasswordError{PasswordMatchesIdentity}
		}
	}
	return nil
}

// identityForms returns the person's details lower-cased without spaces:
// email, its local part, phone with +62 / 62 / 0 prefixes, name, church name.
func identityForms(id PasswordIdentity) []string {
	forms := []string{squash(strings.ToLower(id.Name)), squash(strings.ToLower(id.ChurchName))}
	if e := strings.ToLower(id.Email); e != "" {
		forms = append(forms, e, strings.SplitN(e, "@", 2)[0])
	}
	if p := id.Phone; strings.HasPrefix(p, "+62") {
		rest := p[3:]
		forms = append(forms, p, "62"+rest, "0"+rest)
	} else if p != "" {
		forms = append(forms, p, strings.TrimPrefix(p, "+"))
	}
	return forms
}

func squash(s string) string { return strings.ReplaceAll(s, " ", "") }
