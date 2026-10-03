// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package app holds use cases and the ports they depend on (02 §2).
package app

import (
	"context"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Tx runs fn inside one database transaction. fn may run up to 3 times
// (retries), so it must have no side effects outside the transaction, and only
// values from the committed attempt may be used afterwards.
type Tx interface {
	Read(ctx context.Context, fn func(s Store) error) error
	Write(ctx context.Context, fn func(s Store) error) error
}

// Store gives access to repositories inside a transaction. Repositories are
// added by the slices that need them.
type Store interface {
	Users() UserRepo
	Sessions() SessionRepo
	AuthThrottle() ThrottleRepo
	Churches() ChurchRepo
	Translations() TranslationRepo
	SetupTokens() SetupTokenRepo
	PasswordResets() PasswordResetRepo
	InviteTokens() InviteTokenRepo // platform lookups by token; the church comes from the invite

	LockInstall(ctx context.Context) error                // setup, setup-link issuing (02 §2.1)
	LockUser(ctx context.Context, id domain.UserID) error // reset-link creation
	ForChurch(ctx context.Context, id domain.ChurchID) (ChurchStore, error)
}

// ChurchStore gives access to church-scoped repositories. Every query they
// run filters by the church; IT-P-007 checks each method.
type ChurchStore interface {
	ChurchID() domain.ChurchID
	LockChurch(ctx context.Context) error // first statement of every check-then-write rule
	Church() ChurchSettingsRepo
	Memberships() MembershipRepo
	Roles() RoleRepo
	Invites() InviteRepo
	Songs() SongRepo       // songs, sections, arrangements, groups and search (06)
	Readings() ReadingRepo // saved Bible readings (07)
}

// Clock returns the current time in UTC, truncated to microseconds (02 §4).
type Clock interface{ Now() time.Time }

// IDGenerator returns new ULIDs.
type IDGenerator interface{ NewID() string }

// PasswordHasher hashes and verifies passwords. Implementations normalise with
// domain.NormalizePassword, bound concurrency, and must never be called inside
// a database transaction (03 §3).
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Verify reports whether password matches encoded, and whether encoded
	// uses outdated parameters and should be replaced after a successful login.
	Verify(ctx context.Context, encoded, password string) (ok, needsRehash bool, err error)
	// VerifyDummy costs the same as Verify; used when no user exists, so
	// timing doesn't reveal whether an account exists.
	VerifyDummy(ctx context.Context, password string) error
}

// UserRepo stores platform-wide users.
type UserRepo interface {
	ByIdentifier(ctx context.Context, id domain.Identifier) (domain.User, error) // ErrNotFound
	ByID(ctx context.Context, id domain.UserID) (domain.User, error)             // ErrNotFound
	Create(ctx context.Context, u domain.User) error                             // UniqueError users_email_key / users_phone_key
	SetPasswordHash(ctx context.Context, id domain.UserID, hash string, now time.Time) error
	UpdateProfile(ctx context.Context, id domain.UserID, name string, prefs domain.Preferences, now time.Time) error
	Touch(ctx context.Context, id domain.UserID, now time.Time) error // last_seen_at
	MembershipChurchIDs(ctx context.Context, id domain.UserID) ([]domain.ChurchID, error)
	List(ctx context.Context) ([]domain.User, error) // operator CLI, ordered by name
}

// SessionRepo stores login sessions.
type SessionRepo interface {
	Create(ctx context.Context, s domain.Session) error
	ByTokenHash(ctx context.Context, hash string) (domain.Session, error) // ErrNotFound
	// Extend is the conditional update of 03 §4: false when the session was
	// deleted or had expired meanwhile.
	Extend(ctx context.Context, hash string, now, expires time.Time) (bool, error)
	Delete(ctx context.Context, hash string) error
	DeleteOthers(ctx context.Context, user domain.UserID, keepHash string) error
	DeleteAllForUser(ctx context.Context, user domain.UserID) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

// ThrottleRepo stores login-throttle counters (03 §5).
type ThrottleRepo interface {
	// LockedUntil returns the latest lock among keys still active at now (zero if none).
	LockedUntil(ctx context.Context, keys []string, now time.Time) (time.Time, error)
	// RecordFailure atomically adds one failure (02 §2.1).
	RecordFailure(ctx context.Context, key string, rule domain.ThrottleRule, now time.Time) error
	Delete(ctx context.Context, keys ...string) error
	DeleteAll(ctx context.Context) error
	// DeleteForIdentifier removes the id and idip counters of an identifier
	// (normalised, as ThrottleKeys takes it), from every address.
	DeleteForIdentifier(ctx context.Context, identifier string) error
	// DeleteForAddr removes the ip and idip counters of a client address key.
	DeleteForAddr(ctx context.Context, addrKey string) error
	// DeleteEnded removes counters of kind whose window started before
	// windowStart and whose lock (if any) has ended (03 §11).
	DeleteEnded(ctx context.Context, kind domain.ThrottleKind, windowStart, now time.Time) error
}

// ChurchRepo is the platform view of churches (setup, resolver, invites).
type ChurchRepo interface {
	Create(ctx context.Context, c domain.Church) error
	Count(ctx context.Context) (int, error)
	IDs(ctx context.Context, limit int) ([]domain.ChurchID, error) // ordered by ID
	ByID(ctx context.Context, id domain.ChurchID) (domain.Church, error)
}

// TranslationRepo reads the seeded Bible translations.
type TranslationRepo interface {
	List(ctx context.Context) ([]domain.Translation, error) // ordered by code
	ByCode(ctx context.Context, code string) (domain.Translation, error)
	ByID(ctx context.Context, id domain.TranslationID) (domain.Translation, error)
}

// SetupTokenRepo stores the singleton setup token (03 §10).
type SetupTokenRepo interface {
	Put(ctx context.Context, hash string, created, expires time.Time) error // upsert of row 1
	// Claim deletes the token if hash matches and it hasn't expired (atomic claim).
	Claim(ctx context.Context, hash string, now time.Time) (bool, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}

// PasswordResetRepo stores reset links (03 §9).
type PasswordResetRepo interface {
	Create(ctx context.Context, r domain.PasswordReset) error
	// CloseOpen marks every unused link of the user as used (expired ones included).
	CloseOpen(ctx context.Context, user domain.UserID, now time.Time) error
	ByTokenHash(ctx context.Context, hash string) (domain.PasswordReset, error) // ErrNotFound
	// Claim marks an unused, unexpired link used (atomic claim); false if none.
	Claim(ctx context.Context, hash string, now time.Time) (domain.PasswordReset, bool, error)
	Latest(ctx context.Context, users []domain.UserID) (map[domain.UserID]domain.PasswordReset, error)
	DeleteOld(ctx context.Context, cutoff time.Time) error // expired or used before cutoff
}

// InviteTokenRepo finds and claims invites by token, across churches.
type InviteTokenRepo interface {
	ByTokenHash(ctx context.Context, hash string) (domain.Invite, error) // ErrNotFound; RoleIDs not loaded
	// Claim marks a pending invite accepted by user ("" = set later with
	// SetAcceptedUser) as one atomic claim; false if no pending invite matched.
	Claim(ctx context.Context, hash string, user domain.UserID, now time.Time) (domain.Invite, bool, error)
	SetAcceptedUser(ctx context.Context, id domain.InviteID, user domain.UserID) error
}

// ChurchSettingsRepo reads and writes the scoped church row.
type ChurchSettingsRepo interface {
	Get(ctx context.Context) (domain.Church, error)
	Update(ctx context.Context, c domain.Church) error // name, languages, translation, zone, settings, updated_at
}

// MembershipRepo stores the church's memberships and their roles.
type MembershipRepo interface {
	List(ctx context.Context) ([]domain.Member, error) // with users and role IDs, ordered by name
	ByID(ctx context.Context, id domain.MembershipID) (domain.Membership, error)
	ByUser(ctx context.Context, user domain.UserID) (domain.Membership, error)
	Create(ctx context.Context, m domain.Membership) error // with its roles
	SetRoles(ctx context.Context, id domain.MembershipID, roles []domain.RoleID) error
	Delete(ctx context.Context, id domain.MembershipID) error
	Count(ctx context.Context) (int, error)
}

// RoleRepo stores the church's roles. Unknown stored scopes are dropped on load.
type RoleRepo interface {
	List(ctx context.Context) ([]domain.Role, error) // ordered by name
	ByID(ctx context.Context, id domain.RoleID) (domain.Role, error)
	ByOrigin(ctx context.Context, o domain.RoleOrigin) (domain.Role, error)
	Create(ctx context.Context, r domain.Role) error // UniqueError roles_church_name_key
	Update(ctx context.Context, r domain.Role) error // name, description, scopes, updated_at
	Delete(ctx context.Context, id domain.RoleID) error
	MemberCounts(ctx context.Context) (map[domain.RoleID]int, error)
}

// InviteRepo stores the church's invites and their roles.
type InviteRepo interface {
	List(ctx context.Context) ([]domain.Invite, error) // not accepted or cancelled, newest first
	ByID(ctx context.Context, id domain.InviteID) (domain.Invite, error)
	Create(ctx context.Context, inv domain.Invite) error // with its roles; UniqueError on the open-identifier indexes
	// CancelExpired cancels expired open invites for the email or phone (03 §7 step 3).
	CancelExpired(ctx context.Context, email, phone string, now time.Time) error
	CountPending(ctx context.Context, now time.Time) (int, error)
	// Renew gives an open invite a new token and expiry; false if not open.
	Renew(ctx context.Context, id domain.InviteID, hash string, expires time.Time) (bool, error)
	// Cancel cancels an open invite; false if not open.
	Cancel(ctx context.Context, id domain.InviteID, now time.Time) (bool, error)
}

// SongRef names a song in lists of linked versions.
type SongRef struct {
	ID       domain.SongID
	Title    string
	Language string
}

// SongRow is one line of a song list or search result (no lyrics).
type SongRow struct {
	ID            domain.SongID
	GroupID       domain.SongGroupID
	Title         string
	AltTitles     []string
	Language      string
	HymnalSource  string
	HymnalNumber  string
	LicenceStatus domain.LicenceStatus
}

// SongSearch is a prepared search (06 §5.2): the use case has folded the
// query and parsed the hymnal reference. Terms are folded and ANDed; no
// terms lists every song.
type SongSearch struct {
	Terms           []string
	HymnalQueryKey  string // exact hymnal key to list first, "" if the query is not a hymnal reference
	Language        string // filters; "" = any
	LicenceStatus   domain.LicenceStatus
	HymnalSourceKey string
	HymnalKey       string // exact key filter (source and number)
	Limit, Offset   int
}

// SongPage is one page of search results.
type SongPage struct {
	Items []SongRow
	Total int
}

// SongRepo stores the church's songs with their sections, default
// arrangements, language groups and search index (06). Every write that
// changes a song also rewrites its index rows, in the same transaction.
type SongRepo interface {
	ByID(ctx context.Context, id domain.SongID) (domain.Song, error) // with sections and arrangement; ErrNotFound
	Create(ctx context.Context, s domain.Song) error                 // song, sections, arrangement entries, index
	// Update replaces the song row, its sections, arrangement and index rows if
	// the stored version is expectedVersion (atomic conditional update, 02 §2.1);
	// false when the song is missing or its version differs.
	// UniqueError songs_group_language_key when the language is taken in its group.
	Update(ctx context.Context, s domain.Song, expectedVersion int) (bool, error)
	Delete(ctx context.Context, id domain.SongID) error
	Search(ctx context.Context, q SongSearch) (SongPage, error)
	CreateGroup(ctx context.Context, id domain.SongGroupID, now time.Time) error
	DeleteGroup(ctx context.Context, id domain.SongGroupID) error
	// SetGroup puts the songs into the group ("" = no group) and adds one to
	// each version. UniqueError songs_group_language_key.
	SetGroup(ctx context.Context, songs []domain.SongID, group domain.SongGroupID, now time.Time) error
	GroupMembers(ctx context.Context, group domain.SongGroupID) ([]SongRef, error) // ordered by language
	Reindex(ctx context.Context) error                                             // rebuilds this church's index rows
}

// SongUsage tells whether unpublished liturgies use a song or its sections
// (06 §6). Step 2 has no liturgies: NeverUsed answers "unused"; step 3
// replaces it with a query.
type SongUsage interface {
	SongInUse(ctx context.Context, church domain.ChurchID, song domain.SongID) (bool, error)
	SectionsInUse(ctx context.Context, church domain.ChurchID, song domain.SongID, sections []domain.SectionID) ([]domain.SectionID, error)
}

// NeverUsed is the step-2 usage check: nothing is ever in use.
type NeverUsed struct{}

// SongInUse implements SongUsage.
func (NeverUsed) SongInUse(context.Context, domain.ChurchID, domain.SongID) (bool, error) {
	return false, nil
}

// SectionsInUse implements SongUsage.
func (NeverUsed) SectionsInUse(context.Context, domain.ChurchID, domain.SongID, []domain.SectionID) ([]domain.SectionID, error) {
	return nil, nil
}

// ReadingRepo stores the church's readings (07 §6). Unique on (reference,
// translation): a second Create returns a UniqueError named readings_church_ref_key.
type ReadingRepo interface {
	ByID(ctx context.Context, id domain.ReadingID) (domain.Reading, error)
	ByReference(ctx context.Context, reference string, translation domain.TranslationID) (domain.Reading, error)
	Create(ctx context.Context, r domain.Reading) error
	// Update writes text, attribution, display and version when the stored
	// version is expectedVersion; false means it was not.
	Update(ctx context.Context, r domain.Reading, expectedVersion int) (bool, error)
	Delete(ctx context.Context, id domain.ReadingID) error
	// List returns every reading that matches, in no particular order; the use
	// case orders them (readings are few).
	List(ctx context.Context, q ReadingSearch) ([]ReadingRow, error)
	// LatestAttribution is the attribution of the most recently updated reading
	// in a translation, or "" when there is none or it has none.
	LatestAttribution(ctx context.Context, translation domain.TranslationID) (string, error)
}

// ReadingSearch filters a list. Fold and FoldZh are the folded query for
// Latin and Chinese readings; both empty lists everything.
type ReadingSearch struct {
	Fold, FoldZh string
	Translation  string // translation code, or ""
}

// ReadingRow is a reading in a list; TextStart is the first characters of the text.
type ReadingRow struct {
	ID               domain.ReadingID
	Reference        string
	ReferenceDisplay string
	Translation      domain.Translation
	TextStart        string
}

// ReadingUsage tells whether unpublished liturgies use a reading (07 §6).
// Step 2 has no liturgies: NeverUsed answers "unused".
type ReadingUsage interface {
	ReadingInUse(ctx context.Context, church domain.ChurchID, reading domain.ReadingID) (bool, error)
}

// ReadingInUse implements ReadingUsage.
func (NeverUsed) ReadingInUse(context.Context, domain.ChurchID, domain.ReadingID) (bool, error) {
	return false, nil
}
