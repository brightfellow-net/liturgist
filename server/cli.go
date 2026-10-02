// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// Operator runs the command-line operations of 01 §6 against the database.
// It never migrates: a database with pending migrations is refused.
type Operator struct {
	cfg    Config
	db     *sqlstore.DB
	single *tenancy.SingleChurch
	uc     useCases
	urls   app.URLBuilder
}

// CLI error classes, for exit codes (01 §6).
var (
	ErrAlreadySetUp    = app.ErrAlreadySetUp        // exit 4
	ErrNoSuchUser      = errors.New("no such user") // exit 5
	ErrNotMember       = app.ErrNotMember           // exit 6
	ErrTooManyChurches = app.ErrTooManyChurches     // exit 7
)

// OpenOperator opens the database for a CLI command.
func OpenOperator(ctx context.Context, cfg Config) (*Operator, error) {
	cfg = withDefaults(cfg)
	db, mo, err := openDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	current, target, err := db.CheckVersion(ctx, mo)
	if err == nil && current < target {
		err = fmt.Errorf("database version %d needs migrating to %d; run \"liturgist migrate\"", current, target)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	var o options
	op := &Operator{cfg: cfg, db: db, single: &tenancy.SingleChurch{Tx: db}}
	op.uc = wire(cfg, db, &o, func() {})
	op.urls = o.urls
	return op, nil
}

// Close closes the database.
func (o *Operator) Close() error { return o.db.Close() }

// church returns the install's church; ErrTooManyChurches (exit 7) or app.ErrNotSetUp.
func (o *Operator) church(ctx context.Context) (domain.ChurchID, error) {
	return o.single.ChurchID(ctx)
}

// SetupLink issues a new setup token and returns the link (ErrAlreadySetUp: exit 4).
func (o *Operator) SetupLink(ctx context.Context) (string, error) {
	if err := o.single.Check(ctx); err != nil {
		return "", err
	}
	token, _, err := o.uc.setup.IssueToken(ctx)
	if err != nil {
		return "", err
	}
	o.cfg.Logger.Info("setup link issued from the command line")
	return o.urls.AppURL(ctx, "/setup") + "#t=" + token, nil
}

// SetupParams are the flags of "liturgist setup".
type SetupParams struct {
	ChurchName, AdminName, AdminIdentifier, Password string
	UILanguage, Language, Translation, TimeZone      string
	KeyDisplay                                       string
}

// Setup creates the church and first admin without a token, under the same
// lock and church-count check as the web wizard.
func (o *Operator) Setup(ctx context.Context, p SetupParams) error {
	res, err := o.uc.setup.Run(ctx, app.SetupInput{ViaCLI: true,
		Church: app.ChurchInput{Name: p.ChurchName, DefaultUILanguage: p.UILanguage, DefaultLanguage: p.Language,
			DefaultTranslationCode: p.Translation, TimeZone: p.TimeZone, KeyDisplay: p.KeyDisplay},
		AdminName: p.AdminName, AdminIdentifier: p.AdminIdentifier, AdminPassword: p.Password})
	if err != nil {
		return err
	}
	o.cfg.Logger.Info("setup_completed", "church_id", string(res.Church.ID), "user_id", string(res.User.ID), "via", "cli")
	return nil
}

// ResetResult is printed by "liturgist user reset-password".
type ResetResult struct {
	UserName  string
	Link      string
	ExpiresAt time.Time // in the church's time zone when set up
}

// ResetPassword creates a reset link for the user (ErrNoSuchUser: exit 5).
func (o *Operator) ResetPassword(ctx context.Context, identifier string) (ResetResult, error) {
	res, err := o.uc.resets.CreateForUser(ctx, identifier)
	if errors.Is(err, app.ErrNotFound) {
		return ResetResult{}, ErrNoSuchUser
	}
	if err != nil {
		return ResetResult{}, err
	}
	o.cfg.Logger.Info("reset_link_created", "actor", "cli", "target_user_id", string(res.User.ID))
	out := ResetResult{UserName: res.User.Name, Link: res.Link, ExpiresAt: res.ExpiresAt}
	if loc := o.churchLocation(ctx); loc != nil {
		out.ExpiresAt = out.ExpiresAt.In(loc)
	}
	return out, nil
}

func (o *Operator) churchLocation(ctx context.Context) *time.Location {
	id, err := o.church(ctx)
	if err != nil {
		return nil
	}
	var tz string
	_ = o.db.Read(ctx, func(s app.Store) error {
		c, err := s.Churches().ByID(ctx, id)
		tz = c.TimeZone
		return err
	})
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil
	}
	return loc
}

// UserLine is one line of "liturgist user list".
type UserLine struct {
	Name, Email, Phone string
	Roles              []string // nil: not a member of the church
}

// ListUsers lists every user with their roles in the church.
func (o *Operator) ListUsers(ctx context.Context) ([]UserLine, error) {
	church, err := o.church(ctx)
	if err != nil && !errors.Is(err, app.ErrNotSetUp) {
		return nil, err
	}
	lines, err := o.uc.operator.ListUsers(ctx, church)
	if err != nil {
		return nil, err
	}
	out := make([]UserLine, len(lines))
	for i, l := range lines {
		out[i] = UserLine{Name: l.User.Name, Email: l.User.Email, Phone: l.User.Phone, Roles: l.Roles}
	}
	return out, nil
}

// GrantAdmin gives a member the Church admin role (ErrNoSuchUser: exit 5; ErrNotMember: exit 6).
func (o *Operator) GrantAdmin(ctx context.Context, identifier string) error {
	church, err := o.church(ctx)
	if err != nil {
		return err
	}
	usr, err := o.uc.operator.GrantAdmin(ctx, church, identifier)
	if errors.Is(err, app.ErrNotFound) {
		return ErrNoSuchUser
	}
	if err != nil {
		return err
	}
	o.cfg.Logger.Warn("grant_admin_cli", "user_id", string(usr.ID))
	return nil
}

// ClearThrottle deletes login-throttle counters for an identifier, an IP
// address, or all of them.
func (o *Operator) ClearThrottle(ctx context.Context, identifier, ip string, all bool) error {
	var addr string
	if ip != "" {
		a, err := netip.ParseAddr(ip)
		if err != nil {
			return fmt.Errorf("--ip: %q is not an IP address", ip)
		}
		addr = domain.ClientAddrKey(a)
	}
	return o.uc.operator.ClearThrottle(ctx, identifier, addr, all)
}
