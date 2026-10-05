// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// noStore is the cache header of the published routes: archiving, reopening
// and print settings change an answer without changing its number, so no
// validator is offered (13 §5).
const noStore = "private, no-store"

// PublishedRefView is a duty or singing part as it was named when published.
type PublishedRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PublishedSectionView is a section of a published song; its label is fixed.
type PublishedSectionView struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Number int    `json:"number"`
	Label  string `json:"label"`
	Text   string `json:"text"`
}

// PublishedEntryView is one entry of a published sequence.
type PublishedEntryView struct {
	SectionID string            `json:"section_id"`
	Part      *PublishedRefView `json:"part,omitempty" doc:"absent when the whole congregation or nobody in particular sings"`
	KeyChange string            `json:"key_change"`
	Note      string            `json:"note"`
}

// PublishedSongView is a song of a published item.
type PublishedSongView struct {
	SongID          string                 `json:"song_id"`
	Title           string                 `json:"title"`
	HymnalSource    string                 `json:"hymnal_source"`
	HymnalNumber    string                 `json:"hymnal_number"`
	Key             string                 `json:"key" doc:"letters, e.g. G or Bb; the display form is applied by the reader"`
	Note            string                 `json:"note"`
	CopyrightHolder string                 `json:"copyright_holder"`
	CopyrightLine   string                 `json:"copyright_line"`
	CCLISongNumber  string                 `json:"ccli_song_number"`
	Sections        []PublishedSectionView `json:"sections" doc:"only the sections the sequence uses, each once"`
	Entries         []PublishedEntryView   `json:"entries"`
}

// PublishedReadingView is the reading of a published item.
type PublishedReadingView struct {
	ReferenceDisplay string `json:"reference_display"`
	TranslationCode  string `json:"translation_code"`
	Text             string `json:"text"`
	Attribution      string `json:"attribution"`
}

// PublishedItemView is an item of a published copy.
type PublishedItemView struct {
	ID       string                `json:"id"`
	Position int                   `json:"position"`
	Type     string                `json:"type"`
	Title    string                `json:"title"`
	Duty     *PublishedRefView     `json:"duty,omitempty" doc:"absent when the item has no duty"`
	Text     string                `json:"text"`
	Reading  *PublishedReadingView `json:"reading,omitempty" doc:"absent for an item that is not a reading"`
	Songs    []PublishedSongView   `json:"songs"`
}

// PublishedAssignmentView is a person on a duty as shown when published.
type PublishedAssignmentView struct {
	Duty   PublishedRefView `json:"duty"`
	UserID *string          `json:"user_id" nullable:"true" doc:"null for a free-text name"`
	Name   string           `json:"name"`
}

// PublishedLiturgyView is the header of a published copy.
type PublishedLiturgyView struct {
	Date        string `json:"date"`
	Time        string `json:"time"`
	ServiceName string `json:"service_name"`
	Language    string `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	ChurchName  string `json:"church_name"`
}

// PublishedContentView is the self-contained copy (13 §3). Text is plain text
// with newlines and is never HTML.
type PublishedContentView struct {
	Format        int                       `json:"format" doc:"1; a reader that meets a larger number asks for an update"`
	Liturgy       PublishedLiturgyView      `json:"liturgy"`
	Items         []PublishedItemView       `json:"items"`
	Assignments   []PublishedAssignmentView `json:"assignments"`
	LicenceFooter string                    `json:"licence_footer"`
}

// PublishedRenderView is how the church wants the copy shown now.
type PublishedRenderView struct {
	KeyDisplay  string            `json:"key_display" enum:"do,letter"`
	ShowCredits bool              `json:"show_credits"`
	Print       PrintDefaultsView `json:"print"`
}

// PublishedCopyView is the answer of GET /liturgies/{id}/published.
type PublishedCopyView struct {
	Number      int                  `json:"number"`
	PublishedAt time.Time            `json:"published_at"`
	PublishedBy UserRefView          `json:"published_by"`
	Revising    bool                 `json:"revising" doc:"the liturgy has been reopened: this copy is the last one published"`
	Archived    bool                 `json:"archived"`
	Content     PublishedContentView `json:"content"`
	Render      PublishedRenderView  `json:"render"`
	URL         string               `json:"url" doc:"the shareable link; it needs a login"`
}

// PublishedListItemView is a row of the published list.
type PublishedListItemView struct {
	ID          string    `json:"id" doc:"the liturgy's ID"`
	Date        string    `json:"date"`
	Time        string    `json:"time"`
	ServiceName string    `json:"service_name"`
	Language    string    `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Number      int       `json:"number"`
	PublishedAt time.Time `json:"published_at"`
	Revising    bool      `json:"revising"`
	Archived    bool      `json:"archived"`
}

// MyAssignmentLiturgyView is the header of a card of "my assignments".
type MyAssignmentLiturgyView struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	ServiceName string `json:"service_name"`
	Language    string `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Number      int    `json:"number"`
	Revising    bool   `json:"revising"`
}

// MyAssignmentItemView is an item of the viewer on a card.
type MyAssignmentItemView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// MyAssignmentView is one card.
type MyAssignmentView struct {
	Liturgy MyAssignmentLiturgyView `json:"liturgy"`
	Duties  []PublishedRefView      `json:"duties"`
	Items   []MyAssignmentItemView  `json:"items"`
}

func refView(r *domain.PublishedRef) *PublishedRefView {
	if r == nil {
		return nil
	}
	return &PublishedRefView{ID: r.ID, Name: r.Name}
}

func publishedContentView(c domain.PublishedContent) PublishedContentView {
	out := PublishedContentView{Format: c.Format, LicenceFooter: c.LicenceFooter,
		Liturgy: PublishedLiturgyView{Date: c.Liturgy.Date, Time: c.Liturgy.Time, ServiceName: c.Liturgy.ServiceName,
			Language: c.Liturgy.Language, ChurchName: c.Liturgy.ChurchName},
		Items: make([]PublishedItemView, len(c.Items)), Assignments: make([]PublishedAssignmentView, len(c.Assignments))}
	for i, it := range c.Items {
		v := PublishedItemView{ID: string(it.ID), Position: it.Position, Type: string(it.Type), Title: it.Title, Duty: refView(it.Duty),
			Text: it.Text, Songs: make([]PublishedSongView, len(it.Songs))}
		if r := it.Reading; r != nil {
			v.Reading = &PublishedReadingView{ReferenceDisplay: r.ReferenceDisplay, TranslationCode: r.TranslationCode, Text: r.Text, Attribution: r.Attribution}
		}
		for j, s := range it.Songs {
			sv := PublishedSongView{SongID: string(s.SongID), Title: s.Title, HymnalSource: s.HymnalSource, HymnalNumber: s.HymnalNumber, Key: s.Key,
				Note: s.Note, CopyrightHolder: s.CopyrightHolder, CopyrightLine: s.CopyrightLine, CCLISongNumber: s.CCLISongNumber,
				Sections: make([]PublishedSectionView, len(s.Sections)), Entries: make([]PublishedEntryView, len(s.Entries))}
			for k, sec := range s.Sections {
				sv.Sections[k] = PublishedSectionView{ID: string(sec.ID), Kind: string(sec.Kind), Number: sec.Number, Label: sec.Label, Text: sec.Text}
			}
			for k, e := range s.Entries {
				sv.Entries[k] = PublishedEntryView{SectionID: string(e.SectionID), Part: refView(e.Part), KeyChange: e.KeyChange, Note: e.Note}
			}
			v.Songs[j] = sv
		}
		out.Items[i] = v
	}
	for i, a := range c.Assignments {
		out.Assignments[i] = PublishedAssignmentView{Duty: PublishedRefView{ID: a.Duty.ID, Name: a.Duty.Name}, UserID: optional(string(a.UserID)), Name: a.Name}
	}
	return out
}

// SummarySongView is a song as the messages name it: no lyrics.
type SummarySongView struct {
	Title        string `json:"title"`
	HymnalSource string `json:"hymnal_source"`
	HymnalNumber string `json:"hymnal_number"`
	Key          string `json:"key" doc:"letters; the display form is applied by the reader"`
}

// SummaryReadingView is a reading as the messages name it: no text.
type SummaryReadingView struct {
	ReferenceDisplay string `json:"reference_display"`
	TranslationCode  string `json:"translation_code"`
}

// SummaryItemView is an item as the messages name it.
type SummaryItemView struct {
	Title   string              `json:"title"`
	Type    string              `json:"type"`
	Duty    *PublishedRefView   `json:"duty,omitempty"`
	Songs   []SummarySongView   `json:"songs"`
	Reading *SummaryReadingView `json:"reading,omitempty"`
}

// SummaryAssignmentView is the people on one duty.
type SummaryAssignmentView struct {
	Duty  PublishedRefView `json:"duty"`
	Names []string         `json:"names"`
}

// SummaryRecipientView is a person to message. Phone is E.164, present only
// here and only for a member who has one.
type SummaryRecipientView struct {
	Name   string   `json:"name"`
	Duties []string `json:"duties"`
	Phone  string   `json:"phone,omitempty" doc:"E.164; absent for free-text names and members with no number"`
	Member bool     `json:"member" doc:"false for a free-text name"`
}

// ItemChangeView is a change to an item.
type ItemChangeView struct {
	Kind     string            `json:"kind" enum:"added,removed,moved,retitled,duty_changed"`
	ItemID   string            `json:"item_id"`
	Title    string            `json:"title"`
	OldTitle string            `json:"old_title,omitempty"`
	Duty     *PublishedRefView `json:"duty,omitempty"`
	OldDuty  *PublishedRefView `json:"old_duty,omitempty"`
}

// SongChangeView is a change to a song of an item present in both versions.
type SongChangeView struct {
	Kind         string `json:"kind" enum:"added,removed,key_changed"`
	ItemID       string `json:"item_id"`
	ItemTitle    string `json:"item_title"`
	Title        string `json:"title"`
	HymnalSource string `json:"hymnal_source"`
	HymnalNumber string `json:"hymnal_number"`
	OldKey       string `json:"old_key"`
	NewKey       string `json:"new_key"`
	EntryKeys    bool   `json:"entry_keys" doc:"the key changes inside the song differ"`
}

// ReadingChangeView is a change of a reading's reference; an empty side means none.
type ReadingChangeView struct {
	ItemID    string `json:"item_id"`
	ItemTitle string `json:"item_title"`
	Old       string `json:"old"`
	New       string `json:"new"`
}

// AssignmentChangeView is a person added to or removed from a duty.
type AssignmentChangeView struct {
	Kind string           `json:"kind" enum:"added,removed"`
	Duty PublishedRefView `json:"duty"`
	Name string           `json:"name"`
}

// ChangesView is what the new version changed for the team (13 §8).
type ChangesView struct {
	Items       []ItemChangeView       `json:"items"`
	Songs       []SongChangeView       `json:"songs"`
	Reading     []ReadingChangeView    `json:"reading"`
	Assignments []AssignmentChangeView `json:"assignments"`
}

// PublishedSummaryView is the answer of GET /liturgies/{id}/published/summary.
type PublishedSummaryView struct {
	Number      int                     `json:"number"`
	URL         string                  `json:"url"`
	KeyDisplay  string                  `json:"key_display" enum:"do,letter"`
	Liturgy     PublishedLiturgyView    `json:"liturgy"`
	Items       []SummaryItemView       `json:"items"`
	Assignments []SummaryAssignmentView `json:"assignments"`
	Recipients  []SummaryRecipientView  `json:"recipients"`
	Changes     *ChangesView            `json:"changes,omitempty" doc:"present when a previous version exists"`
}

func summaryView(r app.PublishedSummary) PublishedSummaryView {
	out := PublishedSummaryView{Number: r.Number, URL: r.URL, KeyDisplay: r.KeyDisplay,
		Liturgy: PublishedLiturgyView{Date: r.Liturgy.Date, Time: r.Liturgy.Time, ServiceName: r.Liturgy.ServiceName, Language: r.Liturgy.Language, ChurchName: r.Liturgy.ChurchName},
		Items:   make([]SummaryItemView, len(r.Items)), Assignments: make([]SummaryAssignmentView, len(r.Assignments)), Recipients: make([]SummaryRecipientView, len(r.Recipients))}
	for i, it := range r.Items {
		v := SummaryItemView{Title: it.Title, Type: string(it.Type), Duty: refView(it.Duty), Songs: make([]SummarySongView, len(it.Songs))}
		if it.Reading != nil {
			v.Reading = &SummaryReadingView{ReferenceDisplay: it.Reading.ReferenceDisplay, TranslationCode: it.Reading.TranslationCode}
		}
		for j, s := range it.Songs {
			v.Songs[j] = SummarySongView{Title: s.Title, HymnalSource: s.HymnalSource, HymnalNumber: s.HymnalNumber, Key: s.Key}
		}
		out.Items[i] = v
	}
	for i, a := range r.Assignments {
		out.Assignments[i] = SummaryAssignmentView{Duty: PublishedRefView{ID: a.Duty.ID, Name: a.Duty.Name}, Names: a.Names}
	}
	for i, p := range r.Recipients {
		out.Recipients[i] = SummaryRecipientView{Name: p.Name, Duties: p.Duties, Phone: p.Phone, Member: p.Member}
	}
	if c := r.Changes; c != nil {
		cv := ChangesView{Items: make([]ItemChangeView, len(c.Items)), Songs: make([]SongChangeView, len(c.Songs)),
			Reading: make([]ReadingChangeView, len(c.Reading)), Assignments: make([]AssignmentChangeView, len(c.Assignments))}
		for i, x := range c.Items {
			cv.Items[i] = ItemChangeView{Kind: x.Kind, ItemID: string(x.ItemID), Title: x.Title, OldTitle: x.OldTitle, Duty: refView(x.Duty), OldDuty: refView(x.OldDuty)}
		}
		for i, x := range c.Songs {
			cv.Songs[i] = SongChangeView{Kind: x.Kind, ItemID: string(x.ItemID), ItemTitle: x.ItemTitle, Title: x.Song.Title, HymnalSource: x.Song.HymnalSource,
				HymnalNumber: x.Song.HymnalNumber, OldKey: x.OldKey, NewKey: x.NewKey, EntryKeys: x.EntryKeys}
		}
		for i, x := range c.Reading {
			cv.Reading[i] = ReadingChangeView{ItemID: string(x.ItemID), ItemTitle: x.ItemTitle, Old: x.Old, New: x.New}
		}
		for i, x := range c.Assignments {
			cv.Assignments[i] = AssignmentChangeView{Kind: x.Kind, Duty: PublishedRefView{ID: x.Duty.ID, Name: x.Duty.Name}, Name: x.Assignment.Name}
		}
		out.Changes = &cv
	}
	return out
}

// registerPublished registers the three read routes of the published copy
// (13 §5). They are open to every member of the church.
func registerPublished(api huma.API, d LiturgyDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	pop := func(id, path, summary string) huma.Operation {
		return tagged(op(id, http.MethodGet, path, TenancyChurch, http.StatusOK, summary), "published")
	}

	type listOutput struct {
		CacheControl string `header:"Cache-Control"`
		Body         struct {
			Items []PublishedListItemView `json:"items"`
			Total int                     `json:"total"`
		}
	}
	huma.Register(api, pop("listPublished", "/published", "List the published liturgies, newest first"),
		func(ctx context.Context, in *struct {
			From     string `query:"from" format:"date" doc:"first date, inclusive, YYYY-MM-DD"`
			To       string `query:"to" format:"date" doc:"last date, inclusive, YYYY-MM-DD"`
			Archived string `query:"archived" enum:"false,true,all" doc:"default false: archived liturgies are hidden"`
			Limit    int    `query:"limit" minimum:"0" maximum:"100" doc:"default 50"`
			Offset   int    `query:"offset" minimum:"0"`
		}) (*listOutput, error) {
			page, err := d.Liturgies.ListPublished(ctx, sess(ctx), app.PublishedFilter{From: in.From, To: in.To, Archived: archivedFilter(in.Archived),
				Limit: in.Limit, Offset: in.Offset})
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &listOutput{CacheControl: noStore}
			out.Body.Total = page.Total
			out.Body.Items = make([]PublishedListItemView, len(page.Items))
			for i, it := range page.Items {
				l := it.Liturgy
				out.Body.Items[i] = PublishedListItemView{ID: string(l.ID), Date: l.Date, Time: l.Time, ServiceName: l.ServiceName, Language: l.Language,
					Number: it.Number, PublishedAt: it.PublishedAt, Revising: l.State != domain.StatePublished, Archived: l.ArchivedAt != nil}
			}
			return out, nil
		})

	type copyOutput struct {
		CacheControl string `header:"Cache-Control"`
		Body         PublishedCopyView
	}
	huma.Register(api, pop("getPublished", "/liturgies/{id}/published", "The newest published version of a liturgy"),
		func(ctx context.Context, in *struct {
			ID string `path:"id" maxLength:"26"`
		}) (*copyOutput, error) {
			c, err := d.Liturgies.ReadPublished(ctx, sess(ctx), domain.LiturgyID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &copyOutput{CacheControl: noStore, Body: PublishedCopyView{Number: c.Number, PublishedAt: c.PublishedAt,
				PublishedBy: UserRefView{ID: string(c.PublishedByID), Name: c.PublishedBy}, Revising: c.Revising, Archived: c.Archived,
				Content: publishedContentView(c.Content), Render: PublishedRenderView{KeyDisplay: c.Render.KeyDisplay, ShowCredits: c.Render.ShowCredits, Print: printDefaultsView(c.Render.Print)}, URL: c.URL}}, nil
		})

	type mineOutput struct {
		CacheControl string `header:"Cache-Control"`
		Body         struct {
			Items []MyAssignmentView `json:"items"`
			More  bool               `json:"more" doc:"there are more than 50 upcoming liturgies"`
		}
	}
	huma.Register(api, pop("listMyAssignments", "/me/assignments", "The caller's duties in upcoming published liturgies, soonest first"),
		func(ctx context.Context, _ *struct{}) (*mineOutput, error) {
			res, err := d.Liturgies.MyAssignments(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &mineOutput{CacheControl: noStore}
			out.Body.More = res.More
			out.Body.Items = make([]MyAssignmentView, len(res.Items))
			for i, c := range res.Items {
				l := c.Liturgy
				v := MyAssignmentView{Liturgy: MyAssignmentLiturgyView{ID: string(l.ID), Date: l.Date, Time: l.Time, ServiceName: l.ServiceName,
					Language: l.Language, Number: c.Number, Revising: c.Revising},
					Duties: make([]PublishedRefView, len(c.Duties)), Items: make([]MyAssignmentItemView, len(c.Items))}
				for j, du := range c.Duties {
					v.Duties[j] = PublishedRefView{ID: du.ID, Name: du.Name}
				}
				for j, it := range c.Items {
					v.Items[j] = MyAssignmentItemView{ID: string(it.ID), Title: it.Title, Type: string(it.Type)}
				}
				out.Body.Items[i] = v
			}
			return out, nil
		})

	type summaryOutput struct {
		CacheControl string `header:"Cache-Control"`
		Body         PublishedSummaryView
	}
	huma.Register(api, pop("getPublishedSummary", "/liturgies/{id}/published/summary", "The data for the WhatsApp texts of the newest published version (needs liturgy.approve; carries phone numbers)"),
		func(ctx context.Context, in *struct {
			ID string `path:"id" maxLength:"26"`
		}) (*summaryOutput, error) {
			res, err := d.Liturgies.SummaryOfPublished(ctx, sess(ctx), domain.LiturgyID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &summaryOutput{CacheControl: noStore, Body: summaryView(res)}, nil
		})
}
