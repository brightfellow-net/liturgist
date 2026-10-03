// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// importRepo implements app.ImportRepo (08 §7). Every query filters by church_id.
type importRepo struct{ *churchStore }

const candidateColumns = `id, batch_id, position, draft, duplicate_of_id, decision, merge_into, merge_target_version,
	remove_unmatched, warnings, outcome, applied_song_id, error_code`

func scanCandidate(sc interface{ Scan(...any) error }) (domain.ImportCandidate, error) {
	var (
		c                                  domain.ImportCandidate
		dup, mergeInto, outcome, song, err sql.NullString
		version                            sql.NullInt64
	)
	if e := sc.Scan(&c.ID, &c.BatchID, &c.Position, jsonValue{&c.Draft}, &dup, &c.Decision, &mergeInto, &version,
		&c.RemoveUnmatched, jsonValue{&c.Warnings}, &outcome, &song, &err); e != nil {
		return domain.ImportCandidate{}, e
	}
	c.DuplicateOfID, c.MergeInto = domain.SongID(dup.String), domain.SongID(mergeInto.String)
	c.MergeTargetVersion = int(version.Int64)
	c.Outcome, c.AppliedSongID, c.ErrorCode = domain.ImportOutcome(outcome.String), domain.SongID(song.String), err.String
	if c.Warnings == nil {
		c.Warnings = []string{}
	}
	return c, nil
}

func (r importRepo) CreateBatch(ctx context.Context, b domain.ImportBatch, cands []domain.ImportCandidate) error {
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO import_batches (id, church_id, source_format, status, created_by,
		created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		b.ID, r.churchID, string(b.Format), string(b.Status), b.CreatedBy, r.d.TimeArg(b.CreatedAt), r.d.TimeArg(b.UpdatedAt)); err != nil {
		return r.d.MapError(err)
	}
	for _, c := range cands {
		draft, err := jsonArg(c.Draft)
		if err != nil {
			return err
		}
		warnings, err := jsonArg(c.Warnings)
		if err != nil {
			return err
		}
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO import_candidates (id, church_id, batch_id, position, kind,
			draft, duplicate_of_id, decision, merge_into, merge_target_version, remove_unmatched, warnings)
			VALUES (?, ?, ?, ?, 'song', ?, ?, ?, ?, ?, ?, ?)`),
			c.ID, r.churchID, c.BatchID, c.Position, draft, nullString(string(c.DuplicateOfID)), string(c.Decision),
			nullString(string(c.MergeInto)), nullInt(c.MergeTargetVersion), c.RemoveUnmatched, warnings); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func scanBatch(sc interface{ Scan(...any) error }) (domain.ImportBatch, error) {
	var (
		b            domain.ImportBatch
		created, upd Time
	)
	if err := sc.Scan(&b.ID, &b.Format, &b.Status, &b.CreatedBy, &created, &upd); err != nil {
		return domain.ImportBatch{}, err
	}
	b.CreatedAt, b.UpdatedAt = created.Time, upd.Time
	return b, nil
}

const batchColumns = `id, source_format, status, created_by, created_at, updated_at`

func (r importRepo) Batch(ctx context.Context, id domain.ImportBatchID) (domain.ImportBatch, error) {
	b, err := scanBatch(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+batchColumns+
		" FROM import_batches WHERE church_id = ? AND id = ?"), r.churchID, id))
	if err != nil {
		return domain.ImportBatch{}, r.d.MapError(err)
	}
	return b, nil
}

func (r importRepo) OpenBatches(ctx context.Context) ([]domain.ImportBatch, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+batchColumns+
		" FROM import_batches WHERE church_id = ? AND status = 'open' ORDER BY updated_at DESC, id DESC"), r.churchID)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.ImportBatch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, b)
	}
	return out, r.d.MapError(rows.Err())
}

func (r importRepo) Candidates(ctx context.Context, batch domain.ImportBatchID) ([]domain.ImportCandidate, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+candidateColumns+
		" FROM import_candidates WHERE church_id = ? AND batch_id = ? ORDER BY position"), r.churchID, batch)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.ImportCandidate{}
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, c)
	}
	return out, r.d.MapError(rows.Err())
}

func (r importRepo) Candidate(ctx context.Context, batch domain.ImportBatchID, id domain.ImportCandidateID) (domain.ImportCandidate, error) {
	c, err := scanCandidate(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+candidateColumns+
		" FROM import_candidates WHERE church_id = ? AND batch_id = ? AND id = ?"), r.churchID, batch, id))
	if err != nil {
		return domain.ImportCandidate{}, r.d.MapError(err)
	}
	return c, nil
}

func (r importRepo) UpdateCandidate(ctx context.Context, c domain.ImportCandidate) error {
	draft, err := jsonArg(c.Draft)
	if err != nil {
		return err
	}
	_, err = r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE import_candidates SET draft = ?, duplicate_of_id = ?, decision = ?,
		merge_into = ?, merge_target_version = ?, remove_unmatched = ?, outcome = ?, applied_song_id = ?, error_code = ?
		WHERE church_id = ? AND batch_id = ? AND id = ?`),
		draft, nullString(string(c.DuplicateOfID)), string(c.Decision), nullString(string(c.MergeInto)),
		nullInt(c.MergeTargetVersion), c.RemoveUnmatched, nullString(string(c.Outcome)),
		nullString(string(c.AppliedSongID)), nullString(c.ErrorCode), r.churchID, c.BatchID, c.ID)
	return r.d.MapError(err)
}

func (r importRepo) SetBatch(ctx context.Context, id domain.ImportBatchID, status domain.ImportStatus, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE import_batches SET status = ?, updated_at = ? WHERE church_id = ? AND id = ?"),
		string(status), r.d.TimeArg(now), r.churchID, id)
	return r.d.MapError(err)
}

func (r importRepo) Unfinished(ctx context.Context, batch domain.ImportBatchID) (int, error) {
	var n int
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT COUNT(*) FROM import_candidates WHERE church_id = ? AND batch_id = ?
		AND decision <> 'skip' AND COALESCE(outcome, '') <> 'applied'`), r.churchID, batch).Scan(&n)
	return n, r.d.MapError(err)
}

func (r importRepo) DeleteBatch(ctx context.Context, id domain.ImportBatchID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM import_batches WHERE church_id = ? AND id = ?"), r.churchID, id)
	if err != nil {
		return r.d.MapError(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

func (r importRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM import_batches WHERE church_id = ? AND updated_at < ?"),
		r.churchID, r.d.TimeArg(cutoff))
	return r.d.MapError(err)
}
