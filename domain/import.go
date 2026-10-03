// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"slices"
	"strconv"
	"time"
)

// Import limits (08 §5).
const (
	MaxImportSections   = 60
	MaxImportDraftBytes = 300 * 1024
	MaxImportCandidates = 500
	ImportBatchTTL      = 7 * 24 * time.Hour
)

// ImportFormat is the source of a batch.
type ImportFormat string

// Import formats of step 2.
const (
	FormatPaste      ImportFormat = "paste"
	FormatOpenLyrics ImportFormat = "openlyrics"
	FormatChordPro   ImportFormat = "chordpro"
)

// ImportFormats lists the formats a batch may be created with.
var ImportFormats = []ImportFormat{FormatPaste, FormatOpenLyrics, FormatChordPro}

// ImportStatus is the state of a batch (08 §2.1).
type ImportStatus string

// Batch states.
const (
	ImportOpen   ImportStatus = "open"
	ImportClosed ImportStatus = "closed"
)

// ImportDecision is what the member chose for a candidate.
type ImportDecision string

// Decisions.
const (
	DecisionPending ImportDecision = "pending"
	DecisionAccept  ImportDecision = "accept"
	DecisionMerge   ImportDecision = "merge"
	DecisionSkip    ImportDecision = "skip"
)

// ImportOutcome is what Apply did with a candidate.
type ImportOutcome string

// Outcomes ("" = not applied yet).
const (
	OutcomeApplied ImportOutcome = "applied"
	OutcomeFailed  ImportOutcome = "failed"
)

// Warning codes a parser or the batch adds (08 §3, §4).
const (
	WarnBlocksNumbered    = "blocks_numbered"
	WarnChorusGuessed     = "chorus_guessed"
	WarnVerseRenumbered   = "verse_renumbered"
	WarnNumberDropped     = "number_dropped"
	WarnCCLIIgnored       = "ccli_ignored"
	WarnKeyIgnored        = "key_ignored"
	WarnArrangementIgnore = "arrangement_ignored"
	WarnCommentIgnored    = "comment_ignored"
	WarnBlockIgnored      = "block_ignored"
	WarnDirectiveIgnored  = "directive_ignored"
	WarnDuplicateInBatch  = "duplicate_in_batch"
	WarnTitleFromFile     = "title_from_file"
)

// Failure codes of one candidate (08 §10).
const (
	CodeValidationFailed = "validation_failed"
	CodeSectionInUse     = "section_in_use"
	CodeNotFound         = "not_found"
	CodeTargetChanged    = "target_changed"
)

// DraftSection is a section of a draft: no ID, its position is its index.
type DraftSection struct {
	Kind   SectionKind `json:"kind"`
	Number int         `json:"number,omitempty"`
	Label  string      `json:"label,omitempty"`
	Text   string      `json:"text"`
}

// SongDraft is a song as found in a document, before it is saved (08 §3). The
// arrangement lists indexes into Sections.
type SongDraft struct {
	Language           string         `json:"language"`
	Title              string         `json:"title"`
	AltTitles          []string       `json:"alt_titles"`
	HymnalSource       string         `json:"hymnal_source,omitempty"`
	HymnalNumber       string         `json:"hymnal_number,omitempty"`
	Lyricist           string         `json:"lyricist,omitempty"`
	Composer           string         `json:"composer,omitempty"`
	Translator         string         `json:"translator,omitempty"`
	DefaultKey         string         `json:"default_key,omitempty"`
	CopyrightHolder    string         `json:"copyright_holder,omitempty"`
	CopyrightLine      string         `json:"copyright_line,omitempty"`
	CCLISongNumber     string         `json:"ccli_song_number,omitempty"`
	LicenceStatus      LicenceStatus  `json:"licence_status,omitempty"`
	LicenceNotes       string         `json:"licence_notes,omitempty"`
	Sections           []DraftSection `json:"sections"`
	DefaultArrangement []int          `json:"default_arrangement"`
}

// ImportBatch is one import in progress (08 §2).
type ImportBatch struct {
	ID        ImportBatchID
	Format    ImportFormat
	Status    ImportStatus
	CreatedBy UserID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ImportCandidate is one song found in an import.
type ImportCandidate struct {
	ID                 ImportCandidateID
	BatchID            ImportBatchID
	Position           int
	Draft              SongDraft
	DuplicateOfID      SongID // "" = none; computed again whenever it is read
	Decision           ImportDecision
	MergeInto          SongID
	MergeTargetVersion int
	RemoveUnmatched    bool
	Warnings           []string // parser warnings only
	Outcome            ImportOutcome
	AppliedSongID      SongID
	ErrorCode          string
}

// Terminal reports whether nothing more can happen to the candidate (08 §2.1).
func (c ImportCandidate) Terminal() bool {
	return c.Decision == DecisionSkip || c.Outcome == OutcomeApplied
}

// DraftSong turns a draft into a song with placeholder section IDs so the
// ordinary song rules can check it. The IDs are "0", "1" …
func DraftSong(d SongDraft) Song {
	s := Song{Language: d.Language, Title: d.Title, AltTitles: slices.Clone(d.AltTitles),
		HymnalSource: d.HymnalSource, HymnalNumber: d.HymnalNumber, Lyricist: d.Lyricist, Composer: d.Composer,
		Translator: d.Translator, DefaultKey: d.DefaultKey, CopyrightHolder: d.CopyrightHolder,
		CopyrightLine: d.CopyrightLine, CCLISongNumber: d.CCLISongNumber, LicenceStatus: d.LicenceStatus,
		LicenceNotes: d.LicenceNotes}
	if s.AltTitles == nil {
		s.AltTitles = []string{}
	}
	if s.LicenceStatus == "" {
		s.LicenceStatus = LicenceUnknown
	}
	for i, sec := range d.Sections {
		s.Sections = append(s.Sections, Section{ID: SectionID(strconv.Itoa(i)), Kind: sec.Kind, Number: sec.Number,
			Label: sec.Label, Text: sec.Text})
	}
	for _, i := range d.DefaultArrangement {
		if i >= 0 && i < len(d.Sections) {
			s.DefaultArrangement = append(s.DefaultArrangement, SectionID(strconv.Itoa(i)))
		} else {
			s.DefaultArrangement = append(s.DefaultArrangement, SectionID("?"))
		}
	}
	return s
}

// ValidateDraft applies the rules of a typed song and the import limits.
// d may be changed (text is normalised).
func ValidateDraft(d *SongDraft) error {
	if len(d.Sections) > MaxImportSections {
		return &InvalidInputError{Field: "sections", Message: "At most 60 sections."}
	}
	if b, err := json.Marshal(d); err != nil || len(b) > MaxImportDraftBytes {
		return &InvalidInputError{Field: "sections", Message: "This song is too large."}
	}
	if d.LicenceStatus == "" {
		d.LicenceStatus = LicenceUnknown
	}
	s := DraftSong(*d)
	if err := ValidateSong(&s); err != nil {
		return err
	}
	if err := ValidateSections(s.Sections); err != nil {
		return err
	}
	if err := ValidateArrangement(s.DefaultArrangement, s.Sections); err != nil {
		return err
	}
	d.Title, d.AltTitles = s.Title, s.AltTitles
	for i := range d.Sections {
		d.Sections[i].Text, d.Sections[i].Label = s.Sections[i].Text, s.Sections[i].Label
	}
	return nil
}

// Kinds of MergeLine.Status (08 §3.2).
const (
	MergeUpdated = "updated"
	MergeNew     = "new"
	MergeKept    = "kept"
	MergeRemoved = "removed"
)

// MergeLine is one section of the merge result.
type MergeLine struct {
	Status     string
	ID         SectionID // existing section; "" = new
	DraftIndex int       // index in the draft; -1 for kept and removed
	Kind       SectionKind
	Number     int
	Label      string
	OldText    string
	NewText    string
}

// MergePlan is exactly what Apply does for a merge, and what the preview shows.
type MergePlan struct {
	Lines []MergeLine // result sections in order, then the removed ones
	// Fields to fill: the existing song's fields after the merge.
	Metadata Song
}

// Result returns the lines that stay in the song, in order.
func (p MergePlan) Result() []MergeLine {
	var out []MergeLine
	for _, l := range p.Lines {
		if l.Status != MergeRemoved {
			out = append(out, l)
		}
	}
	return out
}

// PlanMerge decides how a draft merges into an existing song (08 §3.2): empty
// fields are filled, never overwritten; sections are matched deterministically.
func PlanMerge(existing Song, d SongDraft, removeUnmatched bool) MergePlan {
	meta := existing
	meta.AltTitles = slices.Clone(existing.AltTitles)
	fill := func(dst *string, src string) {
		if *dst == "" {
			*dst = src
		}
	}
	if meta.HymnalSource == "" && meta.HymnalNumber == "" {
		meta.HymnalSource, meta.HymnalNumber = d.HymnalSource, d.HymnalNumber
	}
	fill(&meta.Lyricist, d.Lyricist)
	fill(&meta.Composer, d.Composer)
	fill(&meta.Translator, d.Translator)
	fill(&meta.DefaultKey, d.DefaultKey)
	fill(&meta.CopyrightHolder, d.CopyrightHolder)
	fill(&meta.CopyrightLine, d.CopyrightLine)
	fill(&meta.CCLISongNumber, d.CCLISongNumber)
	fill(&meta.LicenceNotes, d.LicenceNotes)
	seen := map[string]bool{Fold(meta.Title): true}
	for _, t := range meta.AltTitles {
		seen[Fold(t)] = true
	}
	for _, t := range append([]string{d.Title}, d.AltTitles...) {
		if k := Fold(t); !seen[k] && len(meta.AltTitles) < 100 {
			seen[k] = true
			meta.AltTitles = append(meta.AltTitles, t)
		}
	}

	match := matchSections(existing, d)
	used := map[SectionID]bool{}
	var lines []MergeLine
	for i, ds := range d.Sections {
		l := MergeLine{DraftIndex: i, Kind: ds.Kind, Number: ds.Number, Label: ds.Label, NewText: ds.Text, Status: MergeNew}
		if id, ok := match[i]; ok {
			used[id] = true
			l.ID, l.Status = id, MergeUpdated
			for _, e := range existing.Sections {
				if e.ID == id {
					l.OldText = e.Text
				}
			}
		}
		lines = append(lines, l)
	}
	var removed []MergeLine
	for _, e := range existing.Sections {
		if used[e.ID] {
			continue
		}
		l := MergeLine{ID: e.ID, DraftIndex: -1, Kind: e.Kind, Number: e.Number, Label: e.Label, OldText: e.Text, NewText: e.Text, Status: MergeKept}
		if removeUnmatched {
			l.Status = MergeRemoved
			removed = append(removed, l)
			continue
		}
		lines = append(lines, l)
	}
	return MergePlan{Lines: append(lines, removed...), Metadata: meta}
}

// matchSections maps draft section indexes to existing section IDs (08 §3.2).
func matchSections(existing Song, d SongDraft) map[int]SectionID {
	out := map[int]SectionID{}
	taken := map[SectionID]bool{}
	for _, kind := range SectionKinds {
		var ds []int
		var es []Section
		for i, s := range d.Sections {
			if s.Kind == kind {
				ds = append(ds, i)
			}
		}
		for _, s := range existing.Sections {
			if s.Kind == kind {
				es = append(es, s)
			}
		}
		switch {
		case kind == SectionVerse:
			for _, i := range ds {
				for _, e := range es {
					if e.Number == d.Sections[i].Number && !taken[e.ID] {
						out[i], taken[e.ID] = e.ID, true
					}
				}
			}
		case len(ds) == 1 && len(es) == 1:
			out[ds[0]], taken[es[0].ID] = es[0].ID, true
		case len(ds) > 1 || len(es) > 1:
			for _, i := range ds {
				want := FoldFor(existing.Language, d.Sections[i].Text)
				for _, e := range es {
					if !taken[e.ID] && FoldFor(existing.Language, e.Text) == want {
						out[i], taken[e.ID] = e.ID, true
						break
					}
				}
			}
		}
	}
	return out
}
