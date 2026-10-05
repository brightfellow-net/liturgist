// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strconv"
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
