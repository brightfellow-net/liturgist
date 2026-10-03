// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

func (c *churchStore) Liturgies() app.LiturgyRepo      { return liturgyRepo{c} }
func (c *churchStore) LiturgyItems() app.ItemRepo      { return itemRepo{c} }
func (c *churchStore) Assignments() app.AssignmentRepo { return assignmentRepo{c} }
func (c *churchStore) Edits() app.EditRepo             { return editRepo{c} }
func (c *churchStore) Usage() app.UsageRepo            { return usageRepo{c} }

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- liturgies ---

type liturgyRepo struct{ *churchStore }

const liturgyColumns = `id, date, time, service_id, service_name, language, template_id, state, version,
	archived_at, archived_by, created_by, edit_seq, undo_floor_seq, created_at, updated_at`

func scanLiturgy(scan func(...any) error) (domain.Liturgy, error) {
	var (
		l                domain.Liturgy
		svc, tpl, archBy *string
		archAt           NullTime
		created, updated Time
		state            string
	)
	if err := scan(&l.ID, &l.Date, &l.Time, &svc, &l.ServiceName, &l.Language, &tpl, &state, &l.Version,
		&archAt, &archBy, &l.CreatedBy, &l.EditSeq, &l.UndoFloorSeq, &created, &updated); err != nil {
		return domain.Liturgy{}, err
	}
	l.ServiceID, l.TemplateID, l.State = domain.ServiceID(strOf(svc)), domain.TemplateID(strOf(tpl)), domain.LiturgyState(state)
	l.ArchivedBy = domain.UserID(strOf(archBy))
	if archAt.Valid {
		t := archAt.Time
		l.ArchivedAt = &t
	}
	l.CreatedAt, l.UpdatedAt = created.Time, updated.Time
	return l, nil
}

func (r liturgyRepo) Create(ctx context.Context, l domain.Liturgy) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgies (id, church_id, date, time, service_id, service_name,
		language, template_id, state, version, created_by, edit_seq, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		l.ID, r.churchID, l.Date, l.Time, nullString(string(l.ServiceID)), l.ServiceName, l.Language,
		nullString(string(l.TemplateID)), string(l.State), l.Version, l.CreatedBy, l.EditSeq,
		r.d.TimeArg(l.CreatedAt), r.d.TimeArg(l.UpdatedAt))
	return r.d.MapError(err)
}

func (r liturgyRepo) ByID(ctx context.Context, id domain.LiturgyID) (domain.Liturgy, error) {
	l, err := scanLiturgy(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+liturgyColumns+" FROM liturgies WHERE church_id = ? AND id = ?"),
		r.churchID, id).Scan)
	return l, r.d.MapError(err)
}

func (r liturgyRepo) List(ctx context.Context, f app.LiturgyFilter) ([]app.LiturgyRow, int, error) {
	where, args := "church_id = ?", []any{r.churchID}
	if f.State != "" {
		where += " AND state = ?"
		args = append(args, string(f.State))
	}
	if f.From != "" {
		where += " AND date >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND date <= ?"
		args = append(args, f.To)
	}
	var total int
	if err := r.tx.GetContext(ctx, &total, r.d.Rebind("SELECT COUNT(*) FROM liturgies WHERE "+where), args...); err != nil {
		return nil, 0, r.d.MapError(err)
	}
	dir := " DESC"
	if f.Ascending {
		dir = " ASC"
	}
	q := "SELECT " + liturgyColumns + `, (SELECT COUNT(*) FROM liturgy_items i WHERE i.church_id = liturgies.church_id AND i.liturgy_id = liturgies.id)
		FROM liturgies WHERE ` + where + " ORDER BY date" + dir + ", time" + dir + ", id" + dir + " LIMIT ? OFFSET ?"
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(q), append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []app.LiturgyRow
	for rows.Next() {
		var row app.LiturgyRow
		row.Liturgy, err = scanLiturgy(func(dest ...any) error { return rows.Scan(append(dest, &row.ItemCount)...) })
		if err != nil {
			return nil, 0, r.d.MapError(err)
		}
		out = append(out, row)
	}
	return out, total, r.d.MapError(rows.Err())
}

func (r liturgyRepo) CountActive(ctx context.Context, states []domain.LiturgyState) (int, error) {
	q, args := "SELECT COUNT(*) FROM liturgies WHERE church_id = ? AND archived_at IS NULL", []any{r.churchID}
	if len(states) > 0 {
		q += " AND " + in("state", len(states))
		for _, s := range states {
			args = append(args, string(s))
		}
	}
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind(q), args...)
	return n, r.d.MapError(err)
}

func (r liturgyRepo) Slots(ctx context.Context, from, to string) (map[app.Slot]domain.LiturgyID, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT id, service_id, date, time FROM liturgies
		WHERE church_id = ? AND service_id IS NOT NULL AND date >= ? AND date <= ?`), r.churchID, from, to)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[app.Slot]domain.LiturgyID{}
	for rows.Next() {
		var (
			id domain.LiturgyID
			s  app.Slot
		)
		if err := rows.Scan(&id, &s.ServiceID, &s.Date, &s.Time); err != nil {
			return nil, r.d.MapError(err)
		}
		out[s] = id
	}
	return out, r.d.MapError(rows.Err())
}

func (r liturgyRepo) Update(ctx context.Context, l domain.Liturgy, expectedVersion int) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgies SET date = ?, time = ?, service_name = ?, version = version + 1, updated_at = ?
		WHERE church_id = ? AND id = ? AND version = ?`),
		l.Date, l.Time, l.ServiceName, r.d.TimeArg(l.UpdatedAt), r.churchID, l.ID, expectedVersion)
	return changed(r.d, res, err)
}

func (r liturgyRepo) Bump(ctx context.Context, id domain.LiturgyID, expectedVersion int, now time.Time) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgies SET version = version + 1, updated_at = ?
		WHERE church_id = ? AND id = ? AND version = ?`), r.d.TimeArg(now), r.churchID, id, expectedVersion)
	return changed(r.d, res, err)
}

// changed maps the result of a conditional update to "a row matched".
func changed(d Dialect, res rowsResult, err error) (bool, error) {
	if err != nil {
		return false, d.MapError(err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r liturgyRepo) NextSeq(ctx context.Context, id domain.LiturgyID) (int, error) {
	var seq int
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`UPDATE liturgies SET edit_seq = edit_seq + 1
		WHERE church_id = ? AND id = ? RETURNING edit_seq`), r.churchID, id).Scan(&seq)
	return seq, r.d.MapError(err)
}

func (r liturgyRepo) Delete(ctx context.Context, id domain.LiturgyID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM liturgies WHERE church_id = ? AND id = ?"), r.churchID, id)
	return rowsOrNotFound(r.d, res, err)
}

// --- items, songs and entries ---

type itemRepo struct{ *churchStore }

func (r itemRepo) ByLiturgy(ctx context.Context, liturgy domain.LiturgyID) ([]domain.Item, error) {
	return r.load(ctx, liturgy, "")
}

func (r itemRepo) ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) (domain.Item, error) {
	items, err := r.load(ctx, liturgy, id)
	if err != nil {
		return domain.Item{}, err
	}
	if len(items) == 0 {
		return domain.Item{}, app.ErrNotFound
	}
	return items[0], nil
}

// load reads the items of a liturgy (or one of them) with songs and entries
// in three queries.
func (r itemRepo) load(ctx context.Context, liturgy domain.LiturgyID, item domain.ItemID) ([]domain.Item, error) {
	cond, args := "i.church_id = ? AND i.liturgy_id = ?", []any{r.churchID, liturgy}
	if item != "" {
		cond += " AND i.id = ?"
		args = append(args, item)
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT i.id, i.position, i.title, i.item_type, i.duty_id, i.text,
		i.reading_id, i.reading_label, i.version, i.created_at, i.updated_at
		FROM liturgy_items i WHERE `+cond+" ORDER BY i.position, i.id"), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	var items []domain.Item
	at := map[domain.ItemID]int{}
	for rows.Next() {
		var (
			it               domain.Item
			typ              string
			duty, reading    *string
			created, updated Time
		)
		if err := rows.Scan(&it.ID, &it.Position, &it.Title, &typ, &duty, &it.Text, &reading, &it.ReadingLabel,
			&it.Version, &created, &updated); err != nil {
			_ = rows.Close()
			return nil, r.d.MapError(err)
		}
		it.LiturgyID, it.Type, it.DutyID, it.ReadingID = liturgy, domain.ItemType(typ), domain.DutyID(strOf(duty)), domain.ReadingID(strOf(reading))
		it.CreatedAt, it.UpdatedAt = created.Time, updated.Time
		at[it.ID] = len(items)
		items = append(items, it)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(items) == 0 {
		return items, r.d.MapError(err)
	}

	rows, err = r.tx.QueryContext(ctx, r.d.Rebind(`SELECT s.item_id, s.id, s.position, s.song_id, s.song_title, s.key, s.note
		FROM liturgy_item_songs s JOIN liturgy_items i ON i.church_id = s.church_id AND i.id = s.item_id
		WHERE `+cond+" ORDER BY s.item_id, s.position"), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	songAt := map[domain.ItemSongID][2]int{}
	for rows.Next() {
		var (
			item domain.ItemID
			s    domain.LiturgySong
			song *string
		)
		if err := rows.Scan(&item, &s.ID, &s.Position, &song, &s.SongTitle, &s.Key, &s.Note); err != nil {
			_ = rows.Close()
			return nil, r.d.MapError(err)
		}
		s.SongID = domain.SongID(strOf(song))
		ii := at[item]
		songAt[s.ID] = [2]int{ii, len(items[ii].Songs)}
		items[ii].Songs = append(items[ii].Songs, s)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, r.d.MapError(err)
	}

	rows, err = r.tx.QueryContext(ctx, r.d.Rebind(`SELECT e.item_song_id, e.id, e.position, e.song_section_id, e.singing_part_id,
		e.key_change, e.note, e.section_label
		FROM sequence_entries e
		JOIN liturgy_item_songs s ON s.church_id = e.church_id AND s.id = e.item_song_id
		JOIN liturgy_items i ON i.church_id = s.church_id AND i.id = s.item_id
		WHERE `+cond+" ORDER BY e.item_song_id, e.position"), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			song          domain.ItemSongID
			e             domain.Entry
			section, part *string
		)
		if err := rows.Scan(&song, &e.ID, &e.Position, &section, &part, &e.KeyChange, &e.Note, &e.SectionLabel); err != nil {
			return nil, r.d.MapError(err)
		}
		e.SectionID, e.SingingPartID = domain.SectionID(strOf(section)), domain.SingingPartID(strOf(part))
		p := songAt[song]
		s := &items[p[0]].Songs[p[1]]
		s.Entries = append(s.Entries, e)
	}
	return items, r.d.MapError(rows.Err())
}

func (r itemRepo) IDs(ctx context.Context, liturgy domain.LiturgyID) ([]domain.ItemID, error) {
	var ids []domain.ItemID
	err := r.tx.SelectContext(ctx, &ids, r.d.Rebind("SELECT id FROM liturgy_items WHERE church_id = ? AND liturgy_id = ? ORDER BY position, id"),
		r.churchID, liturgy)
	return ids, r.d.MapError(err)
}

func (r itemRepo) Insert(ctx context.Context, it domain.Item) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgy_items (id, church_id, liturgy_id, position, title, item_type,
		duty_id, text, reading_id, reading_label, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		it.ID, r.churchID, it.LiturgyID, it.Position, it.Title, string(it.Type), nullString(string(it.DutyID)), it.Text,
		nullString(string(it.ReadingID)), it.ReadingLabel, it.Version, r.d.TimeArg(it.CreatedAt), r.d.TimeArg(it.UpdatedAt))
	if err != nil {
		return r.d.MapError(err)
	}
	for i, s := range it.Songs {
		s.Position = i
		if err := r.InsertSong(ctx, it.ID, s); err != nil {
			return err
		}
	}
	return nil
}

func (r itemRepo) SetPositions(ctx context.Context, liturgy domain.LiturgyID, ids []domain.ItemID) error {
	for i, id := range ids {
		res, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE liturgy_items SET position = ? WHERE church_id = ? AND liturgy_id = ? AND id = ?"),
			i, r.churchID, liturgy, id)
		if err := rowsOrNotFound(r.d, res, err); err != nil {
			return err
		}
	}
	return nil
}

func (r itemRepo) Delete(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM liturgy_items WHERE church_id = ? AND liturgy_id = ? AND id = ?"),
		r.churchID, liturgy, id)
	return rowsOrNotFound(r.d, res, err)
}

func (r itemRepo) Update(ctx context.Context, it domain.Item, expectedVersion int) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_items SET title = ?, duty_id = ?, text = ?, reading_id = ?,
		reading_label = ?, version = version + 1, updated_at = ?
		WHERE church_id = ? AND liturgy_id = ? AND id = ? AND version = ?`),
		it.Title, nullString(string(it.DutyID)), it.Text, nullString(string(it.ReadingID)), it.ReadingLabel,
		r.d.TimeArg(it.UpdatedAt), r.churchID, it.LiturgyID, it.ID, expectedVersion)
	return changed(r.d, res, err)
}

func (r itemRepo) Bump(ctx context.Context, id domain.ItemID, expectedVersion int, now time.Time) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_items SET version = version + 1, updated_at = ?
		WHERE church_id = ? AND id = ? AND version = ?`), r.d.TimeArg(now), r.churchID, id, expectedVersion)
	return changed(r.d, res, err)
}

func (r itemRepo) InsertSong(ctx context.Context, item domain.ItemID, s domain.LiturgySong) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgy_item_songs (id, church_id, item_id, position, song_id, song_title, key, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		s.ID, r.churchID, item, s.Position, nullString(string(s.SongID)), s.SongTitle, s.Key, s.Note)
	if err != nil {
		return r.d.MapError(err)
	}
	for i, e := range s.Entries {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO sequence_entries (id, church_id, item_song_id, position, kind,
			song_section_id, singing_part_id, key_change, section_label, note) VALUES (?, ?, ?, ?, 'section', ?, ?, ?, ?, ?)`),
			e.ID, r.churchID, s.ID, i, nullString(string(e.SectionID)), nullString(string(e.SingingPartID)),
			e.KeyChange, e.SectionLabel, e.Note); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r itemRepo) SetSongPositions(ctx context.Context, item domain.ItemID, ids []domain.ItemSongID) error {
	for i, id := range ids {
		res, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE liturgy_item_songs SET position = ? WHERE church_id = ? AND item_id = ? AND id = ?"),
			i, r.churchID, item, id)
		if err := rowsOrNotFound(r.d, res, err); err != nil {
			return err
		}
	}
	return nil
}

func (r itemRepo) ReplaceSongs(ctx context.Context, item domain.ItemID, songs []domain.LiturgySong) error {
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM liturgy_item_songs WHERE church_id = ? AND item_id = ?"), r.churchID, item); err != nil {
		return r.d.MapError(err)
	}
	for i, s := range songs {
		s.Position = i
		if err := r.InsertSong(ctx, item, s); err != nil {
			return err
		}
	}
	return nil
}

// --- assignments ---

type assignmentRepo struct{ *churchStore }

const assignmentColumns = "id, liturgy_id, duty_id, user_id, name, name_key, created_at"

func scanAssignment(scan func(...any) error) (domain.Assignment, error) {
	var (
		a             domain.Assignment
		user, name, k *string
		created       Time
	)
	if err := scan(&a.ID, &a.LiturgyID, &a.DutyID, &user, &name, &k, &created); err != nil {
		return domain.Assignment{}, err
	}
	a.UserID, a.Name, a.NameKey, a.CreatedAt = domain.UserID(strOf(user)), strOf(name), strOf(k), created.Time
	return a, nil
}

func (r assignmentRepo) Add(ctx context.Context, a domain.Assignment) error {
	var name, key any
	if a.Name != "" {
		name, key = a.Name, a.NameKey
	}
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO assignments (id, church_id, liturgy_id, duty_id, user_id, name, name_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		a.ID, r.churchID, a.LiturgyID, a.DutyID, nullString(string(a.UserID)), name, key, r.d.TimeArg(a.CreatedAt))
	return r.d.MapError(err)
}

func (r assignmentRepo) ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.AssignmentID) (domain.Assignment, error) {
	a, err := scanAssignment(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+assignmentColumns+
		" FROM assignments WHERE church_id = ? AND liturgy_id = ? AND id = ?"), r.churchID, liturgy, id).Scan)
	return a, r.d.MapError(err)
}

func (r assignmentRepo) ByLiturgy(ctx context.Context, liturgy domain.LiturgyID) ([]domain.Assignment, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+assignmentColumns+
		" FROM assignments WHERE church_id = ? AND liturgy_id = ? ORDER BY created_at, id"), r.churchID, liturgy)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Assignment
	for rows.Next() {
		a, err := scanAssignment(rows.Scan)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, a)
	}
	return out, r.d.MapError(rows.Err())
}

func (r assignmentRepo) Count(ctx context.Context, liturgy domain.LiturgyID) (int, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM assignments WHERE church_id = ? AND liturgy_id = ?"), r.churchID, liturgy)
	return n, r.d.MapError(err)
}

func (r assignmentRepo) Remove(ctx context.Context, liturgy domain.LiturgyID, id domain.AssignmentID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM assignments WHERE church_id = ? AND liturgy_id = ? AND id = ?"), r.churchID, liturgy, id)
	return rowsOrNotFound(r.d, res, err)
}

// --- history ---

type editRepo struct{ *churchStore }

const editColumns = `id, liturgy_id, user_id, seq, command, target_edit_id, item_id, before, after,
	liturgy_version_after, item_version_after, status, undo_seq, skipped, created_at`

func (r editRepo) Append(ctx context.Context, e domain.Edit) error {
	var before, after any
	if e.Before != nil {
		before = string(e.Before)
	}
	if e.After != nil {
		after = string(e.After)
	}
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO liturgy_edits (id, church_id, liturgy_id, user_id, seq, command,
		target_edit_id, item_id, before, after, liturgy_version_after, item_version_after, status, undo_seq, skipped, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		e.ID, r.churchID, e.LiturgyID, e.UserID, e.Seq, e.Command, nullString(string(e.TargetEditID)),
		nullString(string(e.ItemID)), before, after, e.LiturgyVersionAfter, nullInt(e.ItemVersionAfter), e.Status,
		nullInt(e.UndoSeq), e.Skipped, r.d.TimeArg(e.CreatedAt))
	return r.d.MapError(err)
}

func scanEdit(scan func(...any) error) (domain.Edit, error) {
	var (
		e             domain.Edit
		target, item  *string
		before, after *string
		itemVersion   *int
		undoSeq       *int
		created       Time
	)
	if err := scan(&e.ID, &e.LiturgyID, &e.UserID, &e.Seq, &e.Command, &target, &item, &before, &after,
		&e.LiturgyVersionAfter, &itemVersion, &e.Status, &undoSeq, &e.Skipped, &created); err != nil {
		return domain.Edit{}, err
	}
	e.TargetEditID, e.ItemID, e.CreatedAt = domain.EditID(strOf(target)), domain.ItemID(strOf(item)), created.Time
	if before != nil {
		e.Before = []byte(*before)
	}
	if after != nil {
		e.After = []byte(*after)
	}
	if itemVersion != nil {
		e.ItemVersionAfter = *itemVersion
	}
	if undoSeq != nil {
		e.UndoSeq = *undoSeq
	}
	return e, nil
}

func (r editRepo) one(ctx context.Context, query string, args ...any) (domain.Edit, error) {
	e, err := scanEdit(r.tx.QueryRowContext(ctx, r.d.Rebind(query), args...).Scan)
	return e, r.d.MapError(err)
}

func (r editRepo) List(ctx context.Context, liturgy domain.LiturgyID, limit int) ([]domain.Edit, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT `+editColumns+`
		FROM liturgy_edits WHERE church_id = ? AND liturgy_id = ? ORDER BY seq DESC LIMIT ?`), r.churchID, liturgy, limit)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Edit
	for rows.Next() {
		e, err := scanEdit(rows.Scan)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, e)
	}
	return out, r.d.MapError(rows.Err())
}

func (r editRepo) BySeq(ctx context.Context, liturgy domain.LiturgyID, seq int) (domain.Edit, error) {
	return r.one(ctx, `SELECT `+editColumns+` FROM liturgy_edits WHERE church_id = ? AND liturgy_id = ? AND seq = ?`,
		r.churchID, liturgy, seq)
}

func (r editRepo) LastActing(ctx context.Context, liturgy domain.LiturgyID, target domain.EditID) (domain.Edit, error) {
	return r.one(ctx, `SELECT `+editColumns+` FROM liturgy_edits
		WHERE church_id = ? AND liturgy_id = ? AND target_edit_id = ? ORDER BY seq DESC LIMIT 1`, r.churchID, liturgy, target)
}

func (r editRepo) Newest(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID, floor, window int) (domain.Edit, error) {
	return r.one(ctx, `SELECT `+editColumns+` FROM (
			SELECT `+editColumns+` FROM liturgy_edits
			WHERE church_id = ? AND liturgy_id = ? AND user_id = ? AND seq > ?
			  AND command NOT IN ('undo', 'redo', 'liturgy.create')
			ORDER BY seq DESC LIMIT ?) w
		WHERE status = 'done' AND skipped = ? ORDER BY seq DESC LIMIT 1`,
		r.churchID, liturgy, user, floor, window, false)
}

func (r editRepo) NewestUndone(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID, floor int) (domain.Edit, error) {
	return r.one(ctx, `SELECT `+editColumns+` FROM liturgy_edits
		WHERE church_id = ? AND liturgy_id = ? AND user_id = ? AND status = 'undone' AND seq > ?
		ORDER BY undo_seq DESC LIMIT 1`, r.churchID, liturgy, user, floor)
}

func (r editRepo) Foreign(ctx context.Context, liturgy domain.LiturgyID, afterSeq int, user domain.UserID) ([]domain.Foreign, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT e.command, t.command, e.item_id,
			CASE WHEN COALESCE(t.command, e.command) IN ('assignment.add', 'assignment.remove') THEN e.before END,
			CASE WHEN COALESCE(t.command, e.command) IN ('assignment.add', 'assignment.remove') THEN e.after END
		FROM liturgy_edits e
		LEFT JOIN liturgy_edits t ON t.church_id = e.church_id AND t.id = e.target_edit_id
		WHERE e.church_id = ? AND e.liturgy_id = ? AND e.seq > ? AND e.user_id <> ?
		ORDER BY e.seq`), r.churchID, liturgy, afterSeq, user)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Foreign
	for rows.Next() {
		var (
			own, target, item *string
			before, after     *string
		)
		if err := rows.Scan(&own, &target, &item, &before, &after); err != nil {
			return nil, r.d.MapError(err)
		}
		f := domain.Foreign{Command: strOf(own), ItemID: domain.ItemID(strOf(item))}
		if t := strOf(target); t != "" {
			f.Command = t
		}
		if before != nil {
			f.Before = []byte(*before)
		}
		if after != nil {
			f.After = []byte(*after)
		}
		out = append(out, f)
	}
	return out, r.d.MapError(rows.Err())
}

func (r editRepo) SetStatus(ctx context.Context, id domain.EditID, from, to string, undoSeq int) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_edits SET status = ?, undo_seq = ?
		WHERE church_id = ? AND id = ? AND status = ?`), to, nullInt(undoSeq), r.churchID, id, from)
	return changed(r.d, res, err)
}

func (r editRepo) MarkSkipped(ctx context.Context, id domain.EditID) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_edits SET skipped = ?
		WHERE church_id = ? AND id = ? AND status = 'done'`), true, r.churchID, id)
	return r.d.MapError(err)
}

func (r editRepo) DropUndone(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE liturgy_edits SET status = 'dropped'
		WHERE church_id = ? AND liturgy_id = ? AND user_id = ? AND status = 'undone'`), r.churchID, liturgy, user)
	return r.d.MapError(err)
}

// --- usage ---

type usageRepo struct{ *churchStore }

// unpublished joins an item song to its liturgy and keeps liturgies that are not published.
const unpublishedSongs = `FROM liturgy_item_songs s
	JOIN liturgy_items i ON i.church_id = s.church_id AND i.id = s.item_id
	JOIN liturgies l ON l.church_id = i.church_id AND l.id = i.liturgy_id
	WHERE s.church_id = ? AND l.state <> 'published'`

func (r usageRepo) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM (SELECT 1 "+query+" LIMIT 1) x"), args...)
	return n > 0, r.d.MapError(err)
}

func (r usageRepo) SongInUse(ctx context.Context, song domain.SongID) (bool, error) {
	return r.exists(ctx, unpublishedSongs+" AND s.song_id = ?", r.churchID, song)
}

func (r usageRepo) SectionsInUse(ctx context.Context, song domain.SongID, sections []domain.SectionID) ([]domain.SectionID, error) {
	if len(sections) == 0 {
		return nil, nil
	}
	args := []any{r.churchID, song}
	for _, id := range sections {
		args = append(args, id)
	}
	var busy []domain.SectionID
	err := r.tx.SelectContext(ctx, &busy, r.d.Rebind(`SELECT DISTINCT e.song_section_id
		FROM sequence_entries e
		JOIN liturgy_item_songs s ON s.church_id = e.church_id AND s.id = e.item_song_id
		JOIN liturgy_items i ON i.church_id = s.church_id AND i.id = s.item_id
		JOIN liturgies l ON l.church_id = i.church_id AND l.id = i.liturgy_id
		WHERE e.church_id = ? AND l.state <> 'published' AND s.song_id = ? AND `+in("e.song_section_id", len(sections))), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	// Answer in the order asked, so the response does not depend on the database.
	want := map[domain.SectionID]bool{}
	for _, id := range busy {
		want[id] = true
	}
	var out []domain.SectionID
	for _, id := range sections {
		if want[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r usageRepo) ReadingInUse(ctx context.Context, reading domain.ReadingID) (bool, error) {
	return r.exists(ctx, `FROM liturgy_items i JOIN liturgies l ON l.church_id = i.church_id AND l.id = i.liturgy_id
		WHERE i.church_id = ? AND i.reading_id = ? AND l.state <> 'published'`, r.churchID, reading)
}

func (r usageRepo) DutyInUse(ctx context.Context, duty string) (bool, error) {
	used, err := r.exists(ctx, "FROM liturgy_items WHERE church_id = ? AND duty_id = ?", r.churchID, duty)
	if used || err != nil {
		return used, err
	}
	return r.exists(ctx, "FROM assignments WHERE church_id = ? AND duty_id = ?", r.churchID, duty)
}

func (r usageRepo) SingingPartInUse(ctx context.Context, part string) (bool, error) {
	return r.exists(ctx, "FROM sequence_entries WHERE church_id = ? AND singing_part_id = ?", r.churchID, part)
}
