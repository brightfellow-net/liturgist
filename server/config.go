// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package server is the composition root: it wires adapters to the use cases
// and serves the HTTP API and web app (01 §7).
package server

import (
	"io"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"time"
)

// Set at build time with -ldflags "-X github.com/brightfellow-net/liturgist/server.Version=…".
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Config is the complete server configuration. The community binary fills it
// from environment variables (internal/envconfig); the SaaS builds it in code.
type Config struct {
	DataDir               string
	Listen                string
	BaseURL               *url.URL
	ExtraHosts            []string
	DBDriver              string // "sqlite" | "postgres"
	DBURL                 string
	DBMaxConns            int // PostgreSQL pool size; 0 = 10
	DBMaxReaders          int // SQLite reader pool size; 0 = 4
	AutoMigrate           bool
	AllowNewerSchema      bool // --allow-newer-schema: only skips the refusal to start (02 §5.1)
	RequirePreUpgradeCopy bool
	UpdateCheck           bool   // ask GitHub once a day for the newest release (14 §5, H-13)
	UpdateURL             string // the releases endpoint; "" = the project's
	BackupTime            string // "HH:MM" daily automatic backup; "" = none (14 §4)
	BackupKeepDaily       int    // 0 = 7
	BackupKeepWeekly      int    // 0 = 4; set BackupKeepWeekly to -1 for none
	SessionTTL            time.Duration
	SessionMaxAge         time.Duration
	Domain                string // built-in HTTPS for this domain (Let's Encrypt); "" = off (14 §19)
	ACMEEmail             string // contact address for the certificate authority; optional
	ACMECA                string // ACME directory URL; "" = Let's Encrypt production
	ACMEAgreed            bool   // the operator accepted the CA's subscriber agreement; required with Domain
	HTTPPort              int    // with Domain: certificate challenges and the redirect; 0 = 80
	HTTPSPort             int    // with Domain: the app; 0 = 443
	TrustedProxies        []netip.Prefix
	ClientIPHeader        string
	Logger                *slog.Logger
	Notices               io.Writer // where serve prints the framed setup link (stderr); nil = log only
}

// withDefaults fills settings a hand-built Config (e.g. the SaaS) may leave
// zero, with the same defaults as the environment variables (01 §5).
func withDefaults(cfg Config) Config {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 2160 * time.Hour
	}
	if cfg.SessionMaxAge == 0 {
		cfg.SessionMaxAge = 8760 * time.Hour
	}
	if cfg.BackupKeepDaily == 0 {
		cfg.BackupKeepDaily = 7
	}
	if cfg.BackupKeepWeekly == 0 {
		cfg.BackupKeepWeekly = 4
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return cfg
}

// StartupWarnings returns the configuration warnings of 01 §5 (logged, never fatal).
func StartupWarnings(cfg Config) []string {
	var w []string
	plainHTTP, proxyMissing := httpWarnings(cfg)
	if plainHTTP {
		w = append(w, "Liturgist is reachable on the network over plain HTTP. Passwords and session cookies can be read on the network; use HTTPS (see the install guide).")
	}
	if proxyMissing {
		w = append(w, "BASE_URL says https, but nothing here terminates TLS; configure the TLS proxy as a trusted proxy.")
	}
	return w
}

// httpWarnings are the two conditions of StartupWarnings: reachable over
// plain HTTP, and a BASE_URL that says https with no trusted proxy.
func httpWarnings(cfg Config) (plainHTTP, proxyMissing bool) {
	if cfg.Domain != "" {
		return false, false
	}
	httpBase := cfg.BaseURL.Scheme == "http"
	noProxies := len(cfg.TrustedProxies) == 0
	return !isLoopbackListen(cfg.Listen) && httpBase && noProxies, !httpBase && noProxies
}

func isLoopbackListen(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		return false // ":8080" listens on every interface
	}
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}
