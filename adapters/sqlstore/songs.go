// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// songRepo implements app.SongRepo (06). Every query filters by church_id.
type songRepo struct{ *churchStore }

const songColumns = `id, COALESCE(song_group_id, ''), language, title, alt_titles, hymnal_source, hymnal_number,
	lyricist, composer, translator, default_key, copyright_holder, copyright_line, ccli_song_number,
	licence_status, licence_notes, version, created_at, updated_at`

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func (r songRepo) ByID(ctx context.Context, id domain.SongID) (domain.Song, error) {
	var (
		s            domain.Song
		created, upd Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+songColumns+" FROM songs WHERE church_id = ? AND id = ?"), r.churchID, id).
		Scan(&s.ID, &s.GroupID, &s.Language, &s.Title, jsonValue{&s.AltTitles}, &s.HymnalSource, &s.HymnalNumber,
			&s.Lyricist, &s.Composer, &s.Translator, &s.DefaultKey, &s.CopyrightHolder, &s.CopyrightLine, &s.CCLISongNumber,
			&s.LicenceStatus, &s.LicenceNotes, &s.Version, &created, &upd)
	if err != nil {
		return domain.Song{}, r.d.MapError(err)
	}
	s.CreatedAt, s.UpdatedAt = created.Time, upd.Time
	if s.AltTitles == nil {
		s.AltTitles = []string{}
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT id, kind, number, label, text FROM song_sections
		WHERE church_id = ? AND song_id = ? ORDER BY position`), r.churchID, id)
	if err != nil {
		return domain.Song{}, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			sec    domain.Section
			number sql.NullInt64
			label  sql.NullString
		)
		if err := rows.Scan(&sec.ID, &sec.Kind, &number, &label, &sec.Text); err != nil {
			return domain.Song{}, r.d.MapError(err)
		}
		sec.Number, sec.Label = int(number.Int64), label.String
		s.Sections = append(s.Sections, sec)
	}
	if err := rows.Err(); err != nil {
		return domain.Song{}, r.d.MapError(err)
	}
	s.DefaultArrangement = []domain.SectionID{}
	var arr []domain.SectionID
	if err := r.tx.SelectContext(ctx, &arr, r.d.Rebind(`SELECT section_id FROM song_arrangement_entries
		WHERE church_id = ? AND song_id = ? ORDER BY position`), r.churchID, id); err != nil {
		return domain.Song{}, r.d.MapError(err)
	}
	s.DefaultArrangement = append(s.DefaultArrangement, arr...)
	return s, nil
}

func (r songRepo) Create(ctx context.Context, s domain.Song) error {
	alts, err := jsonArg(s.AltTitles)
	if err != nil {
		return err
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO songs (id, church_id, song_group_id, language, title, title_key,
		alt_titles, hymnal_source, hymnal_number, hymnal_key, lyricist, composer, translator, default_key,
		copyright_holder, copyright_line, ccli_song_number, licence_status, licence_notes, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		s.ID, r.churchID, nullString(string(s.GroupID)), s.Language, s.Title, s.TitleKey(), alts, s.HymnalSource,
		s.HymnalNumber, nullString(s.HymnalKey()), s.Lyricist, s.Composer, s.Translator, s.DefaultKey, s.CopyrightHolder,
		s.CopyrightLine, s.CCLISongNumber, string(s.LicenceStatus), s.LicenceNotes, s.Version,
		r.d.TimeArg(s.CreatedAt), r.d.TimeArg(s.UpdatedAt)); err != nil {
		return r.d.MapError(err)
	}
	for i, sec := range s.Sections {
		if err := r.insertSection(ctx, s.ID, i, sec); err != nil {
			return err
		}
	}
	if err := r.insertArrangement(ctx, s); err != nil {
		return err
	}
	return r.index(ctx, s)
}

func (r songRepo) insertSection(ctx context.Context, song domain.SongID, pos int, sec domain.Section) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO song_sections (id, church_id, song_id, position, kind, number, label, text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		sec.ID, r.churchID, song, pos, string(sec.Kind), nullInt(sec.Number), nullString(sec.Label), sec.Text)
	return r.d.MapError(err)
}

func (r songRepo) insertArrangement(ctx context.Context, s domain.Song) error {
	for i, id := range s.DefaultArrangement {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO song_arrangement_entries (church_id, song_id, position, section_id)
			VALUES (?, ?, ?, ?)`), r.churchID, s.ID, i, id); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r songRepo) Update(ctx context.Context, s domain.Song, expectedVersion int) (bool, error) {
	alts, err := jsonArg(s.AltTitles)
	if err != nil {
		return false, err
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE songs SET language = ?, title = ?, title_key = ?, alt_titles = ?,
		hymnal_source = ?, hymnal_number = ?, hymnal_key = ?, lyricist = ?, composer = ?, translator = ?, default_key = ?,
		copyright_holder = ?, copyright_line = ?, ccli_song_number = ?, licence_status = ?, licence_notes = ?,
		version = ?, updated_at = ? WHERE church_id = ? AND id = ? AND version = ?`),
		s.Language, s.Title, s.TitleKey(), alts, s.HymnalSource, s.HymnalNumber, nullString(s.HymnalKey()), s.Lyricist,
		s.Composer, s.Translator, s.DefaultKey, s.CopyrightHolder, s.CopyrightLine, s.CCLISongNumber,
		string(s.LicenceStatus), s.LicenceNotes, s.Version, r.d.TimeArg(s.UpdatedAt), r.churchID, s.ID, expectedVersion)
	if err != nil {
		return false, r.d.MapError(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if err := r.replaceSections(ctx, s); err != nil {
		return false, err
	}
	return true, r.index(ctx, s)
}

// replaceSections makes the stored sections and arrangement equal s's: kept
// sections are updated in place (their IDs are stable), missing ones deleted,
// new ones inserted.
func (r songRepo) replaceSections(ctx context.Context, s domain.Song) error {
	var have []domain.SectionID
	if err := r.tx.SelectContext(ctx, &have, r.d.Rebind("SELECT id FROM song_sections WHERE church_id = ? AND song_id = ?"),
		r.churchID, s.ID); err != nil {
		return r.d.MapError(err)
	}
	existing := make(map[domain.SectionID]bool, len(have))
	for _, id := range have {
		existing[id] = true
	}
	keep := make(map[domain.SectionID]bool, len(s.Sections))
	for _, sec := range s.Sections {
		keep[sec.ID] = true
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM song_arrangement_entries WHERE church_id = ? AND song_id = ?"),
		r.churchID, s.ID); err != nil {
		return r.d.MapError(err)
	}
	for _, id := range have {
		if !keep[id] {
			if _, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE sequence_entries SET song_section_id = NULL WHERE church_id = ? AND song_section_id = ?"),
				r.churchID, id); err != nil {
				return r.d.MapError(err)
			}
			if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM song_sections WHERE church_id = ? AND song_id = ? AND id = ?"),
				r.churchID, s.ID, id); err != nil {
				return r.d.MapError(err)
			}
		}
	}
	// Verse numbers may be swapped or shifted: park the kept verses outside the
	// unique index for the duration of this transaction, then write the final values.
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE song_sections SET kind = 'other', number = NULL
		WHERE church_id = ? AND song_id = ? AND kind = 'verse'`), r.churchID, s.ID); err != nil {
		return r.d.MapError(err)
	}
	for i, sec := range s.Sections {
		if !existing[sec.ID] {
			if err := r.insertSection(ctx, s.ID, i, sec); err != nil {
				return err
			}
			continue
		}
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE song_sections SET position = ?, kind = ?, number = ?, label = ?, text = ?
			WHERE church_id = ? AND song_id = ? AND id = ?`),
			i, string(sec.Kind), nullInt(sec.Number), nullString(sec.Label), sec.Text, r.churchID, s.ID, sec.ID); err != nil {
			return r.d.MapError(err)
		}
	}
	return r.insertArrangement(ctx, s)
}

func (r songRepo) Delete(ctx context.Context, id domain.SongID) error {
	// Liturgies that still refer to the song or its sections (published ones: the
	// use case refuses the rest) lose the reference and keep their snapshots. The
	// foreign keys stay RESTRICT (schema "Clearing references").
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE sequence_entries SET song_section_id = NULL
		WHERE church_id = ? AND song_section_id IN (SELECT id FROM song_sections WHERE church_id = ? AND song_id = ?)`),
		r.churchID, r.churchID, id); err != nil {
		return r.d.MapError(err)
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE liturgy_item_songs SET song_id = NULL WHERE church_id = ? AND song_id = ?"),
		r.churchID, id); err != nil {
		return r.d.MapError(err)
	}
	if err := r.d.DeleteSongIndex(ctx, r.tx, string(r.churchID), string(id)); err != nil {
		return err
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM songs WHERE church_id = ? AND id = ?"), r.churchID, id)
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

// --- search (06 §5) ---

// index rewrites the song's search rows from its current content.
func (r songRepo) index(ctx context.Context, s domain.Song) error {
	parts := []string{s.Title}
	parts = append(parts, s.AltTitles...)
	parts = append(parts, s.HymnalSource, s.HymnalNumber)
	head := joinFolded(s.Language, parts)
	texts := make([]string, len(s.Sections))
	for i, sec := range s.Sections {
		texts[i] = sec.Text
	}
	return r.d.WriteSongIndex(ctx, r.tx, string(r.churchID), string(s.ID), s.Language, head, joinFolded(s.Language, texts))
}

// joinFolded folds every part for the language and joins the non-empty ones
// with a space, so a term can never match across two parts.
func joinFolded(language string, parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if f := domain.FoldFor(language, p); f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}

const zhLanguages = "ss.language IN ('zh-Hans', 'zh-Hant')"

// termsPredicate is "every term matches" (06 §5.2): per term, full-text
// prefix search for non-Chinese songs and substring search for Chinese ones.
func (r songRepo) termsPredicate(terms []string, headOnly bool) (string, []any) {
	var preds []string
	var args []any
	for _, t := range terms {
		fts, ftsArgs := r.d.TermMatch(t, headOnly)
		sub := r.d.Contains("ss.head_fold")
		subArgs := []any{t}
		if !headOnly {
			sub = "(" + sub + " OR " + r.d.Contains("ss.lyrics_fold") + ")"
			subArgs = append(subArgs, t)
		}
		preds = append(preds, "((NOT "+zhLanguages+" AND "+fts+") OR ("+zhLanguages+" AND "+sub+"))")
		args = append(args, ftsArgs...)
		args = append(args, subArgs...)
	}
	return strings.Join(preds, " AND "), args
}

func (r songRepo) Search(ctx context.Context, q app.SongSearch) (app.SongPage, error) {
	where := []string{"s.church_id = ?"}
	whereArgs := []any{r.churchID}
	if q.Language != "" {
		where, whereArgs = append(where, "s.language = ?"), append(whereArgs, q.Language)
	}
	if q.LicenceStatus != "" {
		where, whereArgs = append(where, "s.licence_status = ?"), append(whereArgs, string(q.LicenceStatus))
	}
	switch {
	case q.HymnalKey != "":
		where, whereArgs = append(where, "s.hymnal_key = ?"), append(whereArgs, q.HymnalKey)
	case q.HymnalSourceKey != "":
		where, whereArgs = append(where, "s.hymnal_key LIKE ?"), append(whereArgs, q.HymnalSourceKey+":%")
	}
	tier, tierArgs := "0", []any(nil)
	if len(q.Terms) > 0 {
		anyPred, anyArgs := r.termsPredicate(q.Terms, false)
		match, matchArgs := "("+anyPred+")", anyArgs
		if q.HymnalQueryKey != "" {
			match, matchArgs = "(s.hymnal_key = ? OR "+match+")", append([]any{q.HymnalQueryKey}, matchArgs...)
		}
		where, whereArgs = append(where, match), append(whereArgs, matchArgs...)

		headPred, headArgs := r.termsPredicate(q.Terms, true)
		tier, tierArgs = "CASE WHEN ("+headPred+") THEN 1 ELSE 2 END", headArgs
		if q.HymnalQueryKey != "" {
			tier = "CASE WHEN s.hymnal_key = ? THEN 0 WHEN (" + headPred + ") THEN 1 ELSE 2 END"
			tierArgs = append([]any{q.HymnalQueryKey}, headArgs...)
		}
	}
	from := " FROM songs s JOIN song_search ss ON ss.church_id = s.church_id AND ss.song_id = s.id WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT count(*)"+from), whereArgs...).Scan(&total); err != nil {
		return app.SongPage{}, r.d.MapError(err)
	}
	query := `SELECT s.id, COALESCE(s.song_group_id, ''), s.title, s.alt_titles, s.language, s.hymnal_source, s.hymnal_number,
		s.licence_status, ` + tier + ` AS tier` + from + ` ORDER BY tier, ` + r.d.OrderBytes("s.title_key") + `, ` +
		r.d.OrderBytes("s.id") + ` LIMIT ? OFFSET ?`
	args := append(append(append([]any{}, tierArgs...), whereArgs...), q.Limit, q.Offset)
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(query), args...)
	if err != nil {
		return app.SongPage{}, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	page := app.SongPage{Total: total, Items: []app.SongRow{}}
	for rows.Next() {
		var (
			row  app.SongRow
			tier int
		)
		if err := rows.Scan(&row.ID, &row.GroupID, &row.Title, jsonValue{&row.AltTitles}, &row.Language, &row.HymnalSource,
			&row.HymnalNumber, &row.LicenceStatus, &tier); err != nil {
			return app.SongPage{}, r.d.MapError(err)
		}
		if row.AltTitles == nil {
			row.AltTitles = []string{}
		}
		page.Items = append(page.Items, row)
	}
	return page, r.d.MapError(rows.Err())
}

// --- groups (06 §2.3) ---

func (r songRepo) CreateGroup(ctx context.Context, id domain.SongGroupID, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO song_groups (id, church_id, created_at) VALUES (?, ?, ?)"),
		id, r.churchID, r.d.TimeArg(now))
	return r.d.MapError(err)
}

func (r songRepo) DeleteGroup(ctx context.Context, id domain.SongGroupID) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM song_groups WHERE church_id = ? AND id = ?"), r.churchID, id)
	return r.d.MapError(err)
}

func (r songRepo) SetGroup(ctx context.Context, songs []domain.SongID, group domain.SongGroupID, now time.Time) error {
	for _, id := range songs {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE songs SET song_group_id = ?, version = version + 1, updated_at = ?
			WHERE church_id = ? AND id = ?`), nullString(string(group)), r.d.TimeArg(now), r.churchID, id); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r songRepo) GroupMembers(ctx context.Context, group domain.SongGroupID) ([]app.SongRef, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT id, title, language FROM songs
		WHERE church_id = ? AND song_group_id = ? ORDER BY language, id`), r.churchID, group)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []app.SongRef
	for rows.Next() {
		var m app.SongRef
		if err := rows.Scan(&m.ID, &m.Title, &m.Language); err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, m)
	}
	return out, r.d.MapError(rows.Err())
}

func (r songRepo) Reindex(ctx context.Context) error {
	if err := r.d.DeleteChurchIndex(ctx, r.tx, string(r.churchID)); err != nil {
		return err
	}
	var ids []domain.SongID
	if err := r.tx.SelectContext(ctx, &ids, r.d.Rebind("SELECT id FROM songs WHERE church_id = ? ORDER BY id"), r.churchID); err != nil {
		return r.d.MapError(err)
	}
	for _, id := range ids {
		s, err := r.ByID(ctx, id)
		if err != nil {
			return err
		}
		if err := r.index(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (r songRepo) FindDuplicate(ctx context.Context, hymnalKey, titleKey, language string) (app.DuplicateRef, bool, error) {
	var d app.DuplicateRef
	find := func(where string, args ...any) (bool, error) {
		err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT id, title, hymnal_source, hymnal_number FROM songs
			WHERE church_id = ? AND `+where+` ORDER BY created_at, id LIMIT 1`), append([]any{r.churchID}, args...)...).
			Scan(&d.ID, &d.Title, &d.HymnalSource, &d.HymnalNumber)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, r.d.MapError(err)
	}
	if hymnalKey != "" {
		if ok, err := find("hymnal_key = ?", hymnalKey); ok || err != nil {
			return d, ok, err
		}
	}
	if titleKey == "" {
		return d, false, nil
	}
	ok, err := find("title_key = ? AND language = ?", titleKey, language)
	return d, ok, err
}
