// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// ChurchDeps are what the setup and church operations need. Fields may be
// nil when only the OpenAPI document is built.
type ChurchDeps struct {
	Setup    *app.Setup
	Churches *app.Churches
	Cookies  Cookies
	Clock    app.Clock
	Log      *slog.Logger
}

func tagged(o huma.Operation, tag string) huma.Operation {
	o.Tags = []string{tag}
	return o
}

type statusOutput struct {
	Body struct {
		SetUp bool `json:"set_up"`
	}
}

type churchOutput struct{ Body ChurchView }

type translationsOutput struct {
	Body []struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Language string `json:"language"`
	}
}

// RegisterChurch registers the setup and church operations (03 §10, 04 §6).
func RegisterChurch(api huma.API, d ChurchDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }

	// --- setup (platform) ---

	huma.Register(api, tagged(op("getSetupStatus", http.MethodGet, "/setup/status", TenancyPlatform, http.StatusOK, "Whether the church has been set up"), "setup"),
		func(ctx context.Context, _ *struct{}) (*statusOutput, error) {
			done, err := d.Setup.Status(ctx)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &statusOutput{}
			out.Body.SetUp = done
			return out, nil
		})

	huma.Register(api, tagged(op("setup", http.MethodPost, "/setup", TenancyPlatform, http.StatusCreated, "Create the church and its first admin"), "setup"),
		func(ctx context.Context, in *struct {
			Body struct {
				Token  string `json:"token" maxLength:"200"`
				Church struct {
					Name                   string `json:"name" maxLength:"500"`
					DefaultUILanguage      string `json:"default_ui_language" maxLength:"10"`
					DefaultLanguage        string `json:"default_language" maxLength:"10"`
					DefaultTranslationCode string `json:"default_translation_code" maxLength:"16"`
					TimeZone               string `json:"time_zone" maxLength:"100"`
					KeyDisplay             string `json:"key_display" maxLength:"10"`
				} `json:"church"`
				Admin struct {
					Name       string `json:"name" maxLength:"500"`
					Identifier string `json:"identifier" maxLength:"300"`
					Password   string `json:"password" maxLength:"1024"`
				} `json:"admin"`
			}
		}) (*cookiesOutput, error) {
			info := RequestInfoFrom(ctx)
			c := in.Body.Church
			res, err := d.Setup.Run(ctx, app.SetupInput{
				Token: in.Body.Token,
				Church: app.ChurchInput{Name: c.Name, DefaultUILanguage: c.DefaultUILanguage, DefaultLanguage: c.DefaultLanguage,
					DefaultTranslationCode: c.DefaultTranslationCode, TimeZone: c.TimeZone, KeyDisplay: c.KeyDisplay},
				AdminName: in.Body.Admin.Name, AdminIdentifier: in.Body.Admin.Identifier, AdminPassword: in.Body.Admin.Password,
				UserAgent: info.UserAgent, PreviousTokenHash: info.PresentedTokenHash,
			})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("setup_completed", "church_id", string(res.Church.ID), "user_id", string(res.User.ID))
			return &cookiesOutput{SetCookie: d.Cookies.Set(res.Token, res.Session.ExpiresAt, d.Clock.Now())}, nil
		})

	huma.Register(api, tagged(op("listTranslations", http.MethodGet, "/translations", TenancyPlatform, http.StatusOK, "Bible translations"), "church"),
		func(ctx context.Context, _ *struct{}) (*translationsOutput, error) {
			list, err := d.Churches.Translations(ctx)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &translationsOutput{}
			for _, t := range list {
				out.Body = append(out.Body, struct {
					Code     string `json:"code"`
					Name     string `json:"name"`
					Language string `json:"language"`
				}{t.Code, t.Name, t.Language})
			}
			return out, nil
		})

	// --- church ---

	huma.Register(api, tagged(op("getChurch", http.MethodGet, "/church", TenancyChurch, http.StatusOK, "The church"), "church"),
		func(ctx context.Context, _ *struct{}) (*churchOutput, error) {
			res, err := d.Churches.Get(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &churchOutput{Body: churchView(res)}, nil
		})

	huma.Register(api, tagged(op("updateChurch", http.MethodPatch, "/church", TenancyChurch, http.StatusOK, "Change church settings"), "church"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name                   *string   `json:"name,omitempty" maxLength:"500"`
				DefaultUILanguage      *string   `json:"default_ui_language,omitempty" maxLength:"10"`
				DefaultLanguage        *string   `json:"default_language,omitempty" maxLength:"10"`
				DefaultTranslationCode *string   `json:"default_translation_code,omitempty" maxLength:"16"`
				TimeZone               *string   `json:"time_zone,omitempty" maxLength:"100"`
				KeyDisplay             *string   `json:"key_display,omitempty" maxLength:"10"`
				FeedbackURL            OptString `json:"feedback_url,omitempty" required:"false" doc:"null or \"\" clears"`
				PrivacyContact         OptString `json:"privacy_contact,omitempty" required:"false" doc:"null or \"\" clears"`
			}
		}) (*churchOutput, error) {
			b := in.Body
			ch := app.ChurchChange{Name: b.Name, DefaultUILanguage: b.DefaultUILanguage, DefaultLanguage: b.DefaultLanguage,
				DefaultTranslationCode: b.DefaultTranslationCode, TimeZone: b.TimeZone, KeyDisplay: b.KeyDisplay,
				FeedbackURL: b.FeedbackURL.ptr(), PrivacyContact: b.PrivacyContact.ptr()}
			res, err := d.Churches.Update(ctx, sess(ctx), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &churchOutput{Body: churchView(res)}, nil
		})
}
