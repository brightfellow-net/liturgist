// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"net/http"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

type readingOutput struct{ Body ReadingView }

type readingListOutput struct {
	Body struct {
		Items []ReadingSummaryView `json:"items"`
		Total int                  `json:"total"`
	}
}

// ParsedReferenceView is the answer of GET /readings/parse.
type ParsedReferenceView struct {
	Reference string `json:"reference" doc:"standard form"`
	Canonical string `json:"canonical" doc:"with the Indonesian book name"`
	Display   string `json:"display" doc:"as typed, spaces collapsed"`
}

// registerReadings registers the readings operations (07 §4).
func registerReadings(api huma.API, d LibraryDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	readingOp := func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), "library")
	}
	view := func(v app.ReadingView) *readingOutput { return &readingOutput{Body: readingView(v.Reading, v.Actions)} }

	huma.Register(api, readingOp("parseReference", http.MethodGet, "/readings/parse", http.StatusOK, "Understand a typed Bible reference"),
		func(ctx context.Context, in *struct {
			Input string `query:"input" maxLength:"2000"`
		}) (*struct{ Body ParsedReferenceView }, error) {
			p, err := d.Readings.Parse(ctx, sess(ctx), in.Input)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &struct{ Body ParsedReferenceView }{ParsedReferenceView{Reference: p.Reference.String(),
				Canonical: p.Reference.Canonical("id"), Display: p.Display}}, nil
		})

	huma.Register(api, readingOp("lookupReading", http.MethodGet, "/readings/lookup", http.StatusOK, "Find the text for a reference"),
		func(ctx context.Context, in *struct {
			Reference   string `query:"reference" maxLength:"2000"`
			Translation string `query:"translation" maxLength:"16" doc:"default: the church's translation"`
		}) (*struct{ Body LookupView }, error) {
			r, err := d.Readings.Lookup(ctx, sess(ctx), in.Reference, in.Translation)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := LookupView{Reference: r.Reference.String(), Canonical: r.Reference.Canonical("id"), Display: r.Display,
				Translation: translationRefView(r.Translation), ProviderError: r.ProviderError,
				SuggestedAttribution: r.SuggestedAttribution}
			if r.Reading != nil {
				v := readingView(*r.Reading, r.Actions)
				out.Reading = &v
			}
			if r.Provider != nil {
				out.Provider = &ProviderTextView{Text: r.Provider.Text, Attribution: r.Provider.Attribution,
					Source: r.Provider.Source, MayStore: r.Provider.MayStore}
			}
			return &struct{ Body LookupView }{out}, nil
		})

	huma.Register(api, readingOp("listReadings", http.MethodGet, "/readings", http.StatusOK, "Search and list readings"),
		func(ctx context.Context, in *struct {
			Q           string `query:"q" maxLength:"2000" doc:"part of the reference or the text"`
			Translation string `query:"translation" maxLength:"16"`
			Limit       int    `query:"limit" minimum:"0" maximum:"1000" doc:"default 50, at most 100"`
			Offset      int    `query:"offset" minimum:"0"`
		}) (*readingListOutput, error) {
			res, err := d.Readings.List(ctx, sess(ctx), app.ReadingQuery{Q: in.Q, Translation: in.Translation, Limit: in.Limit, Offset: in.Offset})
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &readingListOutput{}
			out.Body.Items, out.Body.Total = make([]ReadingSummaryView, len(res.Items)), res.Total
			for i, r := range res.Items {
				out.Body.Items[i] = ReadingSummaryView{ID: string(r.ID), Reference: r.Reference, Canonical: r.Canonical,
					ReferenceDisplay: r.ReferenceDisplay, Translation: translationRefView(r.Translation), Snippet: r.Snippet,
					Actions: r.Actions}
			}
			return out, nil
		})

	huma.Register(api, readingOp("getReading", http.MethodGet, "/readings/{id}", http.StatusOK, "A reading with its text"),
		func(ctx context.Context, in *idPath) (*readingOutput, error) {
			v, err := d.Readings.Get(ctx, sess(ctx), domain.ReadingID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return view(v), nil
		})

	huma.Register(api, readingOp("createReading", http.MethodPost, "/readings", http.StatusCreated, "Save a reading typed or pasted by a member"),
		func(ctx context.Context, in *struct {
			Body struct {
				Reference   string `json:"reference" maxLength:"2000"`
				Translation string `json:"translation,omitempty" maxLength:"16" doc:"default: the church's translation"`
				Text        string `json:"text" maxLength:"200000"`
				Attribution string `json:"attribution,omitempty" maxLength:"2000"`
			}
		}) (*readingOutput, error) {
			b := in.Body
			v, err := d.Readings.Create(ctx, sess(ctx), app.ReadingInput{Reference: b.Reference, Translation: b.Translation,
				Text: b.Text, Attribution: b.Attribution})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reading_created", "actor", string(sess(ctx).UserID), "reading_id", string(v.Reading.ID))
			return view(v), nil
		})

	huma.Register(api, readingOp("createReadingFromProvider", http.MethodPost, "/readings/from-provider", http.StatusCreated,
		"Save the text a registered provider returns"),
		func(ctx context.Context, in *struct {
			Body struct {
				Reference   string `json:"reference" maxLength:"2000"`
				Translation string `json:"translation,omitempty" maxLength:"16"`
				Provider    string `json:"provider" maxLength:"100"`
			}
		}) (*readingOutput, error) {
			b := in.Body
			v, err := d.Readings.FromProvider(ctx, sess(ctx), b.Reference, b.Translation, b.Provider)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reading_created", "actor", string(sess(ctx).UserID), "reading_id", string(v.Reading.ID), "provider", b.Provider)
			return view(v), nil
		})

	huma.Register(api, readingOp("updateReading", http.MethodPatch, "/readings/{id}", http.StatusOK, "Change the text or attribution of a reading"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Version          int     `json:"version" minimum:"1" doc:"the version the client loaded"`
				Text             *string `json:"text,omitempty" maxLength:"200000"`
				Attribution      *string `json:"attribution,omitempty" maxLength:"2000"`
				ReferenceDisplay *string `json:"reference_display,omitempty" maxLength:"2000"`
			}
		}) (*readingOutput, error) {
			b := in.Body
			v, err := d.Readings.Update(ctx, sess(ctx), domain.ReadingID(in.ID), app.ReadingChange{Version: b.Version,
				Text: b.Text, Attribution: b.Attribution, ReferenceDisplay: b.ReferenceDisplay})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reading_updated", "actor", string(sess(ctx).UserID), "reading_id", in.ID)
			return view(v), nil
		})

	huma.Register(api, readingOp("deleteReading", http.MethodDelete, "/readings/{id}", http.StatusNoContent, "Delete a reading"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Readings.Delete(ctx, sess(ctx), domain.ReadingID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reading_deleted", "actor", string(sess(ctx).UserID), "reading_id", in.ID)
			return nil, nil
		})
}
