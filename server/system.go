// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/brightfellow-net/liturgist/internal/backup"
)

// Thresholds of the system page (14 §2, H-5 and H-6).
const (
	lowDiskBytes     = 1 << 30             // below this many free bytes ...
	lowDiskFraction  = 0.05                // ... or this share of the disk, whichever is larger
	staleBackupAfter = 48 * time.Hour      // no backup of any kind for this long
	notCopiedAfter   = 30 * 24 * time.Hour // nothing downloaded or written elsewhere for this long
	copyMarker       = ".last-copy"
)

// diskLow is H-6: free space under 1 GiB or under 5 % of the disk, whichever is larger.
func diskLow(free, total int64) bool {
	threshold := max(int64(lowDiskBytes), int64(float64(total)*lowDiskFraction))
	return free < threshold
}

// backupWarning is H-5. A church younger than the limits has not had the time
// to be warned: the reference time is the later of the newest backup (or copy)
// and the day the church was created.
func backupWarning(now time.Time, lastAt, copiedAt *time.Time, churchCreated time.Time) string {
	since := func(t *time.Time) time.Duration {
		ref := churchCreated
		if t != nil && t.After(ref) {
			ref = *t
		}
		return now.Sub(ref)
	}
	if since(lastAt) > staleBackupAfter {
		return app.BackupWarningStale
	}
	if since(copiedAt) > notCopiedAfter {
		return app.BackupWarningNotCopied
	}
	return app.BackupWarningNone
}

var backupName = regexp.MustCompile(`^(auto|manual)-(\d{8}T\d{6}Z)\.zip$`)

// newestBackup is the newest auto-* or manual-* file in dir.
func newestBackup(dir string) (at *time.Time, kind string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ""
	}
	for _, e := range entries {
		m := backupName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		t, err := time.Parse(autoLayout, m[2])
		if err != nil {
			continue
		}
		if at == nil || t.After(*at) {
			at, kind = &t, m[1]
		}
	}
	return at, kind
}

// markCopied records that a copy left the data folder's backups (a download,
// or a backup written elsewhere): the file's modification time is the answer.
func markCopied(dataDir string) {
	dir := filepath.Join(dataDir, "backups")
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	p := filepath.Join(dir, copyMarker)
	if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600); err == nil { //nolint:gosec // our own folder
		_ = f.Close()
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
}

func lastCopied(dataDir string) *time.Time {
	fi, err := os.Stat(filepath.Join(dataDir, "backups", copyMarker))
	if err != nil {
		return nil
	}
	t := fi.ModTime().UTC()
	return &t
}

// insideBackups reports whether path is in the data folder's backups folder.
func insideBackups(dataDir, path string) bool {
	rel, err := filepath.Rel(filepath.Join(dataDir, "backups"), path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// systemInfo is the community edition's app.SystemInfo.
type systemInfo struct {
	cfg     Config
	db      *sqlstore.DB
	mo      sqlstore.MigrateOptions
	mu      *sync.Mutex // the backup lock shared with the scheduler
	updates *updateChecker
	now     func() time.Time
}

var _ app.SystemInfo = (*systemInfo)(nil)

func (s *systemInfo) Facts(ctx context.Context, church domain.Church) (app.SystemFacts, error) {
	var f app.SystemFacts
	f.Version, f.Commit = Version, Commit
	f.Database.Driver = s.cfg.DBDriver
	f.Database.SizeBytes, _ = s.db.SizeBytes(ctx)
	f.Database.SchemaVersion, _, _ = s.db.CheckVersion(ctx, s.mo)
	if free, total, err := sqlstore.DiskUsage(s.cfg.DataDir); err == nil {
		f.Disk.FreeBytes, f.Disk.TotalBytes, f.Disk.Low = free, total, diskLow(free, total)
	}
	now := s.now()
	f.Backup.Supported = s.cfg.DBDriver == "sqlite"
	f.Backup.Scheduled = f.Backup.Supported && s.cfg.BackupTime != ""
	if f.Backup.Supported {
		f.Backup.LastAt, f.Backup.LastKind = newestBackup(filepath.Join(s.cfg.DataDir, "backups"))
		f.Backup.LastCopiedAt = lastCopied(s.cfg.DataDir)
		f.Backup.Warning = backupWarning(now, f.Backup.LastAt, f.Backup.LastCopiedAt, church.CreatedAt)
	}
	f.HTTPS.PlainHTTPWarning, f.HTTPS.ProxyMissingWarning = httpWarnings(s.cfg)
	f.HTTPS.Mode = "behind_proxy"
	if s.cfg.BaseURL.Scheme == "http" {
		f.HTTPS.Mode = "plain_http"
	}
	if s.updates != nil {
		f.Update.Enabled = true
		f.Update.Latest = s.updates.Latest()
		f.Update.Available = newerVersion(f.Update.Latest, Version)
	}
	return f, nil
}

var slugRun = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(slugRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		return "church"
	}
	return s[:min(len(s), 40)]
}

func (s *systemInfo) OpenBackup(ctx context.Context, church domain.Church, actor domain.UserID) (app.BackupStream, error) {
	if s.cfg.DBDriver != "sqlite" {
		return nil, app.ErrNotSupported
	}
	if !s.mu.TryLock() {
		return nil, app.ErrBackupRunning
	}
	st, err := s.prepare(ctx, church, actor)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	return st, nil
}

// prepare makes the snapshot, so a full disk or a damaged database is an
// ordinary error response and not a broken download.
func (s *systemInfo) prepare(ctx context.Context, church domain.Church, actor domain.UserID) (*backupStream, error) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, storageErr(err)
	}
	dbPath := filepath.Join(s.cfg.DataDir, "liturgist.db")
	free, err := freeBytes(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read free disk space: %w", err)
	}
	if need := sqlstore.SnapshotSpace(dbPath); free < need {
		return nil, fmt.Errorf("%w: %w", app.ErrStorageFull, ErrNotEnoughSpace)
	}
	tmp, err := os.MkdirTemp(dir, backupTempPrefix)
	if err != nil {
		return nil, storageErr(err)
	}
	snap := filepath.Join(tmp, "liturgist.db")
	fail := func(err error) (*backupStream, error) {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := sqlstore.SnapshotFile(ctx, dbPath, snap); err != nil {
		return fail(storageErr(err))
	}
	schema, err := sqlstore.CheckSQLiteFile(ctx, snap)
	if err != nil {
		return fail(fmt.Errorf("the database copy failed its check: %w", err))
	}
	now := s.now().UTC()
	return &backupStream{
		s: s, tmp: tmp, snap: snap, actor: actor,
		name: fmt.Sprintf("liturgist-%s-%s.zip", slug(church.Name), now.Format("2006-01-02")),
		m:    backup.Manifest{Program: Version, Schema: schema, CreatedAt: now.Truncate(time.Second), Driver: "sqlite"},
	}, nil
}

// backupStream is one download in progress.
type backupStream struct {
	s     *systemInfo
	tmp   string
	snap  string
	name  string
	actor domain.UserID
	m     backup.Manifest
}

func (b *backupStream) Filename() string { return b.name }

func (b *backupStream) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	warn := func(msg string) { b.s.cfg.Logger.Warn("backup: " + msg) }
	err := backup.Write(cw, b.m, b.snap, filepath.Join(b.s.cfg.DataDir, "files"), warn)
	if err != nil {
		b.s.cfg.Logger.Warn("backup_download_failed", "actor", string(b.actor), "bytes", cw.n, "error", err)
		return cw.n, err
	}
	markCopied(b.s.cfg.DataDir)
	b.s.cfg.Logger.Info("backup_downloaded", "actor", string(b.actor), "bytes", cw.n)
	return cw.n, nil
}

func (b *backupStream) Close() error {
	err := os.RemoveAll(b.tmp)
	b.s.mu.Unlock()
	return err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
