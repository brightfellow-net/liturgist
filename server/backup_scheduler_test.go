// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestParseBackupTime(t *testing.T) {
	for in, want := range map[string][2]int{"02:00": {2, 0}, "00:00": {0, 0}, "23:59": {23, 59}} {
		if h, m, err := ParseBackupTime(in); err != nil || h != want[0] || m != want[1] {
			t.Errorf("%q: %d:%d %v", in, h, m, err)
		}
	}
	for _, in := range []string{"", "2:00", "24:00", "02:60", "off", "0200", "02:00pm", " 02:00"} {
		if _, _, err := ParseBackupTime(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestNextBackup(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta") // UTC+7
	for _, tc := range []struct {
		name string
		now  time.Time
		loc  *time.Location
		want time.Time
	}{
		{"before the slot", at("2026-10-05 01:00"), time.UTC, at("2026-10-05 02:00")},
		{"at the slot", at("2026-10-05 02:00"), time.UTC, at("2026-10-06 02:00")},
		{"after the slot", at("2026-10-05 03:00"), time.UTC, at("2026-10-06 02:00")},
		{"church time zone", at("2026-10-05 10:00"), jakarta, at("2026-10-05 19:00")}, // 02:00 on the 6th in Jakarta
		{"month end", at("2026-10-31 12:00"), time.UTC, at("2026-11-01 02:00")},
	} {
		if got := nextBackup(tc.now, 2, 0, tc.loc); !got.Equal(tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func names(l []autoBackup) []string {
	var out []string
	for _, b := range l {
		out = append(out, b.name)
	}
	return out
}

func autoAt(t time.Time) autoBackup {
	return autoBackup{name: autoPrefix + t.Format(autoLayout) + ".zip", at: t}
}

// TC-606
func TestRetentionRule(t *testing.T) {
	// 42 daily backups at 02:00 from 2026-08-25 to 2026-10-05, newest first.
	var list []autoBackup
	for d := range 42 {
		list = append(list, autoAt(at("2026-10-05 02:00").AddDate(0, 0, -d)))
	}
	del := backupsToDelete(list, 7, 4, time.UTC)
	kept := map[string]bool{}
	for _, b := range list {
		kept[b.name] = true
	}
	for _, b := range del {
		delete(kept, b.name)
	}
	// 7 daily: Sep 29 .. Oct 5 (Oct 5 is a Monday: ISO weeks 40 and 41). Sep 28
	// is in week 40, already covered, so it goes. Then the newest backup of
	// each of weeks 39, 38, 37, 36: the Sundays Sep 27, 20, 13 and 6.
	want := []string{}
	for _, s := range []string{"2026-10-05", "2026-10-04", "2026-10-03", "2026-10-02", "2026-10-01", "2026-09-30", "2026-09-29",
		"2026-09-27", "2026-09-20", "2026-09-13", "2026-09-06"} {
		want = append(want, autoAt(at(s+" 02:00")).name)
	}
	got := names(slices.DeleteFunc(slices.Clone(list), func(b autoBackup) bool { return !kept[b.name] }))
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("kept %v\nwant %v", got, want)
	}

	t.Run("fewer than the daily count", func(t *testing.T) {
		if d := backupsToDelete(list[:5], 7, 4, time.UTC); len(d) != 0 {
			t.Errorf("deleted %v", names(d))
		}
	})
	t.Run("no weekly backups", func(t *testing.T) {
		if d := backupsToDelete(list, 7, -1, time.UTC); len(d) != len(list)-7 {
			t.Errorf("deleted %d, want %d", len(d), len(list)-7)
		}
	})
	t.Run("a week already covered keeps nothing more", func(t *testing.T) {
		two := []autoBackup{autoAt(at("2026-10-05 02:00")), autoAt(at("2026-10-04 02:00"))} // Oct 5 is in week 41, Oct 4 (a Sunday) in week 40
		if d := backupsToDelete(two, 1, 4, time.UTC); len(d) != 0 {
			t.Errorf("a different week is kept: %v", names(d))
		}
		same := []autoBackup{autoAt(at("2026-10-07 03:00")), autoAt(at("2026-10-06 02:00"))} // both ISO week 41
		if d := backupsToDelete(same, 1, 4, time.UTC); len(d) != 1 {
			t.Errorf("the same week: %v", names(d))
		}
	})
}

func schedulerFor(t *testing.T, cfg Config, now time.Time) (*backupScheduler, chan time.Duration, chan time.Time) {
	t.Helper()
	waits := make(chan time.Duration, 10)
	fire := make(chan time.Time)
	return &backupScheduler{
		cfg: withDefaults(cfg), hour: 2, loc: func(context.Context) *time.Location { return time.UTC },
		now:   func() time.Time { return now },
		after: func(d time.Duration) <-chan time.Time { waits <- d; return fire },
		mu:    &sync.Mutex{},
	}, waits, fire
}

func recv(t *testing.T, ch chan time.Duration) time.Duration {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(10 * time.Second):
		t.Fatal("the scheduler did not wait for its next slot")
		return 0
	}
}

func autoFiles(t *testing.T, cfg Config) []string {
	t.Helper()
	l, err := listAutoBackups(filepath.Join(cfg.DataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	return names(l)
}

// TC-607
func TestSchedulerRuns(t *testing.T) {
	t.Run("no backup yet: catch up after 5 minutes, then the next slot", func(t *testing.T) {
		cfg := seeded(t)
		b, waits, fire := schedulerFor(t, cfg, at("2026-10-05 10:00"))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() { b.run(ctx); close(done) }()
		if d := recv(t, waits); d != 5*time.Minute {
			t.Errorf("first wait %v", d)
		}
		fire <- time.Time{}
		if d := recv(t, waits); d != 16*time.Hour { // 10:00 to 02:00 next day
			t.Errorf("second wait %v", d)
		}
		if got := autoFiles(t, cfg); len(got) != 1 || got[0] != "auto-20261005T100000Z.zip" {
			t.Errorf("files %v", got)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the scheduler did not stop with its context")
		}
	})
	t.Run("a recent backup: wait for the slot", func(t *testing.T) {
		cfg := seeded(t)
		dir := filepath.Join(cfg.DataDir, "backups")
		write(t, filepath.Join(dir, "auto-20261005T020000Z.zip"), "x")
		b, waits, _ := schedulerFor(t, cfg, at("2026-10-05 10:00")) // 8 hours old
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go b.run(ctx)
		if d := recv(t, waits); d != 16*time.Hour {
			t.Errorf("first wait %v", d)
		}
	})
	t.Run("an old backup: catch up", func(t *testing.T) {
		cfg := seeded(t)
		write(t, filepath.Join(cfg.DataDir, "backups", "auto-20261003T020000Z.zip"), "x")
		b, waits, _ := schedulerFor(t, cfg, at("2026-10-05 10:00")) // 56 hours old
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go b.run(ctx)
		if d := recv(t, waits); d != 5*time.Minute {
			t.Errorf("first wait %v", d)
		}
	})
}

func TestSchedulerPrunesOnlyAfterSuccess(t *testing.T) {
	cfg := seeded(t)
	cfg.BackupKeepDaily, cfg.BackupKeepWeekly = 2, -1
	dir := filepath.Join(cfg.DataDir, "backups")
	for d := 1; d <= 5; d++ {
		write(t, filepath.Join(dir, autoAt(at("2026-10-05 02:00").AddDate(0, 0, -d)).name), "old")
	}
	// Files the scheduler must never touch.
	for _, n := range []string{"manual-20260101T000000Z.zip", "pre-upgrade-v00003-20260101T000000Z-ab.db", "pre-restore-20260101T000000Z-ab.db",
		"auto-notatime.zip", "notes.txt"} {
		write(t, filepath.Join(dir, n), "keep")
	}
	b, _, _ := schedulerFor(t, cfg, at("2026-10-05 10:00"))

	t.Run("a failed backup deletes nothing", func(t *testing.T) {
		old := freeBytes
		freeBytes = func(string) (int64, error) { return 1024, nil }
		defer func() { freeBytes = old }()
		b.once(context.Background())
		if got := len(autoFiles(t, cfg)); got != 5 {
			t.Errorf("%d automatic backups left, want 5", got)
		}
	})
	t.Run("a backup that fails for another reason deletes nothing either", func(t *testing.T) {
		live := filepath.Join(cfg.DataDir, "liturgist.db")
		good := read(t, live)
		write(t, live, "garbage, not a database") // the snapshot fails
		b.once(context.Background())
		write(t, live, good)
		if got := len(autoFiles(t, cfg)); got != 5 {
			t.Errorf("%d automatic backups left, want 5", got)
		}
	})
	t.Run("a successful one prunes to the rule", func(t *testing.T) {
		b.once(context.Background())
		want := []string{"auto-20261005T100000Z.zip", "auto-20261004T020000Z.zip"}
		if got := autoFiles(t, cfg); !slices.Equal(got, want) {
			t.Errorf("kept %v, want %v", got, want)
		}
		for _, n := range []string{"manual-20260101T000000Z.zip", "pre-upgrade-v00003-20260101T000000Z-ab.db", "pre-restore-20260101T000000Z-ab.db",
			"auto-notatime.zip", "notes.txt"} {
			if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
				t.Errorf("%s was touched: %v", n, err)
			}
		}
	})
}

func TestSchedulerRemovesStaleTemps(t *testing.T) {
	cfg := seeded(t)
	dir := filepath.Join(cfg.DataDir, "backups")
	old, recent := at("2026-10-01 00:00"), at("2026-10-05 09:30")
	for name, mod := range map[string]time.Time{
		backupTempPrefix + "old": old, autoPrefix + "20260101T000000Z.zip.partial": old,
		backupTempPrefix + "recent": recent, autoPrefix + "20261005T093000Z.zip.partial": recent,
		"manual-20260101T000000Z.zip.partial": old, // a manual backup's leftover is not the scheduler's
	} {
		p := filepath.Join(dir, name)
		if name == backupTempPrefix+"old" || name == backupTempPrefix+"recent" {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
		} else {
			write(t, p, "x")
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	b, _, _ := schedulerFor(t, cfg, at("2026-10-05 10:00"))
	b.prune(context.Background())
	for name, want := range map[string]bool{
		backupTempPrefix + "old": false, autoPrefix + "20260101T000000Z.zip.partial": false,
		backupTempPrefix + "recent": true, autoPrefix + "20261005T093000Z.zip.partial": true,
		"manual-20260101T000000Z.zip.partial": true,
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s: exists %v, want %v", name, err == nil, want)
		}
	}
}

// The scheduler uses the church's time zone, and UTC before setup.
func TestChurchLocation(t *testing.T) {
	cfg := seeded(t)
	srv, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if got := srv.churchLocation(context.Background()).String(); got != "Asia/Jakarta" {
		t.Errorf("location %q", got)
	}
	empty := Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0", BaseURL: cfg.BaseURL, DBDriver: "sqlite", AutoMigrate: true}
	srv2, err := New(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv2.Close() }()
	if got := srv2.churchLocation(context.Background()); got != time.UTC {
		t.Errorf("before setup: %v", got)
	}
}

func TestAutoBackupNamesOnly(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"auto-20261005T020000Z.zip", "auto-20261005T020000Z.zip.partial", "auto-2026.zip", "xauto-20261005T020000Z.zip", "manual-20261005T020000Z.zip"} {
		write(t, filepath.Join(dir, n), "x")
	}
	if err := os.Mkdir(filepath.Join(dir, "auto-20261004T020000Z.zip"), 0o700); err != nil { // a folder is not a backup
		t.Fatal(err)
	}
	l, err := listAutoBackups(dir)
	if err != nil || fmt.Sprint(names(l)) != "[auto-20261005T020000Z.zip]" {
		t.Errorf("%v %v", names(l), err)
	}
}

// The scheduler starts for a community server on SQLite with a time set, and
// for nothing else: not with BackupTime empty, not with a tenant resolver (hosted).
func TestSchedulerStartsOnlyWhereItShould(t *testing.T) {
	started := func(t *testing.T, mutate func(*Config), opts ...Option) bool {
		t.Helper()
		cfg := seeded(t)
		var out strings.Builder
		cfg.Logger = slog.New(slog.NewTextHandler(&out, nil))
		cfg.BackupTime = "02:00"
		if mutate != nil {
			mutate(&cfg)
		}
		srv, err := New(context.Background(), cfg, opts...)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = srv.Close() }()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srv.startBackups(ctx)
		return strings.Contains(out.String(), "automatic backups on")
	}
	if !started(t, nil) {
		t.Error("not started with a time set")
	}
	if started(t, func(c *Config) { c.BackupTime = "" }) {
		t.Error("started with automatic backups off")
	}
	if started(t, nil, WithTenantResolver(headerResolver{})) {
		t.Error("started for a server with a tenant resolver")
	}
}
