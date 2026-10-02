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

// ChurchDeps are what the setup, church, member and role
// operations need. Fields may be nil when only the OpenAPI document is built.
type ChurchDeps struct {
	Setup    *app.Setup
	Churches *app.Churches
	Members  *app.Members
	Roles    *app.Roles
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

type membersOutput struct {
	Body struct {
		Members []MemberView `json:"members"`
		Usage   struct {
			TeamMembers struct {
				Used int  `json:"used"`
				Max  *int `json:"max" doc:"null when unlimited"`
			} `json:"team_members"`
		} `json:"usage"`
	}
}

type memberOutput struct{ Body MemberView }

type rolesOutput struct{ Body []RoleView }

type roleOutput struct{ Body RoleView }

type scopesOutput struct {
	Body []struct {
		Scope       string `json:"scope"`
		Description string `json:"description"`
	}
}

// Path parameter structs are used whole; embedded in another input struct
// they would not be set (Huma can't write through an unexported embedded field).
type membershipPath struct {
	MembershipID string `path:"membershipId" maxLength:"26"`
}

type idPath struct {
	ID string `path:"id" maxLength:"26"`
}

// RegisterChurch registers the setup, church, member and role operations
// (03 §8, §10, 04 §6).
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

	// --- members ---

	huma.Register(api, tagged(op("listMembers", http.MethodGet, "/members", TenancyChurch, http.StatusOK, "Members of the church"), "members"),
		func(ctx context.Context, _ *struct{}) (*membersOutput, error) {
			res, err := d.Members.List(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &membersOutput{}
			out.Body.Members = []MemberView{}
			for _, m := range res.Members {
				out.Body.Members = append(out.Body.Members, memberView(m))
			}
			out.Body.Usage.TeamMembers.Used, out.Body.Usage.TeamMembers.Max = res.Used, res.Max
			return out, nil
		})

	huma.Register(api, tagged(op("setMemberRoles", http.MethodPatch, "/members/{membershipId}", TenancyChurch, http.StatusOK, "Assign roles to a member"), "members"),
		func(ctx context.Context, in *struct {
			MembershipID string `path:"membershipId" maxLength:"26"`
			Body         struct {
				RoleIDs []string `json:"role_ids" maxItems:"100"`
			}
		}) (*memberOutput, error) {
			ids := make([]domain.RoleID, len(in.Body.RoleIDs))
			for i, id := range in.Body.RoleIDs {
				ids[i] = domain.RoleID(id)
			}
			v, err := d.Members.SetRoles(ctx, sess(ctx), domain.MembershipID(in.MembershipID), ids)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &memberOutput{Body: memberView(v)}, nil
		})

	huma.Register(api, tagged(op("removeMember", http.MethodDelete, "/members/{membershipId}", TenancyChurch, http.StatusNoContent, "Remove a member"), "members"),
		func(ctx context.Context, in *membershipPath) (*struct{}, error) {
			if err := d.Members.Remove(ctx, sess(ctx), domain.MembershipID(in.MembershipID)); err != nil {
				return nil, fail(ctx, err)
			}
			return nil, nil
		})

	// --- roles ---

	huma.Register(api, tagged(op("listRoles", http.MethodGet, "/roles", TenancyChurch, http.StatusOK, "Roles of the church"), "roles"),
		func(ctx context.Context, _ *struct{}) (*rolesOutput, error) {
			list, err := d.Roles.List(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &rolesOutput{Body: []RoleView{}}
			for _, r := range list {
				out.Body = append(out.Body, roleView(r))
			}
			return out, nil
		})

	huma.Register(api, tagged(op("createRole", http.MethodPost, "/roles", TenancyChurch, http.StatusCreated, "Create a role"), "roles"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name        string   `json:"name" maxLength:"500"`
				Description string   `json:"description,omitempty" maxLength:"2000"`
				Scopes      []string `json:"scopes" maxItems:"50"`
			}
		}) (*roleOutput, error) {
			v, err := d.Roles.Create(ctx, sess(ctx), app.RoleInput{Name: in.Body.Name, Description: in.Body.Description, Scopes: toScopes(in.Body.Scopes)})
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &roleOutput{Body: roleView(v)}, nil
		})

	huma.Register(api, tagged(op("updateRole", http.MethodPatch, "/roles/{id}", TenancyChurch, http.StatusOK, "Rename a role or change its scopes"), "roles"),
		func(ctx context.Context, in *struct {
			ID   string `path:"id" maxLength:"26"`
			Body struct {
				Name        *string   `json:"name,omitempty" maxLength:"500"`
				Description *string   `json:"description,omitempty" maxLength:"2000"`
				Scopes      *[]string `json:"scopes,omitempty" maxItems:"50"`
			}
		}) (*roleOutput, error) {
			ch := app.RoleChange{Name: in.Body.Name, Description: in.Body.Description}
			if in.Body.Scopes != nil {
				sc := toScopes(*in.Body.Scopes)
				ch.Scopes = &sc
			}
			v, err := d.Roles.Update(ctx, sess(ctx), domain.RoleID(in.ID), ch)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &roleOutput{Body: roleView(v)}, nil
		})

	huma.Register(api, tagged(op("deleteRole", http.MethodDelete, "/roles/{id}", TenancyChurch, http.StatusNoContent, "Delete a role"), "roles"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Roles.Delete(ctx, sess(ctx), domain.RoleID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			return nil, nil
		})

	huma.Register(api, tagged(op("listScopes", http.MethodGet, "/scopes", TenancyChurch, http.StatusOK, "Scopes with descriptions"), "roles"),
		func(ctx context.Context, _ *struct{}) (*scopesOutput, error) {
			list, err := d.Roles.Scopes(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &scopesOutput{}
			for _, s := range list {
				out.Body = append(out.Body, struct {
					Scope       string `json:"scope"`
					Description string `json:"description"`
				}{string(s.Scope), s.Description})
			}
			return out, nil
		})

}

func toScopes(in []string) []domain.Scope {
	out := make([]domain.Scope, len(in))
	for i, s := range in {
		out[i] = domain.Scope(s)
	}
	return out
}
