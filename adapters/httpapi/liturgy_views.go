// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// JSON shapes of liturgies, items, songs, assignments and history (10 §4).

// LiturgySummaryView is a liturgy in a list.
type LiturgySummaryView struct {
	ID          string             `json:"id"`
	Date        string             `json:"date" pattern:"^\\d{4}-\\d{2}-\\d{2}$"`
	Time        string             `json:"time" doc:"HH:MM in the church's time zone, empty for a one-off with no time"`
	ServiceName string             `json:"service_name"`
	Language    string             `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	State       string             `json:"state" enum:"draft,in_review,needs_revision,approved,published"`
	ItemCount   int                `json:"item_count"`
	Version     int                `json:"version"`
	Actions     app.LiturgyActions `json:"actions"`
}

// SectionRefView is a section of a song without its text.
type SectionRefView struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Number int     `json:"number" doc:"verses only; 0 otherwise"`
	Label  *string `json:"label" doc:"null: derive from kind and number"`
}

// LiturgySongSummaryView is what the editor shows of an item song's song: no lyrics.
type LiturgySongSummaryView struct {
	Title        string           `json:"title"`
	Language     string           `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	HymnalSource string           `json:"hymnal_source"`
	HymnalNumber string           `json:"hymnal_number"`
	DefaultKey   string           `json:"default_key"`
	Sections     []SectionRefView `json:"sections"`
}

// EntryView is one entry of a sequence.
type EntryView struct {
	ID            string  `json:"id"`
	Position      int     `json:"position"`
	SectionID     *string `json:"section_id" doc:"null: the section was deleted; section_label keeps its name"`
	SingingPartID *string `json:"singing_part_id"`
	KeyChange     string  `json:"key_change"`
	Note          string  `json:"note"`
	SectionLabel  string  `json:"section_label"`
}

// ItemSongView is a song in a song item with its sequence.
type ItemSongView struct {
	ID        string                  `json:"id"`
	Position  int                     `json:"position"`
	SongID    *string                 `json:"song_id" doc:"null: the song was deleted; song_title keeps its title"`
	SongTitle string                  `json:"song_title"`
	Song      *LiturgySongSummaryView `json:"song" doc:"null when the song was deleted"`
	Key       string                  `json:"key"`
	Note      string                  `json:"note"`
	Entries   []EntryView             `json:"entries"`
}

// LiturgyReadingView is the reading of a reading item, with its text.
type LiturgyReadingView struct {
	ID               string             `json:"id"`
	Reference        string             `json:"reference"`
	ReferenceDisplay string             `json:"reference_display"`
	Translation      TranslationRefView `json:"translation"`
	Text             string             `json:"text"`
	Attribution      string             `json:"attribution"`
}

// LiturgyItemView is one item of a liturgy.
type LiturgyItemView struct {
	ID           string              `json:"id"`
	Position     int                 `json:"position"`
	Title        string              `json:"title"`
	ItemType     string              `json:"item_type" enum:"song,reading,prayer,sermon,free_text,other"`
	DutyID       *string             `json:"duty_id"`
	Text         string              `json:"text"`
	ReadingID    *string             `json:"reading_id" doc:"null: not chosen yet, or the reading was deleted"`
	ReadingLabel string              `json:"reading_label" doc:"the reading's display text, kept when it is deleted"`
	Reading      *LiturgyReadingView `json:"reading"`
	Songs        []ItemSongView      `json:"songs"`
	Version      int                 `json:"version" doc:"guards this item's content; send it back with every change of the item"`
}

// AssignmentView is a person on a duty.
type AssignmentView struct {
	ID           string  `json:"id"`
	DutyID       string  `json:"duty_id"`
	UserID       *string `json:"user_id"`
	Name         string  `json:"name" doc:"the member's name, or the free-text name"`
	FormerMember bool    `json:"former_member" doc:"the member has left the church since; computed on every read"`
}

// ProblemView is something the editor shows and step 4 will check before a submit.
type ProblemView struct {
	Code       string  `json:"code" enum:"song_missing,reading_missing,song_removed,reading_removed,section_removed"`
	ItemID     string  `json:"item_id"`
	ItemSongID *string `json:"item_song_id,omitempty"`
	EntryID    *string `json:"entry_id,omitempty"`
}

// LiturgyView is a whole liturgy.
type LiturgyView struct {
	ID          string  `json:"id"`
	Date        string  `json:"date" pattern:"^\\d{4}-\\d{2}-\\d{2}$"`
	Time        string  `json:"time"`
	ServiceID   *string `json:"service_id" doc:"null for a one-off, and after the service is deleted"`
	ServiceName string  `json:"service_name"`
	Language    string  `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	TemplateID  *string `json:"template_id"`
	State       string  `json:"state" enum:"draft,in_review,needs_revision,approved,published"`
	Version     int     `json:"version" doc:"guards the structure: the fields, and which items exist in what order"`
	EditSeq     int     `json:"edit_seq" doc:"the number of the newest history row; approve and request changes must send the one the reviewer saw"`
	// Review data: absent for a member without a liturgy scope (12 §2, P-74).
	OpenComments *int               `json:"open_comments,omitempty" doc:"the number of unresolved comments"`
	LastChange   *StateChangeView   `json:"last_change,omitempty" doc:"the newest state change; absent when the liturgy never changed state"`
	Items        []LiturgyItemView  `json:"items"`
	Assignments  []AssignmentView   `json:"assignments"`
	Problems     []ProblemView      `json:"problems"`
	Actions      app.LiturgyActions `json:"actions"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// UserRefView names a person.
type UserRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StateChangeView is one row of the review history (12 §3).
type StateChangeView struct {
	ID        string      `json:"id"`
	FromState string      `json:"from_state" enum:"draft,in_review,needs_revision,approved,published"`
	ToState   string      `json:"to_state" enum:"draft,in_review,needs_revision,approved,published"`
	User      UserRefView `json:"user"`
	Note      string      `json:"note"`
	EditSeq   int         `json:"edit_seq"`
	CreatedAt time.Time   `json:"created_at"`
}

func stateChangeView(v app.StateChangeView) StateChangeView {
	c := v.Change
	return StateChangeView{ID: string(c.ID), FromState: string(c.From), ToState: string(c.To),
		User: UserRefView{ID: string(c.UserID), Name: v.UserName}, Note: c.Note, EditSeq: c.EditSeq, CreatedAt: c.CreatedAt}
}

// CommentView is a comment (12 §3).
type CommentView struct {
	ID         string       `json:"id"`
	ItemID     *string      `json:"item_id" doc:"null: the whole liturgy. The item may have been removed since; item_title keeps its name"`
	ItemTitle  string       `json:"item_title" doc:"the item's title when the comment was written"`
	Author     UserRefView  `json:"author"`
	Body       string       `json:"body" doc:"plain text"`
	Resolved   bool         `json:"resolved"`
	ResolvedBy *UserRefView `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time   `json:"resolved_at,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

func commentView(v app.CommentView) CommentView {
	c := v.Comment
	out := CommentView{ID: string(c.ID), ItemID: optional(string(c.ItemID)), ItemTitle: c.ItemTitle,
		Author: UserRefView{ID: string(c.AuthorID), Name: v.AuthorName}, Body: c.Body, Resolved: c.Resolved(), ResolvedAt: c.ResolvedAt, CreatedAt: c.CreatedAt}
	if c.Resolved() {
		out.ResolvedBy = &UserRefView{ID: string(c.ResolvedBy), Name: v.ResolverName}
	}
	return out
}

// ItemResultView is the answer of a write to one item.
type ItemResultView struct {
	Item           LiturgyItemView `json:"item"`
	LiturgyVersion int             `json:"liturgy_version"`
}

// OrderResultView is the answer of a remove or reorder.
type OrderResultView struct {
	LiturgyVersion int      `json:"liturgy_version"`
	ItemIDs        []string `json:"item_ids" doc:"the items in order"`
}

// EditView is a row of the history.
type EditView struct {
	ID                  string          `json:"id"`
	Seq                 int             `json:"seq"`
	UserID              string          `json:"user_id"`
	UserName            string          `json:"user_name"`
	Command             string          `json:"command"`
	ItemID              *string         `json:"item_id"`
	Before              json.RawMessage `json:"before" doc:"JSON image, null when none" nullable:"true"`
	After               json.RawMessage `json:"after" doc:"JSON image, null when none" nullable:"true"`
	LiturgyVersionAfter int             `json:"liturgy_version_after"`
	ItemVersionAfter    *int            `json:"item_version_after"`
	Status              string          `json:"status" enum:"done,undone,dropped"`
	CreatedAt           time.Time       `json:"created_at"`
}

// UndoResultView is the answer of an undo or redo.
type UndoResultView struct {
	Edit           UndoneEditView `json:"edit" doc:"the edit that was undone or redone, with its new status"`
	LiturgyVersion int            `json:"liturgy_version"`
	ItemID         *string        `json:"item_id" doc:"the item the change was about, null when none" nullable:"true"`
	ItemVersion    *int           `json:"item_version" doc:"its version after the change, null when none" nullable:"true"`
}

// UndoneEditView names the edit an undo or redo acted on.
type UndoneEditView struct {
	ID      string  `json:"id"`
	Seq     int     `json:"seq"`
	Command string  `json:"command"`
	ItemID  *string `json:"item_id" nullable:"true"`
	Status  string  `json:"status" enum:"done,undone,dropped"`
}

func undoResultView(r app.UndoResult) UndoResultView {
	out := UndoResultView{Edit: UndoneEditView{ID: string(r.Edit.ID), Seq: r.Edit.Seq, Command: r.Edit.Command,
		ItemID: optional(string(r.Edit.ItemID)), Status: r.Edit.Status}, LiturgyVersion: r.LiturgyVersion, ItemID: optional(string(r.ItemID))}
	if r.ItemVersion != 0 {
		n := r.ItemVersion
		out.ItemVersion = &n
	}
	return out
}

// OccurrenceView is one service time in a week.
type OccurrenceView struct {
	ServiceID    string  `json:"service_id"`
	ServiceName  string  `json:"service_name"`
	Language     string  `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Date         string  `json:"date"`
	Time         string  `json:"time"`
	TemplateID   *string `json:"template_id"`
	TemplateName *string `json:"template_name"`
	LiturgyID    *string `json:"liturgy_id" doc:"set when a liturgy for the slot exists"`
}

// LimitUseView is one limit with its use.
type LimitUseView struct {
	Unlimited bool `json:"unlimited"`
	Max       int  `json:"max"`
	Used      int  `json:"used"`
}

// PrepareWeekView is the answer of GET /liturgies/prepare.
type PrepareWeekView struct {
	Week        string           `json:"week" doc:"the Monday of the week"`
	Occurrences []OccurrenceView `json:"occurrences"`
	Limits      struct {
		Active      LimitUseView `json:"max_active_liturgies"`
		Unpublished LimitUseView `json:"max_unpublished_liturgies"`
	} `json:"limits"`
}

// PreparedView is a liturgy made by POST /liturgies/prepare.
type PreparedView struct {
	LiturgyID    string  `json:"liturgy_id"`
	ServiceName  string  `json:"service_name"`
	Date         string  `json:"date"`
	Time         string  `json:"time"`
	Language     string  `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	TemplateID   *string `json:"template_id"`
	TemplateName *string `json:"template_name"`
}

func entryViews(es []domain.Entry) []EntryView {
	out := make([]EntryView, len(es))
	for i, e := range es {
		out[i] = EntryView{ID: string(e.ID), Position: e.Position, SectionID: optional(string(e.SectionID)),
			SingingPartID: optional(string(e.SingingPartID)), KeyChange: e.KeyChange, Note: e.Note, SectionLabel: e.SectionLabel}
	}
	return out
}

func liturgyItemView(v app.ItemView) LiturgyItemView {
	it := v.Item
	out := LiturgyItemView{ID: string(it.ID), Position: it.Position, Title: it.Title, ItemType: string(it.Type), DutyID: optional(string(it.DutyID)),
		Text: it.Text, ReadingID: optional(string(it.ReadingID)), ReadingLabel: it.ReadingLabel, Version: it.Version,
		Songs: make([]ItemSongView, len(v.Songs))}
	if r := v.Reading; r != nil {
		out.Reading = &LiturgyReadingView{ID: string(r.Reading.ID), Reference: r.Reading.Reference, ReferenceDisplay: r.Reading.ReferenceDisplay,
			Translation: translationRefView(r.Reading.Translation), Text: r.Reading.Text, Attribution: r.Reading.Attribution}
	}
	for i, s := range v.Songs {
		sv := ItemSongView{ID: string(s.Song.ID), Position: s.Song.Position, SongID: optional(string(s.Song.SongID)), SongTitle: s.Song.SongTitle,
			Key: s.Song.Key, Note: s.Song.Note, Entries: entryViews(s.Song.Entries)}
		if sum := s.Summary; sum != nil {
			sv.Song = &LiturgySongSummaryView{Title: sum.Title, Language: sum.Language, HymnalSource: sum.HymnalSource, HymnalNumber: sum.HymnalNumber,
				DefaultKey: sum.DefaultKey, Sections: make([]SectionRefView, len(sum.Sections))}
			for j, sec := range sum.Sections {
				sv.Song.Sections[j] = SectionRefView{ID: string(sec.ID), Kind: string(sec.Kind), Number: sec.Number, Label: optional(sec.Label)}
			}
		}
		out.Songs[i] = sv
	}
	return out
}

func assignmentView(v app.AssignmentView) AssignmentView {
	return AssignmentView{ID: string(v.Assignment.ID), DutyID: string(v.Assignment.DutyID), UserID: optional(string(v.Assignment.UserID)),
		Name: v.UserName, FormerMember: v.FormerMember}
}

func liturgyView(v app.LiturgyView) LiturgyView {
	l := v.Liturgy
	out := LiturgyView{ID: string(l.ID), Date: l.Date, Time: l.Time, ServiceID: optional(string(l.ServiceID)), ServiceName: l.ServiceName,
		Language: l.Language, TemplateID: optional(string(l.TemplateID)), State: string(l.State), Version: l.Version, Actions: v.Actions,
		Items: make([]LiturgyItemView, len(v.Items)), Assignments: make([]AssignmentView, len(v.Assignments)),
		Problems: make([]ProblemView, len(v.Problems)), CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt, EditSeq: l.EditSeq}
	if v.Review != nil {
		n := v.Review.OpenComments
		out.OpenComments = &n
		if v.Review.LastChange != nil {
			c := stateChangeView(*v.Review.LastChange)
			out.LastChange = &c
		}
	}
	for i, it := range v.Items {
		out.Items[i] = liturgyItemView(it)
	}
	for i, a := range v.Assignments {
		out.Assignments[i] = assignmentView(a)
	}
	for i, p := range v.Problems {
		out.Problems[i] = ProblemView{Code: p.Code, ItemID: string(p.ItemID), ItemSongID: optional(string(p.ItemSongID)), EntryID: optional(string(p.EntryID))}
	}
	return out
}

func editView(v app.EditView) EditView {
	e := v.Edit
	out := EditView{ID: string(e.ID), Seq: e.Seq, UserID: string(e.UserID), UserName: v.UserName, Command: e.Command, ItemID: optional(string(e.ItemID)),
		Before: e.Before, After: e.After, LiturgyVersionAfter: e.LiturgyVersionAfter, Status: e.Status, CreatedAt: e.CreatedAt}
	if e.ItemVersionAfter != 0 {
		n := e.ItemVersionAfter
		out.ItemVersionAfter = &n
	}
	return out
}

func limitUseView(l app.PrepareLimit) LimitUseView {
	return LimitUseView{Unlimited: l.Unlimited, Max: l.Max, Used: l.Used}
}
