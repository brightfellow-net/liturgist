// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package chordpro reads ChordPro files (08 §4.3). Chords are dropped.
package chordpro

import (
	"context"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/brightfellow-net/liturgist/adapters/importers/paste"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// MaxLines is the most lines one song may have (08 §5).
const MaxLines = 20_000

var (
	directiveRe = regexp.MustCompile(`^\s*\{\s*([A-Za-z_]+)\s*(?:[:\s]\s*(.*?))?\s*\}\s*$`)
	chordRe     = regexp.MustCompile(`\[[^\]]*\]`)
	keyRe       = regexp.MustCompile(`^[A-G][#b]?m?$`)
	ccliRe      = regexp.MustCompile(`^[0-9]{1,12}$`)
)

// alias → canonical directive name.
var alias = map[string]string{
	"t": "title", "st": "subtitle", "c": "comment", "ci": "comment_italic", "cb": "comment_box",
	"sov": "start_of_verse", "eov": "end_of_verse", "soc": "start_of_chorus", "eoc": "end_of_chorus",
	"sob": "start_of_bridge", "eob": "end_of_bridge", "sot": "start_of_tab", "eot": "end_of_tab",
	"sog": "start_of_grid", "eog": "end_of_grid", "ns": "new_song",
}

// silent directives carry no lyrics and no structure (08 §4.3).
var silent = map[string]bool{
	"capo": true, "tempo": true, "time": true, "duration": true, "columns": true, "column_break": true,
	"new_page": true, "pagetype": true, "define": true, "chord": true, "textfont": true, "textsize": true,
	"textcolour": true, "textcolor": true, "chordfont": true, "chordsize": true, "chordcolour": true,
	"chordcolor": true, "image": true, "meta": true,
}

var comments = map[string]bool{"comment": true, "comment_italic": true, "comment_box": true, "highlight": true}

// Importer reads a file with one or more songs.
type Importer struct{}

// Parse implements app.Importer. Each song is parsed on its own: one that is
// malformed or too long becomes a rejected entry and the others still count.
func (Importer) Parse(ctx context.Context, r io.Reader, hint app.ImportHint) ([]app.ImportCandidate, error) {
	text, err := paste.ReadText(r)
	if err != nil {
		return nil, err
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var songs [][]string
	cur := []string{}
	for i, line := range strings.Split(text, "\n") {
		if i%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if m := directiveRe.FindStringSubmatch(line); m != nil && canon(m[1]) == "new_song" {
			songs = append(songs, cur)
			cur = []string{}
			continue
		}
		cur = append(cur, line)
	}
	songs = append(songs, cur)

	var out []app.ImportCandidate
	for _, lines := range songs {
		if blank(lines) {
			continue // {new_song} first, twice in a row, or after the last song
		}
		c, err := parseSong(ctx, lines, hint)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, &app.ImportUnreadableError{Reason: app.ImportNoSong}
	}
	return out, nil
}

func blank(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return false
		}
	}
	return true
}

func canon(name string) string {
	name = strings.ToLower(name)
	if a, ok := alias[name]; ok {
		return a
	}
	return name
}

// songParser holds the state of reading one song.
type songParser struct {
	d          domain.SongDraft
	warns      []string
	items      []paste.Item
	loose      []string    // lyric lines outside any block, not yet split into blocks
	block      *paste.Item // the open start_of_… block
	blockTx    []string
	skip       string // "tab" or "grid" while their content is dropped
	seenDirect map[string]bool
}

func (p *songParser) warn(code string) {
	for _, w := range p.warns {
		if w == code {
			return
		}
	}
	p.warns = append(p.warns, code)
}

func (p *songParser) flushLoose() {
	if len(p.loose) > 0 {
		p.items = append(p.items, paste.Blocks(strings.Join(p.loose, "\n"))...)
		p.loose = nil
	}
}

// closeBlock ends the open block, if there is one, and keeps it as an item.
func (p *songParser) closeBlock() {
	if p.block != nil {
		p.block.Text = strings.Join(p.blockTx, "\n")
		p.items = append(p.items, *p.block)
		p.block, p.blockTx = nil, nil
	}
}

// parseSong reads one song; a song that cannot be read is returned with Reject set.
func parseSong(ctx context.Context, lines []string, hint app.ImportHint) (app.ImportCandidate, error) {
	if len(lines) > MaxLines {
		return app.ImportCandidate{Reject: app.ImportTooComplex}, nil
	}
	p := &songParser{
		d:          domain.SongDraft{Language: hint.Language, AltTitles: []string{}, Sections: []domain.DraftSection{}, DefaultArrangement: []int{}},
		seenDirect: map[string]bool{},
	}
	for i, line := range lines {
		if i%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return app.ImportCandidate{}, err
			}
		}
		p.line(line)
	}
	p.closeBlock()
	p.flushLoose()
	return p.finish(hint), nil
}

// line reads one line: a comment, a lyric line or a directive.
func (p *songParser) line(line string) {
	if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") && p.skip == "" {
		return
	}
	m := directiveRe.FindStringSubmatch(line)
	if m == nil {
		if p.skip != "" {
			return
		}
		text := strings.TrimRight(chordRe.ReplaceAllString(line, ""), " \t")
		if p.block != nil {
			p.blockTx = append(p.blockTx, text)
		} else {
			p.loose = append(p.loose, text)
		}
		return
	}
	name, arg := canon(m[1]), strings.TrimSpace(m[2])
	if p.skip != "" {
		if name == "end_of_"+p.skip {
			p.skip = ""
		}
		return
	}
	p.directive(name, arg)
}

// directive applies one directive (08 §4.3).
func (p *songParser) directive(name, arg string) {
	d := &p.d
	switch {
	case name == "title":
		if d.Title == "" {
			d.Title = arg
		} else if arg != d.Title {
			d.AltTitles = append(d.AltTitles, arg)
		}
	case name == "subtitle":
		if arg != "" && arg != d.Title && !has(d.AltTitles, arg) {
			d.AltTitles = append(d.AltTitles, arg)
		}
	case name == "lyricist":
		d.Lyricist = arg
	case name == "composer":
		d.Composer = arg
	case name == "copyright":
		d.CopyrightLine = arg
	case name == "key":
		if keyRe.MatchString(arg) {
			d.DefaultKey = arg
		} else {
			p.warn(domain.WarnKeyIgnored)
		}
	case name == "ccli":
		if ccliRe.MatchString(arg) {
			d.CCLISongNumber = arg
		} else {
			p.warn(domain.WarnCCLIIgnored)
		}
	case name == "meta":
		key, val, _ := strings.Cut(arg, " ")
		switch strings.ToLower(key) {
		case "songbook":
			d.HymnalSource = strings.TrimSpace(val)
		case "number":
			d.HymnalNumber = strings.TrimSpace(val)
		}
	case strings.HasPrefix(name, "start_of_"):
		p.startBlock(strings.TrimPrefix(name, "start_of_"), arg)
	case strings.HasPrefix(name, "end_of_"):
		if p.block != nil && strings.TrimPrefix(name, "end_of_") == kindName(p.block.Kind) {
			p.closeBlock()
		}
	case comments[name]:
		p.warn(domain.WarnCommentIgnored)
	case silent[name]:
	default:
		if !p.seenDirect[name] {
			p.seenDirect[name] = true
			p.warn(domain.WarnDirectiveIgnored)
		}
	}
}

// startBlock opens a verse, chorus or bridge, or starts dropping a tab or
// grid. A block that was never closed ends here.
func (p *songParser) startBlock(kind, arg string) {
	switch kind {
	case "tab", "grid":
		p.flushLoose()
		p.skip = kind
		p.warn(domain.WarnBlockIgnored)
	case "verse", "chorus", "bridge":
		p.flushLoose()
		p.closeBlock()
		b := blockStart(kind, arg)
		p.block, p.blockTx = &b, nil
	default:
		p.warn(domain.WarnDirectiveIgnored)
	}
}

// finish builds the candidate from the items read (08 §4.1 rules 3 to 6).
func (p *songParser) finish(hint app.ImportHint) app.ImportCandidate {
	res := paste.Build(p.items, hint.Language)
	switch {
	case res.TooMany:
		return app.ImportCandidate{Reject: app.ImportTooManySections}
	case len(res.Sections) == 0:
		return app.ImportCandidate{Reject: app.ImportNoSong}
	}
	d := p.d
	if d.Title == "" {
		d.Title = strings.TrimSuffix(path.Base(hint.Name), path.Ext(hint.Name))
		p.warn(domain.WarnTitleFromFile)
	}
	d.Sections = res.Sections
	if res.Arrangement != nil {
		d.DefaultArrangement = res.Arrangement
	}
	d.HymnalSource, d.HymnalNumber = pair(d.HymnalSource, d.HymnalNumber)
	return app.ImportCandidate{Draft: d, Warnings: append(p.warns, res.Warnings...)}
}

// blockStart reads the label argument of {start_of_verse: Verse 2} with the
// table of 08 §4.1; an argument that is not a label becomes the section label.
func blockStart(kind, arg string) paste.Item {
	it := paste.Item{Labelled: true}
	switch kind {
	case "verse":
		it.Kind = domain.SectionVerse
	case "chorus":
		it.Kind = domain.SectionChorus
	default:
		it.Kind = domain.SectionBridge
	}
	if arg == "" {
		return it
	}
	if l, ok := paste.ParseLabel(arg); ok && l.Rest == "" {
		it.Kind, it.Number, it.NumberSet = l.Kind, l.Number, l.NumberSet
		return it
	}
	it.Label = arg
	return it
}

func kindName(k domain.SectionKind) string {
	switch k {
	case domain.SectionVerse:
		return "verse"
	case domain.SectionChorus:
		return "chorus"
	}
	return "bridge"
}

// pair keeps a hymnal source and number only together.
func pair(source, number string) (string, string) {
	if source == "" || number == "" {
		return "", ""
	}
	return source, number
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
