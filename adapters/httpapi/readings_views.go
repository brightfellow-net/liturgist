// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// JSON shapes of readings (07 §4).

// TranslationRefView names a Bible translation.
type TranslationRefView struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Language string `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
}

func translationRefView(t domain.Translation) TranslationRefView {
	return TranslationRefView{Code: t.Code, Name: t.Name, Language: t.Language}
}

// ReadingSummaryView is a reading in a list.
type ReadingSummaryView struct {
	ID               string             `json:"id"`
	Reference        string             `json:"reference" doc:"standard form, e.g. JHN 3:16-21"`
	Canonical        string             `json:"canonical" doc:"with the Indonesian book name, e.g. Yohanes 3:16-21"`
	ReferenceDisplay string             `json:"reference_display" doc:"as typed"`
	Translation      TranslationRefView `json:"translation"`
	Snippet          string             `json:"snippet"`
	Actions          app.ReadingActions `json:"actions"`
}

// ReadingView is a whole reading.
type ReadingView struct {
	ID               string             `json:"id"`
	Reference        string             `json:"reference"`
	Canonical        string             `json:"canonical"`
	ReferenceDisplay string             `json:"reference_display"`
	Translation      TranslationRefView `json:"translation"`
	Text             string             `json:"text"`
	Attribution      string             `json:"attribution"`
	SourceProvider   string             `json:"source_provider" doc:"manual, or the ID of the provider the text came from"`
	Version          int                `json:"version" doc:"send it back with every change"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	Actions          app.ReadingActions `json:"actions"`
}

func canonicalOf(reference string) string {
	if ref, err := domain.ParseReference(reference); err == nil {
		return ref.Canonical("id")
	}
	return reference
}

func readingView(r domain.Reading, a app.ReadingActions) ReadingView {
	return ReadingView{ID: string(r.ID), Reference: r.Reference, Canonical: canonicalOf(r.Reference),
		ReferenceDisplay: r.ReferenceDisplay, Translation: translationRefView(r.Translation), Text: r.Text,
		Attribution: r.Attribution, SourceProvider: r.SourceProvider, Version: r.Version, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Actions: a}
}

// ProviderTextView is text a provider offers.
type ProviderTextView struct {
	Text        string `json:"text"`
	Attribution string `json:"attribution"`
	Source      string `json:"source" doc:"the provider's ID, for POST /readings/from-provider"`
	MayStore    bool   `json:"may_store"`
}

// LookupView is the answer of GET /readings/lookup.
type LookupView struct {
	Reference            string             `json:"reference"`
	Canonical            string             `json:"canonical"`
	Display              string             `json:"display"`
	Translation          TranslationRefView `json:"translation"`
	Reading              *ReadingView       `json:"reading" doc:"the church's saved reading, if any"`
	Provider             *ProviderTextView  `json:"provider" doc:"text a registered provider offers, if no reading is saved"`
	ProviderError        bool               `json:"provider_error" doc:"a provider could not be reached and nothing was found"`
	SuggestedAttribution string             `json:"suggested_attribution"`
}
