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

// raw is what the XML says, before it is turned into a draft.
type raw struct {
	titles   []string
	authors  map[string][]string // by author type; "" = untyped
	props    map[string]string   // copyright, ccliNo, key
	order    string              // verseOrder
	book     struct{ name, entry string }
	haveBook bool
	verses   []*verse
}

// reader follows the XML tokens and fills a raw.
type reader struct {
	raw     raw
	path    []string
	curV    *verse
	curLine *strings.Builder
	authTyp string
	sawRoot bool
}

// in reports whether the open elements are exactly parts.
func (rd *reader) in(parts ...string) bool {
	if len(rd.path) != len(parts) {
		return false
	}
	for i, p := range parts {
		if rd.path[i] != p {
			return false
		}
	}
	return true
}

func (rd *reader) inLines() bool {
	return rd.curV != nil && len(rd.path) >= 4 && rd.path[2] == "verse" && rd.path[3] == "lines"
}

// start handles an opening tag; a root that is not <song> is not OpenLyrics.
func (rd *reader) start(t xml.StartElement) error {
	if len(rd.path) == 0 && t.Name.Local != "song" {
		return &app.ImportUnreadableError{Reason: app.ImportNotXML}
	}
	rd.sawRoot = true
	rd.path = append(rd.path, t.Name.Local)
	if len(rd.path) > maxDepth {
		return &app.ImportUnreadableError{Reason: app.ImportTooComplex}
	}
	switch {
	case rd.in("song", "properties", "authors", "author"):
		rd.authTyp = attr(t, "type")
	case rd.in("song", "properties", "songbooks", "songbook") && !rd.raw.haveBook:
		rd.raw.book.name, rd.raw.book.entry, rd.raw.haveBook = attr(t, "name"), attr(t, "entry"), true
	case rd.in("song", "lyrics", "verse"):
		rd.curV = &verse{name: attr(t, "name")}
		rd.raw.verses = append(rd.raw.verses, rd.curV)
	case rd.in("song", "lyrics", "verse", "lines"):
		rd.curLine = &strings.Builder{}
	case rd.inLines() && t.Name.Local == "br" && rd.curLine != nil:
		rd.curLine.WriteByte('\n')
	}
	return nil
}

// end handles a closing tag.
func (rd *reader) end() {
	switch {
	case rd.in("song", "lyrics", "verse", "lines") && rd.curV != nil && rd.curLine != nil:
		rd.curV.lines = append(rd.curV.lines, rd.curLine.String())
		rd.curLine = nil
	case rd.in("song", "lyrics", "verse"):
		rd.curV = nil
	}
	rd.path = rd.path[:len(rd.path)-1]
}

// chars handles text.
func (rd *reader) chars(s string) {
	switch {
	case rd.in("song", "properties", "titles", "title"):
		if v := strings.TrimSpace(s); v != "" {
			rd.raw.titles = append(rd.raw.titles, v)
		}
	case rd.in("song", "properties", "authors", "author"):
		if v := strings.TrimSpace(s); v != "" {
			rd.raw.authors[rd.authTyp] = append(rd.raw.authors[rd.authTyp], v)
		}
	case rd.in("song", "properties", "copyright"), rd.in("song", "properties", "ccliNo"), rd.in("song", "properties", "key"):
		rd.raw.props[rd.path[2]] += s
	case rd.in("song", "properties", "verseOrder"):
		rd.raw.order += s
	case rd.inLines() && rd.curLine != nil && !inComment(rd.path):
		rd.curLine.WriteString(s)
	}
}

// read turns the XML into a raw. Entities and DTDs are never resolved: a
// DOCTYPE is skipped, and an entity other than the five XML ones and numeric
// references is a syntax error.
func read(ctx context.Context, text string) (raw, error) {
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = true
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "utf8") {
			return in, nil
		}
		return nil, errors.New("unsupported encoding")
	}
	rd := &reader{raw: raw{authors: map[string][]string{}, props: map[string]string{}}}
	tokens := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return raw{}, &app.ImportUnreadableError{Reason: app.ImportNotXML}
		}
		if tokens++; tokens > maxTokens {
			return raw{}, &app.ImportUnreadableError{Reason: app.ImportTooComplex}
		}
		if tokens%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return raw{}, err
			}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := rd.start(t); err != nil {
				return raw{}, err
			}
		case xml.EndElement:
			rd.end()
		case xml.CharData:
			rd.chars(string(t))
		}
	}
	if !rd.sawRoot {
		return raw{}, &app.ImportUnreadableError{Reason: app.ImportNotXML}
	}
	if len(rd.raw.titles) == 0 || len(rd.raw.verses) == 0 {
		return raw{}, &app.ImportUnreadableError{Reason: app.ImportNoSong}
	}
	return rd.raw, nil
}

// Parse implements app.Importer.
func (Importer) Parse(ctx context.Context, r io.Reader, hint app.ImportHint) ([]app.ImportCandidate, error) {
	text, err := paste.ReadText(r)
	if err != nil {
		return nil, err
	}
	song, err := read(ctx, text)
	if err != nil {
		return nil, err
	}
	return draftOf(song, hint)
}

// drafter collects the warnings of building one draft.
type drafter struct{ warns []string }

func (dr *drafter) warn(code string) {
	if !contains(dr.warns, code) {
		dr.warns = append(dr.warns, code)
	}
}

// draftOf turns what the XML said into a draft (08 §4.2).
func draftOf(song raw, hint app.ImportHint) ([]app.ImportCandidate, error) {
	dr := &drafter{}
	d := domain.SongDraft{Language: hint.Language, Title: song.titles[0], AltTitles: []string{}, Sections: []domain.DraftSection{},
		DefaultArrangement: []int{}}
	dr.metadata(&d, song)

	// Sections: kind from the first letter of the name; verse numbers from its digits.
	items, index := dr.items(song.verses)
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
	renumber(d.Sections, dr.warn)
	d.DefaultArrangement = dr.arrangement(song.order, index, sectionOf, d.DefaultArrangement)
	return []app.ImportCandidate{{Draft: d, Warnings: dr.warns}}, nil
}

// metadata fills the title list, people, copyright, key, CCLI number and songbook.
func (dr *drafter) metadata(d *domain.SongDraft, song raw) {
	for _, t := range song.titles[1:] {
		if t != d.Title && !contains(d.AltTitles, t) {
			d.AltTitles = append(d.AltTitles, t)
		}
	}
	d.Lyricist = strings.Join(append(append([]string{}, song.authors[""]...), song.authors["words"]...), ", ")
	d.Composer = strings.Join(song.authors["music"], ", ")
	d.Translator = strings.Join(song.authors["translation"], ", ")
	d.CopyrightLine = strings.TrimSpace(song.props["copyright"])
	if v := strings.TrimSpace(song.props["ccliNo"]); v != "" {
		if ccliRe.MatchString(v) {
			d.CCLISongNumber = v
		} else {
			dr.warn(domain.WarnCCLIIgnored)
		}
	}
	if v := strings.TrimSpace(song.props["key"]); v != "" {
		if keyRe.MatchString(v) {
			d.DefaultKey = v
		} else {
			dr.warn(domain.WarnKeyIgnored)
		}
	}
	if song.haveBook {
		d.HymnalSource, d.HymnalNumber = strings.TrimSpace(song.book.name), strings.TrimSpace(song.book.entry)
		if d.HymnalSource == "" || d.HymnalNumber == "" {
			d.HymnalSource, d.HymnalNumber = "", ""
		}
	}
}

// items reads every verse as a labelled item. index maps a verse name to the
// first item that has it.
func (dr *drafter) items(verses []*verse) ([]paste.Item, map[string]int) {
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
			dr.warn(domain.WarnNumberDropped)
		}
	}
	return items, index
}

// arrangement turns the verse order into section indexes; a name that is
// unknown, or has no text, drops the whole order.
func (dr *drafter) arrangement(order string, index map[string]int, sectionOf map[int]int, arr []int) []int {
	for _, name := range strings.Fields(order) {
		i, ok := index[name]
		s, has := sectionOf[i]
		if !ok || !has {
			dr.warn(domain.WarnArrangementIgnore)
			return []int{}
		}
		arr = append(arr, s)
	}
	return arr
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
