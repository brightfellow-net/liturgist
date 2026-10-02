// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package commonpw holds the common-password lists (03 §3): the NCSC top
// 100,000 (from SecLists, MIT licence; see LICENSE-SecLists) and a curated
// Indonesian / church list. Only entries of at least 10 characters are kept,
// because shorter passwords are rejected by length anyway.
package commonpw

import (
	_ "embed" // password lists
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed ncsc-100k.txt
var ncsc string

//go:embed id-church.txt
var idChurch string

// Suffixes people commonly add to a word.
var suffixes = []string{"", "1", "12", "123", "1234", "12345", "123456", "!", "2024", "2025", "2026", "2027"}

const minLength = 10

var list = sync.OnceValue(func() []string {
	var out []string
	add := func(s string) {
		if s = strings.ToLower(strings.TrimSpace(s)); utf8.RuneCountInString(s) >= minLength {
			out = append(out, s)
		}
	}
	for _, line := range strings.Split(ncsc, "\n") {
		add(line)
	}
	for _, word := range strings.Split(idChurch, "\n") {
		word = strings.TrimSpace(word)
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		for _, sfx := range suffixes {
			add(word + sfx)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
})

// Contains reports whether the lower-cased password s is on a list.
func Contains(s string) bool {
	_, found := slices.BinarySearch(list(), s)
	return found
}
