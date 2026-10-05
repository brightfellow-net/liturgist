// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package envconfig reads the LITURGIST_* environment variables (01 §5).
package envconfig

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/brightfellow-net/liturgist/server"
)

// LogOptions configures the logger, which main builds before the server.
type LogOptions struct {
	Format string
	Level  slog.Level
}

type failFunc func(key, format string, a ...any)

// Load reads every variable. All problems are returned together.
func Load(getenv func(string) string) (server.Config, LogOptions, error) {
	var errs []error
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	fail := func(k, format string, a ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", k, fmt.Sprintf(format, a...)))
	}

	cfg := server.Config{
		DataDir:        get("LITURGIST_DATA_DIR", "./data"),
		Listen:         get("LITURGIST_LISTEN", "127.0.0.1:8080"),
		DBDriver:       get("LITURGIST_DB_DRIVER", "sqlite"),
		DBURL:          get("LITURGIST_DB_URL", ""),
		ClientIPHeader: get("LITURGIST_CLIENT_IP_HEADER", ""),
	}

	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		fail("LITURGIST_LISTEN", "must be host:port: %v", err)
	}

	base, err := parseBaseURL(get("LITURGIST_BASE_URL", "http://localhost:8080"))
	if err != nil {
		fail("LITURGIST_BASE_URL", "%v", err)
		base, _ = url.Parse("http://localhost:8080")
	}
	cfg.BaseURL = base

	for _, h := range splitList(get("LITURGIST_EXTRA_HOSTS", "")) {
		cfg.ExtraHosts = append(cfg.ExtraHosts, strings.ToLower(h))
	}

	switch cfg.DBDriver {
	case "sqlite":
	case "postgres":
		if cfg.DBURL == "" {
			fail("LITURGIST_DB_URL", "required when LITURGIST_DB_DRIVER is postgres")
		}
	default:
		fail("LITURGIST_DB_DRIVER", "must be sqlite or postgres, got %q", cfg.DBDriver)
	}

	cfg.AutoMigrate = parseBool(get("LITURGIST_AUTO_MIGRATE", "true"), "LITURGIST_AUTO_MIGRATE", fail)
	cfg.RequirePreUpgradeCopy = parseBool(get("LITURGIST_REQUIRE_PREUPGRADE_COPY", "false"), "LITURGIST_REQUIRE_PREUPGRADE_COPY", fail)

	cfg.UpdateCheck = parseBool(get("LITURGIST_UPDATE_CHECK", "false"), "LITURGIST_UPDATE_CHECK", fail)
	cfg.BackupTime, cfg.BackupKeepDaily, cfg.BackupKeepWeekly = parseBackup(get, fail)

	cfg.SessionTTL = parseDuration(get("LITURGIST_SESSION_TTL", "2160h"), "LITURGIST_SESSION_TTL", fail)
	cfg.SessionMaxAge = parseDuration(get("LITURGIST_SESSION_MAX_AGE", "8760h"), "LITURGIST_SESSION_MAX_AGE", fail)
	if cfg.SessionTTL != 0 && cfg.SessionTTL < time.Hour {
		fail("LITURGIST_SESSION_TTL", "must be at least 1h")
	}
	if cfg.SessionMaxAge != 0 && cfg.SessionMaxAge < cfg.SessionTTL {
		fail("LITURGIST_SESSION_MAX_AGE", "must be at least LITURGIST_SESSION_TTL")
	}

	for _, p := range splitList(get("LITURGIST_TRUSTED_PROXIES", "")) {
		prefix, err := parsePrefix(p)
		if err != nil {
			fail("LITURGIST_TRUSTED_PROXIES", "%q is not an IP address or CIDR range", p)
			continue
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, prefix)
	}

	logOpts := LogOptions{Format: get("LITURGIST_LOG_FORMAT", "text")}
	if logOpts.Format != "text" && logOpts.Format != "json" {
		fail("LITURGIST_LOG_FORMAT", "must be text or json")
	}
	if err := logOpts.Level.UnmarshalText([]byte(get("LITURGIST_LOG_LEVEL", "info"))); err != nil {
		fail("LITURGIST_LOG_LEVEL", "must be debug, info, warn or error")
	}
	return cfg, logOpts, errors.Join(errs...)
}

func parseBaseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("must be an absolute http or https URL")
	}
	if strings.TrimSuffix(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("must not have a path, query or fragment")
	}
	u.Path = ""
	return u, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s, key string, fail failFunc) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		fail(key, "must be true or false")
	}
	return b
}

func parseDuration(s, key string, fail failFunc) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		fail(key, "must be a duration such as 2160h")
	}
	return d
}

// parseBackup reads LITURGIST_BACKUP_TIME ("HH:MM" or "off"),
// LITURGIST_BACKUP_KEEP_DAILY and LITURGIST_BACKUP_KEEP_WEEKLY (14 §4).
func parseBackup(get func(k, def string) string, fail failFunc) (at string, daily, weekly int) {
	at = get("LITURGIST_BACKUP_TIME", "02:00")
	if strings.EqualFold(at, "off") {
		at = ""
	} else if _, _, err := server.ParseBackupTime(at); err != nil {
		fail("LITURGIST_BACKUP_TIME", "must be a time of day such as 02:00, or off")
		at = ""
	}
	count := func(key, def string, min int) int {
		n, err := strconv.Atoi(get(key, def))
		if err != nil || n < min || n > 365 {
			fail(key, "must be a whole number from %d to 365", min)
			return 0
		}
		return n
	}
	daily = count("LITURGIST_BACKUP_KEEP_DAILY", "7", 1)
	weekly = count("LITURGIST_BACKUP_KEEP_WEEKLY", "4", 0)
	if weekly == 0 {
		weekly = -1 // zero in the Config means "the default"
	}
	return at, daily, weekly
}
