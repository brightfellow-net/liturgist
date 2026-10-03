// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package paste_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/importers/paste"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func parse(t *testing.T, text string) app.ImportCandidate {
	t.Helper()
	got, err := paste.Importer{}.Parse(context.Background(), strings.NewReader(text),
		app.ImportHint{Format: domain.FormatPaste, Language: "id", Name: "Besar Setia-Mu"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates", len(got))
	}
	return got[0]
}

// shape renders sections as "v1=a c=x" for comparison.
func shape(d domain.SongDraft) string {
	var parts []string
	for _, s := range d.Sections {
		name := map[domain.SectionKind]string{domain.SectionVerse: "v", domain.SectionChorus: "c", domain.SectionPreChorus: "p",
			domain.SectionBridge: "b", domain.SectionIntro: "i", domain.SectionEnding: "e", domain.SectionOther: "o", domain.SectionTag: "t"}[s.Kind]
		if s.Number > 0 {
			name += strconv.Itoa(s.Number)
		}
		parts = append(parts, name+"="+strings.ReplaceAll(s.Text, "\n", "/"))
	}
	return strings.Join(parts, " ")
}

func arrangement(d domain.SongDraft) string {
	var parts []string
	for _, i := range d.DefaultArrangement {
		s := d.Sections[i]
		name := string(s.Kind[:1])
		if s.Number > 0 {
			name += strconv.Itoa(s.Number)
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, " ")
}

// TC-I-001: the four worked examples of 08 §4.1.
func TestWorkedExamples(t *testing.T) {
	for _, tc := range []struct {
		name, in, sections, arr, warnings string
	}{
		{"unlabelled with repeats", "a\n\nx\n\nb\n\nx\n\nc\n\nx", "v1=a c=x v2=b v3=c", "v1 c v2 c v3 c", "blocks_numbered chorus_guessed"},
		{"labelled with Reff alone", "1. a\n\nReff\nx\n\n2. b\n\nReff\n\n3. c\n\nReff", "v1=a c=x v2=b v3=c", "v1 c v2 c v3 c", ""},
		{"copy of a labelled chorus", "Verse 1\na\n\nChorus\nx\n\nVerse 2\nb\n\nx", "v1=a c=x v2=b", "v1 c v2 c", ""},
		{"three verses", "a\n\nb\n\nc", "v1=a v2=b v3=c", "", "blocks_numbered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := parse(t, tc.in)
			if got := shape(c.Draft); got != tc.sections {
				t.Errorf("sections = %q, want %q", got, tc.sections)
			}
			if got := arrangement(c.Draft); got != tc.arr {
				t.Errorf("arrangement = %q, want %q", got, tc.arr)
			}
			warnings := strings.Join(c.Warnings, " ")
			for _, w := range strings.Fields(tc.warnings) {
				if !strings.Contains(warnings, w) {
					t.Errorf("warnings = %q, missing %s", warnings, w)
				}
			}
			if len(strings.Fields(warnings)) != len(strings.Fields(tc.warnings)) {
				t.Errorf("warnings = %q, want %q", warnings, tc.warnings)
			}
			if c.Draft.Title != "Besar Setia-Mu" || c.Draft.Language != "id" {
				t.Errorf("title/language = %q/%q", c.Draft.Title, c.Draft.Language)
			}
		})
	}
}

func TestLineEndingsAndSpacing(t *testing.T) {
	c := parse(t, "\n\n1. a  \r\nb\t\r\n \t\r\n\r\n2. c\r\n")
	if got := shape(c.Draft); got != "v1=a/b v2=c" {
		t.Errorf("sections = %q", got)
	}
}

// TC-I-002: every label of the table in the three languages.
func TestLabels(t *testing.T) {
	for _, tc := range []struct {
		line string
		kind domain.SectionKind
		num  int
	}{
		{"Bait 1", "verse", 1}, {"Ayat 2", "verse", 2}, {"1.", "verse", 1}, {"3)", "verse", 3}, {"4:", "verse", 4},
		{"Verse 5", "verse", 5}, {"V1", "verse", 1}, {"v2:", "verse", 2},
		{"第一节", "verse", 1}, {"第2节", "verse", 2}, {"三、", "verse", 3}, {"第十节", "verse", 10},
		{"Reff", "chorus", 0}, {"REF", "chorus", 0}, {"Refrein:", "chorus", 0}, {"Chorus", "chorus", 0}, {"Refrain.", "chorus", 0}, {"副歌", "chorus", 0},
		{"Pra-Reff", "pre_chorus", 0}, {"Pre-Reff", "pre_chorus", 0}, {"Pre-Chorus", "pre_chorus", 0}, {"Prechorus", "pre_chorus", 0},
		{"Pre Chorus", "pre_chorus", 0}, {"前副歌", "pre_chorus", 0}, {"导歌", "pre_chorus", 0},
		{"Jembatan", "bridge", 0}, {"Bridge", "bridge", 0}, {"桥段", "bridge", 0},
		{"Tag", "tag", 0},
		{"Intro", "intro", 0}, {"前奏", "intro", 0},
		{"Akhir", "ending", 0}, {"Outro", "ending", 0}, {"Ending", "ending", 0}, {"Coda", "ending", 0}, {"尾声", "ending", 0},
		{"Interlude", "other", 0}, {"Musik", "other", 0}, {"Instrumental", "other", 0}, {"间奏", "other", 0},
	} {
		l, ok := paste.ParseLabel(tc.line)
		if !ok || l.Kind != tc.kind || l.Number != tc.num || l.Rest != "" {
			t.Errorf("ParseLabel(%q) = %+v, %v; want %s %d", tc.line, l, ok, tc.kind, tc.num)
		}
	}
	for _, line := range []string{"Besar setia-Mu", "Chorus of angels sing", "Tagihan", "V", "0.", "Verse one", "Reffrain", "Hallelujah"} {
		if l, ok := paste.ParseLabel(line); ok {
			t.Errorf("ParseLabel(%q) = %+v, want no label", line, l)
		}
	}
	// Lyrics on the label's line.
	l, ok := paste.ParseLabel("Bait 1: Besar setia-Mu")
	if !ok || l.Kind != domain.SectionVerse || l.Number != 1 || l.Rest != "Besar setia-Mu" {
		t.Errorf("label with text = %+v, %v", l, ok)
	}
	l, ok = paste.ParseLabel("1. Besar setia-Mu")
	if !ok || l.Rest != "Besar setia-Mu" {
		t.Errorf("numbered label with text = %+v, %v", l, ok)
	}
}

func TestLabelWithTextAndCorners(t *testing.T) {
	c := parse(t, "Bait 1: Besar setia-Mu\nTuhan\n\nChorus 2\nx\n\nReff\n\nReff\nx")
	if got := shape(c.Draft); got != "v1=Besar setia-Mu/Tuhan c=x" {
		t.Errorf("sections = %q", got)
	}
	if got := arrangement(c.Draft); got != "v1 c c c" {
		t.Errorf("arrangement = %q", got)
	}
	if !contains(c.Warnings, domain.WarnNumberDropped) {
		t.Errorf("warnings = %v, want number_dropped", c.Warnings)
	}
	// An unknown label stays text.
	c = parse(t, "Kata pengantar\nlalu lirik")
	if got := shape(c.Draft); got != "v1=Kata pengantar/lalu lirik" {
		t.Errorf("unknown label: %q", got)
	}
}

// TC-I-003: verse numbering.
func TestVerseNumbering(t *testing.T) {
	c := parse(t, "1. a\n\n3. b\n\nc")
	if got := shape(c.Draft); got != "v1=a v3=b v4=c" {
		t.Errorf("kept and counted on: %q", got)
	}
	c = parse(t, "1. a\n\n1. b\n\n2. c")
	if got := shape(c.Draft); got != "v1=a v3=b v2=c" {
		t.Errorf("duplicate renumbered: %q", got)
	}
	if !contains(c.Warnings, domain.WarnVerseRenumbered) {
		t.Errorf("warnings = %v, want verse_renumbered", c.Warnings)
	}
	// The same verse written twice is a repeat, not a second verse.
	c = parse(t, "1. a\n\n2. b\n\n1. a")
	if got := shape(c.Draft); got != "v1=a v2=b" || arrangement(c.Draft) != "v1 v2 v1" {
		t.Errorf("repeated verse: %q / %q", shape(c.Draft), arrangement(c.Draft))
	}
}

func TestTooManySectionsAndNoSong(t *testing.T) {
	var in strings.Builder
	for i := range 61 {
		in.WriteString("unique line " + strings.Repeat("z", i+1) + "\n\n")
	}
	got, err := paste.Importer{}.Parse(context.Background(), strings.NewReader(in.String()),
		app.ImportHint{Language: "id", Name: "T"})
	if err != nil || len(got) != 1 || got[0].Reject != app.ImportTooManySections {
		t.Errorf("61 sections: %v, %+v", err, got)
	}
	_, err = paste.Importer{}.Parse(context.Background(), strings.NewReader("  \n\n\t\n"), app.ImportHint{Language: "id", Name: "T"})
	var bad *app.ImportUnreadableError
	if !errors.As(err, &bad) || bad.Reason != app.ImportNoSong {
		t.Errorf("empty text: %v", err)
	}
	_, err = paste.Importer{}.Parse(context.Background(), strings.NewReader("a\xffb"), app.ImportHint{Language: "id", Name: "T"})
	if !errors.As(err, &bad) || bad.Reason != app.ImportNotUTF8 {
		t.Errorf("invalid UTF-8: %v", err)
	}
	c := parse(t, "\xEF\xBB\xBFa")
	if shape(c.Draft) != "v1=a" {
		t.Errorf("BOM kept: %q", shape(c.Draft))
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
