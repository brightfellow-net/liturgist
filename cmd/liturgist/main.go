// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Command liturgist is the community edition server and CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "time/tzdata" // time zones on Windows and minimal images

	"github.com/brightfellow-net/liturgist/internal/envconfig"
	"github.com/brightfellow-net/liturgist/internal/logging"
	"github.com/brightfellow-net/liturgist/server"
)

const usage = `usage: liturgist <command>

commands:
  serve     run the server
  openapi   print the OpenAPI document
  version   print version information`

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "serve":
		return serve()
	case "openapi":
		doc, err := server.OpenAPI()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_, _ = os.Stdout.Write(append(doc, '\n'))
		return 0
	case "version":
		fmt.Printf("liturgist %s (commit %s, built %s)\n", server.Version, server.Commit, server.BuildDate)
		return 0
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
}

func serve() int {
	cfg, logOpts, err := envconfig.Load(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration:\n%v\n", err)
		return 2
	}
	cfg.Logger = logging.New(os.Stdout, logOpts.Format, logOpts.Level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, cfg)
	if err != nil {
		cfg.Logger.Error("start-up failed", "error", err)
		return 1
	}
	defer func() { _ = srv.Close() }()
	if err := srv.Run(ctx); err != nil {
		cfg.Logger.Error("server stopped", "error", err)
		return 1
	}
	return 0
}
