// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brightfellow-net/liturgist/app"
)

const (
	autoPrefix       = "auto-"
	autoLayout       = "20060102T150405Z"
	catchUpDelay     = 5 * time.Minute // a missed run starts this long after the server
	catchUpAfter     = 26 * time.Hour  // "missed": the newest automatic backup is older than this
	staleTempAfter   = 24 * time.Hour  // leftovers of a crashed backup older than this are deleted
	backupTempPrefix = ".liturgist-backup-"
)

var autoName = regexp.MustCompile(`^auto-(\d{8}T\d{6}Z)\.zip$`)

// ParseBackupTime parses LITURGIST_BACKUP_TIME, "HH:MM" on a 24-hour clock.
func ParseBackupTime(s string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, fmt.Errorf("%q is not a time of day such as 02:00", s)
	}
	return t.Hour(), t.Minute(), nil
}

// nextBackup is the first hour:minute in loc that is after now.
func nextBackup(now time.Time, hour, minute int, loc *time.Location) time.Time {
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day(), hour, minute, 0, 0, loc)
	if !next.After(n) {
		next = time.Date(n.Year(), n.Month(), n.Day()+1, hour, minute, 0, 0, loc)
	}
	return next
}

// autoBackup is one backups/auto-*.zip.
type autoBackup struct {
	name string
	at   time.Time // from the file name, UTC
}

// listAutoBackups returns the automatic backups in dir, newest first. Only
// names of the exact form auto-<UTC time>.zip count; nothing else in the
// folder is ever the scheduler's business.
func listAutoBackups(dir string) ([]autoBackup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []autoBackup
	for _, e := range entries {
		m := autoName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.Parse(autoLayout, m[1])
		if err != nil {
			continue
		}
		out = append(out, autoBackup{name: e.Name(), at: at})
	}
	slices.SortFunc(out, func(a, b autoBackup) int { return b.at.Compare(a.at) })
	return out, nil
}

// backupsToDelete is the retention rule (14 §2, H-4): keep the newest `daily`
// backups, and of the older ones the newest backup of each of the next
// `weekly` ISO weeks that has any and is not already covered by a kept
// backup. weekly < 0 keeps no weekly backups. list is newest first.
func backupsToDelete(list []autoBackup, daily, weekly int, loc *time.Location) []autoBackup {
	if len(list) <= daily {
		return nil
	}
	type week struct{ year, week int }
	weekOf := func(t time.Time) week { y, w := t.In(loc).ISOWeek(); return week{y, w} }
	covered := map[week]bool{}
	for _, b := range list[:daily] {
		covered[weekOf(b.at)] = true
	}
	var del []autoBackup
	weeks := 0
	for _, b := range list[daily:] {
		w := weekOf(b.at)
		if !covered[w] && weeks < max(weekly, 0) {
			covered[w] = true // the newest of this week: older ones of the same week go
			weeks++
			continue
		}
		del = append(del, b)
	}
	return del
}

// backupScheduler runs the automatic backups of 14 §4.
type backupScheduler struct {
	cfg    Config
	hour   int
	minute int
	loc    func(context.Context) *time.Location
	now    func() time.Time
	after  func(time.Duration) <-chan time.Time
	mu     *sync.Mutex // one backup at a time, shared with the download (14 §5)
}

func (b *backupScheduler) dir() string { return filepath.Join(b.cfg.DataDir, "backups") }

// firstWait is how long to wait after start: a catch-up run when the newest
// automatic backup is older than catchUpAfter (or there is none), else the
// time to the next slot.
func (b *backupScheduler) firstWait(ctx context.Context) time.Duration {
	list, err := listAutoBackups(b.dir())
	if err == nil && (len(list) == 0 || b.now().Sub(list[0].at) > catchUpAfter) {
		return catchUpDelay
	}
	return b.untilNext(ctx)
}

func (b *backupScheduler) untilNext(ctx context.Context) time.Duration {
	now := b.now()
	return nextBackup(now, b.hour, b.minute, b.loc(ctx)).Sub(now)
}

// run waits for each slot and makes a backup, until ctx ends. A failed backup
// is logged and the next slot tried; it is never retried in a loop.
func (b *backupScheduler) run(ctx context.Context) {
	wait := b.firstWait(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.after(wait):
		}
		b.once(ctx)
		wait = b.untilNext(ctx)
	}
}

// once makes one automatic backup and, only if that worked, prunes.
func (b *backupScheduler) once(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	log := b.cfg.Logger
	dest := filepath.Join(b.dir(), autoPrefix+b.now().UTC().Format(autoLayout)+".zip")
	if err := os.MkdirAll(b.dir(), 0o700); err != nil {
		log.Error("backup_failed", "error", storageErr(err))
		return
	}
	_, err := writeBackup(ctx, b.cfg, dest)
	switch {
	case ctx.Err() != nil:
		return
	case errors.Is(err, ErrNotEnoughSpace):
		log.Warn("backup_skipped", "reason", err.Error())
		return
	case err != nil:
		log.Error("backup_failed", "error", err)
		return
	}
	b.prune(ctx)
}

// prune applies the retention rule and removes leftovers of crashed backups.
func (b *backupScheduler) prune(ctx context.Context) {
	log := b.cfg.Logger
	list, err := listAutoBackups(b.dir())
	if err != nil {
		log.Warn("backup_prune_failed", "error", err)
		return
	}
	for _, d := range backupsToDelete(list, b.cfg.BackupKeepDaily, b.cfg.BackupKeepWeekly, b.loc(ctx)) {
		if err := os.Remove(filepath.Join(b.dir(), d.name)); err != nil {
			log.Warn("backup_prune_failed", "file", d.name, "error", err)
			continue
		}
		log.Info("backup_pruned", "file", d.name)
	}
	entries, _ := os.ReadDir(b.dir())
	for _, e := range entries {
		stale := strings.HasPrefix(e.Name(), backupTempPrefix) || (strings.HasPrefix(e.Name(), autoPrefix) && strings.HasSuffix(e.Name(), ".zip.partial"))
		if !stale {
			continue
		}
		if fi, err := e.Info(); err == nil && b.now().Sub(fi.ModTime()) > staleTempAfter {
			_ = os.RemoveAll(filepath.Join(b.dir(), e.Name()))
		}
	}
}

// startBackups starts the scheduler for a community server on SQLite.
func (s *Server) startBackups(ctx context.Context) {
	if s.single == nil || s.cfg.DBDriver != "sqlite" || s.cfg.BackupTime == "" {
		return
	}
	hour, minute, err := ParseBackupTime(s.cfg.BackupTime)
	if err != nil {
		s.cfg.Logger.Error("automatic backups are off", "error", err)
		return
	}
	b := &backupScheduler{cfg: s.cfg, hour: hour, minute: minute, loc: s.churchLocation, now: time.Now, after: time.After, mu: &s.backupMu}
	s.cfg.Logger.Info("automatic backups on", "time", s.cfg.BackupTime, "keep_daily", s.cfg.BackupKeepDaily, "keep_weekly", s.cfg.BackupKeepWeekly)
	go b.run(ctx)
}

// churchLocation is the church's time zone, or UTC before setup.
func (s *Server) churchLocation(ctx context.Context) *time.Location {
	id, err := s.single.ChurchID(ctx)
	if err != nil {
		return time.UTC
	}
	var tz string
	_ = s.db.Read(ctx, func(st app.Store) error {
		c, err := st.Churches().ByID(ctx, id)
		tz = c.TimeZone
		return err
	})
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}
