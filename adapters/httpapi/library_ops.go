// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// LibraryDeps are what the song library operations need. Fields may be nil
// when only the OpenAPI document is built.
type LibraryDeps struct {
	Songs    *app.Songs
	Readings *app.Readings
	Imports  *app.Imports
	Log      *slog.Logger
}

type songSection struct {
	ID     string `json:"id,omitempty" maxLength:"26" doc:"an existing section of this song; omit for a new one"`
	Key    string `json:"key,omitempty" maxLength:"200" doc:"request-local name of a new section, for default_arrangement"`
	Kind   string `json:"kind" enum:"verse,pre_chorus,chorus,bridge,tag,intro,ending,other"`
	Number int    `json:"number,omitempty" minimum:"0" maximum:"1000" doc:"verses only (1-99)"`
	Label  string `json:"label,omitempty" maxLength:"1000"`
	Text   string `json:"text" maxLength:"100000"`
}

func sectionInputs(in []songSection) []app.SectionInput {
	out := make([]app.SectionInput, len(in))
	for i, s := range in {
		out[i] = app.SectionInput{ID: domain.SectionID(s.ID), Key: s.Key, Kind: domain.SectionKind(s.Kind),
			Number: s.Number, Label: s.Label, Text: s.Text}
	}
	return out
}

type songOutput struct{ Body SongView }

type songListOutput struct {
	Body struct {
		Items []SongSummaryView `json:"items"`
		Total int               `json:"total"`
	}
}

// RegisterLibrary registers the song library (06 §3) and readings (07 §4) operations.
func RegisterLibrary(api huma.API, d LibraryDeps) {
	registerReadings(api, d)
	registerImports(api, d)
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	songOp := func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), "library")
	}

	huma.Register(api, songOp("listSongs", http.MethodGet, "/songs", http.StatusOK, "Search and list songs"),
		func(ctx context.Context, in *struct {
			Q             string `query:"q" maxLength:"2000" doc:"title, hymnal number or lyrics"`
			Language      string `query:"language" maxLength:"10"`
			LicenceStatus string `query:"licence_status" maxLength:"30"`
			HymnalSource  string `query:"hymnal_source" maxLength:"100"`
			HymnalNumber  string `query:"hymnal_number" maxLength:"100" doc:"exact; needs hymnal_source"`
			Limit         int    `query:"limit" minimum:"0" maximum:"1000" doc:"default 50, at most 100"`
			Offset        int    `query:"offset" minimum:"0"`
		}) (*songListOutput, error) {
			res, err := d.Songs.List(ctx, sess(ctx), app.SongQuery{Q: in.Q, Language: in.Language, LicenceStatus: in.LicenceStatus,
				HymnalSource: in.HymnalSource, HymnalNumber: in.HymnalNumber, Limit: in.Limit, Offset: in.Offset})
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &songListOutput{}
			out.Body.Items, out.Body.Total = make([]SongSummaryView, len(res.Items)), res.Total
			for i, s := range res.Items {
				out.Body.Items[i] = songSummaryView(s)
			}
			return out, nil
		})

	huma.Register(api, songOp("getSong", http.MethodGet, "/songs/{id}", http.StatusOK, "A song with its sections"),
		func(ctx context.Context, in *idPath) (*songOutput, error) {
			v, err := d.Songs.Get(ctx, sess(ctx), domain.SongID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &songOutput{Body: songView(v)}, nil
		})

	huma.Register(api, songOp("createSong", http.MethodPost, "/songs", http.StatusCreated, "Add a song"),
		func(ctx context.Context, in *struct {
			Body struct {
				Language           string        `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
				Title              string        `json:"title" maxLength:"2000"`
				AltTitles          []string      `json:"alt_titles,omitempty" maxItems:"100"`
				HymnalSource       string        `json:"hymnal_source,omitempty" maxLength:"1000"`
				HymnalNumber       string        `json:"hymnal_number,omitempty" maxLength:"1000"`
				Lyricist           string        `json:"lyricist,omitempty" maxLength:"2000"`
				Composer           string        `json:"composer,omitempty" maxLength:"2000"`
				Translator         string        `json:"translator,omitempty" maxLength:"2000"`
				DefaultKey         string        `json:"default_key,omitempty" maxLength:"100"`
				CopyrightHolder    string        `json:"copyright_holder,omitempty" maxLength:"2000"`
				CopyrightLine      string        `json:"copyright_line,omitempty" maxLength:"5000"`
				CCLISongNumber     string        `json:"ccli_song_number,omitempty" maxLength:"100"`
				LicenceStatus      string        `json:"licence_status,omitempty" enum:"unknown,public_domain,church_licence,permission_obtained"`
				LicenceNotes       string        `json:"licence_notes,omitempty" maxLength:"20000"`
				Sections           []songSection `json:"sections,omitempty" maxItems:"200"`
				DefaultArrangement []string      `json:"default_arrangement,omitempty" maxItems:"1000" doc:"keys of the new sections"`
			}
		}) (*songOutput, error) {
			b := in.Body
			v, err := d.Songs.Create(ctx, sess(ctx), app.SongInput{Language: b.Language, Title: b.Title, AltTitles: b.AltTitles,
				HymnalSource: b.HymnalSource, HymnalNumber: b.HymnalNumber, Lyricist: b.Lyricist, Composer: b.Composer,
				Translator: b.Translator, DefaultKey: b.DefaultKey, CopyrightHolder: b.CopyrightHolder,
				CopyrightLine: b.CopyrightLine, CCLISongNumber: b.CCLISongNumber, LicenceStatus: domain.LicenceStatus(b.LicenceStatus),
				LicenceNotes: b.LicenceNotes, Sections: sectionInputs(b.Sections), Arrangement: b.DefaultArrangement})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("song_created", "actor", string(sess(ctx).UserID), "song_id", string(v.Song.ID))
			return &songOutput{Body: songView(v)}, nil
		})

	huma.Register(api, songOp("updateSong", http.MethodPatch, "/songs/{id}", http.StatusOK, "Change a song, its sections or its arrangement"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Version            int            `json:"version" minimum:"1" doc:"the version the client loaded"`
				Language           *string        `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant"`
				Title              *string        `json:"title,omitempty" maxLength:"2000"`
				AltTitles          *[]string      `json:"alt_titles,omitempty" maxItems:"100"`
				HymnalSource       *string        `json:"hymnal_source,omitempty" maxLength:"1000"`
				HymnalNumber       *string        `json:"hymnal_number,omitempty" maxLength:"1000"`
				Lyricist           *string        `json:"lyricist,omitempty" maxLength:"2000"`
				Composer           *string        `json:"composer,omitempty" maxLength:"2000"`
				Translator         *string        `json:"translator,omitempty" maxLength:"2000"`
				DefaultKey         *string        `json:"default_key,omitempty" maxLength:"100"`
				CopyrightHolder    *string        `json:"copyright_holder,omitempty" maxLength:"2000"`
				CopyrightLine      *string        `json:"copyright_line,omitempty" maxLength:"5000"`
				CCLISongNumber     *string        `json:"ccli_song_number,omitempty" maxLength:"100"`
				LicenceStatus      *string        `json:"licence_status,omitempty" enum:"unknown,public_domain,church_licence,permission_obtained"`
				LicenceNotes       *string        `json:"licence_notes,omitempty" maxLength:"20000"`
				Sections           *[]songSection `json:"sections,omitempty" maxItems:"200" doc:"the complete ordered list; omitted sections are deleted"`
				DefaultArrangement *[]string      `json:"default_arrangement,omitempty" maxItems:"1000" doc:"section IDs or keys of new sections; [] clears it"`
			}
		}) (*songOutput, error) {
			b := in.Body
			ch := app.SongChange{Version: b.Version, Language: b.Language, Title: b.Title, AltTitles: b.AltTitles,
				HymnalSource: b.HymnalSource, HymnalNumber: b.HymnalNumber, Lyricist: b.Lyricist, Composer: b.Composer,
				Translator: b.Translator, DefaultKey: b.DefaultKey, CopyrightHolder: b.CopyrightHolder,
				CopyrightLine: b.CopyrightLine, CCLISongNumber: b.CCLISongNumber, LicenceNotes: b.LicenceNotes,
				Arrangement: b.DefaultArrangement}
			if b.LicenceStatus != nil {
				st := domain.LicenceStatus(*b.LicenceStatus)
				ch.LicenceStatus = &st
			}
			if b.Sections != nil {
				secs := sectionInputs(*b.Sections)
				ch.Sections = &secs
			}
			v, err := d.Songs.Update(ctx, sess(ctx), domain.SongID(in.ID), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("song_updated", "actor", string(sess(ctx).UserID), "song_id", in.ID)
			return &songOutput{Body: songView(v)}, nil
		})

	huma.Register(api, songOp("deleteSong", http.MethodDelete, "/songs/{id}", http.StatusNoContent, "Delete a song"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Songs.Delete(ctx, sess(ctx), domain.SongID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("song_deleted", "actor", string(sess(ctx).UserID), "song_id", in.ID)
			return nil, nil
		})

	huma.Register(api, songOp("linkSong", http.MethodPost, "/songs/{id}/link", http.StatusOK, "Link another-language version of the same hymn"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				OtherSongID string `json:"other_song_id" maxLength:"26"`
			}
		}) (*songOutput, error) {
			v, err := d.Songs.Link(ctx, sess(ctx), domain.SongID(in.ID), domain.SongID(in.Body.OtherSongID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("songs_linked", "actor", string(sess(ctx).UserID), "song_id", in.ID, "other_song_id", in.Body.OtherSongID)
			return &songOutput{Body: songView(v)}, nil
		})

	huma.Register(api, songOp("unlinkSong", http.MethodDelete, "/songs/{id}/link", http.StatusNoContent, "Unlink a song from its other-language versions"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Songs.Unlink(ctx, sess(ctx), domain.SongID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("songs_unlinked", "actor", string(sess(ctx).UserID), "song_id", in.ID)
			return nil, nil
		})
}
