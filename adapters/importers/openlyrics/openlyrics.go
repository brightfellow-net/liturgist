// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package openlyrics reads OpenLyrics song files (OpenLP, 08 §4.2).
package openlyrics

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/brightfellow-net/liturgist/adapters/importers/paste"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// Limits of 08 §5.
const (
	maxTokens = 100_000
	maxDepth  = 32
)

var (
	keyRe  = regexp.MustCompile(`^[A-G][#b]?m?$`)
	ccliRe = regexp.MustCompile(`^[0-9]{1,12}$`)
	nameRe = regexp.MustCompile(`^([a-zA-Z]+)(\d*)`)
)

// Importer reads one song per file.
type Importer struct{}

type verse struct {
	name  string
	lines []string
}

// Parse implements app.Importer. Entities and DTDs are never resolved: a
// DOCTYPE is skipped, and an entity other than the five XML ones and numeric
// references is a syntax error.
func (Importer) Parse(ctx context.Context, r io.Reader, hint app.ImportHint) ([]app.ImportCandidate, error) {
	text, err := paste.ReadText(r)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = true
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "utf8") {
			return in, nil
		}
		return nil, errors.New("unsupported encoding")
	}

	var (
		path    []string
		tokens  int
		titles  []string
		authors = map[string][]string{}
		props   = map[string]string{}
		order   string
		book    struct{ name, entry string }
		haveBk  bool
		verses  []*verse
		curV    *verse
		curLine *strings.Builder
		authTyp string
		sawRoot bool
	)
	in := func(parts ...string) bool {
		if len(path) != len(parts) {
			return false
		}
		for i, p := range parts {
			if path[i] != p {
				return false
			}
		}
		return true
	}
	inLines := func() bool { return curV != nil && len(path) >= 4 && path[2] == "verse" && path[3] == "lines" }
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &app.ImportUnreadableError{Reason: app.ImportNotXML}
		}
		if tokens++; tokens > maxTokens {
			return nil, &app.ImportUnreadableError{Reason: app.ImportTooComplex}
		}
		if tokens%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(path) == 0 && t.Name.Local != "song" {
				return nil, &app.ImportUnreadableError{Reason: app.ImportNotXML}
			}
			sawRoot = true
			path = append(path, t.Name.Local)
			if len(path) > maxDepth {
				return nil, &app.ImportUnreadableError{Reason: app.ImportTooComplex}
			}
			switch {
			case in("song", "properties", "authors", "author"):
				authTyp = attr(t, "type")
			case in("song", "properties", "songbooks", "songbook") && !haveBk:
				book.name, book.entry, haveBk = attr(t, "name"), attr(t, "entry"), true
			case in("song", "lyrics", "verse"):
				curV = &verse{name: attr(t, "name")}
				verses = append(verses, curV)
			case in("song", "lyrics", "verse", "lines"):
				curLine = &strings.Builder{}
			case inLines() && t.Name.Local == "br" && curLine != nil:
				curLine.WriteByte('\n')
			}
		case xml.EndElement:
			switch {
			case in("song", "lyrics", "verse", "lines") && curV != nil && curLine != nil:
				curV.lines = append(curV.lines, curLine.String())
				curLine = nil
			case in("song", "lyrics", "verse"):
				curV = nil
			}
			path = path[:len(path)-1]
		case xml.CharData:
			s := string(t)
			switch {
			case in("song", "properties", "titles", "title"):
				if v := strings.TrimSpace(s); v != "" {
					titles = append(titles, v)
				}
			case in("song", "properties", "authors", "author"):
				if v := strings.TrimSpace(s); v != "" {
					authors[authTyp] = append(authors[authTyp], v)
				}
			case in("song", "properties", "copyright"), in("song", "properties", "ccliNo"), in("song", "properties", "key"):
				props[path[2]] += s
			case in("song", "properties", "verseOrder"):
				order += s
			case inLines() && curLine != nil && !inComment(path):
				curLine.WriteString(s)
			}
		}
	}
	if !sawRoot {
		return nil, &app.ImportUnreadableError{Reason: app.ImportNotXML}
	}
	if len(titles) == 0 || len(verses) == 0 {
		return nil, &app.ImportUnreadableError{Reason: app.ImportNoSong}
	}

	d := domain.SongDraft{Language: hint.Language, Title: titles[0], AltTitles: []string{}, Sections: []domain.DraftSection{},
		DefaultArrangement: []int{}}
	var warns []string
	warn := func(code string) {
		if !contains(warns, code) {
			warns = append(warns, code)
		}
	}
	for _, t := range titles[1:] {
		if t != d.Title && !contains(d.AltTitles, t) {
			d.AltTitles = append(d.AltTitles, t)
		}
	}
	d.Lyricist = strings.Join(append(append([]string{}, authors[""]...), authors["words"]...), ", ")
	d.Composer = strings.Join(authors["music"], ", ")
	d.Translator = strings.Join(authors["translation"], ", ")
	d.CopyrightLine = strings.TrimSpace(props["copyright"])
	if v := strings.TrimSpace(props["ccliNo"]); v != "" {
		if ccliRe.MatchString(v) {
			d.CCLISongNumber = v
		} else {
			warn(domain.WarnCCLIIgnored)
		}
	}
	if v := strings.TrimSpace(props["key"]); v != "" {
		if keyRe.MatchString(v) {
			d.DefaultKey = v
		} else {
			warn(domain.WarnKeyIgnored)
		}
	}
	if haveBk {
		d.HymnalSource, d.HymnalNumber = strings.TrimSpace(book.name), strings.TrimSpace(book.entry)
		if d.HymnalSource == "" || d.HymnalNumber == "" {
			d.HymnalSource, d.HymnalNumber = "", ""
		}
	}

	// Sections: kind from the first letter of the name; verse numbers from its digits.
	items := make([]paste.Item, 0, len(verses))
	index := map[string]int{}
	for _, v := range verses {
		kind, number := verseKind(v.name)
		lines := cleanLines(v.lines)
		items = append(items, paste.Item{Labelled: true, Kind: kind, Number: number, NumberSet: number > 0, Text: lines})
		if _, dup := index[v.name]; !dup {
			index[v.name] = len(items) - 1
		}
		if kind != domain.SectionVerse && number > 0 {
			warn(domain.WarnNumberDropped)
		}
	}
	for _, it := range items {
		if it.Text == "" {
			continue
		}
		d.Sections = append(d.Sections, domain.DraftSection{Kind: it.Kind, Number: it.Number, Text: it.Text})
	}
	if len(d.Sections) == 0 {
		return nil, &app.ImportUnreadableError{Reason: app.ImportNoSong}
	}
	if len(d.Sections) > domain.MaxImportSections {
		return []app.ImportCandidate{{Reject: app.ImportTooManySections}}, nil
	}
	// Verse numbers must be unique and present: renumber the rest (08 §4.1 rule 6).
	sectionOf := map[int]int{} // item index → section index
	n := 0
	for i, it := range items {
		if it.Text != "" {
			sectionOf[i] = n
			n++
		}
	}
	renumber(d.Sections, warn)
	if fields := strings.Fields(order); len(fields) > 0 {
		for _, name := range fields {
			i, ok := index[name]
			s, has := sectionOf[i]
			if !ok || !has {
				d.DefaultArrangement = []int{}
				warn(domain.WarnArrangementIgnore)
				break
			}
			d.DefaultArrangement = append(d.DefaultArrangement, s)
		}
	}
	return []app.ImportCandidate{{Draft: d, Warnings: warns}}, nil
}

func inComment(path []string) bool {
	for _, p := range path {
		if p == "comment" {
			return true
		}
	}
	return false
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// verseKind maps an OpenLyrics verse name ("v1", "c", "p2", "b", "i", "e", "o").
func verseKind(name string) (domain.SectionKind, int) {
	m := nameRe.FindStringSubmatch(name)
	if m == nil {
		return domain.SectionOther, 0
	}
	n, _ := strconv.Atoi(m[2])
	switch strings.ToLower(m[1][:1]) {
	case "v":
		return domain.SectionVerse, n
	case "c":
		return domain.SectionChorus, n
	case "p":
		return domain.SectionPreChorus, n
	case "b":
		return domain.SectionBridge, n
	case "i":
		return domain.SectionIntro, n
	case "e":
		return domain.SectionEnding, n
	}
	return domain.SectionOther, n
}

// cleanLines joins the <lines> parts of a verse, trimming the indentation
// that pretty-printed XML puts around the text.
func cleanLines(parts []string) string {
	var out []string
	for _, p := range parts {
		for _, line := range strings.Split(p, "\n") {
			out = append(out, strings.TrimSpace(strings.Join(strings.Fields(line), " ")))
		}
	}
	return domain.NormalizeLyrics(strings.Join(out, "\n"))
}

// renumber gives every verse a unique number in 1–99 and warns when it had
// to change one.
func renumber(secs []domain.DraftSection, warn func(string)) {
	used, highest := map[int]bool{}, 0
	for _, s := range secs {
		if s.Kind == domain.SectionVerse && s.Number > highest {
			highest = s.Number
		}
	}
	for i := range secs {
		s := &secs[i]
		if s.Kind != domain.SectionVerse {
			s.Number = 0
			continue
		}
		if s.Number < 1 || used[s.Number] {
			highest++
			s.Number = highest
			warn(domain.WarnVerseRenumbered)
		}
		used[s.Number] = true
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
