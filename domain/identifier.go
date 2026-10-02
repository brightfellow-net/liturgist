// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// IdentifierKind says whether an identifier is an email address or a phone number.
type IdentifierKind int

// Identifier kinds.
const (
	Email IdentifierKind = iota + 1
	Phone
)

// Identifier is a normalised login identifier (03 §2). Create it only with ParseIdentifier.
type Identifier struct {
	Kind  IdentifierKind
	Value string // lower-case email, or E.164 phone (+6281234567890)
}

// ErrInvalidIdentifier maps to 422 invalid_identifier.
var ErrInvalidIdentifier = errors.New("invalid email or phone number")

// ParseIdentifier detects and normalises an email address or a phone number.
func ParseIdentifier(input string) (Identifier, error) {
	s := strings.TrimSpace(input)
	if strings.Contains(s, "@") {
		return parseEmail(s)
	}
	return parsePhone(s)
}

func parseEmail(s string) (Identifier, error) {
	v := strings.ToLower(s)
	if len(v) > 254 || strings.Count(v, "@") != 1 {
		return Identifier{}, ErrInvalidIdentifier
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Name != "" || addr.Address != v {
		return Identifier{}, ErrInvalidIdentifier // rejects display names and "<…>" forms
	}
	host := v[strings.IndexByte(v, '@')+1:]
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return Identifier{}, ErrInvalidIdentifier
	}
	return Identifier{Kind: Email, Value: v}, nil
}

func parsePhone(s string) (Identifier, error) {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" -().", r) {
			return -1
		}
		return r
	}, s)
	// Only digits and an optional leading "+": the library would otherwise turn
	// letters into keypad digits ("abc" → "222").
	if !isPhoneText(cleaned) {
		return Identifier{}, ErrInvalidIdentifier
	}
	num, err := phonenumbers.Parse(cleaned, "ID")
	if err != nil || !phonenumbers.IsValidNumber(num) {
		return Identifier{}, ErrInvalidIdentifier
	}
	return Identifier{Kind: Phone, Value: phonenumbers.Format(num, phonenumbers.E164)}, nil
}

func isPhoneText(s string) bool {
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
