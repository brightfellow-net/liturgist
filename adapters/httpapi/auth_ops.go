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

// Tenancy declarations (04 §2); enforced by the tenant middleware in slice 4.
const (
	TenancyKey      = "tenancy"
	TenancyChurch   = "church"
	TenancyOptional = "optional"
	TenancyPlatform = "platform"
)

// AuthDeps are what the auth operations need. Fields may be nil when only the
// OpenAPI document is built.
type AuthDeps struct {
	Auth    *app.Auth
	Account *app.Account
	Cookies Cookies
	Clock   app.Clock
	Log     *slog.Logger
}

type cookiesOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

// UserView is the user as the API shows it.
type UserView struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Email       *string            `json:"email"`
	Phone       *string            `json:"phone"`
	Preferences domain.Preferences `json:"preferences"`
}

func userView(u domain.User) UserView {
	v := UserView{ID: string(u.ID), Name: u.Name, Preferences: u.Preferences}
	if u.Email != "" {
		v.Email = &u.Email
	}
	if u.Phone != "" {
		v.Phone = &u.Phone
	}
	return v
}

type meOutput struct {
	Body struct {
		User       UserView  `json:"user"`
		Membership *struct{} `json:"membership" doc:"Arrives with churches (slice 4)."`
		Church     *struct{} `json:"church"`
	}
}

type userOutput struct{ Body UserView }

func op(id, method, path, tenancy string, status int, summary string) huma.Operation {
	return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary,
		DefaultStatus: status, Metadata: map[string]any{TenancyKey: tenancy}, Tags: []string{"auth"}}
}

// RegisterAuth registers /auth and /me operations (03 §4–§9, 04 §6).
func RegisterAuth(api huma.API, d AuthDeps) {
	fail := func(err error) error { return MapError(err, d.Log) }

	huma.Register(api, op("login", http.MethodPost, "/auth/login", TenancyPlatform, http.StatusNoContent, "Log in"),
		func(ctx context.Context, in *struct {
			Body struct {
				Identifier string `json:"identifier" maxLength:"300" doc:"Email or phone number"`
				Password   string `json:"password" maxLength:"1024"`
			}
		}) (*cookiesOutput, error) {
			info := RequestInfoFrom(ctx)
			res, err := d.Auth.Login(ctx, app.LoginInput{
				Identifier: in.Body.Identifier, Password: in.Body.Password, ClientAddr: info.ClientAddr,
				UserAgent: info.UserAgent, PreviousTokenHash: info.PresentedTokenHash,
			})
			if err != nil {
				return nil, fail(err)
			}
			return &cookiesOutput{SetCookie: d.Cookies.Set(res.Token, res.Session.ExpiresAt, d.Clock.Now())}, nil
		})

	huma.Register(api, op("logout", http.MethodPost, "/auth/logout", TenancyPlatform, http.StatusNoContent, "Log out"),
		func(ctx context.Context, _ *struct{}) (*cookiesOutput, error) {
			if h := RequestInfoFrom(ctx).PresentedTokenHash; h != "" {
				if err := d.Auth.Logout(ctx, h); err != nil {
					return nil, fail(err)
				}
			}
			return &cookiesOutput{SetCookie: d.Cookies.Clear()}, nil
		})

	huma.Register(api, op("getMe", http.MethodGet, "/me", TenancyOptional, http.StatusOK, "The logged-in user"),
		func(ctx context.Context, _ *struct{}) (*meOutput, error) {
			u, err := d.Account.Me(ctx, RequestInfoFrom(ctx).Session)
			if err != nil {
				return nil, fail(err)
			}
			out := &meOutput{}
			out.Body.User = userView(u)
			return out, nil
		})

	huma.Register(api, op("updateMe", http.MethodPatch, "/me", TenancyPlatform, http.StatusOK, "Change name or preferences"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name        *string `json:"name,omitempty"`
				Preferences *struct {
					TextSize   *string   `json:"text_size,omitempty"`
					UILanguage OptString `json:"ui_language,omitempty" required:"false"`
				} `json:"preferences,omitempty"`
			}
		}) (*userOutput, error) {
			ch := app.ProfileChange{Name: in.Body.Name}
			if p := in.Body.Preferences; p != nil {
				ch.TextSize = p.TextSize
				if p.UILanguage.Set && p.UILanguage.Null {
					ch.ClearLanguage = true
				} else if p.UILanguage.Set {
					ch.UILanguage = &p.UILanguage.Value
				}
			}
			u, err := d.Account.UpdateProfile(ctx, RequestInfoFrom(ctx).Session, ch)
			if err != nil {
				return nil, fail(err)
			}
			return &userOutput{Body: userView(u)}, nil
		})

	huma.Register(api, op("changePassword", http.MethodPost, "/me/password", TenancyPlatform, http.StatusNoContent, "Change own password"),
		func(ctx context.Context, in *struct {
			Body struct {
				CurrentPassword string `json:"current_password" maxLength:"1024"`
				NewPassword     string `json:"new_password" maxLength:"1024"`
			}
		}) (*struct{}, error) {
			info := RequestInfoFrom(ctx)
			if err := d.Account.ChangePassword(ctx, info.Session, info.ClientAddr, in.Body.CurrentPassword, in.Body.NewPassword); err != nil {
				return nil, fail(err)
			}
			return nil, nil
		})

	huma.Register(api, op("endOtherSessions", http.MethodPost, "/me/sessions/end-others", TenancyPlatform, http.StatusNoContent, "Log out on all other devices"),
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			s := RequestInfoFrom(ctx).Session
			if s == nil {
				return nil, fail(app.ErrUnauthenticated)
			}
			if err := d.Auth.EndOtherSessions(ctx, s.UserID, s.TokenHash); err != nil {
				return nil, fail(err)
			}
			return nil, nil
		})
}
