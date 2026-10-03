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

// parseSong reads one song; a song that cannot be read is returned with Reject set.
func parseSong(ctx context.Context, lines []string, hint app.ImportHint) (app.ImportCandidate, error) {
	if len(lines) > MaxLines {
		return app.ImportCandidate{Reject: app.ImportTooComplex}, nil
	}
	d := domain.SongDraft{Language: hint.Language, AltTitles: []string{}, Sections: []domain.DraftSection{}, DefaultArrangement: []int{}}
	var (
		warns   []string
		items   []paste.Item
		loose   []string
		block   *paste.Item // the open start_of_… block
		blockTx []string
		skip    string // "tab" or "grid" while their content is dropped
	)
	warn := func(code string) {
		for _, w := range warns {
			if w == code {
				return
			}
		}
		warns = append(warns, code)
	}
	flushLoose := func() {
		if len(loose) > 0 {
			items = append(items, paste.Blocks(strings.Join(loose, "\n"))...)
			loose = nil
		}
	}
	seenDirective := map[string]bool{}
	for i, line := range lines {
		if i%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return app.ImportCandidate{}, err
			}
		}
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") && skip == "" {
			continue
		}
		m := directiveRe.FindStringSubmatch(line)
		if m == nil {
			if skip != "" {
				continue
			}
			text := strings.TrimRight(chordRe.ReplaceAllString(line, ""), " \t")
			if block != nil {
				blockTx = append(blockTx, text)
			} else {
				loose = append(loose, text)
			}
			continue
		}
		name, arg := canon(m[1]), strings.TrimSpace(m[2])
		if skip != "" {
			if name == "end_of_"+skip {
				skip = ""
			}
			continue
		}
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
				warn(domain.WarnKeyIgnored)
			}
		case name == "ccli":
			if ccliRe.MatchString(arg) {
				d.CCLISongNumber = arg
			} else {
				warn(domain.WarnCCLIIgnored)
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
			kind := strings.TrimPrefix(name, "start_of_")
			switch kind {
			case "tab", "grid":
				flushLoose()
				skip = kind
				warn(domain.WarnBlockIgnored)
			case "verse", "chorus", "bridge":
				flushLoose()
				if block != nil { // a block that was never closed ends here
					block.Text = strings.Join(blockTx, "\n")
					items = append(items, *block)
				}
				b := blockStart(kind, arg)
				block, blockTx = &b, nil
			default:
				warn(domain.WarnDirectiveIgnored)
			}
		case strings.HasPrefix(name, "end_of_"):
			if block != nil && strings.TrimPrefix(name, "end_of_") == kindName(block.Kind) {
				block.Text = strings.Join(blockTx, "\n")
				items = append(items, *block)
				block, blockTx = nil, nil
			}
		case comments[name]:
			warn(domain.WarnCommentIgnored)
		case silent[name]:
		default:
			if !seenDirective[name] {
				seenDirective[name] = true
				warn(domain.WarnDirectiveIgnored)
			}
		}
	}
	if block != nil {
		block.Text = strings.Join(blockTx, "\n")
		items = append(items, *block)
	}
	flushLoose()

	res := paste.Build(items, hint.Language)
	switch {
	case res.TooMany:
		return app.ImportCandidate{Reject: app.ImportTooManySections}, nil
	case len(res.Sections) == 0:
		return app.ImportCandidate{Reject: app.ImportNoSong}, nil
	}
	if d.Title == "" {
		d.Title = strings.TrimSuffix(path.Base(hint.Name), path.Ext(hint.Name))
		warn(domain.WarnTitleFromFile)
	}
	d.Sections = res.Sections
	if res.Arrangement != nil {
		d.DefaultArrangement = res.Arrangement
	}
	d.HymnalSource, d.HymnalNumber = pair(d.HymnalSource, d.HymnalNumber)
	return app.ImportCandidate{Draft: d, Warnings: append(warns, res.Warnings...)}, nil
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
