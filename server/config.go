// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package server is the composition root: it wires adapters to the use cases
// and serves the HTTP API and web app (01 §7).
package server

import (
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
	RequirePreUpgradeCopy bool
	SessionTTL            time.Duration
	SessionMaxAge         time.Duration
	TrustedProxies        []netip.Prefix
	ClientIPHeader        string
	Logger                *slog.Logger
}

// StartupWarnings returns the configuration warnings of 01 §5 (logged, never fatal).
func StartupWarnings(cfg Config) []string {
	var w []string
	httpBase := cfg.BaseURL.Scheme == "http"
	noProxies := len(cfg.TrustedProxies) == 0
	if !isLoopbackListen(cfg.Listen) && httpBase && noProxies {
		w = append(w, "Liturgist is reachable on the network over plain HTTP. Passwords and session cookies can be read on the network; use HTTPS (see the install guide).")
	}
	if !httpBase && noProxies {
		w = append(w, "BASE_URL says https, but nothing here terminates TLS; configure the TLS proxy as a trusted proxy.")
	}
	return w
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
