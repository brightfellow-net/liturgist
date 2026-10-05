// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultUpdateURL is where the optional update check looks (14 §5, H-13).
const DefaultUpdateURL = "https://api.github.com/repos/brightfellow-net/liturgist/releases/latest"

// updateChecker asks for the newest release once a day. It sends a plain GET
// with the program's name and version in the User-Agent, and nothing else.
type updateChecker struct {
	url     string
	version string
	client  *http.Client
	log     *slog.Logger
	first   time.Duration // wait before the first check
	every   time.Duration

	mu     sync.Mutex
	latest string
}

func newUpdateChecker(cfg Config) *updateChecker {
	u := &updateChecker{url: cfg.UpdateURL, version: Version, client: &http.Client{Timeout: 10 * time.Second},
		log: cfg.Logger, first: time.Minute, every: 24 * time.Hour}
	if u.url == "" {
		u.url = DefaultUpdateURL
	}
	return u
}

func (u *updateChecker) run(ctx context.Context) {
	wait := u.first
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := u.check(ctx); err != nil && ctx.Err() == nil {
			u.log.Warn("update check failed", "error", err)
		}
		wait = u.every
	}
}

func (u *updateChecker) check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "liturgist/"+u.version)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the release service answered %d", res.StatusCode)
	}
	var body struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return err
	}
	if _, ok := parseVersion(body.Tag); !ok {
		return fmt.Errorf("unreadable release tag %q", body.Tag)
	}
	u.mu.Lock()
	u.latest = body.Tag
	u.mu.Unlock()
	u.log.Info("update check", "latest", body.Tag, "current", u.version)
	return nil
}

// Latest is the newest release tag seen, "" before the first successful check.
func (u *updateChecker) Latest() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.latest
}

// parseVersion reads "v1.2.3" or "1.2.3", with any suffix ("-rc1", "-5-gabc")
// ignored. Anything else, such as "dev", is not a version.
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, _, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	var v [3]int
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// newerVersion reports whether latest is a later release than current. A
// current version that is not a release ("dev") is never out of date.
func newerVersion(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
