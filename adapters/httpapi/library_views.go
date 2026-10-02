// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"time"

	"github.com/brightfellow-net/liturgist/app"
)

// JSON shapes of the song library (06 §3).

// SongSummaryView is a song in a list (no lyrics).
type SongSummaryView struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	AltTitles     []string        `json:"alt_titles"`
	Language      string          `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	HymnalSource  string          `json:"hymnal_source"`
	HymnalNumber  string          `json:"hymnal_number"`
	LicenceStatus string          `json:"licence_status" enum:"unknown,public_domain,church_licence,permission_obtained"`
	HasGroup      bool            `json:"has_group"`
	Actions       app.SongActions `json:"actions"`
}

// SectionView is one section of a song, in order.
type SectionView struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind" enum:"verse,pre_chorus,chorus,bridge,tag,intro,ending,other"`
	Number *int    `json:"number" doc:"verses only"`
	Label  *string `json:"label" doc:"null: derive from kind and number"`
	Text   string  `json:"text"`
}

// SongRefView names another-language version of a song.
type SongRefView struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Language string `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
}

// SongView is a whole song.
type SongView struct {
	ID                 string          `json:"id"`
	Language           string          `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Title              string          `json:"title"`
	AltTitles          []string        `json:"alt_titles"`
	HymnalSource       string          `json:"hymnal_source"`
	HymnalNumber       string          `json:"hymnal_number"`
	Lyricist           string          `json:"lyricist"`
	Composer           string          `json:"composer"`
	Translator         string          `json:"translator"`
	DefaultKey         string          `json:"default_key"`
	CopyrightHolder    string          `json:"copyright_holder"`
	CopyrightLine      string          `json:"copyright_line"`
	CCLISongNumber     string          `json:"ccli_song_number"`
	LicenceStatus      string          `json:"licence_status" enum:"unknown,public_domain,church_licence,permission_obtained"`
	LicenceNotes       string          `json:"licence_notes"`
	Sections           []SectionView   `json:"sections"`
	DefaultArrangement []string        `json:"default_arrangement" doc:"section IDs; empty: none defined, use all sections in order"`
	Versions           []SongRefView   `json:"versions" doc:"the other-language versions linked to this song"`
	Version            int             `json:"version" doc:"send it back with every change"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	Actions            app.SongActions `json:"actions"`
}

func songSummaryView(s app.SongSummary) SongSummaryView {
	alts := s.AltTitles
	if alts == nil {
		alts = []string{}
	}
	return SongSummaryView{ID: string(s.ID), Title: s.Title, AltTitles: alts, Language: s.Language, HymnalSource: s.HymnalSource,
		HymnalNumber: s.HymnalNumber, LicenceStatus: string(s.LicenceStatus), HasGroup: s.HasGroup, Actions: s.Actions}
}

func songView(v app.SongView) SongView {
	s := v.Song
	out := SongView{ID: string(s.ID), Language: s.Language, Title: s.Title, AltTitles: s.AltTitles, HymnalSource: s.HymnalSource,
		HymnalNumber: s.HymnalNumber, Lyricist: s.Lyricist, Composer: s.Composer, Translator: s.Translator,
		DefaultKey: s.DefaultKey, CopyrightHolder: s.CopyrightHolder, CopyrightLine: s.CopyrightLine,
		CCLISongNumber: s.CCLISongNumber, LicenceStatus: string(s.LicenceStatus), LicenceNotes: s.LicenceNotes,
		Sections: make([]SectionView, len(s.Sections)), DefaultArrangement: make([]string, len(s.DefaultArrangement)),
		Versions: make([]SongRefView, len(v.Versions)), Version: s.Version, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		Actions: v.Actions}
	if out.AltTitles == nil {
		out.AltTitles = []string{}
	}
	for i, sec := range s.Sections {
		sv := SectionView{ID: string(sec.ID), Kind: string(sec.Kind), Label: optional(sec.Label), Text: sec.Text}
		if sec.Number != 0 {
			n := sec.Number
			sv.Number = &n
		}
		out.Sections[i] = sv
	}
	for i, id := range s.DefaultArrangement {
		out.DefaultArrangement[i] = string(id)
	}
	for i, r := range v.Versions {
		out.Versions[i] = SongRefView{ID: string(r.ID), Title: r.Title, Language: r.Language}
	}
	return out
}
