// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Command liturgist is the community edition server and CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	_ "time/tzdata" // time zones on Windows and minimal images

	"github.com/brightfellow-net/liturgist/internal/envconfig"
	"github.com/brightfellow-net/liturgist/internal/logging"
	"github.com/brightfellow-net/liturgist/server"
)

// Exit codes (01 §6).
const (
	exitOK          = 0
	exitError       = 1
	exitConfig      = 2
	exitSchemaNewer = 3
)

const usage = `usage: liturgist <command>

commands:
  serve [--allow-newer-schema]           run the server
  migrate                                apply pending database migrations
  migrate status [--allow-newer-schema]  print the database and program schema versions
  openapi                                print the OpenAPI document
  version                                print version information`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitConfig
	}
	switch {
	case args[0] == "serve":
		return serve(args[1:], stdout, stderr)
	case args[0] == "migrate" && len(args) > 1 && args[1] == "status":
		return migrateStatus(args[2:], stdout, stderr)
	case args[0] == "migrate":
		return migrate(args[1:], stdout, stderr)
	case args[0] == "openapi":
		doc, err := server.OpenAPI()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		_, _ = stdout.Write(append(doc, '\n'))
		return exitOK
	case args[0] == "version":
		fmt.Fprintf(stdout, "liturgist %s (commit %s, built %s)\n", server.Version, server.Commit, server.BuildDate)
		return exitOK
	}
	fmt.Fprintln(stderr, usage)
	return exitConfig
}

// load parses flags and the environment into a server configuration.
func load(name string, args []string, allowNewerFlag bool, stdout, stderr io.Writer) (server.Config, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	allowNewer := false
	if allowNewerFlag {
		fs.BoolVar(&allowNewer, "allow-newer-schema", false, "start even if the database is newer than this program (operators only)")
	}
	if err := fs.Parse(args); err != nil {
		return server.Config{}, false
	}
	cfg, logOpts, err := envconfig.Load(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return server.Config{}, false
	}
	cfg.AllowNewerSchema = allowNewer
	cfg.Logger = logging.New(stdout, logOpts.Format, logOpts.Level)
	return cfg, true
}

func exitFor(err error, stderr io.Writer) int {
	fmt.Fprintln(stderr, err)
	if server.IsSchemaNewer(err) {
		return exitSchemaNewer
	}
	return exitError
}

func serve(args []string, stdout, stderr io.Writer) int {
	cfg, ok := load("serve", args, true, stdout, stderr)
	if !ok {
		return exitConfig
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, cfg)
	if err != nil {
		cfg.Logger.Error("start-up failed", "error", err)
		return exitFor(err, stderr)
	}
	defer func() { _ = srv.Close() }()
	if err := srv.Run(ctx); err != nil {
		cfg.Logger.Error("server stopped", "error", err)
		return exitError
	}
	return exitOK
}

func migrate(args []string, stdout, stderr io.Writer) int {
	cfg, ok := load("migrate", args, false, stdout, stderr)
	if !ok {
		return exitConfig
	}
	res, err := server.Migrate(context.Background(), cfg)
	if err != nil {
		return exitFor(err, stderr)
	}
	if res.From == res.To {
		fmt.Fprintf(stdout, "database is up to date (version %d)\n", res.To)
	} else {
		fmt.Fprintf(stdout, "database migrated from version %d to %d\n", res.From, res.To)
	}
	return exitOK
}

func migrateStatus(args []string, stdout, stderr io.Writer) int {
	cfg, ok := load("migrate status", args, true, stdout, stderr)
	if !ok {
		return exitConfig
	}
	current, target, err := server.MigrateStatus(context.Background(), cfg)
	if err != nil {
		return exitFor(err, stderr)
	}
	fmt.Fprintf(stdout, "database version %d, program version %d\n", current, target)
	return exitOK
}
