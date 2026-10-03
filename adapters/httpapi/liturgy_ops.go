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

// LiturgyDeps are what the liturgy operations need (10 §4). Fields may be nil
// when only the OpenAPI document is built.
type LiturgyDeps struct {
	Liturgies *app.Liturgies
	Log       *slog.Logger
}

type liturgyOutput struct{ Body LiturgyView }

type itemResultOutput struct{ Body ItemResultView }

type orderOutput struct{ Body OrderResultView }

func orderResultView(r app.OrderResult) OrderResultView {
	out := OrderResultView{LiturgyVersion: r.LiturgyVersion, ItemIDs: make([]string, len(r.ItemIDs))}
	for i, id := range r.ItemIDs {
		out.ItemIDs[i] = string(id)
	}
	return out
}

func itemResultView(r app.ItemResult) ItemResultView {
	return ItemResultView{Item: liturgyItemView(r.Item), LiturgyVersion: r.LiturgyVersion}
}

// EntryInput is one entry of a sequence in a request.
type EntryInput struct {
	SectionID     string `json:"section_id" maxLength:"26"`
	SingingPartID string `json:"singing_part_id,omitempty" maxLength:"26"`
	KeyChange     string `json:"key_change,omitempty" maxLength:"20"`
	Note          string `json:"note,omitempty" maxLength:"2000"`
}

// ItemSongInput is a song in a PUT …/songs request.
type ItemSongInput struct {
	SongID  string       `json:"song_id" maxLength:"26"`
	Key     string       `json:"key,omitempty" maxLength:"20"`
	Note    string       `json:"note,omitempty" maxLength:"2000"`
	Entries []EntryInput `json:"entries" maxItems:"1000" doc:"the sequence: 0 to 100 entries"`
}

// RegisterLiturgies registers the liturgy operations (10 §4).
func RegisterLiturgies(api huma.API, d LiturgyDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	actor := func(ctx context.Context) string { return string(sess(ctx).UserID) }
	lop := func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), "liturgies")
	}
	type liturgyPath struct {
		ID string `path:"id" maxLength:"26"`
	}

	// The fixed paths prepare and assignable come before {id}: a ULID can never equal them.

	huma.Register(api, lop("getPrepareWeek", http.MethodGet, "/liturgies/prepare", http.StatusOK, "The service times of a week and which have a liturgy"),
		func(ctx context.Context, in *struct {
			Week string `query:"week" maxLength:"10" doc:"any date of the week as YYYY-MM-DD; default: next week in the church's time zone"`
		}) (*struct{ Body PrepareWeekView }, error) {
			w, err := d.Liturgies.Prepare(ctx, sess(ctx), in.Week)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct{ Body PrepareWeekView }{}
			out.Body = PrepareWeekView{Week: w.Week, Occurrences: make([]OccurrenceView, len(w.Occurrences))}
			out.Body.Limits.Active, out.Body.Limits.Unpublished = limitUseView(w.Active), limitUseView(w.Unpublished)
			for i, o := range w.Occurrences {
				out.Body.Occurrences[i] = OccurrenceView{ServiceID: string(o.ServiceID), ServiceName: o.ServiceName, Language: o.Language,
					Date: o.Date, Time: o.Time, TemplateID: optional(string(o.TemplateID)), TemplateName: optional(o.TemplateName),
					LiturgyID: optional(string(o.LiturgyID))}
			}
			return out, nil
		})
	huma.Register(api, lop("prepareLiturgies", http.MethodPost, "/liturgies/prepare", http.StatusCreated, "Create the liturgies of the chosen service times, all or nothing"),
		func(ctx context.Context, in *struct {
			Body struct {
				Occurrences []struct {
					ServiceID string `json:"service_id" maxLength:"26"`
					Date      string `json:"date" maxLength:"10"`
					Time      string `json:"time" maxLength:"5"`
				} `json:"occurrences" maxItems:"100" doc:"1 to 50 occurrences of services"`
			}
		}) (*struct {
			Body struct {
				Items []PreparedView `json:"items"`
			}
		}, error) {
			entries := make([]app.PrepareEntry, len(in.Body.Occurrences))
			for i, o := range in.Body.Occurrences {
				entries[i] = app.PrepareEntry{ServiceID: domain.ServiceID(o.ServiceID), Date: o.Date, Time: o.Time}
			}
			made, err := d.Liturgies.PrepareCreate(ctx, sess(ctx), entries)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("liturgies_prepared", "actor", actor(ctx), "count", len(made))
			out := &struct {
				Body struct {
					Items []PreparedView `json:"items"`
				}
			}{}
			out.Body.Items = make([]PreparedView, len(made))
			for i, m := range made {
				out.Body.Items[i] = PreparedView{LiturgyID: string(m.LiturgyID), ServiceName: m.ServiceName, Date: m.Date, Time: m.Time,
					Language: m.Language, TemplateID: optional(string(m.TemplateID)), TemplateName: optional(m.TemplateName)}
			}
			return out, nil
		})
	huma.Register(api, lop("listAssignable", http.MethodGet, "/liturgies/assignable", http.StatusOK, "The members who can be assigned: names only"),
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Items []struct {
					UserID string `json:"user_id"`
					Name   string `json:"name"`
				} `json:"items"`
			}
		}, error) {
			list, err := d.Liturgies.Assignable(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []struct {
						UserID string `json:"user_id"`
						Name   string `json:"name"`
					} `json:"items"`
				}
			}{}
			for _, a := range list {
				out.Body.Items = append(out.Body.Items, struct {
					UserID string `json:"user_id"`
					Name   string `json:"name"`
				}{string(a.UserID), a.Name})
			}
			return out, nil
		})

	// --- liturgies ---

	huma.Register(api, lop("listLiturgies", http.MethodGet, "/liturgies", http.StatusOK, "List the liturgies the caller may see"),
		func(ctx context.Context, in *struct {
			State  string `query:"state" enum:"draft,in_review,needs_revision,approved,published"`
			From   string `query:"from" maxLength:"10" doc:"first date, inclusive"`
			To     string `query:"to" maxLength:"10" doc:"last date, inclusive"`
			Order  string `query:"order" enum:"date_asc,date_desc" doc:"default date_desc; ties by time, then ID"`
			Limit  int    `query:"limit" minimum:"0" maximum:"1000" doc:"default 50, at most 100"`
			Offset int    `query:"offset" minimum:"0"`
		}) (*struct {
			Body struct {
				Items []LiturgySummaryView `json:"items"`
				Total int                  `json:"total"`
			}
		}, error) {
			limit := in.Limit
			if limit < 1 || limit > domain.MaxLiturgyQuery {
				limit = 50
			}
			page, err := d.Liturgies.List(ctx, sess(ctx), app.LiturgyFilter{State: domain.LiturgyState(in.State), From: in.From, To: in.To,
				Ascending: in.Order == "date_asc", Limit: limit, Offset: in.Offset})
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []LiturgySummaryView `json:"items"`
					Total int                  `json:"total"`
				}
			}{}
			out.Body.Total = page.Total
			out.Body.Items = make([]LiturgySummaryView, len(page.Items))
			for i, it := range page.Items {
				l := it.Liturgy
				out.Body.Items[i] = LiturgySummaryView{ID: string(l.ID), Date: l.Date, Time: l.Time, ServiceName: l.ServiceName, Language: l.Language,
					State: string(l.State), ItemCount: it.ItemCount, Version: l.Version, Actions: it.Actions}
			}
			return out, nil
		})
	huma.Register(api, lop("createLiturgy", http.MethodPost, "/liturgies", http.StatusCreated, "Create a liturgy from a service or as a one-off"),
		func(ctx context.Context, in *struct {
			Body struct {
				Date        string  `json:"date" maxLength:"10"`
				Time        string  `json:"time,omitempty" maxLength:"5" doc:"required with service_id; any HH:MM"`
				ServiceID   string  `json:"service_id,omitempty" maxLength:"26"`
				ServiceName string  `json:"service_name,omitempty" maxLength:"2000" doc:"required for a one-off; overrides the service's name"`
				Language    string  `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant" doc:"default: the service's, the template's, the church's"`
				TemplateID  *string `json:"template_id,omitempty" maxLength:"26" doc:"omitted: the service's default; empty: no template"`
			}
		}) (*liturgyOutput, error) {
			b := in.Body
			ci := app.CreateInput{Date: b.Date, Time: b.Time, ServiceID: domain.ServiceID(b.ServiceID), ServiceName: b.ServiceName, Language: b.Language}
			if b.TemplateID != nil {
				t := domain.TemplateID(*b.TemplateID)
				ci.TemplateID = &t
			}
			v, err := d.Liturgies.Create(ctx, sess(ctx), ci)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("liturgy_created", "actor", actor(ctx), "liturgy_id", string(v.Liturgy.ID))
			return &liturgyOutput{Body: liturgyView(v)}, nil
		})
	huma.Register(api, lop("getLiturgy", http.MethodGet, "/liturgies/{id}", http.StatusOK, "A liturgy with its items, assignments and problems"),
		func(ctx context.Context, in *liturgyPath) (*liturgyOutput, error) {
			v, err := d.Liturgies.Get(ctx, sess(ctx), domain.LiturgyID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &liturgyOutput{Body: liturgyView(v)}, nil
		})
	huma.Register(api, lop("updateLiturgy", http.MethodPatch, "/liturgies/{id}", http.StatusOK, "Change the date, time or name of a liturgy"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Version     int     `json:"version" minimum:"1" doc:"the liturgy version the client loaded"`
				Date        *string `json:"date,omitempty" maxLength:"10"`
				Time        *string `json:"time,omitempty" maxLength:"5"`
				ServiceName *string `json:"service_name,omitempty" maxLength:"2000"`
			}
		}) (*liturgyOutput, error) {
			b := in.Body
			v, err := d.Liturgies.Update(ctx, sess(ctx), domain.LiturgyID(in.ID), app.LiturgyChange{Version: b.Version, Date: b.Date, Time: b.Time, ServiceName: b.ServiceName})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("liturgy_updated", "actor", actor(ctx), "liturgy_id", in.ID)
			return &liturgyOutput{Body: liturgyView(v)}, nil
		})
	huma.Register(api, lop("deleteLiturgy", http.MethodDelete, "/liturgies/{id}", http.StatusNoContent, "Delete a liturgy that is not published"),
		func(ctx context.Context, in *liturgyPath) (*struct{}, error) {
			if err := d.Liturgies.Delete(ctx, sess(ctx), domain.LiturgyID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("liturgy_deleted", "actor", actor(ctx), "liturgy_id", in.ID)
			return nil, nil
		})
	huma.Register(api, lop("listLiturgyEdits", http.MethodGet, "/liturgies/{id}/edits", http.StatusOK, "The history of a liturgy, newest first"),
		func(ctx context.Context, in *struct {
			ID    string `path:"id" maxLength:"26"`
			Limit int    `query:"limit" minimum:"0" maximum:"1000" doc:"default 50, at most 100"`
		}) (*struct {
			Body struct {
				Items []EditView `json:"items"`
			}
		}, error) {
			rows, err := d.Liturgies.Edits(ctx, sess(ctx), domain.LiturgyID(in.ID), in.Limit)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []EditView `json:"items"`
				}
			}{}
			out.Body.Items = make([]EditView, len(rows))
			for i, r := range rows {
				out.Body.Items[i] = editView(r)
			}
			return out, nil
		})

	// --- items ---

	huma.Register(api, lop("addLiturgyItem", http.MethodPost, "/liturgies/{id}/items", http.StatusCreated, "Add an item"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				LiturgyVersion int    `json:"liturgy_version" minimum:"1"`
				Title          string `json:"title" maxLength:"2000"`
				ItemType       string `json:"item_type" enum:"song,reading,prayer,sermon,free_text,other"`
				DutyID         string `json:"duty_id,omitempty" maxLength:"26"`
				Text           string `json:"text,omitempty" maxLength:"100000"`
				Position       *int   `json:"position,omitempty" minimum:"0" doc:"0 to the number of items; default: the end"`
			}
		}) (*itemResultOutput, error) {
			b := in.Body
			r, err := d.Liturgies.AddItem(ctx, sess(ctx), domain.LiturgyID(in.ID), app.ItemInput{LiturgyVersion: b.LiturgyVersion, Title: b.Title,
				Type: domain.ItemType(b.ItemType), DutyID: domain.DutyID(b.DutyID), Text: b.Text, Position: b.Position})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("item_added", "actor", actor(ctx), "liturgy_id", in.ID, "item_id", string(r.Item.Item.ID))
			return &itemResultOutput{Body: itemResultView(r)}, nil
		})
	huma.Register(api, lop("updateLiturgyItem", http.MethodPatch, "/liturgies/{id}/items/{iid}", http.StatusOK, "Change an item"),
		func(ctx context.Context, in *struct {
			ID     string `path:"id" maxLength:"26"`
			ItemID string `path:"iid" maxLength:"26"`
			Body   struct {
				Version   int     `json:"version" minimum:"1" doc:"the item version the client loaded"`
				Title     *string `json:"title,omitempty" maxLength:"2000"`
				DutyID    *string `json:"duty_id,omitempty" maxLength:"26" doc:"empty clears it"`
				Text      *string `json:"text,omitempty" maxLength:"100000"`
				ReadingID *string `json:"reading_id,omitempty" maxLength:"26" doc:"reading items only; empty clears it"`
			}
		}) (*itemResultOutput, error) {
			b := in.Body
			ch := app.ItemChange{Version: b.Version, Title: b.Title, Text: b.Text}
			if b.DutyID != nil {
				v := domain.DutyID(*b.DutyID)
				ch.DutyID = &v
			}
			if b.ReadingID != nil {
				v := domain.ReadingID(*b.ReadingID)
				ch.ReadingID = &v
			}
			r, err := d.Liturgies.UpdateItem(ctx, sess(ctx), domain.LiturgyID(in.ID), domain.ItemID(in.ItemID), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("item_updated", "actor", actor(ctx), "liturgy_id", in.ID, "item_id", in.ItemID)
			return &itemResultOutput{Body: itemResultView(r)}, nil
		})
	huma.Register(api, lop("removeLiturgyItem", http.MethodDelete, "/liturgies/{id}/items/{iid}", http.StatusOK, "Remove an item with its songs"),
		func(ctx context.Context, in *struct {
			ID             string `path:"id" maxLength:"26"`
			ItemID         string `path:"iid" maxLength:"26"`
			LiturgyVersion int    `query:"liturgy_version" minimum:"1" required:"true"`
		}) (*orderOutput, error) {
			r, err := d.Liturgies.RemoveItem(ctx, sess(ctx), domain.LiturgyID(in.ID), domain.ItemID(in.ItemID), in.LiturgyVersion)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("item_removed", "actor", actor(ctx), "liturgy_id", in.ID, "item_id", in.ItemID)
			return &orderOutput{Body: orderResultView(r)}, nil
		})
	huma.Register(api, lop("reorderLiturgyItems", http.MethodPut, "/liturgies/{id}/items/order", http.StatusOK, "Set the order of the items"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				LiturgyVersion int      `json:"liturgy_version" minimum:"1"`
				ItemIDs        []string `json:"item_ids" maxItems:"100" doc:"exactly the current IDs, once each"`
			}
		}) (*orderOutput, error) {
			ids := make([]domain.ItemID, len(in.Body.ItemIDs))
			for i, id := range in.Body.ItemIDs {
				ids[i] = domain.ItemID(id)
			}
			r, err := d.Liturgies.ReorderItems(ctx, sess(ctx), domain.LiturgyID(in.ID), in.Body.LiturgyVersion, ids)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("items_reordered", "actor", actor(ctx), "liturgy_id", in.ID)
			return &orderOutput{Body: orderResultView(r)}, nil
		})
	huma.Register(api, lop("addItemSong", http.MethodPost, "/liturgies/{id}/items/{iid}/songs", http.StatusCreated, "Add a song with its sequence filled"),
		func(ctx context.Context, in *struct {
			ID     string `path:"id" maxLength:"26"`
			ItemID string `path:"iid" maxLength:"26"`
			Body   struct {
				Version  int    `json:"version" minimum:"1" doc:"the item version the client loaded"`
				SongID   string `json:"song_id" maxLength:"26"`
				Position *int   `json:"position,omitempty" minimum:"0"`
			}
		}) (*itemResultOutput, error) {
			b := in.Body
			r, err := d.Liturgies.AddSong(ctx, sess(ctx), domain.LiturgyID(in.ID), domain.ItemID(in.ItemID), b.Version, domain.SongID(b.SongID), b.Position)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("item_songs_set", "actor", actor(ctx), "liturgy_id", in.ID, "item_id", in.ItemID)
			return &itemResultOutput{Body: itemResultView(r)}, nil
		})
	huma.Register(api, lop("setItemSongs", http.MethodPut, "/liturgies/{id}/items/{iid}/songs", http.StatusOK, "Replace all songs of an item; every song and entry ID is new"),
		func(ctx context.Context, in *struct {
			ID     string `path:"id" maxLength:"26"`
			ItemID string `path:"iid" maxLength:"26"`
			Body   struct {
				Version int             `json:"version" minimum:"1" doc:"the item version the client loaded"`
				Songs   []ItemSongInput `json:"songs" maxItems:"100" doc:"0 to 10 songs"`
			}
		}) (*itemResultOutput, error) {
			songs := make([]app.ItemSongInput, len(in.Body.Songs))
			for i, s := range in.Body.Songs {
				songs[i] = app.ItemSongInput{SongID: domain.SongID(s.SongID), Key: s.Key, Note: s.Note, Entries: make([]app.EntryInput, len(s.Entries))}
				for j, e := range s.Entries {
					songs[i].Entries[j] = app.EntryInput{SectionID: domain.SectionID(e.SectionID), SingingPartID: domain.SingingPartID(e.SingingPartID),
						KeyChange: e.KeyChange, Note: e.Note}
				}
			}
			r, err := d.Liturgies.SetSongs(ctx, sess(ctx), domain.LiturgyID(in.ID), domain.ItemID(in.ItemID), in.Body.Version, songs)
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("item_songs_set", "actor", actor(ctx), "liturgy_id", in.ID, "item_id", in.ItemID)
			return &itemResultOutput{Body: itemResultView(r)}, nil
		})

	// --- assignments ---

	huma.Register(api, lop("addAssignment", http.MethodPost, "/liturgies/{id}/assignments", http.StatusCreated, "Assign a member or a named person to a duty"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				DutyID string `json:"duty_id" maxLength:"26"`
				UserID string `json:"user_id,omitempty" maxLength:"26" doc:"a member of the church; exactly one of user_id and name"`
				Name   string `json:"name,omitempty" maxLength:"2000"`
			}
		}) (*struct{ Body AssignmentView }, error) {
			b := in.Body
			v, err := d.Liturgies.AddAssignment(ctx, sess(ctx), domain.LiturgyID(in.ID), app.AssignmentInput{DutyID: domain.DutyID(b.DutyID),
				UserID: domain.UserID(b.UserID), Name: b.Name})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("assignment_added", "actor", actor(ctx), "liturgy_id", in.ID, "assignment_id", string(v.Assignment.ID))
			return &struct{ Body AssignmentView }{Body: assignmentView(v)}, nil
		})
	huma.Register(api, lop("removeAssignment", http.MethodDelete, "/liturgies/{id}/assignments/{aid}", http.StatusNoContent, "Remove an assignment"),
		func(ctx context.Context, in *struct {
			ID           string `path:"id" maxLength:"26"`
			AssignmentID string `path:"aid" maxLength:"26"`
		}) (*struct{}, error) {
			if err := d.Liturgies.RemoveAssignment(ctx, sess(ctx), domain.LiturgyID(in.ID), domain.AssignmentID(in.AssignmentID)); err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("assignment_removed", "actor", actor(ctx), "liturgy_id", in.ID, "assignment_id", in.AssignmentID)
			return nil, nil
		})
}
