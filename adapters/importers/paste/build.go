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

// builder holds what Build has worked out so far.
type builder struct {
	language   string
	r          Result
	maxVerse   int              // highest verse number written or given out
	unlabelled map[string]int   // how often each unlabelled text occurs (rule 4)
	byText     map[string]int   // folded text → section index
	byLabel    map[labelKey]int // written label → section index
	usedVerse  map[int]bool     // verse numbers given out
	repeated   bool
}

func (b *builder) fold(s string) string { return domain.FoldFor(b.language, s) }

func (b *builder) warn(code string) {
	for _, w := range b.r.Warnings {
		if w == code {
			return
		}
	}
	b.r.Warnings = append(b.r.Warnings, code)
}

// add appends a section and puts it in the arrangement.
func (b *builder) add(sec domain.DraftSection, key *labelKey) {
	b.r.Sections = append(b.r.Sections, sec)
	i := len(b.r.Sections) - 1
	b.byText[b.fold(sec.Text)] = i
	if key != nil {
		if _, ok := b.byLabel[*key]; !ok {
			b.byLabel[*key] = i
		}
	}
	b.r.Arrangement = append(b.r.Arrangement, i)
}

// repeat puts an earlier section in the arrangement again.
func (b *builder) repeat(i int) {
	b.repeated = true
	b.r.Arrangement = append(b.r.Arrangement, i)
}

func (b *builder) nextVerse() int {
	b.maxVerse++
	return b.maxVerse
}

// Build turns blocks into sections and an arrangement (08 §4.1 rules 3 to 6).
func Build(items []Item, language string) Result {
	b := &builder{language: language, unlabelled: map[string]int{}, byText: map[string]int{},
		byLabel: map[labelKey]int{}, usedVerse: map[int]bool{}}
	for _, it := range items {
		// Highest verse number the writer used; unlabelled verses count on from it.
		if it.Labelled && it.Kind == domain.SectionVerse && it.Number > b.maxVerse {
			b.maxVerse = it.Number
		}
		if !it.Labelled && b.fold(it.Text) != "" {
			b.unlabelled[b.fold(it.Text)]++
		}
	}
	for _, it := range items {
		text := domain.NormalizeLyrics(it.Text)
		if it.Labelled {
			b.labelled(it, text)
		} else {
			b.unlabelledBlock(text)
		}
	}
	if !b.repeated {
		b.r.Arrangement = nil
	}
	b.r.TooMany = len(b.r.Sections) > domain.MaxImportSections
	return b.r
}

// labelled handles a block the writer gave a label (rule 3).
func (b *builder) labelled(it Item, text string) {
	key := labelKey{it.Kind, it.Number}
	if it.Kind != domain.SectionVerse {
		key.number = 0
	}
	if b.fold(text) == "" {
		if i, ok := b.byLabel[key]; ok {
			b.repeat(i)
		}
		return
	}
	if i, ok := b.byLabel[key]; ok && b.fold(b.r.Sections[i].Text) == b.fold(text) {
		b.repeat(i)
		return
	}
	sec := domain.DraftSection{Kind: it.Kind, Label: it.Label, Text: text}
	b.number(&sec, it)
	if sec.Kind == domain.SectionVerse {
		b.usedVerse[sec.Number] = true
	}
	b.add(sec, &key)
}

// number gives a new section the number it keeps (rule 6): a verse keeps its
// written number unless that is missing or already taken; other kinds have none.
func (b *builder) number(sec *domain.DraftSection, it Item) {
	switch {
	case it.Kind != domain.SectionVerse:
		if it.NumberSet {
			b.warn(domain.WarnNumberDropped)
		}
	case !it.NumberSet || b.usedVerse[it.Number]:
		sec.Number = b.nextVerse()
		if it.NumberSet {
			b.warn(domain.WarnVerseRenumbered)
		} else {
			b.warn(domain.WarnBlocksNumbered)
		}
	default:
		sec.Number = it.Number
	}
}

// unlabelledBlock handles a block without a label (rules 4 and 5): a text
// seen before repeats its section, a text seen twice or more is a chorus,
// anything else is the next verse.
func (b *builder) unlabelledBlock(text string) {
	f := b.fold(text)
	if f == "" {
		return
	}
	if i, ok := b.byText[f]; ok {
		b.repeat(i)
		return
	}
	if b.unlabelled[f] >= 2 {
		b.warn(domain.WarnChorusGuessed)
		b.add(domain.DraftSection{Kind: domain.SectionChorus, Text: text}, nil)
		return
	}
	b.warn(domain.WarnBlocksNumbered)
	n := b.nextVerse()
	b.usedVerse[n] = true
	b.add(domain.DraftSection{Kind: domain.SectionVerse, Number: n, Text: text}, nil)
}

// CharCount is a helper for callers that limit pasted text.
func CharCount(s string) int { return utf8.RuneCountInString(s) }
