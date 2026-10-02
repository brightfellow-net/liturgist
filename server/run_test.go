// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

// running is a server started with Run on a free local port.
type running struct {
	base   string
	cfg    Config
	cancel context.CancelFunc // what SIGTERM does in "liturgist serve"
	done   chan error         // Run's result
}

func startServer(t *testing.T, opts ...Option) running {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	base := "http://localhost:" + strconv.Itoa(port)
	cfg := testConfig(t, base, nil)
	cfg.Listen = "127.0.0.1:" + strconv.Itoa(port)
	srv, err := New(context.Background(), cfg, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	r := running{base: "http://127.0.0.1:" + strconv.Itoa(port), cfg: cfg, cancel: cancel, done: make(chan error, 1)}
	go func() { r.done <- srv.Run(ctx) }()
	t.Cleanup(cancel)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if res, err := r.get("/healthz"); err == nil {
			_ = res.Body.Close()
			return r
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// get sends a request with the Host the server allows (01 §8).
func (r running) get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, r.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = r.cfg.BaseURL.Host
	return http.DefaultClient.Do(req)
}

// IT-F-001: serve starts on an empty data folder.
func TestServeStartsEmpty(t *testing.T) {
	r := startServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		res, err := r.get(path)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", path, res.StatusCode)
		}
	}
	if _, err := os.Stat(filepath.Join(r.cfg.DataDir, "liturgist.db")); err != nil {
		t.Errorf("database file: %v", err)
	}
	r.cancel()
	if err := <-r.done; err != nil {
		t.Errorf("Run: %v", err)
	}
}

// IT-F-004: on SIGTERM a request in flight completes, with a live context,
// and new connections are refused.
func TestGracefulShutdown(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r := startServer(t, WithRoutes(func(api huma.API) {
		huma.Register(api, huma.Operation{OperationID: "slow", Method: http.MethodGet, Path: "/slow",
			Metadata: map[string]any{httpapi.TenancyKey: httpapi.TenancyPlatform}},
			func(ctx context.Context, _ *struct{}) (*struct{ Body string }, error) {
				close(started)
				<-release
				if err := ctx.Err(); err != nil { // e.g. a database call would fail now
					return nil, err
				}
				return &struct{ Body string }{Body: "done"}, nil
			})
	}))

	type result struct {
		status int
		body   string
		err    error
	}
	slow := make(chan result, 1)
	go func() {
		res, err := r.get("/api/v1/slow")
		if err != nil {
			slow <- result{err: err}
			return
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		slow <- result{status: res.StatusCode, body: string(b)}
	}()
	<-started
	r.cancel()

	// The listener closes while the slow request is still running.
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", r.cfg.Listen, 100*time.Millisecond)
		if err != nil {
			break
		}
		_ = c.Close()
		if time.Now().After(deadline) {
			t.Fatal("new connections still accepted after shutdown began")
		}
		time.Sleep(20 * time.Millisecond)
	}

	close(release)
	if got := <-slow; got.err != nil || got.status != http.StatusOK || strings.TrimSpace(got.body) != `"done"` {
		t.Errorf("in-flight request: %+v", got)
	}
	if err := <-r.done; err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run: %v", err)
	}
}
