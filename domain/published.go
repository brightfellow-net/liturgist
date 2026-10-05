// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strconv"
	"strings"
	"time"
)

// PublishedFormat is the format number of PublishedContent (13 §3).
const PublishedFormat = 1

// MaxPublishedBytes caps the stored content of one version (13 §2).
const MaxPublishedBytes = 4 << 20

// PublishedVersionID identifies one published version.
type PublishedVersionID string

// PublishedVersion is one immutable publication of a liturgy (13 §3). Content
// holds the JSON of a PublishedContent exactly as stored.
type PublishedVersion struct {
	ID          PublishedVersionID
	LiturgyID   LiturgyID
	Number      int // 1 for the first publication, then +1
	Content     []byte
	PublishedBy UserID
	PublishedAt time.Time
}

// PublishedContent is the self-contained copy of a liturgy as published: it
// needs no other table to be shown (13 §3).
type PublishedContent struct {
	Format        int                   `json:"format"`
	Liturgy       PublishedLiturgy      `json:"liturgy"`
	Items         []PublishedItem       `json:"items"`
	Assignments   []PublishedAssignment `json:"assignments"`
	LicenceFooter string                `json:"licence_footer"`
}

// PublishedLiturgy is the header of the copy.
type PublishedLiturgy struct {
	Date        string `json:"date"`
	Time        string `json:"time"`
	ServiceName string `json:"service_name"`
	Language    string `json:"language"`
	ChurchName  string `json:"church_name"`
}

// PublishedRef is a duty or singing part as it was named when published.
type PublishedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PublishedItem is one item of the copy.
type PublishedItem struct {
	ID       ItemID            `json:"id"`
	Position int               `json:"position"`
	Type     ItemType          `json:"type"`
	Title    string            `json:"title"`
	Duty     *PublishedRef     `json:"duty"`
	Text     string            `json:"text,omitempty"`
	Reading  *PublishedReading `json:"reading"`
	Songs    []PublishedSong   `json:"songs,omitempty"`
}

// PublishedReading is the reading of a reading item, with its text.
type PublishedReading struct {
	ReferenceDisplay string `json:"reference_display"`
	TranslationCode  string `json:"translation_code"`
	Text             string `json:"text"`
	Attribution      string `json:"attribution"`
}

// PublishedSong is one song of an item with the sections its sequence uses,
// each stored once, and the sequence itself.
type PublishedSong struct {
	SongID          SongID             `json:"song_id"`
	Title           string             `json:"title"`
	HymnalSource    string             `json:"hymnal_source"`
	HymnalNumber    string             `json:"hymnal_number"`
	Key             string             `json:"key"`
	Note            string             `json:"note"`
	CopyrightHolder string             `json:"copyright_holder"`
	CopyrightLine   string             `json:"copyright_line"`
	CCLISongNumber  string             `json:"ccli_song_number"`
	Sections        []PublishedSection `json:"sections"`
	Entries         []PublishedEntry   `json:"entries"`
}

// PublishedSection is a section's text and label, fixed at publishing.
type PublishedSection struct {
	ID     SectionID   `json:"id"`
	Kind   SectionKind `json:"kind"`
	Number int         `json:"number"`
	Label  string      `json:"label"`
	Text   string      `json:"text"`
}

// PublishedEntry is one entry of the sequence.
type PublishedEntry struct {
	SectionID SectionID     `json:"section_id"`
	Part      *PublishedRef `json:"part"`
	KeyChange string        `json:"key_change"`
	Note      string        `json:"note"`
}

// PublishedAssignment is a person on a duty, named as shown when published.
type PublishedAssignment struct {
	Duty   PublishedRef `json:"duty"`
	UserID UserID       `json:"user_id,omitempty"`
	Name   string       `json:"name"`
}

// sectionNames are the words of the derived section labels per liturgy language.
// Only English and Indonesian exist; other languages use English until their
// words are reviewed (13 §3). The Indonesian words are a draft for the owner.
var sectionNames = map[string]map[SectionKind]string{
	"en": {SectionVerse: "Verse", SectionPreChorus: "Pre-chorus", SectionChorus: "Chorus", SectionBridge: "Bridge",
		SectionTag: "Tag", SectionIntro: "Intro", SectionEnding: "Ending", SectionOther: "Section"},
	"id": {SectionVerse: "Bait", SectionPreChorus: "Pra-refren", SectionChorus: "Refren", SectionBridge: "Jembatan",
		SectionTag: "Tag", SectionIntro: "Intro", SectionEnding: "Penutup", SectionOther: "Bagian"},
}

// SectionLabelIn is the label of a section in a liturgy's language: its own
// label, or one derived from kind and number (13 §3).
func SectionLabelIn(sec Section, language string) string {
	if sec.Label != "" {
		return sec.Label
	}
	names, ok := sectionNames[language]
	if !ok {
		names = sectionNames["en"]
	}
	name, ok := names[sec.Kind]
	if !ok {
		name = names[SectionOther]
	}
	if sec.Kind == SectionVerse && sec.Number > 0 {
		return name + " " + strconv.Itoa(sec.Number)
	}
	return name
}

// Changes is what a republished version changed for the team (13 §8). Text
// edits (lyrics, the words of a reading, free text) are not reported: they
// cannot be sent in a message.
type Changes struct {
	Items       []ItemChange
	Songs       []SongChange
	Reading     []ReadingChange
	Assignments []AssignmentChange
}

// Empty reports whether nothing the team needs to know changed.
func (c Changes) Empty() bool {
	return len(c.Items)+len(c.Songs)+len(c.Reading)+len(c.Assignments) == 0
}

// Item change kinds.
const (
	ItemAdded       = "added"
	ItemRemoved     = "removed"
	ItemMoved       = "moved"
	ItemRetitled    = "retitled"
	ItemDutyChanged = "duty_changed"
)

// ItemChange is one change to an item. Title is the item's title now (or, for
// a removed item, before); OldTitle is set for a retitled item. Duty and
// OldDuty are the duty names (nil for none) for an added, removed or
// duty-changed item.
type ItemChange struct {
	Kind     string
	ItemID   ItemID
	Title    string
	OldTitle string
	Duty     *PublishedRef
	OldDuty  *PublishedRef
}

// Song change kinds.
const (
	SongAdded      = "added"
	SongRemoved    = "removed"
	SongKeyChanged = "key_changed"
)

// SongChange is one change to a song inside an item present in both versions.
// For SongKeyChanged, OldKey and NewKey are the song's keys, and EntryKeys is
// true when the key changes within the song (key_change of its entries) differ.
type SongChange struct {
	Kind      string
	ItemID    ItemID
	ItemTitle string
	Song      PublishedSong
	OldKey    string
	NewKey    string
	EntryKeys bool
}

// ReadingChange is a change of the reading of an item present in both
// versions: an empty Old or New means there was none.
type ReadingChange struct {
	ItemID    ItemID
	ItemTitle string
	Old       string
	New       string
}

// Assignment change kinds.
const (
	AssignmentAdded   = "added"
	AssignmentRemoved = "removed"
)

// AssignmentChange is a person added to or removed from a duty.
type AssignmentChange struct {
	Kind       string
	Duty       PublishedRef
	Assignment PublishedAssignment
}

// Compare reports what latest changed against previous (13 §8). Items are
// matched by ID, songs by song ID within an item (the n-th occurrence of a song
// matches the n-th), and assignments by duty ID and person: the user ID when
// set, otherwise the free text, trimmed and case-folded.
func Compare(previous, latest PublishedContent) Changes {
	var ch Changes
	old := map[ItemID]PublishedItem{}
	for _, it := range previous.Items {
		old[it.ID] = it
	}
	now := map[ItemID]PublishedItem{}
	for _, it := range latest.Items {
		now[it.ID] = it
	}
	for _, it := range latest.Items {
		if _, ok := old[it.ID]; !ok {
			ch.Items = append(ch.Items, ItemChange{Kind: ItemAdded, ItemID: it.ID, Title: it.Title, Duty: it.Duty})
		}
	}
	for _, it := range previous.Items {
		if _, ok := now[it.ID]; !ok {
			ch.Items = append(ch.Items, ItemChange{Kind: ItemRemoved, ItemID: it.ID, Title: it.Title, Duty: it.Duty})
		}
	}
	// Order of the items present in both versions, each in its own order.
	var before, after []ItemID
	for _, it := range previous.Items {
		if _, ok := now[it.ID]; ok {
			before = append(before, it.ID)
		}
	}
	for _, it := range latest.Items {
		if _, ok := old[it.ID]; ok {
			after = append(after, it.ID)
		}
	}
	stays := longestCommon(before, after)
	for _, it := range latest.Items {
		o, ok := old[it.ID]
		if !ok {
			continue
		}
		if !stays[it.ID] {
			ch.Items = append(ch.Items, ItemChange{Kind: ItemMoved, ItemID: it.ID, Title: it.Title})
		}
		if o.Title != it.Title {
			ch.Items = append(ch.Items, ItemChange{Kind: ItemRetitled, ItemID: it.ID, Title: it.Title, OldTitle: o.Title})
		}
		if refID(o.Duty) != refID(it.Duty) {
			ch.Items = append(ch.Items, ItemChange{Kind: ItemDutyChanged, ItemID: it.ID, Title: it.Title, Duty: it.Duty, OldDuty: o.Duty})
		}
		ch.Songs = append(ch.Songs, compareSongs(o, it)...)
		if or, nr := readingRef(o.Reading), readingRef(it.Reading); or != nr {
			ch.Reading = append(ch.Reading, ReadingChange{ItemID: it.ID, ItemTitle: it.Title, Old: or, New: nr})
		}
	}
	ch.Assignments = compareAssignments(previous.Assignments, latest.Assignments)
	return ch
}

func refID(r *PublishedRef) string {
	if r == nil {
		return ""
	}
	return r.ID
}

func readingRef(r *PublishedReading) string {
	if r == nil {
		return ""
	}
	if r.TranslationCode == "" {
		return r.ReferenceDisplay
	}
	return r.ReferenceDisplay + " (" + r.TranslationCode + ")"
}

// longestCommon returns the items of a longest common subsequence of a and b
// (both permutations of the same set): those whose relative order is kept.
func longestCommon(a, b []ItemID) map[ItemID]bool {
	n, m := len(a), len(b)
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	keep := map[ItemID]bool{}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			keep[a[i]] = true
			i++
			j++
		case l[i+1][j] >= l[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keep
}

// songKey is the n-th occurrence of a song in an item.
type songKey struct {
	id SongID
	n  int
}

func songsByKey(songs []PublishedSong) (map[songKey]PublishedSong, []songKey) {
	seen := map[SongID]int{}
	m := map[songKey]PublishedSong{}
	var order []songKey
	for _, s := range songs {
		k := songKey{s.SongID, seen[s.SongID]}
		seen[s.SongID]++
		m[k] = s
		order = append(order, k)
	}
	return m, order
}

func compareSongs(o, n PublishedItem) []SongChange {
	oldSongs, oldOrder := songsByKey(o.Songs)
	newSongs, newOrder := songsByKey(n.Songs)
	var out []SongChange
	for _, k := range newOrder {
		s := newSongs[k]
		prev, ok := oldSongs[k]
		switch {
		case !ok:
			out = append(out, SongChange{Kind: SongAdded, ItemID: n.ID, ItemTitle: n.Title, Song: s, NewKey: s.Key})
		case prev.Key != s.Key || !equalStrings(entryKeys(prev), entryKeys(s)):
			out = append(out, SongChange{Kind: SongKeyChanged, ItemID: n.ID, ItemTitle: n.Title, Song: s, OldKey: prev.Key, NewKey: s.Key,
				EntryKeys: !equalStrings(entryKeys(prev), entryKeys(s))})
		}
	}
	for _, k := range oldOrder {
		if _, ok := newSongs[k]; !ok {
			s := oldSongs[k]
			out = append(out, SongChange{Kind: SongRemoved, ItemID: n.ID, ItemTitle: n.Title, Song: s, OldKey: s.Key})
		}
	}
	return out
}

// entryKeys are the key changes of a song's sequence, in order, ignoring entries without one.
func entryKeys(s PublishedSong) []string {
	var out []string
	for _, e := range s.Entries {
		if e.KeyChange != "" {
			out = append(out, e.KeyChange)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type personOnDuty struct{ duty, person string }

func personKey(a PublishedAssignment) personOnDuty {
	if a.UserID != "" {
		return personOnDuty{a.Duty.ID, "u:" + string(a.UserID)}
	}
	return personOnDuty{a.Duty.ID, "n:" + strings.ToLower(strings.TrimSpace(a.Name))}
}

func compareAssignments(previous, latest []PublishedAssignment) []AssignmentChange {
	had := map[personOnDuty]bool{}
	for _, a := range previous {
		had[personKey(a)] = true
	}
	has := map[personOnDuty]bool{}
	for _, a := range latest {
		has[personKey(a)] = true
	}
	var out []AssignmentChange
	done := map[personOnDuty]bool{}
	for _, a := range latest {
		if k := personKey(a); !had[k] && !done[k] {
			done[k] = true
			out = append(out, AssignmentChange{Kind: AssignmentAdded, Duty: a.Duty, Assignment: a})
		}
	}
	for _, a := range previous {
		if k := personKey(a); !has[k] && !done[k] {
			done[k] = true
			out = append(out, AssignmentChange{Kind: AssignmentRemoved, Duty: a.Duty, Assignment: a})
		}
	}
	return out
}
