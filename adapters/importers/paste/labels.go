// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package paste splits pasted lyrics into sections (08 §4.1). Its label table
// and builder are also used for the loose lines of a ChordPro file.
package paste

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/brightfellow-net/liturgist/domain"
)

// labelWords is the label table of 08 §4.1: lower-case spelling → kind. The
// bare word "v" only counts with a number ("V1").
var labelWords = map[string]domain.SectionKind{
	"bait": domain.SectionVerse, "ayat": domain.SectionVerse, "verse": domain.SectionVerse, "v": domain.SectionVerse,
	"reff": domain.SectionChorus, "ref": domain.SectionChorus, "refrein": domain.SectionChorus,
	"chorus": domain.SectionChorus, "refrain": domain.SectionChorus, "副歌": domain.SectionChorus,
	"pra-reff": domain.SectionPreChorus, "pre-reff": domain.SectionPreChorus, "pre-chorus": domain.SectionPreChorus,
	"prechorus": domain.SectionPreChorus, "pre chorus": domain.SectionPreChorus, "前副歌": domain.SectionPreChorus,
	"导歌":       domain.SectionPreChorus,
	"jembatan": domain.SectionBridge, "bridge": domain.SectionBridge, "桥段": domain.SectionBridge,
	"tag":   domain.SectionTag,
	"intro": domain.SectionIntro, "前奏": domain.SectionIntro,
	"akhir": domain.SectionEnding, "outro": domain.SectionEnding, "ending": domain.SectionEnding,
	"coda": domain.SectionEnding, "尾声": domain.SectionEnding,
	"interlude": domain.SectionOther, "musik": domain.SectionOther, "instrumental": domain.SectionOther,
	"间奏": domain.SectionOther,
}

var (
	wordLabelRe = func() *regexp.Regexp {
		words := make([]string, 0, len(labelWords))
		for w := range labelWords {
			words = append(words, regexp.QuoteMeta(w))
		}
		// Longest first, so "pre-chorus" is not read as "pre" and "refrein" not as "ref".
		sort.Slice(words, func(i, j int) bool { return len(words[i]) > len(words[j]) })
		return regexp.MustCompile(`^(?i)(` + strings.Join(words, "|") + `)[ \t]*(\d{1,2})?[ \t]*([:.：])?[ \t]*(.*)$`)
	}()
	numberLabelRe  = regexp.MustCompile(`^(\d{1,2})[ \t]*[.):：][ \t]*(.*)$`)
	chineseVerseRe = regexp.MustCompile(`^第[ \t]*([一二三四五六七八九十]|\d{1,2})[ \t]*节[ \t]*[:：]?[ \t]*(.*)$`)
	chineseListRe  = regexp.MustCompile(`^([一二三四五六七八九十])、[ \t]*(.*)$`)
)

var chineseDigits = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}

// Label is a section label read from the first line of a block.
type Label struct {
	Kind      domain.SectionKind
	Number    int  // 0 = none
	NumberSet bool // a number was written
	Rest      string
}

// ParseLabel reads line as a label (08 §4.1). A word label may be followed by
// lyrics on the same line only after a ":" or ".".
func ParseLabel(line string) (Label, bool) {
	line = strings.TrimSpace(line)
	if m := numberLabelRe.FindStringSubmatch(line); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 {
			return Label{Kind: domain.SectionVerse, Number: n, NumberSet: true, Rest: strings.TrimSpace(m[2])}, true
		}
		return Label{}, false
	}
	if m := chineseVerseRe.FindStringSubmatch(line); m != nil {
		return Label{Kind: domain.SectionVerse, Number: chineseNumber(m[1]), NumberSet: true, Rest: strings.TrimSpace(m[2])}, true
	}
	if m := chineseListRe.FindStringSubmatch(line); m != nil {
		return Label{Kind: domain.SectionVerse, Number: chineseNumber(m[1]), NumberSet: true, Rest: strings.TrimSpace(m[2])}, true
	}
	m := wordLabelRe.FindStringSubmatch(line)
	if m == nil {
		return Label{}, false
	}
	kind := labelWords[strings.ToLower(m[1])]
	rest := strings.TrimSpace(m[4])
	if rest != "" && m[3] == "" {
		return Label{}, false // "Chorus of angels": lyrics, not a label
	}
	l := Label{Kind: kind, Rest: rest}
	if m[2] != "" {
		l.Number, _ = strconv.Atoi(m[2])
		l.NumberSet = true
	}
	if strings.EqualFold(m[1], "v") && !l.NumberSet {
		return Label{}, false
	}
	if l.NumberSet && l.Number < 1 && kind == domain.SectionVerse {
		return Label{}, false
	}
	return l, true
}

func chineseNumber(s string) int {
	if n, ok := chineseDigits[s]; ok {
		return n
	}
	n, _ := strconv.Atoi(s)
	return n
}
