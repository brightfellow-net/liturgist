// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package paste

import (
	"strings"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain"
)

// Item is one block of lyrics: a section the writer marked with a label, or
// an unlabelled block. A labelled item with no text repeats an earlier
// section with the same label.
type Item struct {
	Labelled  bool
	Kind      domain.SectionKind
	Number    int  // as written; 0 = none
	NumberSet bool // a number was written
	Label     string
	Text      string
}

// Result is the sections, arrangement and warnings of Build.
type Result struct {
	Sections    []domain.DraftSection
	Arrangement []int
	Warnings    []string
	TooMany     bool // more than 60 sections
}

// Blocks splits text into blocks at blank lines and reads each block's label
// (08 §4.1 rules 1 and 2). Text is already UTF-8.
func Blocks(text string) []Item {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var items []Item
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		items = append(items, blockItem(cur))
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return items
}

func blockItem(lines []string) Item {
	l, ok := ParseLabel(lines[0])
	if !ok {
		return Item{Text: strings.Join(lines, "\n")}
	}
	rest := lines[1:]
	if l.Rest != "" {
		rest = append([]string{l.Rest}, rest...)
	}
	return Item{Labelled: true, Kind: l.Kind, Number: l.Number, NumberSet: l.NumberSet, Text: strings.Join(rest, "\n")}
}

type labelKey struct {
	kind   domain.SectionKind
	number int
}

// Build turns blocks into sections and an arrangement (08 §4.1 rules 3 to 6).
func Build(items []Item, language string) Result {
	var r Result
	warn := func(code string) {
		for _, w := range r.Warnings {
			if w == code {
				return
			}
		}
		r.Warnings = append(r.Warnings, code)
	}
	fold := func(s string) string { return domain.FoldFor(language, s) }

	// Highest verse number the writer used; unlabelled verses count on from it.
	maxVerse := 0
	for _, it := range items {
		if it.Labelled && it.Kind == domain.SectionVerse && it.Number > maxVerse {
			maxVerse = it.Number
		}
	}
	// How often each unlabelled text occurs (rule 4).
	unlabelled := map[string]int{}
	for _, it := range items {
		if !it.Labelled && fold(it.Text) != "" {
			unlabelled[fold(it.Text)]++
		}
	}

	byText := map[string]int{}    // folded text → section index
	byLabel := map[labelKey]int{} // written label → section index
	usedVerse := map[int]bool{}   // verse numbers given out
	repeated := false
	add := func(sec domain.DraftSection, key *labelKey) int {
		r.Sections = append(r.Sections, sec)
		i := len(r.Sections) - 1
		byText[fold(sec.Text)] = i
		if key != nil {
			if _, ok := byLabel[*key]; !ok {
				byLabel[*key] = i
			}
		}
		r.Arrangement = append(r.Arrangement, i)
		return i
	}
	repeat := func(i int) {
		repeated = true
		r.Arrangement = append(r.Arrangement, i)
	}
	nextVerse := func() int {
		maxVerse++
		return maxVerse
	}

	for _, it := range items {
		text := domain.NormalizeLyrics(it.Text)
		if it.Labelled {
			key := labelKey{it.Kind, it.Number}
			if it.Kind != domain.SectionVerse {
				key.number = 0
			}
			if fold(text) == "" {
				if i, ok := byLabel[key]; ok {
					repeat(i)
				}
				continue
			}
			if i, ok := byLabel[key]; ok && fold(r.Sections[i].Text) == fold(text) {
				repeat(i)
				continue
			}
			sec := domain.DraftSection{Kind: it.Kind, Label: it.Label, Text: text}
			switch {
			case it.Kind != domain.SectionVerse:
				if it.NumberSet {
					warn(domain.WarnNumberDropped)
				}
			case !it.NumberSet || usedVerse[it.Number]:
				sec.Number = nextVerse()
				if it.NumberSet {
					warn(domain.WarnVerseRenumbered)
				} else {
					warn(domain.WarnBlocksNumbered)
				}
			default:
				sec.Number = it.Number
			}
			if sec.Kind == domain.SectionVerse {
				usedVerse[sec.Number] = true
			}
			add(sec, &key)
			continue
		}
		f := fold(text)
		if f == "" {
			continue
		}
		if i, ok := byText[f]; ok {
			repeat(i)
			continue
		}
		if unlabelled[f] >= 2 {
			warn(domain.WarnChorusGuessed)
			add(domain.DraftSection{Kind: domain.SectionChorus, Text: text}, nil)
			continue
		}
		warn(domain.WarnBlocksNumbered)
		n := nextVerse()
		usedVerse[n] = true
		add(domain.DraftSection{Kind: domain.SectionVerse, Number: n, Text: text}, nil)
	}
	if !repeated {
		r.Arrangement = nil
	}
	r.TooMany = len(r.Sections) > domain.MaxImportSections
	return r
}

// CharCount is a helper for callers that limit pasted text.
func CharCount(s string) int { return utf8.RuneCountInString(s) }
