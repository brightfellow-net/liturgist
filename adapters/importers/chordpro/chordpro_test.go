// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package chordpro_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/importers/chordpro"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func parse(t *testing.T, doc string) []app.ImportCandidate {
	t.Helper()
	got, err := chordpro.Importer{}.Parse(context.Background(), strings.NewReader(doc),
		app.ImportHint{Format: domain.FormatChordPro, Language: "en", Name: "grace.cho"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func count(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

// TC-I-005: a song with chords, blocks, subtitle and key.
func TestSong(t *testing.T) {
	got := parse(t, `# a comment
{title: Amazing Grace}
{st: Grace}
{subtitle: Newton's hymn}
{lyricist: John Newton}
{composer: Traditional}
{key: G}
{ccli: 4755360}
{copyright: Public Domain}
{meta: songbook Pelengkap}
{meta: number 12}
{capo: 2}
{sov: Verse 1}
[G]Amazing [C]grace, how [G]sweet
the sound
{eov}

{soc}
[D]That saved a wretch
{eoc}
{sov}
I once was lost
{eov}
{start_of_chorus}
[D]That saved a wretch
{end_of_chorus}
`)
	if len(got) != 1 {
		t.Fatalf("got %d songs", len(got))
	}
	d := got[0].Draft
	if d.Title != "Amazing Grace" || len(d.AltTitles) != 2 || d.AltTitles[0] != "Grace" || d.AltTitles[1] != "Newton's hymn" {
		t.Errorf("titles: %q %v", d.Title, d.AltTitles)
	}
	if d.Lyricist != "John Newton" || d.Composer != "Traditional" || d.DefaultKey != "G" || d.CCLISongNumber != "4755360" ||
		d.CopyrightLine != "Public Domain" || d.HymnalSource != "Pelengkap" || d.HymnalNumber != "12" {
		t.Errorf("metadata: %+v", d)
	}
	want := []domain.DraftSection{
		{Kind: domain.SectionVerse, Number: 1, Text: "Amazing grace, how sweet\nthe sound"},
		{Kind: domain.SectionChorus, Text: "That saved a wretch"},
		{Kind: domain.SectionVerse, Number: 2, Text: "I once was lost"},
	}
	if len(d.Sections) != len(want) {
		t.Fatalf("sections = %+v", d.Sections)
	}
	for i, w := range want {
		if d.Sections[i] != w {
			t.Errorf("section %d = %+v, want %+v", i, d.Sections[i], w)
		}
	}
	// The chorus came back at the end: it is a repeat, so there is an arrangement.
	if arr := d.DefaultArrangement; len(arr) != 4 || arr[0] != 0 || arr[1] != 1 || arr[2] != 2 || arr[3] != 1 {
		t.Errorf("arrangement = %v", d.DefaultArrangement)
	}
	if !has(got[0].Warnings, domain.WarnBlocksNumbered) {
		t.Errorf("the unnumbered {sov} needs blocks_numbered: %v", got[0].Warnings)
	}
	if has(got[0].Warnings, domain.WarnDirectiveIgnored) || has(got[0].Warnings, domain.WarnCommentIgnored) {
		t.Errorf("silent directives gave warnings: %v", got[0].Warnings)
	}
	if err := domain.ValidateDraft(&d); err != nil {
		t.Errorf("draft is not valid: %v", err)
	}
}

func TestSilentAndWarnedDirectives(t *testing.T) {
	silent := []string{"capo: 1", "tempo: 90", "time: 4/4", "duration: 3:00", "columns: 2", "column_break", "new_page",
		"pagetype: a4", "define: G base-fret 1", "chord: G", "textfont: Arial", "textsize: 12", "textcolour: red",
		"chordfont: Arial", "chordsize: 10", "chordcolour: blue", "image: src=x.png", "meta: artist Someone"}
	doc := "{title: T}\n{" + strings.Join(silent, "}\n{") + "}\nline one\n"
	c := parse(t, doc)[0]
	if len(c.Warnings) != 1 || c.Warnings[0] != domain.WarnBlocksNumbered {
		t.Errorf("silent directives: warnings = %v", c.Warnings)
	}
	c = parse(t, "{title: T}\n{comment: play softly}\n{ci: x}\n{cb: y}\n{highlight: z}\n{foo}\n{foo: 2}\n{bar}\n{sot}\nE|---\n{eot}\n{start_of_grid}\n| G |\n{end_of_grid}\nreal lyrics\n")[0]
	if count(c.Warnings, domain.WarnCommentIgnored) != 1 || count(c.Warnings, domain.WarnDirectiveIgnored) != 1 ||
		count(c.Warnings, domain.WarnBlockIgnored) != 1 {
		t.Errorf("warnings = %v (each code once)", c.Warnings)
	}
	if len(c.Draft.Sections) != 1 || c.Draft.Sections[0].Text != "real lyrics" {
		t.Errorf("tab and grid content leaked: %+v", c.Draft.Sections)
	}
}

func TestLooseLinesAreSplitLikePaste(t *testing.T) {
	c := parse(t, "{t: T}\n1. [C]first\n\nReff\n[G]shout\n\n2. second\n\nReff\n")[0]
	if len(c.Draft.Sections) != 3 || c.Draft.Sections[1].Kind != domain.SectionChorus {
		t.Errorf("sections = %+v", c.Draft.Sections)
	}
	if arr := c.Draft.DefaultArrangement; len(arr) != 4 {
		t.Errorf("arrangement = %v", arr)
	}
}

// IT-I-009's parser half: several songs, one malformed.
func TestSeveralSongs(t *testing.T) {
	got := parse(t, `{title: One}
{sov}
a
{eov}
{new_song}
{title: Broken}
{capo: 1}
{ns}
{title: Three}
b

c
`)
	if len(got) != 3 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0].Draft.Title != "One" || got[1].Reject != app.ImportNoSong || got[2].Draft.Title != "Three" {
		t.Errorf("entries: %q / reject %q / %q", got[0].Draft.Title, got[1].Reject, got[2].Draft.Title)
	}
	// {new_song} on the first line adds no empty song.
	if got := parse(t, "{new_song}\n{title: Only}\nx\n"); len(got) != 1 {
		t.Errorf("leading {new_song}: %d songs", len(got))
	}
}

func TestTitleFromFileNameAndBadValues(t *testing.T) {
	c := parse(t, "{key: H}\n{ccli: abc}\nx\n")[0]
	if c.Draft.Title != "grace" || !has(c.Warnings, domain.WarnTitleFromFile) || !has(c.Warnings, domain.WarnKeyIgnored) ||
		!has(c.Warnings, domain.WarnCCLIIgnored) {
		t.Errorf("title %q warnings %v", c.Draft.Title, c.Warnings)
	}
	if c.Draft.DefaultKey != "" || c.Draft.CCLISongNumber != "" {
		t.Errorf("bad values kept: %+v", c.Draft)
	}
}

func TestLimitsAndErrors(t *testing.T) {
	long := "{title: Long}\n" + strings.Repeat("x\n", chordpro.MaxLines+1)
	got := parse(t, long+"{new_song}\n{title: Fine}\nlyrics\n")
	if len(got) != 2 || got[0].Reject != app.ImportTooComplex || got[1].Draft.Title != "Fine" {
		t.Errorf("a long song must be rejected alone: %+v", got)
	}
	var many strings.Builder
	many.WriteString("{title: T}\n")
	for i := range 61 {
		many.WriteString("{sov}\nline " + strings.Repeat("a", i+1) + "\n{eov}\n")
	}
	if got := parse(t, many.String()); got[0].Reject != app.ImportTooManySections {
		t.Errorf("61 sections: %+v", got[0])
	}
	// A file with no lyrics gives a rejected entry; the use case turns "nothing but rejections" into 422.
	if got := parse(t, "# only a comment\n"); len(got) != 1 || got[0].Reject != app.ImportNoSong {
		t.Errorf("no song: %+v", got)
	}
	_, err := chordpro.Importer{}.Parse(context.Background(), strings.NewReader("\n\n"), app.ImportHint{Language: "en"})
	var bad *app.ImportUnreadableError
	if !errors.As(err, &bad) || bad.Reason != app.ImportNoSong {
		t.Errorf("empty file: %v", err)
	}
	_, err = chordpro.Importer{}.Parse(context.Background(), strings.NewReader("a\xffb"), app.ImportHint{Language: "en"})
	if !errors.As(err, &bad) || bad.Reason != app.ImportNotUTF8 {
		t.Errorf("not UTF-8: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = (chordpro.Importer{}).Parse(ctx, strings.NewReader(strings.Repeat("x\n", 5000)), app.ImportHint{Language: "en"}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}
