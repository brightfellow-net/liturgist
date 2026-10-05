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
	Songs() SongRepo            // songs, sections, arrangements, groups and search (06)
	Readings() ReadingRepo      // saved Bible readings (07)
	Imports() ImportRepo        // import batches and their candidates (08)
	Duties() NameListRepo       // the church's duties (09 §2.1)
	SingingParts() NameListRepo // the church's singing parts (09 §2.1)
	Templates() TemplateRepo    // liturgy templates (09 §2.2)
	Services() ServiceRepo      // regular services (09 §2.4)
	Seeds() SeedRepo            // the markers of seeded defaults (09 §3)
	Liturgies() LiturgyRepo     // liturgies (10 §2.1)
	LiturgyItems() ItemRepo     // items, item songs and sequences (10 §2.2, §2.3)
	Assignments() AssignmentRepo
	Edits() EditRepo               // the history of a liturgy (10 §7)
	Usage() UsageRepo              // what unpublished liturgies refer to (10 §6)
	StateChanges() StateChangeRepo // the review history of a liturgy (12 §5)
	Comments() CommentRepo         // comments on liturgies and items (12 §4)
	Published() PublishedRepo      // published versions (13 §3)
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
	// FindDuplicate returns the oldest song with the hymnal key (when not
	// empty), else the oldest with the folded title in the language (08 §3).
	FindDuplicate(ctx context.Context, hymnalKey, titleKey, language string) (DuplicateRef, bool, error)
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

// DuplicateRef is an existing song a candidate may duplicate.
type DuplicateRef struct {
	ID                         domain.SongID
	Title                      string
	HymnalSource, HymnalNumber string
}

// ImportRepo stores batches and candidates (08 §7). The use case holds the
// church lock for every change, so the methods are plain reads and writes.
type ImportRepo interface {
	CreateBatch(ctx context.Context, b domain.ImportBatch, candidates []domain.ImportCandidate) error
	Batch(ctx context.Context, id domain.ImportBatchID) (domain.ImportBatch, error) // ErrNotFound
	// OpenBatches lists the batches with status open, newest first.
	OpenBatches(ctx context.Context) ([]domain.ImportBatch, error)
	Candidates(ctx context.Context, batch domain.ImportBatchID) ([]domain.ImportCandidate, error) // by position
	Candidate(ctx context.Context, batch domain.ImportBatchID, id domain.ImportCandidateID) (domain.ImportCandidate, error)
	// UpdateCandidate writes every mutable field of the candidate.
	UpdateCandidate(ctx context.Context, c domain.ImportCandidate) error
	// SetBatch sets status and updated_at.
	SetBatch(ctx context.Context, id domain.ImportBatchID, status domain.ImportStatus, now time.Time) error
	// Unfinished counts the candidates that are not terminal.
	Unfinished(ctx context.Context, batch domain.ImportBatchID) (int, error)
	DeleteBatch(ctx context.Context, id domain.ImportBatchID) error // with its candidates
	// DeleteOlderThan deletes this church's batches not changed since cutoff.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) error
}

// NameListRepo stores one of the two ordered name lists of a church, duties or
// singing parts (09 §2.1). Names are unique by folded key.
type NameListRepo interface {
	List(ctx context.Context) ([]domain.NameEntry, error)          // by position
	ByID(ctx context.Context, id string) (domain.NameEntry, error) // ErrNotFound
	Count(ctx context.Context) (int, error)
	// Create adds the entry; UniqueError duties_church_name_key / singing_parts_church_name_key.
	Create(ctx context.Context, e domain.NameEntry) error
	// Rename changes name and key; UniqueError as for Create; ErrNotFound.
	Rename(ctx context.Context, id, name, nameKey string) error
	// SetOrder gives the entries the positions 0..n-1 in the order of ids.
	SetOrder(ctx context.Context, ids []string) error
	// Delete removes the entry. For duties it first clears the default duty of
	// template items that name it, in the same transaction (the foreign key
	// stays RESTRICT, schema "Clearing references").
	Delete(ctx context.Context, id string) error
}

// TemplateRow is a template in a list.
type TemplateRow struct {
	ID        domain.TemplateID
	Name      string
	Language  string
	ItemCount int
	Version   int
}

// TemplateRepo stores templates with their items (09 §2.2).
type TemplateRepo interface {
	List(ctx context.Context) ([]TemplateRow, error)                         // by name key
	ByID(ctx context.Context, id domain.TemplateID) (domain.Template, error) // with items; ErrNotFound
	Count(ctx context.Context) (int, error)
	// Create stores the template and its items; UniqueError templates_church_name_key.
	Create(ctx context.Context, t domain.Template) error
	// Update replaces the row and the complete item list if the stored version
	// is expectedVersion (atomic conditional update, 02 §2.1); false when the
	// template is missing or the version differs. UniqueError as for Create.
	Update(ctx context.Context, t domain.Template, expectedVersion int) (bool, error)
	Delete(ctx context.Context, id domain.TemplateID) error
	// ServicesUsing lists the services that have the template as default.
	ServicesUsing(ctx context.Context, id domain.TemplateID) ([]domain.ServiceID, error)
}

// ServiceRepo stores services with their weekly times (09 §2.4).
type ServiceRepo interface {
	List(ctx context.Context) ([]domain.Service, error) // with times, by name key
	ByID(ctx context.Context, id domain.ServiceID) (domain.Service, error)
	Count(ctx context.Context) (int, error)
	// Create stores the service and its times; UniqueError services_church_name_key.
	Create(ctx context.Context, s domain.Service) error
	// Update is the conditional update of TemplateRepo.Update.
	Update(ctx context.Context, s domain.Service, expectedVersion int) (bool, error)
	Delete(ctx context.Context, id domain.ServiceID) error
}

// SeedRepo reads and writes the markers of seeded defaults (09 §3). Only
// app.Seed and Setup use it, inside the transaction that creates the rows.
type SeedRepo interface {
	Applied(ctx context.Context, key string) (bool, error)
	Mark(ctx context.Context, key string, at time.Time) error
}

// LiturgyFilter selects the liturgies of a list (10 §4). Empty fields do not filter.
type LiturgyFilter struct {
	State         domain.LiturgyState
	Archived      ArchivedFilter // zero value: not archived only (13 §5)
	From, To      string         // inclusive dates
	Ascending     bool           // by date, then time, then ID
	Limit, Offset int
}

// ArchivedFilter selects archived liturgies in a list.
type ArchivedFilter string

// The filters: the zero value shows liturgies that are not archived.
const (
	ArchivedExclude ArchivedFilter = ""
	ArchivedOnly    ArchivedFilter = "true"
	ArchivedAll     ArchivedFilter = "all"
)

// PublishedRepo stores the published versions of liturgies (13 §3). Every
// method filters by church; rows are never updated.
type PublishedRepo interface {
	// Create stores the next version of the liturgy (v.Number is ignored) and
	// the user IDs of its assignments, and returns the number it took:
	// MAX(number)+1, safe under the liturgy's row lock held by the caller.
	Create(ctx context.Context, v domain.PublishedVersion, assignees []domain.UserID) (int, error)
	// Info returns the newest version without its content; ErrNotFound when
	// the liturgy has none.
	Info(ctx context.Context, liturgy domain.LiturgyID) (domain.PublishedVersion, error)
	// Exists reports whether the liturgy has any version.
	Exists(ctx context.Context, liturgy domain.LiturgyID) (bool, error)
}

// LiturgyRow is a liturgy in a list.
type LiturgyRow struct {
	Liturgy     domain.Liturgy
	ItemCount   int
	HasVersions bool // a published version exists (13 §2)
}

// Slot is the (service, date, time) a service liturgy occupies (10 §2.1).
type Slot struct {
	ServiceID  domain.ServiceID
	Date, Time string
}

// LiturgyRepo stores liturgies (10 §2.1).
type LiturgyRepo interface {
	// Create stores the liturgy; UniqueError liturgies_service_slot_key.
	Create(ctx context.Context, l domain.Liturgy) error
	ByID(ctx context.Context, id domain.LiturgyID) (domain.Liturgy, error) // ErrNotFound
	List(ctx context.Context, f LiturgyFilter) ([]LiturgyRow, int, error)  // page and total
	// CountActive counts the liturgies that are not archived and are in one of
	// states (any state when states is empty).
	CountActive(ctx context.Context, states []domain.LiturgyState) (int, error)
	// Slots maps the slots of service liturgies with a date in from..to to the liturgy.
	Slots(ctx context.Context, from, to string) (map[Slot]domain.LiturgyID, error)
	// Update writes date, time and service name and sets the version to
	// expectedVersion+1 when the stored version is expectedVersion (atomic
	// conditional update, 02 §2.1); false when it is not or the liturgy is missing.
	// UniqueError liturgies_service_slot_key.
	Update(ctx context.Context, l domain.Liturgy, expectedVersion int) (bool, error)
	// Bump adds one to the version under the same condition.
	Bump(ctx context.Context, id domain.LiturgyID, expectedVersion int, now time.Time) (bool, error)
	// NextSeq takes the next number of the history (UPDATE … RETURNING), which
	// holds the liturgy's row lock until the transaction ends (10 §5). It
	// matches only an editable liturgy (12 §2, P-71): ErrNoSeq when the liturgy
	// is gone or locked.
	NextSeq(ctx context.Context, id domain.LiturgyID) (int, error)
	// Transition moves the liturgy from one state to another when it is in from
	// and its edit_seq is expectSeq, and sets undo_floor_seq to edit_seq (12 §2).
	// It returns the edit_seq; false when no row matched (gone, another state
	// or changed since).
	// LockForComment takes the liturgy's row lock with a no-op update that
	// matches only a state that allows comments, so a comment waits for a
	// transition and sees its result (12 §4, P-74). False when the liturgy is
	// gone or in another state.
	LockForComment(ctx context.Context, id domain.LiturgyID) (bool, error)
	// It also requires the liturgy not to be archived (13 §2).
	Transition(ctx context.Context, id domain.LiturgyID, from, to domain.LiturgyState, expectSeq int, now time.Time) (int, bool, error)
	// Archive sets archived_at and archived_by on a published, not archived
	// liturgy; false when no row matched (13 §4).
	Archive(ctx context.Context, id domain.LiturgyID, by domain.UserID, now time.Time) (bool, error)
	// Unarchive clears them; false when the liturgy is gone or not archived.
	Unarchive(ctx context.Context, id domain.LiturgyID, now time.Time) (bool, error)
	// Delete removes the liturgy with its items, songs, entries, assignments and
	// history, unless it is published or has a published version: ErrNotFound
	// when no row matched (gone, or not deletable: the use case re-reads), and
	// ErrReferenced when a version appeared after the check (13 §2).
	Delete(ctx context.Context, id domain.LiturgyID) error
}

// StateChangeRepo stores the review history of liturgies (12 §5).
type StateChangeRepo interface {
	Append(ctx context.Context, c domain.StateChange) error
	// List returns the newest rows first, and the total number of rows.
	List(ctx context.Context, liturgy domain.LiturgyID, limit, offset int) ([]domain.StateChange, int, error)
	// Last returns the newest row; ErrNotFound when there is none.
	Last(ctx context.Context, liturgy domain.LiturgyID) (domain.StateChange, error)
}

// CommentRepo stores comments (12 §4). Every method filters by church and liturgy.
type CommentRepo interface {
	Create(ctx context.Context, c domain.Comment) error
	// List returns the comments oldest first; resolved filters when not nil.
	List(ctx context.Context, liturgy domain.LiturgyID, resolved *bool) ([]domain.Comment, error)
	Count(ctx context.Context, liturgy domain.LiturgyID) (int, error)
	Open(ctx context.Context, liturgy domain.LiturgyID) (int, error) // unresolved
	ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.CommentID) (domain.Comment, error)
	// SetResolved resolves (by, now) or reopens the comment; false when it was
	// already in that state or does not exist.
	SetResolved(ctx context.Context, liturgy domain.LiturgyID, id domain.CommentID, resolved bool, by domain.UserID, now time.Time) (bool, error)
}

// ItemRepo stores the items of liturgies with their songs and sequences (10 §2.2, §2.3).
type ItemRepo interface {
	// ByLiturgy returns the items by position, complete with songs and entries.
	ByLiturgy(ctx context.Context, liturgy domain.LiturgyID) ([]domain.Item, error)
	// ByID returns one complete item of the liturgy; ErrNotFound.
	ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) (domain.Item, error)
	IDs(ctx context.Context, liturgy domain.LiturgyID) ([]domain.ItemID, error) // by position
	// Insert stores the item with its songs and entries.
	Insert(ctx context.Context, it domain.Item) error
	// SetPositions gives the items the positions 0..n-1 in the order of ids.
	SetPositions(ctx context.Context, liturgy domain.LiturgyID, ids []domain.ItemID) error
	// Delete removes the item with its songs and entries.
	Delete(ctx context.Context, liturgy domain.LiturgyID, id domain.ItemID) error
	// Update writes title, duty, text and reading, and sets the version to
	// expectedVersion+1 when the stored version is expectedVersion; false otherwise.
	Update(ctx context.Context, it domain.Item, expectedVersion int) (bool, error)
	// Bump adds one to the item's version under the same condition.
	Bump(ctx context.Context, id domain.ItemID, expectedVersion int, now time.Time) (bool, error)
	// InsertSong adds one song with its entries to the item.
	InsertSong(ctx context.Context, item domain.ItemID, s domain.LiturgySong) error
	// SetSongPositions gives the item's songs the positions 0..n-1 in the order of ids.
	SetSongPositions(ctx context.Context, item domain.ItemID, ids []domain.ItemSongID) error
	// ReplaceSongs deletes every song and entry of the item and stores songs.
	ReplaceSongs(ctx context.Context, item domain.ItemID, songs []domain.LiturgySong) error
}

// AssignmentRepo stores assignments (10 §2.4).
type AssignmentRepo interface {
	// Add stores the assignment; UniqueError assignments_user_key / assignments_name_key.
	Add(ctx context.Context, a domain.Assignment) error
	ByID(ctx context.Context, liturgy domain.LiturgyID, id domain.AssignmentID) (domain.Assignment, error) // ErrNotFound
	ByLiturgy(ctx context.Context, liturgy domain.LiturgyID) ([]domain.Assignment, error)                  // by creation
	Count(ctx context.Context, liturgy domain.LiturgyID) (int, error)
	Remove(ctx context.Context, liturgy domain.LiturgyID, id domain.AssignmentID) error // ErrNotFound
}

// EditRepo stores the history of liturgies (10 §7).
type EditRepo interface {
	Append(ctx context.Context, e domain.Edit) error
	// List returns the newest rows first, at most limit.
	List(ctx context.Context, liturgy domain.LiturgyID, limit int) ([]domain.Edit, error)
	// BySeq returns the row with the number; ErrNotFound.
	BySeq(ctx context.Context, liturgy domain.LiturgyID, seq int) (domain.Edit, error)
	// LastActing returns the newest undo or redo row that acted on the edit;
	// ErrNotFound when none did.
	LastActing(ctx context.Context, liturgy domain.LiturgyID, target domain.EditID) (domain.Edit, error)
	// Newest returns the undo target (11 §7.2): among the user's last window
	// editing rows with seq above floor, the newest that is done and not
	// skipped; ErrNotFound when there is none.
	Newest(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID, floor, window int) (domain.Edit, error)
	// NewestUndone returns the redo target: the user's undone edit above
	// floor with the highest undo_seq; ErrNotFound when there is none.
	NewestUndone(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID, floor int) (domain.Edit, error)
	// Foreign returns the rows with seq above afterSeq written by anyone but
	// user, oldest first, for the conflict test (domain.Edit.TouchedBy).
	Foreign(ctx context.Context, liturgy domain.LiturgyID, afterSeq int, user domain.UserID) ([]domain.Foreign, error)
	// SetStatus changes the status (and undo_seq, 0 = none) when the row has
	// status from; false when it has not.
	SetStatus(ctx context.Context, id domain.EditID, from, to string, undoSeq int) (bool, error)
	// MarkSkipped sets the skipped flag of a done edit.
	MarkSkipped(ctx context.Context, id domain.EditID) error
	// DropUndone turns all undone edits of the user in the liturgy into dropped.
	DropUndone(ctx context.Context, user domain.UserID, liturgy domain.LiturgyID) error
}

// UsageRepo tells what liturgies refer to. It runs in the caller's
// transaction, under LockChurch, so a delete cannot pass while a liturgy adds
// the reference (10 §6). A song, section or reading is in use when a liturgy
// that is not published refers to it.
type UsageRepo interface {
	SongInUse(ctx context.Context, song domain.SongID) (bool, error)
	// SectionsInUse returns those of sections that entries refer to.
	SectionsInUse(ctx context.Context, song domain.SongID, sections []domain.SectionID) ([]domain.SectionID, error)
	ReadingInUse(ctx context.Context, reading domain.ReadingID) (bool, error)
	DutyInUse(ctx context.Context, duty string) (bool, error)
	SingingPartInUse(ctx context.Context, part string) (bool, error)
}
