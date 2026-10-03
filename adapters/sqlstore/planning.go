// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// nameListRepo implements app.NameListRepo for duties and singing parts (09
// §2.1). table is one of two constants, never user input. Every query filters
// by church_id.
type nameListRepo struct {
	*churchStore
	table string
}

func (c *churchStore) Duties() app.NameListRepo       { return nameListRepo{c, "duties"} }
func (c *churchStore) SingingParts() app.NameListRepo { return nameListRepo{c, "singing_parts"} }
func (c *churchStore) Templates() app.TemplateRepo    { return templateRepo{c} }
func (c *churchStore) Services() app.ServiceRepo      { return serviceRepo{c} }
func (c *churchStore) Seeds() app.SeedRepo            { return seedRepo{c} }

const nameListColumns = "id, name, name_key, position, created_at"

func scanEntry(scan func(...any) error) (domain.NameEntry, error) {
	var (
		e       domain.NameEntry
		created Time
	)
	if err := scan(&e.ID, &e.Name, &e.NameKey, &e.Position, &created); err != nil {
		return domain.NameEntry{}, err
	}
	e.CreatedAt = created.Time
	return e, nil
}

func (r nameListRepo) List(ctx context.Context) ([]domain.NameEntry, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+nameListColumns+" FROM "+r.table+
		" WHERE church_id = ? ORDER BY position, id"), r.churchID)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.NameEntry
	for rows.Next() {
		e, err := scanEntry(rows.Scan)
		if err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, e)
	}
	return out, r.d.MapError(rows.Err())
}

func (r nameListRepo) ByID(ctx context.Context, id string) (domain.NameEntry, error) {
	e, err := scanEntry(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+nameListColumns+" FROM "+r.table+
		" WHERE church_id = ? AND id = ?"), r.churchID, id).Scan)
	return e, r.d.MapError(err)
}

func (r nameListRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM "+r.table+" WHERE church_id = ?"), r.churchID)
	return n, r.d.MapError(err)
}

func (r nameListRepo) Create(ctx context.Context, e domain.NameEntry) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO "+r.table+" (id, church_id, name, name_key, position, created_at) VALUES (?, ?, ?, ?, ?, ?)"),
		e.ID, r.churchID, e.Name, e.NameKey, e.Position, r.d.TimeArg(e.CreatedAt))
	return r.d.MapError(err)
}

func (r nameListRepo) Rename(ctx context.Context, id, name, nameKey string) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE "+r.table+" SET name = ?, name_key = ? WHERE church_id = ? AND id = ?"),
		name, nameKey, r.churchID, id)
	return r.affected(res, err)
}

func (r nameListRepo) SetOrder(ctx context.Context, ids []string) error {
	for i, id := range ids {
		res, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE "+r.table+" SET position = ? WHERE church_id = ? AND id = ?"), i, r.churchID, id)
		if err := r.affected(res, err); err != nil {
			return err
		}
	}
	return nil
}

func (r nameListRepo) Delete(ctx context.Context, id string) error {
	if r.table == "duties" {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind("UPDATE template_items SET default_duty_id = NULL WHERE church_id = ? AND default_duty_id = ?"),
			r.churchID, id); err != nil {
			return r.d.MapError(err)
		}
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM "+r.table+" WHERE church_id = ? AND id = ?"), r.churchID, id)
	return r.affected(res, err)
}

// affected maps an exec result to ErrNotFound when no row matched.
func (r nameListRepo) affected(res rowsResult, err error) error {
	return rowsOrNotFound(r.d, res, err)
}

type rowsResult interface{ RowsAffected() (int64, error) }

func rowsOrNotFound(d Dialect, res rowsResult, err error) error {
	if err != nil {
		return d.MapError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

// --- templates ---

type templateRepo struct{ *churchStore }

func (r templateRepo) List(ctx context.Context) ([]app.TemplateRow, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT t.id, t.name, t.language, t.version,
		(SELECT COUNT(*) FROM template_items i WHERE i.church_id = t.church_id AND i.template_id = t.id)
		FROM templates t WHERE t.church_id = ? ORDER BY t.name_key, t.id`), r.churchID)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []app.TemplateRow
	for rows.Next() {
		var row app.TemplateRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Language, &row.Version, &row.ItemCount); err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, row)
	}
	return out, r.d.MapError(rows.Err())
}

func (r templateRepo) ByID(ctx context.Context, id domain.TemplateID) (domain.Template, error) {
	var (
		t            domain.Template
		created, upd Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind(`SELECT id, name, name_key, language, version, created_at, updated_at
		FROM templates WHERE church_id = ? AND id = ?`), r.churchID, id).
		Scan(&t.ID, &t.Name, &t.NameKey, &t.Language, &t.Version, &created, &upd)
	if err != nil {
		return domain.Template{}, r.d.MapError(err)
	}
	t.CreatedAt, t.UpdatedAt = created.Time, upd.Time
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT id, title, item_type, default_text, default_duty_id
		FROM template_items WHERE church_id = ? AND template_id = ? ORDER BY position`), r.churchID, id)
	if err != nil {
		return domain.Template{}, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			it   domain.TemplateItem
			duty *string
		)
		if err := rows.Scan(&it.ID, &it.Title, &it.Type, &it.DefaultText, &duty); err != nil {
			return domain.Template{}, r.d.MapError(err)
		}
		if duty != nil {
			it.DefaultDutyID = domain.DutyID(*duty)
		}
		t.Items = append(t.Items, it)
	}
	return t, r.d.MapError(rows.Err())
}

func (r templateRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM templates WHERE church_id = ?"), r.churchID)
	return n, r.d.MapError(err)
}

func (r templateRepo) Create(ctx context.Context, t domain.Template) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO templates (id, church_id, name, name_key, language, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		t.ID, r.churchID, t.Name, t.NameKey, t.Language, t.Version, r.d.TimeArg(t.CreatedAt), r.d.TimeArg(t.UpdatedAt))
	if err != nil {
		return r.d.MapError(err)
	}
	return r.insertItems(ctx, t)
}

func (r templateRepo) insertItems(ctx context.Context, t domain.Template) error {
	for i, it := range t.Items {
		var duty any
		if it.DefaultDutyID != "" {
			duty = string(it.DefaultDutyID)
		}
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO template_items (id, church_id, template_id, position, title,
			item_type, default_text, default_duty_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
			it.ID, r.churchID, t.ID, i, it.Title, string(it.Type), it.DefaultText, duty); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r templateRepo) Update(ctx context.Context, t domain.Template, expectedVersion int) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE templates SET name = ?, name_key = ?, language = ?, version = ?, updated_at = ?
		WHERE church_id = ? AND id = ? AND version = ?`),
		t.Name, t.NameKey, t.Language, t.Version, r.d.TimeArg(t.UpdatedAt), r.churchID, t.ID, expectedVersion)
	if err != nil {
		return false, r.d.MapError(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM template_items WHERE church_id = ? AND template_id = ?"), r.churchID, t.ID); err != nil {
		return false, r.d.MapError(err)
	}
	return true, r.insertItems(ctx, t)
}

func (r templateRepo) Delete(ctx context.Context, id domain.TemplateID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM templates WHERE church_id = ? AND id = ?"), r.churchID, id)
	return rowsOrNotFound(r.d, res, err)
}

func (r templateRepo) ServicesUsing(ctx context.Context, id domain.TemplateID) ([]domain.ServiceID, error) {
	var ids []domain.ServiceID
	err := r.tx.SelectContext(ctx, &ids, r.d.Rebind("SELECT id FROM services WHERE church_id = ? AND default_template_id = ? ORDER BY name_key, id"),
		r.churchID, id)
	return ids, r.d.MapError(err)
}

// --- services ---

type serviceRepo struct{ *churchStore }

const serviceColumns = "id, name, name_key, language, default_template_id, version, created_at, updated_at"

func scanService(scan func(...any) error) (domain.Service, error) {
	var (
		s            domain.Service
		tpl          *string
		created, upd Time
	)
	if err := scan(&s.ID, &s.Name, &s.NameKey, &s.Language, &tpl, &s.Version, &created, &upd); err != nil {
		return domain.Service{}, err
	}
	if tpl != nil {
		s.DefaultTemplateID = domain.TemplateID(*tpl)
	}
	s.CreatedAt, s.UpdatedAt = created.Time, upd.Time
	return s, nil
}

// times loads the times of one service, or of every service when id is "".
func (r serviceRepo) times(ctx context.Context, id string) (map[domain.ServiceID][]domain.ServiceTime, error) {
	q := "SELECT service_id, id, weekday, time FROM service_times WHERE church_id = ?"
	args := []any{r.churchID}
	if id != "" {
		q += " AND service_id = ?"
		args = append(args, id)
	}
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(q+" ORDER BY weekday, time"), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[domain.ServiceID][]domain.ServiceTime{}
	for rows.Next() {
		var (
			sid domain.ServiceID
			t   domain.ServiceTime
		)
		if err := rows.Scan(&sid, &t.ID, &t.Weekday, &t.Time); err != nil {
			return nil, r.d.MapError(err)
		}
		out[sid] = append(out[sid], t)
	}
	return out, r.d.MapError(rows.Err())
}

func (r serviceRepo) List(ctx context.Context) ([]domain.Service, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT "+serviceColumns+" FROM services WHERE church_id = ? ORDER BY name_key, id"), r.churchID)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	var out []domain.Service
	for rows.Next() {
		s, err := scanService(rows.Scan)
		if err != nil {
			_ = rows.Close()
			return nil, r.d.MapError(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, r.d.MapError(err)
	}
	_ = rows.Close()
	times, err := r.times(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Times = times[out[i].ID]
	}
	return out, nil
}

func (r serviceRepo) ByID(ctx context.Context, id domain.ServiceID) (domain.Service, error) {
	s, err := scanService(r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+serviceColumns+" FROM services WHERE church_id = ? AND id = ?"),
		r.churchID, id).Scan)
	if err != nil {
		return domain.Service{}, r.d.MapError(err)
	}
	times, err := r.times(ctx, string(id))
	s.Times = times[s.ID]
	return s, err
}

func (r serviceRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM services WHERE church_id = ?"), r.churchID)
	return n, r.d.MapError(err)
}

func (r serviceRepo) Create(ctx context.Context, s domain.Service) error {
	var tpl any
	if s.DefaultTemplateID != "" {
		tpl = string(s.DefaultTemplateID)
	}
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO services (id, church_id, name, name_key, language, default_template_id,
		version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		s.ID, r.churchID, s.Name, s.NameKey, s.Language, tpl, s.Version, r.d.TimeArg(s.CreatedAt), r.d.TimeArg(s.UpdatedAt))
	if err != nil {
		return r.d.MapError(err)
	}
	return r.insertTimes(ctx, s)
}

func (r serviceRepo) insertTimes(ctx context.Context, s domain.Service) error {
	for _, t := range s.Times {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO service_times (id, church_id, service_id, weekday, time)
			VALUES (?, ?, ?, ?, ?)`), t.ID, r.churchID, s.ID, t.Weekday, t.Time); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r serviceRepo) Update(ctx context.Context, s domain.Service, expectedVersion int) (bool, error) {
	var tpl any
	if s.DefaultTemplateID != "" {
		tpl = string(s.DefaultTemplateID)
	}
	res, err := r.tx.ExecContext(ctx, r.d.Rebind(`UPDATE services SET name = ?, name_key = ?, language = ?, default_template_id = ?,
		version = ?, updated_at = ? WHERE church_id = ? AND id = ? AND version = ?`),
		s.Name, s.NameKey, s.Language, tpl, s.Version, r.d.TimeArg(s.UpdatedAt), r.churchID, s.ID, expectedVersion)
	if err != nil {
		return false, r.d.MapError(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM service_times WHERE church_id = ? AND service_id = ?"), r.churchID, s.ID); err != nil {
		return false, r.d.MapError(err)
	}
	return true, r.insertTimes(ctx, s)
}

func (r serviceRepo) Delete(ctx context.Context, id domain.ServiceID) error {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM services WHERE church_id = ? AND id = ?"), r.churchID, id)
	return rowsOrNotFound(r.d, res, err)
}

// --- seed markers ---

type seedRepo struct{ *churchStore }

func (r seedRepo) Applied(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.tx.GetContext(ctx, &n, r.d.Rebind("SELECT COUNT(*) FROM church_seeds WHERE church_id = ? AND seed_key = ?"), r.churchID, key)
	return n > 0, r.d.MapError(err)
}

func (r seedRepo) Mark(ctx context.Context, key string, at time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO church_seeds (church_id, seed_key, applied_at) VALUES (?, ?, ?)"),
		r.churchID, key, r.d.TimeArg(at))
	return r.d.MapError(err)
}
