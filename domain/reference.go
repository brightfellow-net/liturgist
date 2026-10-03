// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxReferenceInput is the longest reference text the parser accepts, in
// characters (07 §2.1).
const MaxReferenceInput = 100

// Limits of chapter and verse numbers (07 §2.1). The parser has no
// versification data: it checks structure only.
const (
	maxChapter = 150
	maxVerse   = 176
)

// ReferenceReason says why a reference could not be understood (07 §2.2).
type ReferenceReason string

// Reasons of ReferenceError.
const (
	RefEmpty          ReferenceReason = "empty"
	RefTooLong        ReferenceReason = "too_long"
	RefUnknownBook    ReferenceReason = "unknown_book"
	RefMissingChapter ReferenceReason = "missing_chapter"
	RefBadNumber      ReferenceReason = "bad_number"
	RefBadRange       ReferenceReason = "bad_range"
	RefUnsupported    ReferenceReason = "unsupported"
)

// ReferenceError maps to 422 invalid_reference with the reason.
type ReferenceError struct{ Reason ReferenceReason }

func (e *ReferenceError) Error() string { return "invalid reference: " + string(e.Reason) }

func refErr(r ReferenceReason) error { return &ReferenceError{Reason: r} }

// Reference is a Bible reference in standard form: a USFM book code and a
// passage, e.g. JHN 3:16-21 (07 §2).
type Reference struct {
	Book    string // USFM code
	Passage string // canonical text after the code: "3:16-21", "23-25", "5:3,5-7", "1:1-2:3"
	Chapter int    // first chapter of the passage
	Verse   int    // first verse; 0 for whole chapters
}

// String is the standard form, the key readings are stored under.
func (r Reference) String() string { return r.Book + " " + r.Passage }

// Canonical is the reference with the book's name in a language ("id" only
// in step 2): "Yohanes 3:16-21".
func (r Reference) Canonical(string) string {
	b, ok := bookByCode[r.Book]
	if !ok {
		return r.String()
	}
	return b.Name + " " + r.Passage
}

// BookOrdinal is the book's place in the canonical order (0 when unknown).
func (r Reference) BookOrdinal() int {
	if b, ok := bookByCode[r.Book]; ok {
		return b.Ordinal
	}
	return 0
}

// CollapseSpaces trims s and turns every run of white space into one space
// (the display form of a typed reference, 07 §2.1).
func CollapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// ParseReference understands a typed reference (07 §2.1). The length limit is
// checked first, before anything else.
func ParseReference(input string) (Reference, error) {
	if utf8.RuneCountInString(input) > MaxReferenceInput {
		return Reference{}, refErr(RefTooLong)
	}
	text := CollapseSpaces(input)
	if text == "" {
		return Reference{}, refErr(RefEmpty)
	}
	book, rest, ok := splitBook(text)
	if !ok {
		return Reference{}, refErr(RefUnknownBook)
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return Reference{}, refErr(RefMissingChapter)
	}
	toks, err := tokenize(rest)
	if err != nil {
		return Reference{}, err
	}
	ref := Reference{Book: book.Code}
	if err := parsePassage(&ref, toks, oneChapterBooks[book.Code]); err != nil {
		return Reference{}, err
	}
	return ref, nil
}

// splitBook separates the book from the passage. A book name is made of an
// optional ordinal (1, 2, 3 or I, II, III), letters, dots, hyphens and spaces;
// the passage starts at the first digit or punctuation mark after it.
func splitBook(text string) (*Book, string, bool) {
	runes := []rune(text)
	i := 0
	ordinal := ""
	switch {
	case runes[0] >= '1' && runes[0] <= '3':
		j := 1
		for j < len(runes) && runes[j] == ' ' {
			j++
		}
		if j >= len(runes) || !unicode.IsLetter(runes[j]) {
			return nil, "", false
		}
		ordinal, i = string(runes[0]), j
	default:
		if o, n := romanOrdinal(runes); n > 0 {
			ordinal, i = o, n
		}
	}
	start := i
	for i < len(runes) && (unicode.IsLetter(runes[i]) || runes[i] == '.' || runes[i] == ' ' || runes[i] == '-' || runes[i] == ' ') {
		i++
	}
	name := ordinal + string(runes[start:i])
	b, ok := bookByAlias[bookKey(name)]
	if !ok {
		return nil, "", false
	}
	return b, string(runes[i:]), true
}

// romanOrdinal recognises a leading I, II or III followed by a space or dot
// ("II Korintus"); it returns the digit and the number of runes used.
func romanOrdinal(runes []rune) (string, int) {
	n := 0
	for n < len(runes) && n < 3 && (runes[n] == 'I' || runes[n] == 'i') {
		n++
	}
	if n == 0 || n >= len(runes) || (runes[n] != ' ' && runes[n] != '.') {
		return "", 0
	}
	for n < len(runes) && (runes[n] == ' ' || runes[n] == '.') {
		n++
	}
	if n >= len(runes) || !unicode.IsLetter(runes[n]) {
		return "", 0
	}
	count := 0
	for _, r := range runes {
		if r != 'I' && r != 'i' {
			break
		}
		count++
	}
	return strconv.Itoa(count), n
}

type tokKind int

const (
	tNum tokKind = iota
	tColon
	tDash
	tComma
)

type token struct {
	kind tokKind
	num  int
}

// tokenize splits the passage into numbers and separators. Spaces are only
// allowed around separators: "3 16" is not "316".
func tokenize(s string) ([]token, error) {
	var toks []token
	runes := []rune(s)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == ' ':
			i++
		case r >= '0' && r <= '9':
			if len(toks) > 0 && toks[len(toks)-1].kind == tNum {
				return nil, refErr(RefBadNumber)
			}
			n := 0
			for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
				if n = n*10 + int(runes[i]-'0'); n > 100000 {
					return nil, refErr(RefBadNumber)
				}
				i++
			}
			toks = append(toks, token{tNum, n})
		case r == ':':
			toks, i = append(toks, token{kind: tColon}), i+1
		case r == '-' || r == '–' || r == '—':
			toks, i = append(toks, token{kind: tDash}), i+1
		case r == ',':
			toks, i = append(toks, token{kind: tComma}), i+1
		case r == ';' || unicode.IsLetter(r):
			return nil, refErr(RefUnsupported)
		default: // "3.16" and everything else
			return nil, refErr(RefBadNumber)
		}
	}
	return toks, nil
}

// parser reads tokens left to right.
type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.pos], true
}

func (p *parser) accept(k tokKind) bool {
	if t, ok := p.peek(); ok && t.kind == k {
		p.pos++
		return true
	}
	return false
}

func (p *parser) number() (int, error) {
	t, ok := p.peek()
	if !ok || t.kind != tNum {
		return 0, refErr(RefBadNumber)
	}
	p.pos++
	return t.num, nil
}

func chapterOK(n int) error {
	if n < 1 || n > maxChapter {
		return refErr(RefBadNumber)
	}
	return nil
}

func verseOK(n int) error {
	if n < 1 || n > maxVerse {
		return refErr(RefBadNumber)
	}
	return nil
}

// parsePassage fills ref from the tokens after the book (07 §2.1 shapes).
func parsePassage(ref *Reference, toks []token, oneChapter bool) error {
	p := &parser{toks: toks}
	first, err := p.number()
	if err != nil {
		return err
	}
	if p.accept(tColon) {
		if err := chapterOK(first); err != nil {
			return err
		}
		if oneChapter && first != 1 {
			return refErr(RefBadNumber)
		}
		return parseVerses(ref, p, first)
	}
	if oneChapter { // "Yud 3-5": verses of chapter 1
		p.pos = 0
		return parseVerses(ref, p, 1)
	}
	if err := chapterOK(first); err != nil {
		return err
	}
	ref.Chapter, ref.Passage = first, strconv.Itoa(first)
	if p.accept(tDash) {
		last, err := p.number()
		if err != nil {
			return err
		}
		if err := chapterOK(last); err != nil {
			return err
		}
		if last < first {
			return refErr(RefBadRange)
		}
		if last > first {
			ref.Passage = strconv.Itoa(first) + "-" + strconv.Itoa(last)
		}
	}
	if _, more := p.peek(); more {
		return refErr(RefUnsupported) // "Yoh 3,4" and "Yoh 3-4:5"
	}
	return nil
}

// parseVerses reads the verse ranges of one chapter, or one range across
// chapters ("1:1-2:3"). With the chapter already known, the next token is
// the first verse (after a ":").
func parseVerses(ref *Reference, p *parser, chapter int) error {
	ref.Chapter = chapter
	var parts []string
	prevEnd := 0
	for first := true; ; first = false {
		start, err := p.number()
		if err != nil {
			return err
		}
		if err := verseOK(start); err != nil {
			return err
		}
		if first {
			ref.Verse = start
		}
		if start <= prevEnd {
			return refErr(RefBadRange)
		}
		end := start
		if p.accept(tDash) {
			n, err := p.number()
			if err != nil {
				return err
			}
			if p.accept(tColon) { // across chapters
				if !first {
					return refErr(RefUnsupported)
				}
				return acrossChapters(ref, p, chapter, start, n)
			}
			if err := verseOK(n); err != nil {
				return err
			}
			if n < start {
				return refErr(RefBadRange)
			}
			end = n
		}
		if end > start {
			parts = append(parts, strconv.Itoa(start)+"-"+strconv.Itoa(end))
		} else {
			parts = append(parts, strconv.Itoa(start))
		}
		prevEnd = end
		if !p.accept(tComma) {
			break
		}
	}
	if _, more := p.peek(); more {
		return refErr(RefUnsupported)
	}
	ref.Passage = strconv.Itoa(chapter) + ":" + strings.Join(parts, ",")
	return nil
}

// acrossChapters finishes "C:V-C2:V2": endChapter was read before the ":".
func acrossChapters(ref *Reference, p *parser, chapter, start, endChapter int) error {
	endVerse, err := p.number()
	if err != nil {
		return err
	}
	if _, more := p.peek(); more {
		return refErr(RefUnsupported)
	}
	if err := chapterOK(endChapter); err != nil {
		return err
	}
	if err := verseOK(endVerse); err != nil {
		return err
	}
	switch {
	case endChapter < chapter:
		return refErr(RefBadRange)
	case endChapter == chapter: // "1:1-1:3" is "1:1-3"
		if endVerse < start {
			return refErr(RefBadRange)
		}
		text := strconv.Itoa(start)
		if endVerse > start {
			text += "-" + strconv.Itoa(endVerse)
		}
		ref.Passage = strconv.Itoa(chapter) + ":" + text
	default:
		ref.Passage = strconv.Itoa(chapter) + ":" + strconv.Itoa(start) + "-" + strconv.Itoa(endChapter) + ":" + strconv.Itoa(endVerse)
	}
	return nil
}
