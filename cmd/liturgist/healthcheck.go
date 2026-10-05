// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/brightfellow-net/liturgist/internal/envconfig"
)

// healthTimeout is how long healthcheck waits for the server (H-11).
const healthTimeout = 3 * time.Second

// healthcheck asks the server on the configured listen address for /healthz.
// The distroless image has no curl, so Docker's HEALTHCHECK runs this.
func healthcheck(getenv func(string) string, stderr io.Writer) int {
	cfg, _, err := envconfig.Load(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return exitError
	}
	if err := probe(context.Background(), healthURL(cfg.Listen)); err != nil {
		fmt.Fprintln(stderr, "unhealthy:", err)
		return exitError
	}
	return exitOK
}

// healthURL turns a listen address into the loopback URL of /healthz.
func healthURL(listen string) string {
	host, port, _ := net.SplitHostPort(listen)
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz"
}

func probe(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return nil
}
