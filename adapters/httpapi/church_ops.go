// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// ChurchDeps are what the setup, church, member, role, invite and reset
// operations need. Fields may be nil when only the OpenAPI document is built.
type ChurchDeps struct {
	Setup    *app.Setup
	Churches *app.Churches
	Members  *app.Members
	Roles    *app.Roles
	Invites  *app.Invites
	Resets   *app.Resets
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

type invitesOutput struct{ Body []InviteView }

type createdInviteOutput struct {
	Body struct {
		Invite    InviteView `json:"invite"`
		Link      string     `json:"link" doc:"Shown only now and after regenerating"`
		ExpiresAt time.Time  `json:"expires_at"`
	}
}

type linkOutput struct{ Body LinkView }

type inviteInfoOutput struct {
	Body struct {
		ChurchName  string  `json:"church_name"`
		InviteeName string  `json:"invitee_name"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		Status      string  `json:"status" enum:"pending"`
		OwnerExists bool    `json:"owner_exists" doc:"The invite's email or phone belongs to an existing account"`
	}
}

type resetInfoOutput struct {
	Body struct {
		UserName      string    `json:"user_name"`
		CreatedByName *string   `json:"created_by_name" doc:"null: created by the server administrator"`
		ExpiresAt     time.Time `json:"expires_at"`
	}
}

type statusOnlyOutput struct{ Status int }

type tokenBody struct {
	Body struct {
		Token string `json:"token" maxLength:"200"`
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

// RegisterChurch registers the setup, church, member, role, invite and reset
// operations (03 §7–§10, 04 §6).
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
				ShowCredits            *bool     `json:"show_credits,omitempty"`
				LicenceFooter          *string   `json:"licence_footer,omitempty" maxLength:"1000" doc:"\"\" clears; at most 200 characters"`
				Print                  *struct {
					Paper       string `json:"paper" enum:"a4,f4"`
					Lyrics      string `json:"lyrics" enum:"full,first_lines"`
					Readings    bool   `json:"readings"`
					Assignments bool   `json:"assignments"`
					Keys        bool   `json:"keys"`
					Notes       bool   `json:"notes"`
					Size        string `json:"size" enum:"normal,large"`
				} `json:"print,omitempty" doc:"A complete object that replaces the old one"`
			}
		}) (*churchOutput, error) {
			b := in.Body
			ch := app.ChurchChange{Name: b.Name, DefaultUILanguage: b.DefaultUILanguage, DefaultLanguage: b.DefaultLanguage,
				DefaultTranslationCode: b.DefaultTranslationCode, TimeZone: b.TimeZone, KeyDisplay: b.KeyDisplay,
				FeedbackURL: b.FeedbackURL.ptr(), PrivacyContact: b.PrivacyContact.ptr(),
				ShowCredits: b.ShowCredits, LicenceFooter: b.LicenceFooter}
			if p := b.Print; p != nil {
				ch.Print = &domain.PrintDefaults{Paper: p.Paper, Lyrics: p.Lyrics, Readings: p.Readings, Assignments: p.Assignments,
					Keys: p.Keys, Notes: p.Notes, Size: p.Size}
			}
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

	huma.Register(api, tagged(op("createResetLink", http.MethodPost, "/members/{membershipId}/password-reset", TenancyChurch, http.StatusCreated, "Create a password-reset link for a member"), "members"),
		func(ctx context.Context, in *membershipPath) (*linkOutput, error) {
			res, err := d.Resets.CreateForMember(ctx, sess(ctx), domain.MembershipID(in.MembershipID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reset_link_created", "actor", string(sess(ctx).UserID), "target_user_id", string(res.User.ID))
			return &linkOutput{Body: LinkView{Link: res.Link, ExpiresAt: res.ExpiresAt}}, nil
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

	// --- invites ---

	huma.Register(api, tagged(op("listInvites", http.MethodGet, "/invites", TenancyChurch, http.StatusOK, "Pending and expired invites"), "invites"),
		func(ctx context.Context, _ *struct{}) (*invitesOutput, error) {
			list, err := d.Invites.List(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &invitesOutput{Body: []InviteView{}}
			for _, v := range list {
				out.Body = append(out.Body, inviteView(v))
			}
			return out, nil
		})

	huma.Register(api, tagged(op("createInvite", http.MethodPost, "/invites", TenancyChurch, http.StatusCreated, "Invite a person"), "invites"),
		func(ctx context.Context, in *struct {
			Body struct {
				Name    string   `json:"name" maxLength:"500"`
				Email   string   `json:"email,omitempty" maxLength:"300"`
				Phone   string   `json:"phone,omitempty" maxLength:"300"`
				RoleIDs []string `json:"role_ids,omitempty" maxItems:"100"`
			}
		}) (*createdInviteOutput, error) {
			ids := make([]domain.RoleID, len(in.Body.RoleIDs))
			for i, id := range in.Body.RoleIDs {
				ids[i] = domain.RoleID(id)
			}
			v, link, err := d.Invites.Create(ctx, sess(ctx), app.InviteInput{Name: in.Body.Name, Email: in.Body.Email, Phone: in.Body.Phone, RoleIDs: ids})
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &createdInviteOutput{}
			out.Body.Invite, out.Body.Link, out.Body.ExpiresAt = inviteView(v), link.Link, link.ExpiresAt
			return out, nil
		})

	huma.Register(api, tagged(op("regenerateInvite", http.MethodPost, "/invites/{id}/regenerate", TenancyChurch, http.StatusOK, "New link for an invite"), "invites"),
		func(ctx context.Context, in *idPath) (*linkOutput, error) {
			link, err := d.Invites.Regenerate(ctx, sess(ctx), domain.InviteID(in.ID))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &linkOutput{Body: LinkView{Link: link.Link, ExpiresAt: link.ExpiresAt}}, nil
		})

	huma.Register(api, tagged(op("cancelInvite", http.MethodDelete, "/invites/{id}", TenancyChurch, http.StatusNoContent, "Cancel an invite"), "invites"),
		func(ctx context.Context, in *idPath) (*struct{}, error) {
			if err := d.Invites.Cancel(ctx, sess(ctx), domain.InviteID(in.ID)); err != nil {
				return nil, fail(ctx, err)
			}
			return nil, nil
		})

	huma.Register(api, tagged(op("inspectInvite", http.MethodPost, "/invites/inspect", TenancyOptional, http.StatusOK, "Describe an invite link"), "invites"),
		func(ctx context.Context, in *tokenBody) (*inviteInfoOutput, error) {
			info, err := d.Invites.Inspect(ctx, in.Body.Token)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &inviteInfoOutput{}
			out.Body.ChurchName, out.Body.InviteeName, out.Body.Email, out.Body.Phone = info.ChurchName, info.InviteeName, optional(info.Email), optional(info.Phone)
			out.Body.Status, out.Body.OwnerExists = string(info.Status), info.OwnerExists
			return out, nil
		})

	huma.Register(api, tagged(op("acceptInvite", http.MethodPost, "/invites/accept", TenancyOptional, http.StatusCreated, "Accept an invite as a new user"), "invites"),
		func(ctx context.Context, in *struct {
			Body struct {
				Token    string `json:"token" maxLength:"200"`
				Name     string `json:"name" maxLength:"500"`
				Email    string `json:"email,omitempty" maxLength:"300"`
				Phone    string `json:"phone,omitempty" maxLength:"300"`
				Password string `json:"password" maxLength:"1024"`
			}
		}) (*cookiesOutput, error) {
			info := RequestInfoFrom(ctx)
			b := in.Body
			res, err := d.Invites.Accept(ctx, app.AcceptInput{Token: b.Token, Name: b.Name, Email: b.Email, Phone: b.Phone,
				Password: b.Password, UserAgent: info.UserAgent, PreviousTokenHash: info.PresentedTokenHash})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("invite_accepted", "user_id", string(res.Session.UserID))
			return &cookiesOutput{SetCookie: d.Cookies.Set(res.Token, res.Session.ExpiresAt, d.Clock.Now())}, nil
		})

	huma.Register(api, tagged(op("acceptInviteExisting", http.MethodPost, "/invites/accept-existing", TenancyOptional, http.StatusCreated, "Accept an invite with the logged-in account"), "invites"),
		func(ctx context.Context, in *tokenBody) (*statusOnlyOutput, error) {
			created, err := d.Invites.AcceptExisting(ctx, sess(ctx), in.Body.Token)
			if err != nil {
				return nil, fail(ctx, err)
			}
			status := http.StatusOK // already a member
			if created {
				status = http.StatusCreated
			}
			d.Log.Info("invite_accepted", "user_id", string(sess(ctx).UserID))
			return &statusOnlyOutput{Status: status}, nil
		})

	// --- password reset (platform) ---

	huma.Register(api, tagged(op("inspectReset", http.MethodPost, "/auth/reset/inspect", TenancyPlatform, http.StatusOK, "Describe a reset link"), "auth"),
		func(ctx context.Context, in *tokenBody) (*resetInfoOutput, error) {
			info, err := d.Resets.Inspect(ctx, in.Body.Token)
			if err != nil {
				return nil, fail(ctx, err)
			}
			out := &resetInfoOutput{}
			out.Body.UserName, out.Body.CreatedByName, out.Body.ExpiresAt = info.UserName, info.CreatedByName, info.ExpiresAt
			return out, nil
		})

	huma.Register(api, tagged(op("resetPassword", http.MethodPost, "/auth/reset", TenancyPlatform, http.StatusNoContent, "Set a new password with a reset link"), "auth"),
		func(ctx context.Context, in *struct {
			Body struct {
				Token       string `json:"token" maxLength:"200"`
				NewPassword string `json:"new_password" maxLength:"1024"`
			}
		}) (*cookiesOutput, error) {
			info := RequestInfoFrom(ctx)
			res, err := d.Resets.Use(ctx, app.ResetInput{Token: in.Body.Token, NewPassword: in.Body.NewPassword,
				UserAgent: info.UserAgent, PreviousTokenHash: info.PresentedTokenHash})
			if err != nil {
				return nil, fail(ctx, err)
			}
			d.Log.Info("reset_link_used", "user_id", string(res.UserID))
			return &cookiesOutput{SetCookie: d.Cookies.Set(res.Token, res.Session.ExpiresAt, d.Clock.Now())}, nil
		})
}

func toScopes(in []string) []domain.Scope {
	out := make([]domain.Scope, len(in))
	for i, s := range in {
		out[i] = domain.Scope(s)
	}
	return out
}
