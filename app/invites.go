// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Invites holds the invite use cases (03 §7).
type Invites struct {
	Tx           Tx
	Hasher       PasswordHasher
	Clock        Clock
	IDs          IDGenerator
	URLs         URLBuilder
	Entitlements Entitlements
	Auth         *Auth
}

// InviteActions are the advisory actions on an invite.
type InviteActions struct {
	Regenerate bool `json:"regenerate"`
	Cancel     bool `json:"cancel"`
}

// InviteView is an invite as the members page shows it (never the link).
type InviteView struct {
	Invite        domain.Invite
	Status        domain.InviteStatus
	Roles         []domain.Role
	CreatedByName *string
	Actions       InviteActions
}

// InviteLink is a link shown once, after creating or regenerating.
type InviteLink struct {
	Link      string
	ExpiresAt time.Time
}

// InviteInput creates an invite.
type InviteInput struct {
	Name, Email, Phone string
	RoleIDs            []domain.RoleID
}

// InviteInfo is what POST /invites/inspect returns.
type InviteInfo struct {
	ChurchName  string
	InviteeName string
	Email       string
	Phone       string
	Status      domain.InviteStatus
	OwnerExists bool
}

// AcceptInput accepts an invite as a new user (03 §7).
type AcceptInput struct {
	Token, Name, Email, Phone, Password string
	UserAgent, PreviousTokenHash        string
}

// List returns pending and expired invites (members.manage).
func (u *Invites) List(ctx context.Context, sess *domain.Session) ([]InviteView, error) {
	var res []InviteView
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		list, err := sc.cs.Invites().List(ctx)
		if err != nil {
			return err
		}
		res = make([]InviteView, 0, len(list))
		for _, inv := range list {
			v, err := sc.inviteView(ctx, s, inv, u.Clock.Now())
			if err != nil {
				return err
			}
			res = append(res, v)
		}
		return nil
	})
	return res, err
}

func (c churchScope) inviteView(ctx context.Context, s Store, inv domain.Invite, now time.Time) (InviteView, error) {
	v := InviteView{Invite: inv, Status: inv.Status(now)}
	for _, id := range inv.RoleIDs {
		if r, ok := c.roles[id]; ok {
			v.Roles = append(v.Roles, r)
		}
	}
	if inv.CreatedBy != "" {
		cu, err := s.Users().ByID(ctx, inv.CreatedBy)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return InviteView{}, err
		}
		if err == nil {
			v.CreatedByName = &cu.Name
		}
	}
	manage := c.actor.Scopes.Has(domain.ScopeMembersManage)
	v.Actions = InviteActions{Cancel: manage,
		Regenerate: manage && len(c.actor.Scopes.Missing(c.scopesOf(inv.RoleIDs))) == 0}
	return v, nil
}

// Create invites a person (members.manage) under LockChurch: already a
// member → already_member; expired invites for the identifier are cancelled;
// an open one → invite_exists; the team-member limit applies.
func (u *Invites) Create(ctx context.Context, sess *domain.Session, in InviteInput) (InviteView, InviteLink, error) {
	inv := domain.Invite{Name: in.Name}
	if err := domain.ValidatePersonName(&inv.Name, "name"); err != nil {
		return InviteView{}, InviteLink{}, err
	}
	if in.Email == "" && in.Phone == "" {
		return InviteView{}, InviteLink{}, &domain.InvalidInputError{Field: "email", Message: "Enter an email address or a phone number."}
	}
	var err error
	if inv.Email, inv.Phone, err = parseIdentifiers(in.Email, in.Phone); err != nil {
		return InviteView{}, InviteLink{}, err
	}
	limit, err := teamLimit(ctx, u.Entitlements)
	if err != nil {
		return InviteView{}, InviteLink{}, err
	}
	token, hash := domain.NewToken()
	var view InviteView
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		roleIDs := uniqueRoles(in.RoleIDs)
		for _, r := range roleIDs {
			if _, ok := sc.roles[r]; !ok {
				return &domain.InvalidInputError{Field: "role_ids", Message: "Unknown role."}
			}
		}
		if err := sc.actor.RequireHeld(sc.scopesOf(roleIDs)); err != nil {
			return err
		}
		if err := sc.checkNotMember(ctx, s, inv.Email, inv.Phone); err != nil {
			return err
		}
		if err := sc.cs.Invites().CancelExpired(ctx, inv.Email, inv.Phone, now); err != nil {
			return err
		}
		if err := checkLimit(ctx, sc.cs, limit, now); err != nil {
			return err
		}
		i := inv
		i.ID, i.ChurchID, i.TokenHash, i.RoleIDs = domain.InviteID(u.IDs.NewID()), sc.actor.ChurchID, hash, roleIDs
		i.CreatedBy, i.CreatedAt, i.ExpiresAt = sc.actor.UserID, now, now.Add(domain.InviteLifetime)
		if err := sc.cs.Invites().Create(ctx, i); err != nil {
			return mapInviteExists(err)
		}
		view, err = sc.inviteView(ctx, s, i, now)
		return err
	})
	if err != nil {
		return InviteView{}, InviteLink{}, err
	}
	return view, u.link(ctx, "/invite", token, view.Invite.ExpiresAt), nil
}

func (u *Invites) link(ctx context.Context, route, token string, expires time.Time) InviteLink {
	return InviteLink{Link: u.URLs.AppURL(ctx, route) + "#t=" + token, ExpiresAt: expires}
}

// Regenerate gives a pending or expired invite a new token and 7-day expiry
// (members.manage, plus the scopes of its roles). An expired invite counts
// against the limit again.
func (u *Invites) Regenerate(ctx context.Context, sess *domain.Session, id domain.InviteID) (InviteLink, error) {
	limit, err := teamLimit(ctx, u.Entitlements)
	if err != nil {
		return InviteLink{}, err
	}
	token, hash := domain.NewToken()
	var expires time.Time
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		inv, err := sc.cs.Invites().ByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return notFound(ReasonMissing)
		}
		if err != nil {
			return err
		}
		st := inv.Status(now)
		if st != domain.InvitePending && st != domain.InviteExpired {
			return notFound(ReasonMissing)
		}
		if err := sc.actor.RequireHeld(sc.scopesOf(inv.RoleIDs)); err != nil {
			return err
		}
		if st == domain.InviteExpired {
			if err := checkLimit(ctx, sc.cs, limit, now); err != nil {
				return err
			}
		}
		expires = now.Add(domain.InviteLifetime)
		ok, err := sc.cs.Invites().Renew(ctx, id, hash, expires)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(ReasonMissing)
		}
		return nil
	})
	if err != nil {
		return InviteLink{}, err
	}
	return u.link(ctx, "/invite", token, expires), nil
}

// Cancel cancels a pending or expired invite (members.manage).
func (u *Invites) Cancel(ctx context.Context, sess *domain.Session, id domain.InviteID) error {
	return u.Tx.Write(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, true)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeMembersManage); err != nil {
			return err
		}
		ok, err := sc.cs.Invites().Cancel(ctx, id, u.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return notFound(ReasonMissing)
		}
		return nil
	})
}

// Inspect describes the invite behind a token (public).
func (u *Invites) Inspect(ctx context.Context, token string) (InviteInfo, error) {
	var info InviteInfo
	err := u.Tx.Read(ctx, func(s Store) error {
		inv, err := u.pending(ctx, s, token)
		if err != nil {
			return err
		}
		church, err := s.Churches().ByID(ctx, inv.ChurchID)
		if err != nil {
			return err
		}
		owners, err := inviteOwners(ctx, s, inv)
		if err != nil {
			return err
		}
		info = InviteInfo{ChurchName: church.Name, InviteeName: inv.Name, Email: inv.Email, Phone: inv.Phone,
			Status: domain.InvitePending, OwnerExists: len(owners) > 0}
		return nil
	})
	return info, err
}

// pending looks up a pending invite by token and checks it belongs to the
// request's tenant, if there is one (04 §2: optional tenancy).
func (u *Invites) pending(ctx context.Context, s Store, token string) (domain.Invite, error) {
	inv, err := s.InviteTokens().ByTokenHash(ctx, domain.HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return domain.Invite{}, &InvalidTokenError{Reason: domain.TokenUnknown}
	}
	if err != nil {
		return domain.Invite{}, err
	}
	if t, err := TenantFrom(ctx); err == nil && t.ChurchID != inv.ChurchID {
		return domain.Invite{}, notFound(ReasonMissing)
	}
	if st := inv.Status(u.Clock.Now()); st != domain.InvitePending {
		return domain.Invite{}, &InvalidTokenError{Reason: st.Reason()}
	}
	return inv, nil
}

// claim atomically marks the invite accepted by user; when nothing was
// claimed it reports why (03 §7).
func (u *Invites) claim(ctx context.Context, s Store, token string, user domain.UserID, now time.Time) (domain.Invite, error) {
	inv, ok, err := s.InviteTokens().Claim(ctx, domain.HashToken(token), user, now)
	if err != nil {
		return domain.Invite{}, err
	}
	if !ok {
		if _, err := u.pending(ctx, s, token); err != nil {
			return domain.Invite{}, err
		}
		return domain.Invite{}, &InvalidTokenError{Reason: domain.TokenUsed}
	}
	if t, err := TenantFrom(ctx); err == nil && t.ChurchID != inv.ChurchID {
		return domain.Invite{}, notFound(ReasonMissing)
	}
	return inv, nil
}

// Accept creates a new user from the invite, with the invitee's corrections
// to name and identifiers, a membership with the invite's current roles, and
// a session. The password is checked and hashed before the transaction.
func (u *Invites) Accept(ctx context.Context, in AcceptInput) (LoginResult, error) {
	name := in.Name
	if err := domain.ValidatePersonName(&name, "name"); err != nil {
		return LoginResult{}, err
	}
	if in.Email == "" && in.Phone == "" {
		return LoginResult{}, &domain.InvalidInputError{Field: "email", Message: "Enter an email address or a phone number."}
	}
	email, phone, err := parseIdentifiers(in.Email, in.Phone)
	if err != nil {
		return LoginResult{}, err
	}
	var churchName string
	err = u.Tx.Read(ctx, func(s Store) error {
		inv, err := u.pending(ctx, s, in.Token)
		if err != nil {
			return err
		}
		c, err := s.Churches().ByID(ctx, inv.ChurchID)
		churchName = c.Name
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	if err := domain.CheckPassword(in.Password, domain.PasswordIdentity{Name: name, Email: email, Phone: phone, ChurchName: churchName}); err != nil {
		return LoginResult{}, err
	}
	pwHash, err := u.Hasher.Hash(ctx, in.Password)
	if err != nil {
		return LoginResult{}, err
	}
	var res LoginResult
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		userID := domain.UserID(u.IDs.NewID())
		inv, err := u.claim(ctx, s, in.Token, "", now) // the user doesn't exist yet (foreign key)
		if err != nil {
			return err
		}
		owners, err := inviteOwners(ctx, s, inv)
		if err != nil {
			return err
		}
		if len(owners) > 0 {
			return ErrIdentifierTaken
		}
		usr := domain.User{ID: userID, Name: name, Email: email, Phone: phone, PasswordHash: pwHash,
			CreatedAt: now, UpdatedAt: now, LastSeenAt: now}
		if err := s.Users().Create(ctx, usr); err != nil {
			return mapIdentifierTaken(err)
		}
		if err := u.addMember(ctx, s, inv, userID, now); err != nil {
			return err
		}
		if err := s.InviteTokens().SetAcceptedUser(ctx, inv.ID, userID); err != nil {
			return err
		}
		res.Token, res.Session, err = u.Auth.newSession(ctx, s, userID, in.UserAgent, in.PreviousTokenHash, now)
		return err
	})
	return res, err
}

// AcceptExisting joins the logged-in user to the invite's church. If the
// invite's email or phone belongs to another account → invite_identifier_mismatch.
// Already a member → the invite is marked accepted and created is false.
func (u *Invites) AcceptExisting(ctx context.Context, sess *domain.Session, token string) (created bool, err error) {
	if sess == nil {
		return false, ErrUnauthenticated
	}
	err = u.Tx.Write(ctx, func(s Store) error {
		now := u.Clock.Now()
		inv, err := u.pending(ctx, s, token)
		if err != nil {
			return err
		}
		owners, err := inviteOwners(ctx, s, inv)
		if err != nil {
			return err
		}
		if len(owners) > 0 && !owners[sess.UserID] {
			return ErrInviteMismatch
		}
		if inv, err = u.claim(ctx, s, token, sess.UserID, now); err != nil {
			return err
		}
		cs, err := s.ForChurch(ctx, inv.ChurchID)
		if err != nil {
			return err
		}
		_, err = cs.Memberships().ByUser(ctx, sess.UserID)
		if err == nil {
			created = false
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		created = true
		return u.addMember(ctx, s, inv, sess.UserID, now)
	})
	return created, err
}

// addMember creates the membership with the invite's roles as they are now
// (live roles); roles deleted meanwhile are already gone from invite_roles.
func (u *Invites) addMember(ctx context.Context, s Store, inv domain.Invite, user domain.UserID, now time.Time) error {
	cs, err := s.ForChurch(ctx, inv.ChurchID)
	if err != nil {
		return err
	}
	full, err := cs.Invites().ByID(ctx, inv.ID)
	if err != nil {
		return err
	}
	return cs.Memberships().Create(ctx, domain.Membership{
		ID: domain.MembershipID(u.IDs.NewID()), UserID: user, RoleIDs: full.RoleIDs, CreatedAt: now})
}

// inviteOwners returns the users who own the invite's email or phone.
func inviteOwners(ctx context.Context, s Store, inv domain.Invite) (map[domain.UserID]bool, error) {
	owners := map[domain.UserID]bool{}
	for _, id := range identifiersOf(inv.Email, inv.Phone) {
		usr, err := s.Users().ByIdentifier(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		owners[usr.ID] = true
	}
	return owners, nil
}

// checkNotMember returns already_member if a member has the email or phone.
func (c churchScope) checkNotMember(ctx context.Context, s Store, email, phone string) error {
	owners, err := inviteOwners(ctx, s, domain.Invite{Email: email, Phone: phone})
	if err != nil {
		return err
	}
	for user := range owners {
		_, err := c.cs.Memberships().ByUser(ctx, user)
		if err == nil {
			return ErrAlreadyMember
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

func checkLimit(ctx context.Context, cs ChurchStore, limit Limit, now time.Time) error {
	if limit.Unlimited {
		return nil
	}
	used, err := teamUsage(ctx, cs, now)
	if err != nil {
		return err
	}
	if used >= limit.Max {
		return &LimitReachedError{Limit: LimitMaxTeamMembers, Used: used, Max: limit.Max}
	}
	return nil
}

func identifiersOf(email, phone string) []domain.Identifier {
	var ids []domain.Identifier
	if email != "" {
		ids = append(ids, domain.Identifier{Kind: domain.Email, Value: email})
	}
	if phone != "" {
		ids = append(ids, domain.Identifier{Kind: domain.Phone, Value: phone})
	}
	return ids
}

// parseIdentifiers normalises an optional email and an optional phone; each
// must parse as its own kind.
func parseIdentifiers(email, phone string) (string, string, error) {
	var e, p string
	if email != "" {
		id, err := domain.ParseIdentifier(email)
		if err != nil || id.Kind != domain.Email {
			return "", "", domain.ErrInvalidIdentifier
		}
		e = id.Value
	}
	if phone != "" {
		id, err := domain.ParseIdentifier(phone)
		if err != nil || id.Kind != domain.Phone {
			return "", "", domain.ErrInvalidIdentifier
		}
		p = id.Value
	}
	return e, p, nil
}

func mapInviteExists(err error) error {
	var u *UniqueError
	if errors.As(err, &u) && (u.Constraint == "invites_church_email_open_key" || u.Constraint == "invites_church_phone_open_key") {
		return ErrInviteExists
	}
	return err
}
