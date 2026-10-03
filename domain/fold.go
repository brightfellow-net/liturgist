// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold is the only text normalisation for search (06 §5.1), applied to stored
// text and to queries in this order: NFKC, lower-case, NFD, drop combining
// marks (Mn), replace every run of characters that are not letters or digits
// with one space, trim. The result is not recomposed.
func Fold(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	var b strings.Builder
	pendingSpace := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.Is(unicode.Nd, r):
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		default:
			pendingSpace = true
		}
	}
	return b.String()
}

// FoldZh is Fold with all spaces removed, used for Chinese songs and queries
// against them: Chinese has no word boundaries (06 §5.1).
func FoldZh(s string) string { return strings.ReplaceAll(Fold(s), " ", "") }

// IsChinese reports whether a content language is Chinese.
func IsChinese(language string) bool { return language == "zh-Hans" || language == "zh-Hant" }

// FoldFor folds s the way songs of the given language are indexed.
func FoldFor(language, s string) string {
	if IsChinese(language) {
		return FoldZh(s)
	}
	return Fold(s)
}

// HymnalKey is the canonical key of a hymnal source and number (06 §2.1):
// the folded source without spaces, ":", the folded number without spaces,
// e.g. "kj:12", "pkj:12a". It is empty when there is no number.
func HymnalKey(source, number string) string {
	n := strings.ReplaceAll(Fold(number), " ", "")
	if n == "" {
		return ""
	}
	return strings.ReplaceAll(Fold(source), " ", "") + ":" + n
}

// HymnalSourceKey is the folded source without spaces ("KJ" → "kj").
func HymnalSourceKey(source string) string { return strings.ReplaceAll(Fold(source), " ", "") }

var hymnalQueryRe = regexp.MustCompile(`^([a-z]+) ?([0-9]+[a-z]?)$`)

// ParseHymnalQuery recognises a query such as "kj 12", "PKJ12a" or "KJ. 12"
// and returns its hymnal key (06 §5.2).
func ParseHymnalQuery(q string) (key string, ok bool) {
	m := hymnalQueryRe.FindStringSubmatch(Fold(q))
	if m == nil {
		return "", false
	}
	return m[1] + ":" + m[2], true
}

// SearchTerms folds a query and splits it into terms (06 §5.2).
func SearchTerms(q string) []string { return strings.Fields(Fold(q)) }
