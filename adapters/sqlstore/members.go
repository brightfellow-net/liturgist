// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"sync"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// --- roles ---

type roleRepo struct{ *churchStore }

// warnedScopes remembers unknown stored scopes already logged (once per process).
var warnedScopes sync.Map

func (r roleRepo) query(ctx context.Context, where string, args ...any) ([]domain.Role, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT id, name, description, origin, created_at, updated_at
		FROM roles WHERE church_id = ? `+where), append([]any{r.churchID}, args...)...)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var (
		out   []domain.Role
		index = map[domain.RoleID]int{}
	)
	for rows.Next() {
		var (
			ro           domain.Role
			origin       sql.NullString
			created, upd Time
		)
		if err := rows.Scan(&ro.ID, &ro.Name, &ro.Description, &origin, &created, &upd); err != nil {
			return nil, r.d.MapError(err)
		}
		ro.Origin, ro.CreatedAt, ro.UpdatedAt, ro.Scopes = domain.RoleOrigin(origin.String), created.Time, upd.Time, domain.ScopeSet{}
		index[ro.ID] = len(out)
		out = append(out, ro)
	}
	if err := rows.Err(); err != nil {
		return nil, r.d.MapError(err)
	}
	if len(out) == 0 {
		return out, nil
	}
	var scopes []struct {
		RoleID domain.RoleID `db:"role_id"`
		Scope  domain.Scope  `db:"scope"`
	}
	if err := r.tx.SelectContext(ctx, &scopes, r.d.Rebind("SELECT role_id, scope FROM role_scopes WHERE church_id = ?"), r.churchID); err != nil {
		return nil, r.d.MapError(err)
	}
	for _, sc := range scopes {
		i, ok := index[sc.RoleID]
		if !ok {
			continue
		}
		if !domain.ValidScope(sc.Scope) { // unknown stored scopes grant nothing (schema role_scopes)
			if _, seen := warnedScopes.LoadOrStore(sc.Scope, true); !seen {
				r.log.Warn("ignoring unknown scope stored in a role", "scope", string(sc.Scope))
			}
			continue
		}
		out[i].Scopes[sc.Scope] = true
	}
	return out, nil
}

func (r roleRepo) one(ctx context.Context, where string, arg any) (domain.Role, error) {
	list, err := r.query(ctx, where, arg)
	if err != nil {
		return domain.Role{}, err
	}
	if len(list) == 0 {
		return domain.Role{}, app.ErrNotFound
	}
	return list[0], nil
}

func (r roleRepo) List(ctx context.Context) ([]domain.Role, error) {
	return r.query(ctx, "ORDER BY name_key, id")
}

func (r roleRepo) ByID(ctx context.Context, id domain.RoleID) (domain.Role, error) {
	return r.one(ctx, "AND id = ?", id)
}

func (r roleRepo) ByOrigin(ctx context.Context, o domain.RoleOrigin) (domain.Role, error) {
	return r.one(ctx, "AND origin = ?", string(o))
}

func (r roleRepo) Create(ctx context.Context, ro domain.Role) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind(`INSERT INTO roles
		(id, church_id, name, name_key, description, origin, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		ro.ID, r.churchID, ro.Name, domain.RoleNameKey(ro.Name), ro.Description, nullString(string(ro.Origin)),
		r.d.TimeArg(ro.CreatedAt), r.d.TimeArg(ro.UpdatedAt))
	if err != nil {
		return r.d.MapError(err)
	}
	return r.setScopes(ctx, ro)
}

func (r roleRepo) setScopes(ctx context.Context, ro domain.Role) error {
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM role_scopes WHERE church_id = ? AND role_id = ?"), r.churchID, ro.ID); err != nil {
		return r.d.MapError(err)
	}
	for _, sc := range ro.Scopes.Sorted() {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO role_scopes (church_id, role_id, scope) VALUES (?, ?, ?)"),
			r.churchID, ro.ID, string(sc)); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r roleRepo) Update(ctx context.Context, ro domain.Role) error {
	if err := r.exec1(ctx, `UPDATE roles SET name = ?, name_key = ?, description = ?, updated_at = ?
		WHERE church_id = ? AND id = ?`, ro.Name, domain.RoleNameKey(ro.Name), ro.Description, r.d.TimeArg(ro.UpdatedAt),
		r.churchID, ro.ID); err != nil {
		return err
	}
	return r.setScopes(ctx, ro)
}

func (r roleRepo) Delete(ctx context.Context, id domain.RoleID) error {
	return r.exec1(ctx, "DELETE FROM roles WHERE church_id = ? AND id = ?", r.churchID, id)
}

func (r roleRepo) MemberCounts(ctx context.Context) (map[domain.RoleID]int, error) {
	var rows []struct {
		RoleID domain.RoleID `db:"role_id"`
		N      int           `db:"n"`
	}
	err := r.tx.SelectContext(ctx, &rows, r.d.Rebind(
		"SELECT role_id, count(*) AS n FROM membership_roles WHERE church_id = ? GROUP BY role_id"), r.churchID)
	out := make(map[domain.RoleID]int, len(rows))
	for _, x := range rows {
		out[x.RoleID] = x.N
	}
	return out, r.d.MapError(err)
}

// --- memberships ---

type membershipRepo struct{ *churchStore }

func (r membershipRepo) roleIDs(ctx context.Context, membership domain.MembershipID) (map[domain.MembershipID][]domain.RoleID, error) {
	q, args := "SELECT membership_id, role_id FROM membership_roles WHERE church_id = ?", []any{r.churchID}
	if membership != "" {
		q, args = q+" AND membership_id = ?", append(args, membership)
	}
	var rows []struct {
		MembershipID domain.MembershipID `db:"membership_id"`
		RoleID       domain.RoleID       `db:"role_id"`
	}
	if err := r.tx.SelectContext(ctx, &rows, r.d.Rebind(q+" ORDER BY role_id"), args...); err != nil {
		return nil, r.d.MapError(err)
	}
	out := map[domain.MembershipID][]domain.RoleID{}
	for _, x := range rows {
		out[x.MembershipID] = append(out[x.MembershipID], x.RoleID)
	}
	return out, nil
}

func (r membershipRepo) List(ctx context.Context) ([]domain.Member, error) {
	rows, err := r.tx.QueryContext(ctx, r.d.Rebind(`SELECT m.id, m.created_at, `+prefixed("u.", userColumns)+`
		FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.church_id = ? ORDER BY lower(u.name), m.id`), r.churchID)
	if err != nil {
		return nil, r.d.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Member
	for rows.Next() {
		var (
			m       domain.Member
			created Time
		)
		u, err := userRepo{r.store}.scanWith(rows, &m.ID, &created)
		if err != nil {
			return nil, err
		}
		m.User, m.UserID, m.CreatedAt = u, u.ID, created.Time
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, r.d.MapError(err)
	}
	roles, err := r.roleIDs(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].RoleIDs = roles[out[i].ID]
	}
	return out, nil
}

func (r membershipRepo) one(ctx context.Context, col string, arg any) (domain.Membership, error) {
	var (
		m       domain.Membership
		created Time
	)
	err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT id, user_id, created_at FROM memberships WHERE church_id = ? AND "+col+" = ?"),
		r.churchID, arg).Scan(&m.ID, &m.UserID, &created)
	if err != nil {
		return domain.Membership{}, r.d.MapError(err)
	}
	m.CreatedAt = created.Time
	roles, err := r.roleIDs(ctx, m.ID)
	m.RoleIDs = roles[m.ID]
	return m, err
}

func (r membershipRepo) ByID(ctx context.Context, id domain.MembershipID) (domain.Membership, error) {
	return r.one(ctx, "id", id)
}

func (r membershipRepo) ByUser(ctx context.Context, user domain.UserID) (domain.Membership, error) {
	return r.one(ctx, "user_id", user)
}

func (r membershipRepo) Create(ctx context.Context, m domain.Membership) error {
	_, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO memberships (id, church_id, user_id, created_at) VALUES (?, ?, ?, ?)"),
		m.ID, r.churchID, m.UserID, r.d.TimeArg(m.CreatedAt))
	if err != nil {
		return r.d.MapError(err)
	}
	return r.insertRoles(ctx, m.ID, m.RoleIDs)
}

func (r membershipRepo) insertRoles(ctx context.Context, id domain.MembershipID, roles []domain.RoleID) error {
	for _, role := range roles {
		if _, err := r.tx.ExecContext(ctx, r.d.Rebind("INSERT INTO membership_roles (church_id, membership_id, role_id) VALUES (?, ?, ?)"),
			r.churchID, id, role); err != nil {
			return r.d.MapError(err)
		}
	}
	return nil
}

func (r membershipRepo) SetRoles(ctx context.Context, id domain.MembershipID, roles []domain.RoleID) error {
	if _, err := r.one(ctx, "id", id); err != nil {
		return err
	}
	if _, err := r.tx.ExecContext(ctx, r.d.Rebind("DELETE FROM membership_roles WHERE church_id = ? AND membership_id = ?"), r.churchID, id); err != nil {
		return r.d.MapError(err)
	}
	return r.insertRoles(ctx, id, roles)
}

func (r membershipRepo) Delete(ctx context.Context, id domain.MembershipID) error {
	return r.exec1(ctx, "DELETE FROM memberships WHERE church_id = ? AND id = ?", r.churchID, id)
}

func (r membershipRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.tx.QueryRowContext(ctx, r.d.Rebind("SELECT count(*) FROM memberships WHERE church_id = ?"), r.churchID).Scan(&n)
	return n, r.d.MapError(err)
}
