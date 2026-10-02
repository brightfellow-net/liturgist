// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

const churchColumns = `id, name, default_ui_language, default_language, default_translation_id, time_zone,
	settings, created_at, updated_at`

func scanChurch(d Dialect, row scanner) (domain.Church, error) {
	var (
		c            domain.Church
		created, upd Time
	)
	err := row.Scan(&c.ID, &c.Name, &c.DefaultUILanguage, &c.DefaultLanguage, &c.DefaultTranslationID, &c.TimeZone,
		jsonValue{&c.Settings}, &created, &upd)
	if err != nil {
		return domain.Church{}, d.MapError(err)
	}
	c.CreatedAt, c.UpdatedAt = created.Time, upd.Time
	return c, nil
}

// --- platform: churches ---

type churchRepo struct{ *store }

func (s *store) Churches() app.ChurchRepo { return churchRepo{s} }

func (r churchRepo) Create(ctx context.Context, c domain.Church) error {
	settings, err := jsonArg(c.Settings)
	if err != nil {
		return err
	}
	_, err = r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO churches (`+churchColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.Name, c.DefaultUILanguage, c.DefaultLanguage, c.DefaultTranslationID, c.TimeZone, settings,
		r.d.TimeArg(c.CreatedAt), r.d.TimeArg(c.UpdatedAt))
	return r.d.MapError(err)
}

func (r churchRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.tx.QueryRowContext(ctx, "SELECT count(*) FROM churches").Scan(&n)
	return n, r.d.MapError(err)
}

func (r churchRepo) IDs(ctx context.Context, limit int) ([]domain.ChurchID, error) {
	var ids []domain.ChurchID
	err := r.tx.SelectContext(ctx, &ids, r.d.Rebind("SELECT id FROM churches ORDER BY id LIMIT ?"), limit)
	return ids, r.d.MapError(err)
}

func (r churchRepo) ByID(ctx context.Context, id domain.ChurchID) (domain.Church, error) {
	return scanChurch(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+churchColumns+" FROM churches WHERE id = ?"), id))
}

// --- church-scoped: the church row itself ---

type churchSettingsRepo struct{ *churchStore }

func (r churchSettingsRepo) Get(ctx context.Context) (domain.Church, error) {
	return scanChurch(r.d, r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT "+churchColumns+" FROM churches WHERE id = ?"), r.churchID))
}

func (r churchSettingsRepo) Update(ctx context.Context, c domain.Church) error {
	settings, err := jsonArg(c.Settings)
	if err != nil {
		return err
	}
	return r.exec1(ctx, `UPDATE churches SET name = ?, default_ui_language = ?, default_language = ?,
		default_translation_id = ?, time_zone = ?, settings = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.DefaultUILanguage, c.DefaultLanguage, c.DefaultTranslationID, c.TimeZone, settings,
		r.d.TimeArg(c.UpdatedAt), r.churchID)
}

// --- platform: translations ---

type translationRepo struct{ *store }

func (s *store) Translations() app.TranslationRepo { return translationRepo{s} }

func (r translationRepo) query(ctx context.Context, where string, args ...any) ([]domain.Translation, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind("SELECT id, code, name, language FROM translations "+where), args...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Translation
	for rows.Next() {
		var t domain.Translation
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Language); err != nil {
			return nil, r.d.MapError(err)
		}
		out = append(out, t)
	}
	return out, r.d.MapError(rows.Err())
}

func (r translationRepo) one(ctx context.Context, where string, arg any) (domain.Translation, error) {
	list, err := r.query(ctx, where, arg)
	if err != nil {
		return domain.Translation{}, err
	}
	if len(list) == 0 {
		return domain.Translation{}, app.ErrNotFound
	}
	return list[0], nil
}

func (r translationRepo) List(ctx context.Context) ([]domain.Translation, error) {
	return r.query(ctx, "ORDER BY code")
}

func (r translationRepo) ByCode(ctx context.Context, code string) (domain.Translation, error) {
	return r.one(ctx, "WHERE code = ?", code)
}

func (r translationRepo) ByID(ctx context.Context, id domain.TranslationID) (domain.Translation, error) {
	return r.one(ctx, "WHERE id = ?", id)
}

// --- platform: setup token ---

type setupTokenRepo struct{ *store }

func (s *store) SetupTokens() app.SetupTokenRepo { return setupTokenRepo{s} }

func (r setupTokenRepo) Put(ctx context.Context, hash string, created, expires time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO setup_tokens (id, token_hash, created_at, expires_at)
		VALUES (1, ?, ?, ?) ON CONFLICT (id) DO UPDATE SET token_hash = excluded.token_hash,
		created_at = excluded.created_at, expires_at = excluded.expires_at`),
		hash, r.d.TimeArg(created), r.d.TimeArg(expires))
	return r.d.MapError(err)
}

func (r setupTokenRepo) Claim(ctx context.Context, hash string, now time.Time) (bool, error) {
	res, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM setup_tokens WHERE id = 1 AND token_hash = ? AND expires_at > ?"),
		hash, r.d.TimeArg(now))
	if err != nil {
		return false, r.d.MapError(err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r setupTokenRepo) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM setup_tokens WHERE expires_at < ?"), r.d.TimeArg(now))
	return r.d.MapError(err)
}
