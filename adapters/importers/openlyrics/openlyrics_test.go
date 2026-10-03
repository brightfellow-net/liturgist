// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package openlyrics_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/importers/openlyrics"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<song xmlns="http://openlyrics.info/namespace/2009/song" version="0.8" createdIn="OpenLP 2.4">
  <properties>
    <titles>
      <title>Amazing Grace</title>
      <title>Grace Amazing</title>
    </titles>
    <authors>
      <author>John Newton</author>
      <author type="words">William Cowper</author>
      <author type="music">Edwin Example</author>
      <author type="music">Traditional</author>
      <author type="translation">Anon</author>
    </authors>
    <copyright>Public Domain</copyright>
    <ccliNo>4755360</ccliNo>
    <key>G</key>
    <songbooks><songbook name="Pelengkap" entry="12"/><songbook name="Other" entry="3"/></songbooks>
    <verseOrder>v1 c v2 c</verseOrder>
  </properties>
  <lyrics>
    <verse name="v1">
      <lines>Amazing <chord name="G"/>grace<br/>how <tag name="i">sweet</tag> the sound</lines>
    </verse>
    <verse name="c">
      <lines part="1">That saved<br/>a wretch</lines>
      <lines part="2">like me</lines>
    </verse>
    <verse name="v2"><lines>I once was lost</lines></verse>
  </lyrics>
</song>`

func parse(t *testing.T, doc string) []app.ImportCandidate {
	t.Helper()
	got, err := openlyrics.Importer{}.Parse(context.Background(), strings.NewReader(doc),
		app.ImportHint{Format: domain.FormatOpenLyrics, Language: "en", Name: "ag.xml"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func reason(err error) string {
	var bad *app.ImportUnreadableError
	if errors.As(err, &bad) {
		return bad.Reason
	}
	return "other: " + err.Error()
}

// TC-I-004: every mapping of 08 §4.2.
func TestSample(t *testing.T) {
	got := parse(t, sample)
	if len(got) != 1 {
		t.Fatalf("got %d candidates", len(got))
	}
	d := got[0].Draft
	if d.Title != "Amazing Grace" || len(d.AltTitles) != 1 || d.AltTitles[0] != "Grace Amazing" {
		t.Errorf("titles: %q %v", d.Title, d.AltTitles)
	}
	if d.Lyricist != "John Newton, William Cowper" || d.Composer != "Edwin Example, Traditional" || d.Translator != "Anon" {
		t.Errorf("authors: %q / %q / %q", d.Lyricist, d.Composer, d.Translator)
	}
	if d.CopyrightLine != "Public Domain" || d.CCLISongNumber != "4755360" || d.DefaultKey != "G" {
		t.Errorf("properties: %q %q %q", d.CopyrightLine, d.CCLISongNumber, d.DefaultKey)
	}
	if d.HymnalSource != "Pelengkap" || d.HymnalNumber != "12" {
		t.Errorf("songbook: %q %q", d.HymnalSource, d.HymnalNumber)
	}
	if d.Language != "en" {
		t.Errorf("language = %q", d.Language)
	}
	want := []domain.DraftSection{
		{Kind: domain.SectionVerse, Number: 1, Text: "Amazing grace\nhow sweet the sound"},
		{Kind: domain.SectionChorus, Text: "That saved\na wretch\nlike me"},
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
	if arr := d.DefaultArrangement; len(arr) != 4 || arr[0] != 0 || arr[1] != 1 || arr[2] != 2 || arr[3] != 1 {
		t.Errorf("arrangement = %v", arr)
	}
	if len(got[0].Warnings) != 0 {
		t.Errorf("warnings = %v", got[0].Warnings)
	}
	if err := domain.ValidateDraft(&d); err != nil {
		t.Errorf("draft is not valid: %v", err)
	}
}

func TestWarnings(t *testing.T) {
	doc := `<song><properties><titles><title>T</title></titles><ccliNo>12x</ccliNo><key>H#</key>
		<verseOrder>v1 zz</verseOrder></properties>
		<lyrics><verse name="v1"><lines>a</lines></verse><verse name="v1a"><lines>b</lines></verse>
		<verse name="c2"><lines>c</lines></verse><verse name="x"><lines>d</lines></verse></lyrics></song>`
	c := parse(t, doc)[0]
	for _, w := range []string{domain.WarnCCLIIgnored, domain.WarnKeyIgnored, domain.WarnArrangementIgnore,
		domain.WarnVerseRenumbered, domain.WarnNumberDropped} {
		found := false
		for _, x := range c.Warnings {
			found = found || x == w
		}
		if !found {
			t.Errorf("warnings %v lack %s", c.Warnings, w)
		}
	}
	if len(c.Draft.DefaultArrangement) != 0 || c.Draft.CCLISongNumber != "" || c.Draft.DefaultKey != "" {
		t.Errorf("ignored values kept: %+v", c.Draft)
	}
	// v1 and v1a both claim verse 1: the second is renumbered, kinds of c2 and x are right.
	if s := c.Draft.Sections; s[0].Number != 1 || s[1].Number != 2 || s[2].Kind != domain.SectionChorus || s[2].Number != 0 || s[3].Kind != domain.SectionOther {
		t.Errorf("sections = %+v", s)
	}
}

func TestUnreadable(t *testing.T) {
	for name, tc := range map[string]struct{ doc, reason string }{
		"not xml":       {"hello", app.ImportNotXML},
		"other root":    {"<html></html>", app.ImportNotXML},
		"broken":        {"<song><properties></song>", app.ImportNotXML},
		"no title":      {`<song><lyrics><verse name="v1"><lines>a</lines></verse></lyrics></song>`, app.ImportNoSong},
		"no verses":     {`<song><properties><titles><title>T</title></titles></properties></song>`, app.ImportNoSong},
		"empty verses":  {`<song><properties><titles><title>T</title></titles></properties><lyrics><verse name="v1"><lines> </lines></verse></lyrics></song>`, app.ImportNoSong},
		"latin-1 label": {`<?xml version="1.0" encoding="ISO-8859-1"?><song/>`, app.ImportNotXML},
		"bad entity":    {`<song><properties><titles><title>&bogus;</title></titles></properties></song>`, app.ImportNotXML},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := openlyrics.Importer{}.Parse(context.Background(), strings.NewReader(tc.doc), app.ImportHint{Language: "en"})
			if reason(err) != tc.reason {
				t.Errorf("reason = %q, want %q", reason(err), tc.reason)
			}
		})
	}
	_, err := openlyrics.Importer{}.Parse(context.Background(), strings.NewReader("<song>\xff</song>"), app.ImportHint{Language: "en"})
	if reason(err) != app.ImportNotUTF8 {
		t.Errorf("invalid UTF-8: %q", reason(err))
	}
}

// A DOCTYPE with an entity is never expanded (08 §8).
func TestEntitiesAreNotExpanded(t *testing.T) {
	doc := `<?xml version="1.0"?><!DOCTYPE song [<!ENTITY secret SYSTEM "file:///etc/passwd"><!ENTITY big "aaaa">]>
		<song><properties><titles><title>T &secret;</title></titles></properties>
		<lyrics><verse name="v1"><lines>a</lines></verse></lyrics></song>`
	_, err := openlyrics.Importer{}.Parse(context.Background(), strings.NewReader(doc), app.ImportHint{Language: "en"})
	if reason(err) != app.ImportNotXML {
		t.Errorf("reason = %q, want not_xml (entity reference must not resolve)", reason(err))
	}
	// Declaring an entity but not using it is harmless.
	ok := `<!DOCTYPE song [<!ENTITY big "aaaa">]><song><properties><titles><title>T</title></titles></properties>
		<lyrics><verse name="v1"><lines>a</lines></verse></lyrics></song>`
	if got := parse(t, ok); got[0].Draft.Title != "T" {
		t.Errorf("title = %q", got[0].Draft.Title)
	}
}

func TestComplexityLimits(t *testing.T) {
	var many strings.Builder
	many.WriteString(`<song><properties><titles><title>T</title></titles></properties><lyrics><verse name="v1"><lines>`)
	for range 100_000 {
		many.WriteString("<br/>")
	}
	many.WriteString(`</lines></verse></lyrics></song>`)
	_, err := openlyrics.Importer{}.Parse(context.Background(), strings.NewReader(many.String()), app.ImportHint{Language: "en"})
	if reason(err) != app.ImportTooComplex {
		t.Errorf("many tokens: %q", reason(err))
	}
	deep := strings.Repeat("<a>", 40)
	_, err = openlyrics.Importer{}.Parse(context.Background(), strings.NewReader("<song>"+deep+strings.Repeat("</a>", 40)+"</song>"), app.ImportHint{Language: "en"})
	if reason(err) != app.ImportTooComplex {
		t.Errorf("deep: %q", reason(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	small := `<song><properties><titles><title>T</title></titles></properties><lyrics><verse name="v1"><lines>` +
		strings.Repeat("<br/>", 3000) + `</lines></verse></lyrics></song>`
	if _, err = (openlyrics.Importer{}).Parse(ctx, strings.NewReader(small), app.ImportHint{Language: "en"}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context must stop the parser, got %v", err)
	}
}
