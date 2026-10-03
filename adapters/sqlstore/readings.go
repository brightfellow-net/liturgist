// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"strings"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// readingRepo implements app.ReadingRepo (07). Every query filters by church_id.
type readingRepo struct{ *churchStore }

const readingColumns = `r.id, r.reference, r.reference_display, r.text, r.attribution, r.source_provider, r.version,
	r.created_at, r.updated_at, t.id, t.code, t.name, t.language`

const readingFrom = ` FROM readings r JOIN translations t ON t.id = r.translation_id WHERE r.church_id = ?`

func (r readingRepo) one(ctx context.Context, where string, args ...any) (domain.Reading, error) {
	var (
		rd           domain.Reading
		created, upd Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+readingColumns+readingFrom+where), append([]any{r.churchID}, args...)...).
		Scan(&rd.ID, &rd.Reference, &rd.ReferenceDisplay, &rd.Text, &rd.Attribution, &rd.SourceProvider, &rd.Version,
			&created, &upd, &rd.Translation.ID, &rd.Translation.Code, &rd.Translation.Name, &rd.Translation.Language)
	if err != nil {
		return domain.Reading{}, r.d.MapError(err)
	}
	rd.CreatedAt, rd.UpdatedAt = created.Time, upd.Time
	return rd, nil
}

func (r readingRepo) ByID(ctx context.Context, id domain.ReadingID) (domain.Reading, error) {
	return r.one(ctx, " AND r.id = ?", id)
}

func (r readingRepo) ByReference(ctx context.Context, reference string, tr domain.TranslationID) (domain.Reading, error) {
	return r.one(ctx, " AND r.reference = ? AND r.translation_id = ?", reference, tr)
}

func (r readingRepo) Create(ctx context.Context, rd domain.Reading) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO readings (id, church_id, reference, reference_display, translation_id,
		text, attribution, source_provider, search_fold, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		rd.ID, r.churchID, rd.Reference, rd.ReferenceDisplay, rd.Translation.ID, rd.Text, rd.Attribution,
		rd.SourceProvider, rd.SearchFold(), rd.Version, r.d.TimeArg(rd.CreatedAt), r.d.TimeArg(rd.UpdatedAt))
	return r.d.MapError(err)
}

func (r readingRepo) Update(ctx context.Context, rd domain.Reading, expectedVersion int) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE readings SET reference_display = ?, text = ?, attribution = ?,
		search_fold = ?, version = ?, updated_at = ? WHERE church_id = ? AND id = ? AND version = ?`),
		rd.ReferenceDisplay, rd.Text, rd.Attribution, rd.SearchFold(), rd.Version, r.d.TimeArg(rd.UpdatedAt),
		r.churchID, rd.ID, expectedVersion)
	if err != nil {
		return false, r.d.MapError(err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r readingRepo) Delete(ctx context.Context, id domain.ReadingID) error {
	// Items of published liturgies lose the reference and keep reading_label.
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE liturgy_items SET reading_id = NULL WHERE church_id = ? AND reading_id = ?"),
		r.churchID, id); err != nil {
		return r.d.MapError(err)
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM readings WHERE church_id = ? AND id = ?"), r.churchID, id)
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

func (r readingRepo) List(ctx context.Context, q app.ReadingSearch) ([]app.ReadingRow, error) {
	var (
		where []string
		args  = []any{r.churchID}
	)
	if q.Translation != "" {
		where = append(where, "t.code = ?")
		args = append(args, q.Translation)
	}
	if q.Fold != "" || q.FoldZh != "" {
		// Chinese readings are folded without spaces, the others with them
		// (domain.FoldFor), so each kind is matched with its own folded query.
		zh := `t.language IN ('zh-Hans', 'zh-Hant')`
		where = append(where, "(("+zh+" AND "+r.d.Contains("r.search_fold")+") OR (NOT ("+zh+") AND "+r.d.Contains("r.search_fold")+"))")
		args = append(args, q.FoldZh, q.Fold)
	}
	query := "SELECT r.id, r.reference, r.reference_display, substr(r.text, 1, 300), t.id, t.code, t.name, t.language" +
		readingFrom
	if len(where) > 0 {
		query += " AND " + strings.Join(where, " AND ")
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(query), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []app.ReadingRow
	for rows.Next() {
		var row app.ReadingRow
		if err := rows.Scan(&row.ID, &row.Reference, &row.ReferenceDisplay, &row.TextStart,
			&row.Translation.ID, &row.Translation.Code, &row.Translation.Name, &row.Translation.Language); err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, row)
	}
	return out, r.d.MapError(rows.Err())
}

func (r readingRepo) LatestAttribution(ctx context.Context, tr domain.TranslationID) (string, error) {
	var got []string
	err := r.tx.SelectContext(ctx, &got, r.d.Rebind(`SELECT attribution FROM readings WHERE church_id = ? AND translation_id = ?
		ORDER BY updated_at DESC, id DESC LIMIT 1`), r.churchID, tr)
	if err != nil || len(got) == 0 {
		return "", r.d.MapError(err)
	}
	return got[0], nil
}
