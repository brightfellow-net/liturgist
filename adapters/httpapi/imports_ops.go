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

// maxImportBody is the body limit of POST /imports (08 §5); every other
// operation keeps Huma's 1 MiB.
const maxImportBody = 6 << 20

// DraftSectionBody is a section of a draft song.
type DraftSectionBody struct {
	Kind   string `json:"kind" enum:"verse,pre_chorus,chorus,bridge,tag,intro,ending,other"`
	Number int    `json:"number,omitempty" minimum:"0" maximum:"1000" doc:"verses only (1-99)"`
	Label  string `json:"label,omitempty" maxLength:"1000"`
	Text   string `json:"text" maxLength:"100000"`
}

// SongDraftBody is a song found in a document, before it is saved (08 §3).
type SongDraftBody struct {
	Language           string             `json:"language" enum:"id,en,zh-Hans,zh-Hant"`
	Title              string             `json:"title" maxLength:"2000"`
	AltTitles          []string           `json:"alt_titles" maxItems:"100"`
	HymnalSource       string             `json:"hymnal_source,omitempty" maxLength:"1000"`
	HymnalNumber       string             `json:"hymnal_number,omitempty" maxLength:"1000"`
	Lyricist           string             `json:"lyricist,omitempty" maxLength:"2000"`
	Composer           string             `json:"composer,omitempty" maxLength:"2000"`
	Translator         string             `json:"translator,omitempty" maxLength:"2000"`
	DefaultKey         string             `json:"default_key,omitempty" maxLength:"100"`
	CopyrightHolder    string             `json:"copyright_holder,omitempty" maxLength:"2000"`
	CopyrightLine      string             `json:"copyright_line,omitempty" maxLength:"5000"`
	CCLISongNumber     string             `json:"ccli_song_number,omitempty" maxLength:"100"`
	LicenceStatus      string             `json:"licence_status,omitempty" enum:"unknown,public_domain,church_licence,permission_obtained"`
	LicenceNotes       string             `json:"licence_notes,omitempty" maxLength:"20000"`
	Sections           []DraftSectionBody `json:"sections" maxItems:"200"`
	DefaultArrangement []int              `json:"default_arrangement" maxItems:"1000" doc:"indexes into sections"`
}

func (b SongDraftBody) domain() domain.SongDraft {
	d := domain.SongDraft{Language: b.Language, Title: b.Title, AltTitles: b.AltTitles, HymnalSource: b.HymnalSource,
		HymnalNumber: b.HymnalNumber, Lyricist: b.Lyricist, Composer: b.Composer, Translator: b.Translator,
		DefaultKey: b.DefaultKey, CopyrightHolder: b.CopyrightHolder, CopyrightLine: b.CopyrightLine,
		CCLISongNumber: b.CCLISongNumber, LicenceStatus: domain.LicenceStatus(b.LicenceStatus), LicenceNotes: b.LicenceNotes,
		DefaultArrangement: b.DefaultArrangement}
	for _, s := range b.Sections {
		d.Sections = append(d.Sections, domain.DraftSection{Kind: domain.SectionKind(s.Kind), Number: s.Number, Label: s.Label, Text: s.Text})
	}
	return d
}

func draftBody(d domain.SongDraft) SongDraftBody {
	b := SongDraftBody{Language: d.Language, Title: d.Title, AltTitles: d.AltTitles, HymnalSource: d.HymnalSource,
		HymnalNumber: d.HymnalNumber, Lyricist: d.Lyricist, Composer: d.Composer, Translator: d.Translator,
		DefaultKey: d.DefaultKey, CopyrightHolder: d.CopyrightHolder, CopyrightLine: d.CopyrightLine,
		CCLISongNumber: d.CCLISongNumber, LicenceStatus: string(d.LicenceStatus), LicenceNotes: d.LicenceNotes,
		Sections: []DraftSectionBody{}, DefaultArrangement: d.DefaultArrangement}
	if b.AltTitles == nil {
		b.AltTitles = []string{}
	}
	if b.DefaultArrangement == nil {
		b.DefaultArrangement = []int{}
	}
	for _, s := range d.Sections {
		b.Sections = append(b.Sections, DraftSectionBody{Kind: string(s.Kind), Number: s.Number, Label: s.Label, Text: s.Text})
	}
	return b
}

// DuplicateView is an existing song a candidate may duplicate.
type DuplicateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	HymnalSource string `json:"hymnal_source"`
	HymnalNumber string `json:"hymnal_number"`
}

// ImportCandidateView is one candidate song of a batch.
type ImportCandidateView struct {
	ID                 string         `json:"id"`
	Draft              SongDraftBody  `json:"draft"`
	DuplicateOf        *DuplicateView `json:"duplicate_of,omitempty" doc:"absent when there is none"`
	Decision           string         `json:"decision" enum:"pending,accept,merge,skip"`
	MergeInto          string         `json:"merge_into,omitempty"`
	MergeTargetVersion int            `json:"merge_target_version,omitempty"`
	RemoveUnmatched    bool           `json:"remove_unmatched"`
	Warnings           []string       `json:"warnings"`
	Outcome            string         `json:"outcome,omitempty" enum:"applied,failed"`
	AppliedSongID      string         `json:"applied_song_id,omitempty"`
	ErrorCode          string         `json:"error_code,omitempty"`
}

// ImportRejectedView is a file or song that gave no candidate.
type ImportRejectedView struct {
	Name      string `json:"name"`
	SongIndex int    `json:"song_index"`
	Reason    string `json:"reason"`
}

// ImportBatchView is a batch with its candidates.
type ImportBatchView struct {
	ID           string                `json:"id"`
	SourceFormat string                `json:"source_format" enum:"paste,openlyrics,chordpro"`
	Status       string                `json:"status" enum:"open,closed"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Candidates   []ImportCandidateView `json:"candidates"`
	Rejected     []ImportRejectedView  `json:"rejected"`
}

func candidateView(c app.CandidateView) ImportCandidateView {
	v := ImportCandidateView{ID: string(c.ID), Draft: draftBody(c.Draft), Decision: string(c.Decision),
		MergeInto: string(c.MergeInto), MergeTargetVersion: c.MergeTargetVersion, RemoveUnmatched: c.RemoveUnmatched,
		Warnings: c.Warnings, Outcome: string(c.Outcome), AppliedSongID: string(c.AppliedSongID), ErrorCode: c.ErrorCode}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	if c.DuplicateOf != nil {
		v.DuplicateOf = &DuplicateView{ID: string(c.DuplicateOf.ID), Title: c.DuplicateOf.Title,
			HymnalSource: c.DuplicateOf.HymnalSource, HymnalNumber: c.DuplicateOf.HymnalNumber}
	}
	return v
}

func batchView(b app.BatchView) ImportBatchView {
	v := ImportBatchView{ID: string(b.Batch.ID), SourceFormat: string(b.Batch.Format), Status: string(b.Batch.Status),
		CreatedAt: b.Batch.CreatedAt, UpdatedAt: b.Batch.UpdatedAt, Candidates: make([]ImportCandidateView, len(b.Candidates)),
		Rejected: []ImportRejectedView{}}
	for i, c := range b.Candidates {
		v.Candidates[i] = candidateView(c)
	}
	for _, r := range b.Rejected {
		v.Rejected = append(v.Rejected, ImportRejectedView{Name: r.Name, SongIndex: r.SongIndex, Reason: r.Reason})
	}
	return v
}

// ImportSummaryView is an unfinished batch in the list.
type ImportSummaryView struct {
	ID           string    `json:"id"`
	SourceFormat string    `json:"source_format" enum:"paste,openlyrics,chordpro"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// MergeSectionView is one line of a merge preview.
type MergeSectionView struct {
	Status  string `json:"status" enum:"updated,new,kept,removed"`
	Label   string `json:"label"`
	OldText string `json:"old_text,omitempty"`
	NewText string `json:"new_text,omitempty"`
}

// MergePreviewView is exactly what Apply would do now.
type MergePreviewView struct {
	TargetVersion int                `json:"target_version"`
	Sections      []MergeSectionView `json:"sections"`
}

// ApplyFailureView is a candidate that could not be applied.
type ApplyFailureView struct {
	CandidateID string `json:"candidate_id"`
	Code        string `json:"code"`
}

// ApplyResultView counts what an Apply did.
type ApplyResultView struct {
	Created int                `json:"created"`
	Merged  int                `json:"merged"`
	Skipped int                `json:"skipped"`
	Failed  []ApplyFailureView `json:"failed"`
	Status  string             `json:"status" enum:"open,closed"`
}

type importBatchOutput struct{ Body ImportBatchView }

type importPath struct {
	ID string `path:"id" maxLength:"26"`
}

// registerImports registers the import operations (08 §5). Every one needs library.edit.
func registerImports(api huma.API, d LibraryDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	importOp := func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), "library")
	}

	create := importOp("createImport", http.MethodPost, "/imports", http.StatusCreated, "Parse files into an import batch")
	create.MaxBodyBytes = maxImportBody
	huma.Register(api, create, func(ctx context.Context, in *struct {
		Body struct {
			Format   string `json:"format" enum:"paste,openlyrics,chordpro"`
			Language string `json:"language,omitempty" enum:"id,en,zh-Hans,zh-Hant" doc:"default: the church's content language"`
			Files    []struct {
				Name string `json:"name" maxLength:"1000" doc:"the file name; for pasted lyrics, the title"`
				Text string `json:"text" maxLength:"6291456"`
			} `json:"files" maxItems:"1000" doc:"at most 200; pasted lyrics: exactly one"`
		}
	}) (*importBatchOutput, error) {
		req := app.ImportRequest{Format: in.Body.Format, Language: in.Body.Language}
		for _, f := range in.Body.Files {
			req.Files = append(req.Files, app.ImportFile{Name: f.Name, Text: f.Text})
		}
		v, err := d.Imports.Create(ctx, sess(ctx), req)
		if err != nil {
			return nil, fail(ctx, err)
		}
		d.Log.Info("import_created", "actor", string(sess(ctx).UserID), "batch_id", string(v.Batch.ID),
			"format", string(v.Batch.Format), "candidates", len(v.Candidates))
		return &importBatchOutput{Body: batchView(v)}, nil
	})

	huma.Register(api, importOp("listImports", http.MethodGet, "/imports", http.StatusOK, "Imports that are not finished"),
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Items []ImportSummaryView `json:"items"`
			}
		}, error) {
			list, err := d.Imports.Open(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &struct {
				Body struct {
					Items []ImportSummaryView `json:"items"`
				}
			}{}
			out.Body.Items = make([]ImportSummaryView, len(list))
			for i, b := range list {
				out.Body.Items[i] = ImportSummaryView{ID: string(b.ID), SourceFormat: string(b.Format), CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
			}
			return out, nil
		})

	huma.Register(api, importOp("getImport", http.MethodGet, "/imports/{id}", http.StatusOK, "An import batch with its candidates"),
		func(ctx context.Context, in *importPath) (*importBatchOutput, error) {
			v, err := d.Imports.Get(ctx, sess(ctx), domain.ImportBatchID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &importBatchOutput{Body: batchView(v)}, nil
		})

	huma.Register(api, importOp("updateImportCandidate", http.MethodPatch, "/imports/{id}/candidates/{cid}", http.StatusOK, "Change a candidate's draft"),
		func(ctx context.Context, in *struct {
			ID          string `path:"id" maxLength:"26"`
			CandidateID string `path:"cid" maxLength:"26"`
			Body        struct {
				Draft SongDraftBody `json:"draft"`
			}
		}) (*struct{ Body ImportCandidateView }, error) {
			v, err := d.Imports.EditDraft(ctx, sess(ctx), domain.ImportBatchID(in.ID), domain.ImportCandidateID(in.CandidateID), in.Body.Draft.domain())
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &struct{ Body ImportCandidateView }{candidateView(v)}, nil
		})

	huma.Register(api, importOp("decideImportCandidates", http.MethodPatch, "/imports/{id}/candidates", http.StatusOK, "Accept, merge or skip candidates"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Decisions []struct {
					ID                 string `json:"id" maxLength:"26"`
					Decision           string `json:"decision" enum:"pending,accept,merge,skip"`
					MergeInto          string `json:"merge_into,omitempty" maxLength:"26"`
					MergeTargetVersion int    `json:"merge_target_version,omitempty" minimum:"0" doc:"the target_version of the merge preview"`
					RemoveUnmatched    bool   `json:"remove_unmatched,omitempty"`
				} `json:"decisions" maxItems:"500"`
			}
		}) (*importBatchOutput, error) {
			decisions := make([]app.DecisionInput, len(in.Body.Decisions))
			for i, x := range in.Body.Decisions {
				decisions[i] = app.DecisionInput{ID: domain.ImportCandidateID(x.ID), Decision: domain.ImportDecision(x.Decision),
					MergeInto: domain.SongID(x.MergeInto), MergeTargetVersion: x.MergeTargetVersion, RemoveUnmatched: x.RemoveUnmatched}
			}
			v, err := d.Imports.Decide(ctx, sess(ctx), domain.ImportBatchID(in.ID), decisions)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &importBatchOutput{Body: batchView(v)}, nil
		})

	huma.Register(api, importOp("previewImportMerge", http.MethodGet, "/imports/{id}/candidates/{cid}/merge-preview", http.StatusOK, "What merging a candidate would change"),
		func(ctx context.Context, in *struct {
			ID              string `path:"id" maxLength:"26"`
			CandidateID     string `path:"cid" maxLength:"26"`
			MergeInto       string `query:"merge_into" maxLength:"26" doc:"the song to merge into"`
			RemoveUnmatched bool   `query:"remove_unmatched"`
		}) (*struct{ Body MergePreviewView }, error) {
			p, err := d.Imports.MergePreview(ctx, sess(ctx), domain.ImportBatchID(in.ID), domain.ImportCandidateID(in.CandidateID),
				domain.SongID(in.MergeInto), in.RemoveUnmatched)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := MergePreviewView{TargetVersion: p.TargetVersion, Sections: make([]MergeSectionView, len(p.Sections))}
			for i, s := range p.Sections {
				out.Sections[i] = MergeSectionView{Status: s.Status, Label: s.Label, OldText: s.OldText, NewText: s.NewText}
			}
			return &struct{ Body MergePreviewView }{out}, nil
		})

	huma.Register(api, importOp("applyImport", http.MethodPost, "/imports/{id}/apply", http.StatusOK, "Create or merge the chosen songs"),
		func(ctx context.Context, in *importPath) (*struct{ Body ApplyResultView }, error) {
			r, err := d.Imports.Apply(ctx, sess(ctx), domain.ImportBatchID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("import_applied", "actor", string(sess(ctx).UserID), "batch_id", in.ID,
				"created", r.Created, "merged", r.Merged, "skipped", r.Skipped, "failed", len(r.Failed))
			out := ApplyResultView{Created: r.Created, Merged: r.Merged, Skipped: r.Skipped, Status: string(r.Status),
				Failed: make([]ApplyFailureView, len(r.Failed))}
			for i, f := range r.Failed {
				out.Failed[i] = ApplyFailureView{CandidateID: string(f.CandidateID), Code: f.Code}
			}
			return &struct{ Body ApplyResultView }{out}, nil
		})

	huma.Register(api, importOp("deleteImport", http.MethodDelete, "/imports/{id}", http.StatusNoContent, "Discard an import batch"),
		func(ctx context.Context, in *importPath) (*struct{}, error) {
			if err := d.Imports.Discard(ctx, sess(ctx), domain.ImportBatchID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			return nil, nil
		})
}
