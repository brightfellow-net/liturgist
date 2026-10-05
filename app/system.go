// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/brightfellow-net/liturgist/domain"
)

// Errors of the system page (14 §5).
var (
	ErrBackupRunning = errors.New("a backup is already running")              // 409 backup_running
	ErrNotSupported  = errors.New("this is not supported with this database") // 400 not_supported
)

// Backup warnings of SystemFacts.
const (
	BackupWarningNone      = ""
	BackupWarningStale     = "stale"      // no backup in the last 48 hours
	BackupWarningNotCopied = "not_copied" // none downloaded or written elsewhere in the last 30 days
)

// SystemFacts is what the church admin's system page shows (14 §5).
type SystemFacts struct {
	Version, Commit string
	Database        struct {
		Driver        string
		SizeBytes     int64
		SchemaVersion int64
	}
	Disk struct {
		FreeBytes, TotalBytes int64
		Low                   bool
	}
	Backup struct {
		Supported    bool       // SQLite
		Scheduled    bool       // automatic backups are on
		LastAt       *time.Time // the newest automatic or manual backup file
		LastKind     string     // "auto" | "manual"
		LastCopiedAt *time.Time // the last download, or backup written outside the data folder
		Warning      string     // BackupWarning*
	}
	EmailConfigured bool
	HTTPS           struct {
		Mode                string // "plain_http" | "behind_proxy"
		PlainHTTPWarning    bool
		ProxyMissingWarning bool
	}
	Update struct {
		Enabled   bool
		Latest    string
		Available bool
	}
}

// BackupStream is a backup ready to be sent: the snapshot has been made, so
// the failures that can be told in a normal response already have been.
type BackupStream interface {
	io.WriterTo
	// Filename is the suggested download name.
	Filename() string
	// Close releases the snapshot and the backup lock.
	Close() error
}

// SystemInfo is what the server knows about itself and its machine: the
// community edition's implementation lives in package server.
type SystemInfo interface {
	Facts(ctx context.Context, church domain.Church) (SystemFacts, error)
	// OpenBackup starts a download. ErrBackupRunning, ErrNotSupported and
	// ErrStorageFull are returned before anything is sent.
	OpenBackup(ctx context.Context, church domain.Church, actor domain.UserID) (BackupStream, error)
}

// System is the church admin's system page: facts and the backup download
// (church.settings).
type System struct {
	Tx   Tx
	Info SystemInfo
}

func (u *System) church(ctx context.Context, sess *domain.Session) (domain.Church, domain.UserID, error) {
	var ch domain.Church
	var actor domain.UserID
	err := u.Tx.Read(ctx, func(s Store) error {
		sc, err := actorIn(ctx, s, sess, false)
		if err != nil {
			return err
		}
		if err := sc.actor.Require(domain.ScopeChurchSettings); err != nil {
			return err
		}
		actor = sc.actor.UserID
		ch, err = sc.cs.Church().Get(ctx)
		return err
	})
	return ch, actor, err
}

// Status returns the facts of the system page.
func (u *System) Status(ctx context.Context, sess *domain.Session) (SystemFacts, error) {
	ch, _, err := u.church(ctx, sess)
	if err != nil {
		return SystemFacts{}, err
	}
	return u.Info.Facts(ctx, ch)
}

// OpenBackup checks the permission and prepares the download. The caller
// must Close the stream.
func (u *System) OpenBackup(ctx context.Context, sess *domain.Session) (BackupStream, error) {
	ch, actor, err := u.church(ctx, sess)
	if err != nil {
		return nil, err
	}
	return u.Info.OpenBackup(ctx, ch, actor)
}
