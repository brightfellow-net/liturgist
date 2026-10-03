// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/domain"
)

// Import limits (08 §5).
const (
	MaxImportFiles     = 200
	MaxImportFileBytes = 1 << 20
	MaxImportTotal     = 5 << 20
	MaxPasteChars      = 200_000
)

// Imports holds the import use cases (08). Everything needs library.edit,
// reading included; every member holding it sees all of the church's batches.
type Imports struct {
	Tx        Tx
	Clock     Clock
	IDs       IDGenerator
	Songs     *Songs
	Importers map[domain.ImportFormat]Importer
}

// ImportFile is one document of an import request.
type ImportFile struct{ Name, Text string }

// ImportRequest creates a batch.
type ImportRequest struct {
	Format   string
	Language string // "" = the church's content language
	Files    []ImportFile
}

// ImportRejection is a file or song that produced no candidate.
type ImportRejection struct {
	Name      string
	SongIndex int
	Reason    string
}

// CandidateView is a candidate with what is computed when it is read.
type CandidateView struct {
	domain.ImportCandidate
	DuplicateOf *DuplicateRef
	Warnings    []string // parser warnings plus duplicate_in_batch
}

// BatchView is a batch with its candidates.
type BatchView struct {
	Batch      domain.ImportBatch
	Candidates []CandidateView
	Rejected   []ImportRejection
}

// DecisionInput is one decision of a PATCH.
type DecisionInput struct {
	ID                 domain.ImportCandidateID
	Decision           domain.ImportDecision
	MergeInto          domain.SongID
	MergeTargetVersion int
	RemoveUnmatched    bool
}

// MergeSection is one line of the merge preview.
type MergeSection struct {
	Status  string
	Label   string
	OldText string
	NewText string
}

// MergePreview is exactly what Apply would do now (08 §3.2).
type MergePreview struct {
	TargetVersion int
	Sections      []MergeSection
}

// ApplyResult counts what one Apply did.
type ApplyResult struct {
	Created, Merged, Skipped int
	Failed                   []ApplyFailure
	Status                   domain.ImportStatus
}

// ApplyFailure is a candidate that Apply could not apply.
type ApplyFailure struct {
	CandidateID domain.ImportCandidateID
	Code        string
}

func (u *Imports) require(sc churchScope) error { return sc.actor.Require(domain.ScopeLibraryEdit) }

// Create parses the files and stores the batch (08 §2). Nothing reaches the
// library.
func (u *Imports) Create(ctx context.Context, sess *domain.Session, req ImportRequest) (BatchView, error) {
	format := domain.ImportFormat(req.Format)
	imp, ok := u.Importers[format]
	if !ok || !slices.Contains(domain.ImportFormats, format) {
		return BatchView{}, &domain.InvalidInputError{Field: "format", Message: "Unknown import format."}
	}
	if len(req.Files) < 1 || len(req.Files) > MaxImportFiles || (format == domain.FormatPaste && len(req.Files) != 1) {
		return BatchView{}, &domain.InvalidInputError{Field: "files", Message: "Send 1 to 200 files (pasted lyrics: exactly one)."}
	}
	if req.Language != "" && !slices.Contains(domain.ContentLanguages, req.Language) {
		return BatchView{}, &domain.InvalidInputError{Field: "language", Message: "Unknown language."}
	}
	total := 0
	for _, f := range req.Files {
		total += len(f.Text)
	}
	if total > MaxImportTotal {
		return BatchView{}, ErrImportTooLarge
	}
	if format == domain.FormatPaste {
		f := req.Files[0]
		if strings.TrimSpace(f.Name) == "" || utf8.RuneCountInString(f.Name) > domain.MaxSongTitle {
			return BatchView{}, &domain.InvalidInputError{Field: "files.0.name", Message: "Title must be 1 to 200 characters."}
		}
		if utf8.RuneCountInString(f.Text) > MaxPasteChars || len(f.Text) > MaxImportFileBytes {
			return BatchView{}, &domain.InvalidInputError{Field: "files.0.text", Message: "At most 200,000 characters."}
		}
	}

	language := req.Language
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		if language == "" {
			ch, err := sc.cs.Church().Get(ctx)
			if err != nil {
				return err
			}
			language = ch.DefaultLanguage
		}
		return nil
	})
	if err != nil {
		return BatchView{}, err
	}

	// Parse outside any transaction: parsers are pure and may take a while.
	var cands []ImportCandidate
	var rejected []ImportRejection
	for _, f := range req.Files {
		if len(f.Text) > MaxImportFileBytes {
			rejected = append(rejected, ImportRejection{Name: f.Name, Reason: ImportFileTooLarge})
			continue
		}
		found, err := imp.Parse(ctx, strings.NewReader(f.Text), ImportHint{Format: format, Language: language, Name: f.Name})
		var bad *ImportUnreadableError
		switch {
		case errors.As(err, &bad):
			rejected = append(rejected, ImportRejection{Name: f.Name, Reason: bad.Reason})
			continue
		case err != nil:
			return BatchView{}, err
		}
		for i, c := range found {
			if c.Reject != "" {
				rejected = append(rejected, ImportRejection{Name: f.Name, SongIndex: i, Reason: c.Reject})
				continue
			}
			cands = append(cands, c)
		}
	}
	if len(cands) == 0 {
		reason := ImportNoSong
		if len(rejected) > 0 {
			reason = rejected[0].Reason
		}
		return BatchView{}, &ImportUnreadableError{Reason: reason}
	}
	if len(cands) > domain.MaxImportCandidates {
		return BatchView{}, &domain.InvalidInputError{Field: "files", Message: "At most 500 songs in one import."}
	}

	var res BatchView
	err = u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		now := u.Clock.Now()
		b := domain.ImportBatch{ID: domain.ImportBatchID(u.IDs.NewID()), Format: format, Status: domain.ImportOpen,
			CreatedBy: sess.UserID, CreatedAt: now, UpdatedAt: now}
		rows := make([]domain.ImportCandidate, len(cands))
		for i, c := range cands {
			rows[i] = domain.ImportCandidate{ID: domain.ImportCandidateID(u.IDs.NewID()), BatchID: b.ID, Position: i,
				Draft: normaliseDraft(c.Draft), Decision: domain.DecisionPending, Warnings: nonNil(c.Warnings)}
			if d, found, err := findDuplicate(ctx, sc, rows[i].Draft); err != nil {
				return err
			} else if found {
				rows[i].DuplicateOfID = d.ID
			}
		}
		if err := sc.cs.Imports().CreateBatch(ctx, b, rows); err != nil {
			return err
		}
		res, err = u.view(ctx, sc, b.ID)
		return err
	})
	res.Rejected = rejected
	return res, err
}

// Get returns a batch with its candidates. Duplicates are looked up again.
func (u *Imports) Get(ctx context.Context, sess *domain.Session, id domain.ImportBatchID) (BatchView, error) {
	var res BatchView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		res, err = u.view(ctx, sc, id)
		return err
	})
	return res, err
}

// Open lists the church's unfinished batches, newest first (the library banner).
func (u *Imports) Open(ctx context.Context, sess *domain.Session) ([]domain.ImportBatch, error) {
	var res []domain.ImportBatch
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		res, err = sc.cs.Imports().OpenBatches(ctx)
		return err
	})
	return res, err
}

// EditDraft replaces a candidate's draft (08 §5). A failed candidate becomes
// retryable again.
func (u *Imports) EditDraft(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID,
	id domain.ImportCandidateID, draft domain.SongDraft) (CandidateView, error) {
	if err := domain.ValidateDraft(&draft); err != nil {
		return CandidateView{}, err
	}
	var res CandidateView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		repo := sc.cs.Imports()
		if _, err := repo.Batch(ctx, batch); err != nil {
			return missing(err)
		}
		c, err := repo.Candidate(ctx, batch, id)
		if err != nil {
			return missing(err)
		}
		if c.Outcome == domain.OutcomeApplied {
			return &ImportConflictError{Reason: ImportAlreadyApplied}
		}
		c.Draft = normaliseDraft(draft)
		c.Outcome, c.ErrorCode = "", ""
		c.DuplicateOfID = ""
		if d, found, err := findDuplicate(ctx, sc, c.Draft); err != nil {
			return err
		} else if found {
			c.DuplicateOfID = d.ID
		}
		if err := repo.UpdateCandidate(ctx, c); err != nil {
			return err
		}
		if err := repo.SetBatch(ctx, batch, domain.ImportOpen, u.Clock.Now()); err != nil {
			return err
		}
		v, err := u.view(ctx, sc, batch)
		if err != nil {
			return err
		}
		for _, cv := range v.Candidates {
			if cv.ID == id {
				res = cv
			}
		}
		return nil
	})
	return res, err
}

// Decide saves the decisions of several candidates together, or none (08 §5).
func (u *Imports) Decide(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID,
	decisions []DecisionInput) (BatchView, error) {
	if len(decisions) > domain.MaxImportCandidates {
		return BatchView{}, &domain.InvalidInputError{Field: "decisions", Message: "At most 500 at once."}
	}
	for i, d := range decisions {
		field := "decisions." + strconv.Itoa(i)
		switch d.Decision {
		case domain.DecisionPending, domain.DecisionAccept, domain.DecisionSkip:
		case domain.DecisionMerge:
			if d.MergeInto == "" {
				return BatchView{}, &domain.InvalidInputError{Field: field + ".merge_into", Message: "Choose the song to merge into."}
			}
			if d.MergeTargetVersion < 1 {
				return BatchView{}, &domain.InvalidInputError{Field: field + ".merge_target_version", Message: "Look at the preview first."}
			}
		default:
			return BatchView{}, &domain.InvalidInputError{Field: field + ".decision", Message: "Unknown decision."}
		}
	}
	var res BatchView
	err := u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		repo := sc.cs.Imports()
		b, err := repo.Batch(ctx, batch)
		if err != nil {
			return missing(err)
		}
		for _, d := range decisions {
			c, err := repo.Candidate(ctx, batch, d.ID)
			if err != nil {
				return missing(err)
			}
			if c.Outcome == domain.OutcomeApplied {
				return &ImportConflictError{Reason: ImportAlreadyApplied}
			}
			c.Decision, c.Outcome, c.ErrorCode = d.Decision, "", ""
			c.MergeInto, c.MergeTargetVersion, c.RemoveUnmatched = "", 0, false
			if d.Decision == domain.DecisionMerge {
				target, err := sc.cs.Songs().ByID(ctx, d.MergeInto)
				if err != nil {
					return missing(err)
				}
				if target.Version != d.MergeTargetVersion {
					return &ImportConflictError{Reason: ImportTargetChanged}
				}
				c.MergeInto, c.MergeTargetVersion, c.RemoveUnmatched = d.MergeInto, d.MergeTargetVersion, d.RemoveUnmatched
			}
			if err := repo.UpdateCandidate(ctx, c); err != nil {
				return err
			}
		}
		status := b.Status
		if status == domain.ImportClosed {
			if n, err := repo.Unfinished(ctx, batch); err != nil {
				return err
			} else if n > 0 {
				status = domain.ImportOpen
			}
		}
		if err := repo.SetBatch(ctx, batch, status, u.Clock.Now()); err != nil {
			return err
		}
		res, err = u.view(ctx, sc, batch)
		return err
	})
	return res, err
}

// MergePreview shows what Apply would do now for a merge (any member who may import).
func (u *Imports) MergePreview(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID,
	id domain.ImportCandidateID, into domain.SongID, removeUnmatched bool) (MergePreview, error) {
	var res MergePreview
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		if _, err := sc.cs.Imports().Batch(ctx, batch); err != nil {
			return missing(err)
		}
		c, err := sc.cs.Imports().Candidate(ctx, batch, id)
		if err != nil {
			return missing(err)
		}
		target, err := sc.cs.Songs().ByID(ctx, into)
		if err != nil {
			return missing(err)
		}
		plan := domain.PlanMerge(target, c.Draft, removeUnmatched)
		res = MergePreview{TargetVersion: target.Version}
		for _, l := range plan.Lines {
			res.Sections = append(res.Sections, MergeSection{Status: l.Status, Label: sectionLabel(l), OldText: l.OldText, NewText: l.NewText})
		}
		return nil
	})
	return res, err
}

// Apply creates or merges the chosen songs, each in its own transaction
// under the church lock (08 §2). Applying again, or twice at once, applies
// every candidate once.
func (u *Imports) Apply(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID) (ApplyResult, error) {
	var ids []domain.ImportCandidateID
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		repo := sc.cs.Imports()
		if _, err := repo.Batch(ctx, batch); err != nil {
			return missing(err)
		}
		list, err := repo.Candidates(ctx, batch)
		if err != nil {
			return err
		}
		ids = ids[:0]
		for _, c := range list {
			ids = append(ids, c.ID)
		}
		return nil
	})
	if err != nil {
		return ApplyResult{}, err
	}
	var out ApplyResult
	for _, id := range ids {
		r, err := u.applyOne(ctx, sess, batch, id)
		if err != nil {
			return ApplyResult{}, err
		}
		out.Created += r.Created
		out.Merged += r.Merged
		out.Skipped += r.Skipped
		out.Failed = append(out.Failed, r.Failed...)
	}
	// A batch whose candidates were all skipped or already applied closes here.
	err = u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		out.Status, err = u.closeIfDone(ctx, sc, batch)
		return err
	})
	return out, err
}

// applyOne applies one candidate in one transaction, or records why it failed.
func (u *Imports) applyOne(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID,
	id domain.ImportCandidateID) (ApplyResult, error) {
	var out ApplyResult
	err := u.Tx.Write(ctx, func(s Store) error {
		out = ApplyResult{}
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		repo := sc.cs.Imports()
		c, err := repo.Candidate(ctx, batch, id)
		if err != nil {
			return missing(err)
		}
		switch {
		case c.Decision == domain.DecisionSkip:
			out.Skipped = 1
			return nil
		case c.Terminal() || c.Decision == domain.DecisionPending:
			return nil
		}
		songID, merged, cerr := u.applyCandidate(ctx, sc, c)
		if code := candidateFailure(cerr); code != "" {
			c.Outcome, c.ErrorCode, c.AppliedSongID = domain.OutcomeFailed, code, ""
			out.Failed = []ApplyFailure{{CandidateID: c.ID, Code: code}}
		} else if cerr != nil {
			return cerr
		} else {
			c.Outcome, c.ErrorCode, c.AppliedSongID = domain.OutcomeApplied, "", songID
			if merged {
				out.Merged = 1
			} else {
				out.Created = 1
			}
		}
		if err := repo.UpdateCandidate(ctx, c); err != nil {
			return err
		}
		_, err = u.closeIfDone(ctx, sc, batch)
		return err
	})
	return out, err
}

// closeIfDone sets the batch to closed when every candidate is terminal.
func (u *Imports) closeIfDone(ctx context.Context, sc churchScope, batch domain.ImportBatchID) (domain.ImportStatus, error) {
	repo := sc.cs.Imports()
	b, err := repo.Batch(ctx, batch)
	if err != nil {
		return "", missing(err)
	}
	n, err := repo.Unfinished(ctx, batch)
	if err != nil {
		return "", err
	}
	status := domain.ImportOpen
	if n == 0 {
		status = domain.ImportClosed
	}
	if status != b.Status || status == domain.ImportOpen {
		if err := repo.SetBatch(ctx, batch, status, u.Clock.Now()); err != nil {
			return "", err
		}
	}
	return status, nil
}

// applyCandidate does the song work. Errors that belong to the candidate
// (candidateFailure) are recorded by the caller; others abort the Apply.
func (u *Imports) applyCandidate(ctx context.Context, sc churchScope, c domain.ImportCandidate) (domain.SongID, bool, error) {
	d := c.Draft
	if err := domain.ValidateDraft(&d); err != nil {
		return "", false, err
	}
	if c.Decision == domain.DecisionAccept {
		in, song := songInputFromDraft(d)
		v, err := u.Songs.createIn(ctx, sc, song, in)
		if err != nil {
			return "", false, err
		}
		return v.Song.ID, false, nil
	}
	target, err := sc.cs.Songs().ByID(ctx, c.MergeInto)
	if err != nil {
		return "", true, missing(err)
	}
	if target.Version != c.MergeTargetVersion {
		return "", true, &ImportConflictError{Reason: ImportTargetChanged}
	}
	ch := mergeChange(target, domain.PlanMerge(target, d, c.RemoveUnmatched), d)
	v, err := u.Songs.updateIn(ctx, sc, target.ID, ch)
	if err != nil {
		return "", true, err
	}
	return v.Song.ID, true, nil
}

// candidateFailure returns the code of an error that only concerns one
// candidate, or "" for any other error.
func candidateFailure(err error) string {
	var (
		invalid *domain.InvalidInputError
		inUse   *SectionInUseError
		conflct *ImportConflictError
		group   *GroupConflictError
	)
	switch {
	case err == nil:
		return ""
	case errors.As(err, &invalid):
		return domain.CodeValidationFailed
	case errors.As(err, &inUse):
		return domain.CodeSectionInUse
	case errors.Is(err, ErrNotFound):
		return domain.CodeNotFound
	case errors.As(err, &conflct) && conflct.Reason == ImportTargetChanged, errors.Is(err, ErrVersionConflict):
		return domain.CodeTargetChanged
	case errors.As(err, &group):
		return domain.CodeValidationFailed
	}
	return ""
}

// Discard deletes a batch and its candidates at once.
func (u *Imports) Discard(ctx context.Context, sess *domain.Session, batch domain.ImportBatchID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := u.require(sc); err != nil {
			return err
		}
		if _, err := sc.cs.Imports().Batch(ctx, batch); err != nil {
			return missing(err)
		}
		return sc.cs.Imports().DeleteBatch(ctx, batch)
	})
}

// view reads a batch with its candidates and computes duplicates and the
// in-batch duplicate warning.
func (u *Imports) view(ctx context.Context, sc churchScope, id domain.ImportBatchID) (BatchView, error) {
	repo := sc.cs.Imports()
	b, err := repo.Batch(ctx, id)
	if err != nil {
		return BatchView{}, missing(err)
	}
	list, err := repo.Candidates(ctx, id)
	if err != nil {
		return BatchView{}, err
	}
	res := BatchView{Batch: b, Candidates: make([]CandidateView, len(list))}
	hymnal, title := map[string]int{}, map[string]int{}
	for _, c := range list {
		if k := draftHymnalKey(c.Draft); k != "" {
			hymnal[k]++
		}
		title[draftTitleKey(c.Draft)]++
	}
	for i, c := range list {
		v := CandidateView{ImportCandidate: c, Warnings: nonNil(slices.Clone(c.Warnings))}
		d, found, err := findDuplicate(ctx, sc, c.Draft)
		if err != nil {
			return BatchView{}, err
		}
		if found {
			v.DuplicateOf = &d
		}
		if k := draftHymnalKey(c.Draft); (k != "" && hymnal[k] > 1) || title[draftTitleKey(c.Draft)] > 1 {
			v.Warnings = append(v.Warnings, domain.WarnDuplicateInBatch)
		}
		res.Candidates[i] = v
	}
	return res, nil
}

func findDuplicate(ctx context.Context, sc churchScope, d domain.SongDraft) (DuplicateRef, bool, error) {
	return sc.cs.Songs().FindDuplicate(ctx, draftHymnalKey(d), domain.Fold(d.Title), d.Language)
}

func draftHymnalKey(d domain.SongDraft) string {
	return domain.HymnalKey(d.HymnalSource, d.HymnalNumber)
}

func draftTitleKey(d domain.SongDraft) string { return d.Language + "\x00" + domain.Fold(d.Title) }

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// normaliseDraft gives a draft the empty lists JSON needs.
func normaliseDraft(d domain.SongDraft) domain.SongDraft {
	d.AltTitles, d.Sections = nonNil(d.AltTitles), append([]domain.DraftSection{}, d.Sections...)
	if d.DefaultArrangement == nil {
		d.DefaultArrangement = []int{}
	}
	return d
}

func sectionLabel(l domain.MergeLine) string {
	if l.Label != "" {
		return l.Label
	}
	if l.Kind == domain.SectionVerse {
		return "verse " + strconv.Itoa(l.Number)
	}
	return string(l.Kind)
}

// songInputFromDraft builds the request that typing the song in would send:
// request-local keys "s0", "s1" … name the sections.
func songInputFromDraft(d domain.SongDraft) (SongInput, domain.Song) {
	in := SongInput{Language: d.Language, Title: d.Title, AltTitles: d.AltTitles, HymnalSource: d.HymnalSource,
		HymnalNumber: d.HymnalNumber, Lyricist: d.Lyricist, Composer: d.Composer, Translator: d.Translator,
		DefaultKey: d.DefaultKey, CopyrightHolder: d.CopyrightHolder, CopyrightLine: d.CopyrightLine,
		CCLISongNumber: d.CCLISongNumber, LicenceStatus: d.LicenceStatus, LicenceNotes: d.LicenceNotes}
	for i, s := range d.Sections {
		in.Sections = append(in.Sections, SectionInput{Key: "s" + strconv.Itoa(i), Kind: s.Kind, Number: s.Number, Label: s.Label, Text: s.Text})
	}
	for _, i := range d.DefaultArrangement {
		in.Arrangement = append(in.Arrangement, "s"+strconv.Itoa(i))
	}
	song := domain.Song{Language: d.Language, Title: d.Title, AltTitles: slices.Clone(d.AltTitles), HymnalSource: d.HymnalSource,
		HymnalNumber: d.HymnalNumber, Lyricist: d.Lyricist, Composer: d.Composer, Translator: d.Translator,
		DefaultKey: d.DefaultKey, CopyrightHolder: d.CopyrightHolder, CopyrightLine: d.CopyrightLine,
		CCLISongNumber: d.CCLISongNumber, LicenceStatus: d.LicenceStatus, LicenceNotes: d.LicenceNotes}
	if song.AltTitles == nil {
		song.AltTitles = []string{}
	}
	return in, song
}

// mergeChange turns a merge plan into the update the song use case applies.
func mergeChange(target domain.Song, plan domain.MergePlan, d domain.SongDraft) SongChange {
	m := plan.Metadata
	secs := make([]SectionInput, 0, len(plan.Lines))
	entry := map[int]string{} // draft index → ID or key in the request
	for i, l := range plan.Result() {
		si := SectionInput{ID: l.ID, Kind: l.Kind, Number: l.Number, Label: l.Label, Text: l.NewText}
		if l.ID == "" {
			si.Key = "n" + strconv.Itoa(i)
		}
		if l.Status == domain.MergeKept {
			si.Text = l.OldText
		}
		if l.DraftIndex >= 0 {
			if l.ID != "" {
				entry[l.DraftIndex] = string(l.ID)
			} else {
				entry[l.DraftIndex] = si.Key
			}
		}
		secs = append(secs, si)
	}
	ch := SongChange{Version: target.Version, AltTitles: &m.AltTitles, HymnalSource: &m.HymnalSource,
		HymnalNumber: &m.HymnalNumber, Lyricist: &m.Lyricist, Composer: &m.Composer, Translator: &m.Translator,
		DefaultKey: &m.DefaultKey, CopyrightHolder: &m.CopyrightHolder, CopyrightLine: &m.CopyrightLine,
		CCLISongNumber: &m.CCLISongNumber, LicenceNotes: &m.LicenceNotes, Sections: &secs}
	if len(d.DefaultArrangement) > 0 {
		arr := make([]string, 0, len(d.DefaultArrangement))
		for _, i := range d.DefaultArrangement {
			arr = append(arr, entry[i])
		}
		ch.Arrangement = &arr
	}
	return ch
}
