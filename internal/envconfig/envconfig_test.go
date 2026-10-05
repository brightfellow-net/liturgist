// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package envconfig

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TC-F-001
func TestDefaults(t *testing.T) {
	cfg, logOpts, err := Load(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checks := map[string][2]any{
		"DataDir":       {cfg.DataDir, "./data"},
		"Listen":        {cfg.Listen, "127.0.0.1:8080"},
		"BaseURL":       {cfg.BaseURL.String(), "http://localhost:8080"},
		"DBDriver":      {cfg.DBDriver, "sqlite"},
		"AutoMigrate":   {cfg.AutoMigrate, true},
		"StrictCopy":    {cfg.RequirePreUpgradeCopy, false},
		"UpdateCheck":   {cfg.UpdateCheck, false},
		"BackupTime":    {cfg.BackupTime, "02:00"},
		"BackupDaily":   {cfg.BackupKeepDaily, 7},
		"BackupWeekly":  {cfg.BackupKeepWeekly, 4},
		"SessionTTL":    {cfg.SessionTTL, 2160 * time.Hour},
		"SessionMaxAge": {cfg.SessionMaxAge, 8760 * time.Hour},
		"LogFormat":     {logOpts.Format, "text"},
		"LogLevel":      {logOpts.Level, slog.LevelInfo},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	if len(cfg.TrustedProxies) != 0 || len(cfg.ExtraHosts) != 0 {
		t.Errorf("expected no proxies or extra hosts")
	}
}

// TC-F-002: missing PostgreSQL URL, reported together with other problems.
func TestErrorsReportedTogether(t *testing.T) {
	_, _, err := Load(env(map[string]string{
		"LITURGIST_DB_DRIVER":  "postgres",
		"LITURGIST_LOG_FORMAT": "xml",
		"LITURGIST_LISTEN":     "8080",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"LITURGIST_DB_URL", "LITURGIST_LOG_FORMAT", "LITURGIST_LISTEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// TC-F-003
func TestBaseURL(t *testing.T) {
	cases := map[string]bool{ // value -> valid
		"https://liturgi.example.org":       true,
		"https://liturgi.example.org/":      true,
		"http://192.168.1.10:8080":          true,
		"https://liturgi.example.org/path":  false,
		"https://liturgi.example.org/?a=1":  false,
		"https://liturgi.example.org/#frag": false,
		"ftp://example.org":                 false,
		"example.org":                       false,
	}
	for v, valid := range cases {
		cfg, _, err := Load(env(map[string]string{"LITURGIST_BASE_URL": v}))
		if valid && err != nil {
			t.Errorf("%q: unexpected error %v", v, err)
		}
		if !valid && err == nil {
			t.Errorf("%q: expected an error", v)
		}
		if valid && strings.HasSuffix(cfg.BaseURL.String(), "/") {
			t.Errorf("%q: trailing slash not removed: %s", v, cfg.BaseURL)
		}
	}
}

func TestTrustedProxiesAndHosts(t *testing.T) {
	cfg, _, err := Load(env(map[string]string{
		"LITURGIST_TRUSTED_PROXIES": "127.0.0.1, 10.0.0.0/8, ::1",
		"LITURGIST_EXTRA_HOSTS":     "Gereja.Local, 192.168.1.10",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"127.0.0.1/32", "10.0.0.0/8", "::1/128"}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("got %v", cfg.TrustedProxies)
	}
	for i, p := range cfg.TrustedProxies {
		if p.String() != want[i] {
			t.Errorf("proxy %d = %s, want %s", i, p, want[i])
		}
	}
	if cfg.ExtraHosts[0] != "gereja.local" {
		t.Errorf("extra hosts not lower-cased: %v", cfg.ExtraHosts)
	}

	if _, _, err := Load(env(map[string]string{"LITURGIST_TRUSTED_PROXIES": "not-an-ip"})); err == nil {
		t.Error("expected an error for an invalid proxy")
	}
}

func TestSessionDurations(t *testing.T) {
	if _, _, err := Load(env(map[string]string{"LITURGIST_SESSION_TTL": "30m"})); err == nil {
		t.Error("TTL below 1h must fail")
	}
	if _, _, err := Load(env(map[string]string{"LITURGIST_SESSION_TTL": "48h", "LITURGIST_SESSION_MAX_AGE": "24h"})); err == nil {
		t.Error("max age below TTL must fail")
	}
}

// The backup settings of 14 §4: off, a custom time, and invalid values reported together.
func TestBackupSettings(t *testing.T) {
	cfg, _, err := Load(env(map[string]string{"LITURGIST_BACKUP_TIME": "off"}))
	if err != nil || cfg.BackupTime != "" {
		t.Errorf("off: %q %v", cfg.BackupTime, err)
	}
	cfg, _, err = Load(env(map[string]string{"LITURGIST_BACKUP_TIME": "OFF"}))
	if err != nil || cfg.BackupTime != "" {
		t.Errorf("OFF: %q %v", cfg.BackupTime, err)
	}
	cfg, _, err = Load(env(map[string]string{"LITURGIST_BACKUP_TIME": "23:30", "LITURGIST_BACKUP_KEEP_DAILY": "14", "LITURGIST_BACKUP_KEEP_WEEKLY": "0"}))
	if err != nil || cfg.BackupTime != "23:30" || cfg.BackupKeepDaily != 14 || cfg.BackupKeepWeekly != -1 {
		t.Errorf("custom: %+v %v", cfg, err)
	}
	_, _, err = Load(env(map[string]string{"LITURGIST_BACKUP_TIME": "2am", "LITURGIST_BACKUP_KEEP_DAILY": "0", "LITURGIST_BACKUP_KEEP_WEEKLY": "x"}))
	if err == nil {
		t.Fatal("invalid values accepted")
	}
	for _, key := range []string{"LITURGIST_BACKUP_TIME", "LITURGIST_BACKUP_KEEP_DAILY", "LITURGIST_BACKUP_KEEP_WEEKLY"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

// H-13: the update check is off unless asked for.
func TestUpdateCheckSetting(t *testing.T) {
	if cfg, _, err := Load(env(map[string]string{"LITURGIST_UPDATE_CHECK": "true"})); err != nil || !cfg.UpdateCheck {
		t.Errorf("true: %v %v", cfg.UpdateCheck, err)
	}
	if _, _, err := Load(env(map[string]string{"LITURGIST_UPDATE_CHECK": "maybe"})); err == nil || !strings.Contains(err.Error(), "LITURGIST_UPDATE_CHECK") {
		t.Errorf("maybe: %v", err)
	}
}
