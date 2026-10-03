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

// PlanningDeps are what the planning operations need (09 §4). Fields may be
// nil when only the OpenAPI document is built.
type PlanningDeps struct {
	Vocabulary *app.Vocabulary
	Templates  *app.Templates
	Services   *app.Services
	Log        *slog.Logger
}

type nameEntryOutput struct{ Body NameEntryView }

type nameListOutput struct {
	Body struct {
		Items []NameEntryView `json:"items"`
	}
}

type templateOutput struct{ Body TemplateView }

type serviceOutput struct{ Body ServiceView }

// RegisterPlanning registers the duties, singing parts, templates and
// services operations (09 §4).
func RegisterPlanning(api huma.API, d PlanningDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	planOp := func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), "planning")
	}
	actor := func(ctx context.Context) string { return string(sess(ctx).UserID) }

	registerList := func(kind domain.ListKind, path, noun, one, many string) {
		huma.Register(api, planOp("list"+many, http.MethodGet, path, http.StatusOK, "List the "+noun+" in order"),
			func(ctx context.Context, _ *struct{}) (*nameListOutput, error) {
				v, err := d.Vocabulary.List(ctx, sess(ctx), kind)
				if err != nil {
					return nil, fail(ctx, err)
				}
				out := &nameListOutput{}
				out.Body.Items = nameEntryViews(v)
				return out, nil
			})
		huma.Register(api, planOp("create"+one, http.MethodPost, path, http.StatusCreated, "Add to the "+noun),
			func(ctx context.Context, in *struct {
				Body struct {
					Name string `json:"name" maxLength:"2000"`
				}
			}) (*nameEntryOutput, error) {
				v, err := d.Vocabulary.Create(ctx, sess(ctx), kind, in.Body.Name)
				if err != nil {
					return nil, fail(ctx, err)
				}
				d.Log.Info(string(kind)+"_created", "actor", actor(ctx), string(kind)+"_id", v.Entry.ID)
				return &nameEntryOutput{Body: nameEntryView(v)}, nil
			})
		huma.Register(api, planOp("reorder"+many, http.MethodPut, path+"/order", http.StatusOK, "Set the order of the "+noun),
			func(ctx context.Context, in *struct {
				Body struct {
					IDs []string `json:"ids" maxItems:"100" doc:"exactly the current IDs, once each"`
				}
			}) (*nameListOutput, error) {
				v, err := d.Vocabulary.Reorder(ctx, sess(ctx), kind, in.Body.IDs)
				if err != nil {
					return nil, fail(ctx, err)
				}
				d.Log.Info(string(kind)+"_reordered", "actor", actor(ctx))
				out := &nameListOutput{}
				out.Body.Items = nameEntryViews(v)
				return out, nil
			})
		huma.Register(api, planOp("rename"+one, http.MethodPatch, path+"/{id}", http.StatusOK, "Rename an entry of the "+noun),
			func(ctx context.Context, in *struct {
				ID   string `path:"id" maxLength:"26"`
				Body struct {
					Name string `json:"name" maxLength:"2000"`
				}
			}) (*nameEntryOutput, error) {
				v, err := d.Vocabulary.Rename(ctx, sess(ctx), kind, in.ID, in.Body.Name)
				if err != nil {
					return nil, fail(ctx, err)
				}
				d.Log.Info(string(kind)+"_renamed", "actor", actor(ctx), string(kind)+"_id", in.ID)
				return &nameEntryOutput{Body: nameEntryView(v)}, nil
			})
		huma.Register(api, planOp("delete"+one, http.MethodDelete, path+"/{id}", http.StatusNoContent, "Delete an entry of the "+noun),
			func(ctx context.Context, in *idPath) (*struct{}, error) {
				if err := d.Vocabulary.Delete(ctx, sess(ctx), kind, in.ID); err != nil {
					return nil, fail(ctx, err)
				}
				d.Log.Info(string(kind)+"_deleted", "actor", actor(ctx), string(kind)+"_id", in.ID)
				return nil, nil
			})
	}
	registerList(domain.KindDuty, "/duties", "duties", "Duty", "Duties")
	registerList(domain.KindSingingPart, "/singing-parts", "singing parts", "SingingPart", "SingingParts")

	// --- templates ---

	huma.Register(api, planOp("listTemplates", http.MethodGet, "/templates", http.StatusOK, "List the templates"),
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Items []TemplateSummaryView `json:"items"`
			}
		}, error) {
			rows, err := d.Templates.List(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []TemplateSummaryView `json:"items"`
				}
			}{}
			out.Body.Items = make([]TemplateSummaryView, len(rows))
			for i, r := range rows {
				out.Body.Items[i] = TemplateSummaryView{ID: string(r.Row.ID), Name: r.Row.Name, Language: r.Row.Language,
					ItemCount: r.Row.ItemCount, Version: r.Row.Version, Actions: r.Actions}
			}
			return out, nil
		})
	huma.Register(api, planOp("getTemplate", http.MethodGet, "/templates/{id}", http.StatusOK, "A template with its items"),
		func(ctx context.Context, in *idPath) (*templateOutput, error) {
			v, err := d.Templates.Get(ctx, sess(ctx), domain.TemplateID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &templateOutput{Body: templateView(v)}, nil
		})
	huma.Register(api, planOp("createTemplate", http.MethodPost, "/templates", http.StatusCreated, "Add a template"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name     string              `json:"name" maxLength:"2000"`
				Language string              `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant" doc:"default: the church's content language"`
				Items    []TemplateItemInput `json:"items" maxItems:"1000"`
			}
		}) (*templateOutput, error) {
			b := in.Body
			v, err := d.Templates.Create(ctx, sess(ctx), app.TemplateInput{Name: b.Name, Language: b.Language, Items: templateItems(b.Items)})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("template_created", "actor", actor(ctx), "template_id", string(v.Template.ID))
			return &templateOutput{Body: templateView(v)}, nil
		})
	huma.Register(api, planOp("updateTemplate", http.MethodPatch, "/templates/{id}", http.StatusOK, "Change a template"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Version  int                  `json:"version" minimum:"1" doc:"the version the client loaded"`
				Name     *string              `json:"name,omitempty" maxLength:"2000"`
				Language *string              `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant"`
				Items    *[]TemplateItemInput `json:"items,omitempty" maxItems:"1000" doc:"the complete list of items"`
			}
		}) (*templateOutput, error) {
			b := in.Body
			ch := app.TemplateChange{Version: b.Version, Name: b.Name, Language: b.Language}
			if b.Items != nil {
				items := templateItems(*b.Items)
				ch.Items = &items
			}
			v, err := d.Templates.Update(ctx, sess(ctx), domain.TemplateID(in.ID), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("template_updated", "actor", actor(ctx), "template_id", in.ID)
			return &templateOutput{Body: templateView(v)}, nil
		})
	huma.Register(api, planOp("deleteTemplate", http.MethodDelete, "/templates/{id}", http.StatusNoContent, "Delete a template"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Templates.Delete(ctx, sess(ctx), domain.TemplateID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("template_deleted", "actor", actor(ctx), "template_id", in.ID)
			return nil, nil
		})

	// --- services ---

	huma.Register(api, planOp("listServices", http.MethodGet, "/services", http.StatusOK, "List the regular services"),
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Items []ServiceView `json:"items"`
			}
		}, error) {
			rows, err := d.Services.List(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []ServiceView `json:"items"`
				}
			}{}
			out.Body.Items = make([]ServiceView, len(rows))
			for i, r := range rows {
				out.Body.Items[i] = serviceView(r)
			}
			return out, nil
		})
	huma.Register(api, planOp("getService", http.MethodGet, "/services/{id}", http.StatusOK, "A service with its times"),
		func(ctx context.Context, in *idPath) (*serviceOutput, error) {
			v, err := d.Services.Get(ctx, sess(ctx), domain.ServiceID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &serviceOutput{Body: serviceView(v)}, nil
		})
	huma.Register(api, planOp("createService", http.MethodPost, "/services", http.StatusCreated, "Add a regular service"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name              string            `json:"name" maxLength:"2000"`
				Language          string            `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant" doc:"default: the church's content language"`
				DefaultTemplateID string            `json:"default_template_id,omitempty" maxLength:"26"`
				Times             []ServiceTimeView `json:"times" maxItems:"100" doc:"1 to 14 weekly times"`
			}
		}) (*serviceOutput, error) {
			b := in.Body
			v, err := d.Services.Create(ctx, sess(ctx), app.ServiceInput{Name: b.Name, Language: b.Language,
				DefaultTemplateID: domain.TemplateID(b.DefaultTemplateID), Times: serviceTimes(b.Times)})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("service_created", "actor", actor(ctx), "service_id", string(v.Service.ID))
			return &serviceOutput{Body: serviceView(v)}, nil
		})
	huma.Register(api, planOp("updateService", http.MethodPatch, "/services/{id}", http.StatusOK, "Change a service"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Version           int                `json:"version" minimum:"1" doc:"the version the client loaded"`
				Name              *string            `json:"name,omitempty" maxLength:"2000"`
				Language          *string            `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant"`
				DefaultTemplateID *string            `json:"default_template_id,omitempty" maxLength:"26" doc:"empty clears it"`
				Times             *[]ServiceTimeView `json:"times,omitempty" maxItems:"100" doc:"the complete list; omit to keep it"`
			}
		}) (*serviceOutput, error) {
			b := in.Body
			ch := app.ServiceChange{Version: b.Version, Name: b.Name, Language: b.Language}
			if b.DefaultTemplateID != nil {
				id := domain.TemplateID(*b.DefaultTemplateID)
				ch.DefaultTemplateID = &id
			}
			if b.Times != nil {
				times := serviceTimes(*b.Times)
				ch.Times = &times
			}
			v, err := d.Services.Update(ctx, sess(ctx), domain.ServiceID(in.ID), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("service_updated", "actor", actor(ctx), "service_id", in.ID)
			return &serviceOutput{Body: serviceView(v)}, nil
		})
	huma.Register(api, planOp("deleteService", http.MethodDelete, "/services/{id}", http.StatusNoContent, "Delete a service"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Services.Delete(ctx, sess(ctx), domain.ServiceID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("service_deleted", "actor", actor(ctx), "service_id", in.ID)
			return nil, nil
		})
}
