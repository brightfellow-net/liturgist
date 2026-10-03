// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

func existingSong() domain.Song {
	return domain.Song{
		ID: "S1", Language: "id", Title: "Besar Setia-Mu", AltTitles: []string{"Great Is Thy Faithfulness"},
		Lyricist: "Thomas Chisholm", DefaultKey: "",
		Sections: []domain.Section{
			{ID: "V1", Kind: domain.SectionVerse, Number: 1, Text: "old one"},
			{ID: "C1", Kind: domain.SectionChorus, Text: "old chorus"},
			{ID: "V2", Kind: domain.SectionVerse, Number: 2, Text: "old two"},
			{ID: "B1", Kind: domain.SectionBridge, Text: "old bridge"},
		},
		DefaultArrangement: []domain.SectionID{"V1", "C1", "V2", "C1"},
		Version:            3,
	}
}

func draftOf(secs ...domain.DraftSection) domain.SongDraft {
	return domain.SongDraft{Language: "id", Title: "Besar setia Mu", AltTitles: []string{"Tuhan Setia"}, Sections: secs, DefaultArrangement: []int{}}
}

func verse(n int, text string) domain.DraftSection {
	return domain.DraftSection{Kind: domain.SectionVerse, Number: n, Text: text}
}

func of(kind domain.SectionKind, text string) domain.DraftSection {
	return domain.DraftSection{Kind: kind, Text: text}
}

// summary renders the plan as "status:id" in order.
func summary(p domain.MergePlan) string {
	var out []string
	for _, l := range p.Lines {
		id := string(l.ID)
		if id == "" {
			id = "-"
		}
		out = append(out, l.Status+":"+id)
	}
	return strings.Join(out, " ")
}

// TC-I-007: matching rules of 08 §3.2.
func TestPlanMergeMatching(t *testing.T) {
	for _, tc := range []struct {
		name   string
		draft  domain.SongDraft
		remove bool
		want   string
	}{
		{"verses by number, single chorus matches, others kept",
			draftOf(verse(1, "new one"), of(domain.SectionChorus, "new chorus"), verse(2, "new two")), false,
			"updated:V1 updated:C1 updated:V2 kept:B1"},
		{"new verse number is new", draftOf(verse(3, "three")), false, "new:- kept:V1 kept:C1 kept:V2 kept:B1"},
		{"removal only when ticked", draftOf(verse(1, "new one")), true, "updated:V1 removed:C1 removed:V2 removed:B1"},
		{"kept sections come after, in their old order", draftOf(verse(2, "x")), false, "updated:V2 kept:V1 kept:C1 kept:B1"},
		{"a kind absent from the song is new", draftOf(of(domain.SectionIntro, "intro")), false, "new:- kept:V1 kept:C1 kept:V2 kept:B1"},
		{"a single bridge matches", draftOf(of(domain.SectionBridge, "new bridge")), false, "updated:B1 kept:V1 kept:C1 kept:V2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := summary(domain.PlanMerge(existingSong(), tc.draft, tc.remove)); got != tc.want {
				t.Errorf("plan = %s\nwant   %s", got, tc.want)
			}
		})
	}
}

func TestPlanMergeRepeatedKinds(t *testing.T) {
	two := existingSong()
	two.Sections = append(two.Sections, domain.Section{ID: "C2", Kind: domain.SectionChorus, Text: "second chorus"})

	// Two choruses on the song's side: only identical folded text matches.
	got := summary(domain.PlanMerge(two, draftOf(of(domain.SectionChorus, "Second  CHORUS!")), false))
	if got != "updated:C2 kept:V1 kept:C1 kept:V2 kept:B1" {
		t.Errorf("identical text matches: %s", got)
	}
	got = summary(domain.PlanMerge(two, draftOf(of(domain.SectionChorus, "something else")), false))
	if got != "new:- kept:V1 kept:C1 kept:V2 kept:B1 kept:C2" {
		t.Errorf("different text does not match: %s", got)
	}
	// Equal counts (2 and 2) with no identical text: no match at all.
	got = summary(domain.PlanMerge(two, draftOf(of(domain.SectionChorus, "a"), of(domain.SectionChorus, "b")), false))
	if got != "new:- new:- kept:V1 kept:C1 kept:V2 kept:B1 kept:C2" {
		t.Errorf("equal counts, no identical text: %s", got)
	}
	// Two draft choruses against one existing: also only by text.
	got = summary(domain.PlanMerge(existingSong(), draftOf(of(domain.SectionChorus, "old chorus"), of(domain.SectionChorus, "other")), false))
	if got != "updated:C1 new:- kept:V1 kept:V2 kept:B1" {
		t.Errorf("one existing, two drafts: %s", got)
	}
	// An existing section is never matched twice.
	got = summary(domain.PlanMerge(existingSong(), draftOf(of(domain.SectionChorus, "old chorus"), of(domain.SectionChorus, "old chorus")), false))
	if got != "updated:C1 new:- kept:V1 kept:V2 kept:B1" {
		t.Errorf("same text twice: %s", got)
	}
}

func TestPlanMergeTextAndMetadata(t *testing.T) {
	d := draftOf(verse(1, "new one"))
	d.Lyricist, d.Composer, d.DefaultKey, d.CopyrightLine = "Someone Else", "W. Runyan", "G", "© 1923"
	d.HymnalSource, d.HymnalNumber = "Pelengkap", "12"
	p := domain.PlanMerge(existingSong(), d, false)
	m := p.Metadata
	if m.Lyricist != "Thomas Chisholm" {
		t.Errorf("a filled field was overwritten: %q", m.Lyricist)
	}
	if m.Composer != "W. Runyan" || m.DefaultKey != "G" || m.CopyrightLine != "© 1923" || m.HymnalSource != "Pelengkap" || m.HymnalNumber != "12" {
		t.Errorf("empty fields not filled: %+v", m)
	}
	// Alternative titles are united without duplicates (folded), and the title itself is not added.
	if !slices.Equal(m.AltTitles, []string{"Great Is Thy Faithfulness", "Tuhan Setia"}) {
		t.Errorf("alt titles = %v", m.AltTitles)
	}
	if m.Title != "Besar Setia-Mu" {
		t.Errorf("title = %q", m.Title)
	}
	// Matched sections carry old and new text; kept ones have the old text.
	first := p.Lines[0]
	if first.OldText != "old one" || first.NewText != "new one" || first.ID != "V1" {
		t.Errorf("first line = %+v", first)
	}
	// The input song is not changed.
	if existingSong().Lyricist != "Thomas Chisholm" || len(existingSong().AltTitles) != 1 {
		t.Error("PlanMerge changed its input")
	}
	// A hymnal source alone in the song blocks the draft's number.
	e := existingSong()
	e.HymnalSource, e.HymnalNumber = "KJ", "5"
	if m := domain.PlanMerge(e, d, false).Metadata; m.HymnalSource != "KJ" || m.HymnalNumber != "5" {
		t.Errorf("hymnal overwritten: %+v", m)
	}
}

func TestPlanMergeIsDeterministic(t *testing.T) {
	d := draftOf(verse(2, "b"), of(domain.SectionChorus, "c"), verse(1, "a"), of(domain.SectionOther, "o"))
	first := summary(domain.PlanMerge(existingSong(), d, false))
	for range 20 {
		if got := summary(domain.PlanMerge(existingSong(), d, false)); got != first {
			t.Fatalf("plan changed between runs: %s vs %s", got, first)
		}
	}
	if p := domain.PlanMerge(existingSong(), d, false); len(p.Result()) != 5 {
		t.Errorf("result has %d sections", len(p.Result()))
	}
}

func TestValidateDraft(t *testing.T) {
	d := draftOf(verse(1, "one\r\n\r\n\r\ntwo  \n"), of(domain.SectionChorus, "x"))
	d.DefaultArrangement = []int{0, 1, 0}
	if err := domain.ValidateDraft(&d); err != nil {
		t.Fatalf("valid draft: %v", err)
	}
	if d.Sections[0].Text != "one\n\n\ntwo" { // blank lines inside a section stay (06 §2.2)
		t.Errorf("text not normalised: %q", d.Sections[0].Text)
	}
	for name, mutate := range map[string]func(*domain.SongDraft){
		"no title":          func(d *domain.SongDraft) { d.Title = " " },
		"long title":        func(d *domain.SongDraft) { d.Title = strings.Repeat("x", 201) },
		"bad language":      func(d *domain.SongDraft) { d.Language = "fr" },
		"empty section":     func(d *domain.SongDraft) { d.Sections[1].Text = "" },
		"verse number 0":    func(d *domain.SongDraft) { d.Sections[0].Number = 0 },
		"arrangement range": func(d *domain.SongDraft) { d.DefaultArrangement = []int{5} },
		"arrangement neg":   func(d *domain.SongDraft) { d.DefaultArrangement = []int{-1} },
		"bad key":           func(d *domain.SongDraft) { d.DefaultKey = "H" },
		"61 sections": func(d *domain.SongDraft) {
			for i := range 60 {
				d.Sections = append(d.Sections, of(domain.SectionOther, "s"+strings.Repeat("a", i)))
			}
		},
		"too large": func(d *domain.SongDraft) { d.LicenceNotes = strings.Repeat("é", domain.MaxImportDraftBytes) },
	} {
		t.Run(name, func(t *testing.T) {
			x := draftOf(verse(1, "one"), of(domain.SectionChorus, "x"))
			mutate(&x)
			if err := domain.ValidateDraft(&x); err == nil {
				t.Error("invalid draft accepted")
			}
		})
	}
}

func TestCandidateTerminal(t *testing.T) {
	for _, tc := range []struct {
		c    domain.ImportCandidate
		want bool
	}{
		{domain.ImportCandidate{Decision: domain.DecisionPending}, false},
		{domain.ImportCandidate{Decision: domain.DecisionAccept}, false},
		{domain.ImportCandidate{Decision: domain.DecisionAccept, Outcome: domain.OutcomeFailed}, false},
		{domain.ImportCandidate{Decision: domain.DecisionAccept, Outcome: domain.OutcomeApplied}, true},
		{domain.ImportCandidate{Decision: domain.DecisionSkip}, true},
	} {
		if got := tc.c.Terminal(); got != tc.want {
			t.Errorf("%+v Terminal = %v", tc.c, got)
		}
	}
}
