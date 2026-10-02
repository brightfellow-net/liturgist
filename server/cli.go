// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/app"
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
	ErrAlreadySetUp = app.ErrAlreadySetUp // exit 4
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
