// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// JSON shapes of duties, singing parts, templates and services (09 §4).

// NameEntryView is a duty or a singing part.
type NameEntryView struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Position int                 `json:"position"`
	Actions  app.PlanningActions `json:"actions"`
}

func nameEntryView(v app.NameEntryView) NameEntryView {
	return NameEntryView{ID: v.Entry.ID, Name: v.Entry.Name, Position: v.Entry.Position, Actions: v.Actions}
}

func nameEntryViews(in []app.NameEntryView) []NameEntryView {
	out := make([]NameEntryView, len(in))
	for i, v := range in {
		out[i] = nameEntryView(v)
	}
	return out
}

// TemplateItemView is one item of a template. Items have no IDs.
type TemplateItemView struct {
	Title         string  `json:"title"`
	ItemType      string  `json:"item_type" enum:"song,reading,prayer,sermon,free_text,other"`
	DefaultText   string  `json:"default_text"`
	DefaultDutyID *string `json:"default_duty_id" doc:"null: no default duty"`
}

// TemplateSummaryView is a template in a list.
type TemplateSummaryView struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Language  string              `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	ItemCount int                 `json:"item_count"`
	Version   int                 `json:"version"`
	Actions   app.PlanningActions `json:"actions"`
}

// TemplateView is a whole template.
type TemplateView struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Language string              `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Items    []TemplateItemView  `json:"items"`
	Version  int                 `json:"version" doc:"send it back with every change"`
	Actions  app.PlanningActions `json:"actions"`
}

func templateView(v app.TemplateView) TemplateView {
	t := v.Template
	out := TemplateView{ID: string(t.ID), Name: t.Name, Language: t.Language, Version: t.Version, Actions: v.Actions,
		Items: make([]TemplateItemView, len(t.Items))}
	for i, it := range t.Items {
		out.Items[i] = TemplateItemView{Title: it.Title, ItemType: string(it.Type), DefaultText: it.DefaultText,
			DefaultDutyID: optional(string(it.DefaultDutyID))}
	}
	return out
}

// ServiceTimeView is one weekly time of a service.
type ServiceTimeView struct {
	Weekday int    `json:"weekday" minimum:"1" maximum:"7" doc:"ISO: 1 Monday to 7 Sunday"`
	Time    string `json:"time" pattern:"^[0-2][0-9]:[0-5][0-9]$" doc:"HH:MM in the church's time zone"`
}

// ServiceView is a regular service.
type ServiceView struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Language            string              `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	DefaultTemplateID   *string             `json:"default_template_id"`
	DefaultTemplateName *string             `json:"default_template_name"`
	Times               []ServiceTimeView   `json:"times"`
	Version             int                 `json:"version" doc:"send it back with every change"`
	Actions             app.PlanningActions `json:"actions"`
}

func serviceView(v app.ServiceView) ServiceView {
	s := v.Service
	out := ServiceView{ID: string(s.ID), Name: s.Name, Language: s.Language, Version: s.Version, Actions: v.Actions,
		DefaultTemplateID: optional(string(s.DefaultTemplateID)), DefaultTemplateName: optional(v.DefaultTemplateName),
		Times: make([]ServiceTimeView, len(s.Times))}
	for i, t := range s.Times {
		out.Times[i] = ServiceTimeView{Weekday: t.Weekday, Time: t.Time}
	}
	return out
}

func templateItems(in []TemplateItemInput) []domain.TemplateItem {
	out := make([]domain.TemplateItem, len(in))
	for i, it := range in {
		out[i] = domain.TemplateItem{Title: it.Title, Type: domain.ItemType(it.ItemType), DefaultText: it.DefaultText,
			DefaultDutyID: domain.DutyID(it.DefaultDutyID)}
	}
	return out
}

func serviceTimes(in []ServiceTimeView) []domain.ServiceTime {
	out := make([]domain.ServiceTime, len(in))
	for i, t := range in {
		out[i] = domain.ServiceTime{Weekday: t.Weekday, Time: t.Time}
	}
	return out
}

// TemplateItemInput is an item in a request. An empty default_duty_id means none.
type TemplateItemInput struct {
	Title         string `json:"title" maxLength:"2000"`
	ItemType      string `json:"item_type" enum:"song,reading,prayer,sermon,free_text,other"`
	DefaultText   string `json:"default_text,omitempty" maxLength:"100000"`
	DefaultDutyID string `json:"default_duty_id,omitempty" maxLength:"26"`
}
