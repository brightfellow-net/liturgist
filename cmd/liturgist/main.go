// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Command liturgist is the community edition server and CLI.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "time/tzdata" // time zones on Windows and minimal images

	"github.com/brightfellow-net/liturgist/internal/envconfig"
	"github.com/brightfellow-net/liturgist/internal/logging"
	"github.com/brightfellow-net/liturgist/server"
	"golang.org/x/term"
)

// Exit codes (01 §6).
const (
	exitOK              = 0
	exitError           = 1
	exitConfig          = 2
	exitSchemaNewer     = 3
	exitAlreadySetUp    = 4
	exitNoSuchUser      = 5
	exitNotMember       = 6
	exitTooManyChurches = 7
)

const usage = `usage: liturgist <command>

commands:
  serve [--allow-newer-schema]           run the server
  migrate                                apply pending database migrations
  migrate status [--allow-newer-schema]  print the database and program schema versions
  setup --church-name … --admin-name … --admin-identifier … [--password-stdin]
        [--ui-language en] [--language id] [--translation TB] [--time-zone Asia/Jakarta] [--key-display do]
                                         create the church and its first admin
  setup-link                             print a new 24-hour setup link
  user reset-password <email-or-phone>   print a password-reset link
  user list                              list users and their roles
  member grant-admin <email-or-phone>    give a member the Church admin role (recovery)
  auth clear-throttle --identifier X | --ip Y | --all
                                         delete login-throttle counters
  search reindex                         rebuild the song search index
  backup [file]                          write one archive of the database and files (SQLite)
  restore <file> [--yes]                 replace the data with a backup; the server must be stopped
  healthcheck                            exit 0 if the running server answers /healthz (for Docker)
  service install|uninstall|start|stop   Windows only: manage the Windows service
  openapi                                print the OpenAPI document
  version                                print version information`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
	case args[0] == "setup":
		return setup(args[1:], stdin, stdout, stderr)
	case args[0] == "setup-link":
		return withOperator("setup-link", args[1:], stdout, stderr, setupLink)
	case len(args) > 1 && args[0] == "user" && args[1] == "reset-password":
		return withOperator("user reset-password", args[2:], stdout, stderr, resetPassword)
	case len(args) > 1 && args[0] == "user" && args[1] == "list":
		return withOperator("user list", args[2:], stdout, stderr, listUsers)
	case len(args) > 1 && args[0] == "member" && args[1] == "grant-admin":
		return withOperator("member grant-admin", args[2:], stdout, stderr, grantAdmin)
	case len(args) > 1 && args[0] == "auth" && args[1] == "clear-throttle":
		return withOperator("auth clear-throttle", args[2:], stdout, stderr, clearThrottle)
	case len(args) > 1 && args[0] == "search" && args[1] == "reindex":
		return withOperator("search reindex", args[2:], stdout, stderr, reindexSongs)
	case args[0] == "backup":
		return backupCmd(args[1:], stdout, stderr)
	case args[0] == "restore":
		return restoreCmd(args[1:], stdin, stdout, stderr)
	case args[0] == "healthcheck":
		return healthcheck(os.Getenv, stderr)
	case args[0] == "service":
		return serviceCmd(args[1:], stdout, stderr)
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

// loadToStderr is the configuration for commands whose standard output is
// their result: the log goes to standard error.
func loadToStderr(stderr io.Writer) (server.Config, bool) {
	cfg, logOpts, err := envconfig.Load(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return server.Config{}, false
	}
	cfg.Logger = logging.New(stderr, logOpts.Format, logOpts.Level)
	return cfg, true
}

func exitFor(err error, stderr io.Writer) int {
	fmt.Fprintln(stderr, err)
	switch {
	case server.IsSchemaNewer(err):
		return exitSchemaNewer
	case errors.Is(err, server.ErrAlreadySetUp):
		return exitAlreadySetUp
	case errors.Is(err, server.ErrNoSuchUser):
		return exitNoSuchUser
	case errors.Is(err, server.ErrNotMember):
		return exitNotMember
	case server.IsTooManyChurches(err):
		return exitTooManyChurches
	case server.IsBackupNewer(err):
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
	return serveUntil(ctx, cfg, stderr)
}

// serveUntil runs the server until ctx ends. The Windows service calls it too.
func serveUntil(ctx context.Context, cfg server.Config, stderr io.Writer) int {
	cfg.Notices = stderr
	release, err := server.LockRun(cfg)
	if err != nil {
		cfg.Logger.Error("start-up failed", "error", err)
		fmt.Fprintln(stderr, err)
		return exitError
	}
	defer release()

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

// withOperator loads the configuration (no flags besides positional
// arguments, which fn parses), opens the database and runs fn.
func withOperator(name string, args []string, stdout, stderr io.Writer, fn func(context.Context, *server.Operator, []string, io.Writer, io.Writer) error) int {
	cfg, logOpts, err := envconfig.Load(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return exitConfig
	}
	cfg.Logger = logging.New(stderr, logOpts.Format, logOpts.Level)
	ctx := context.Background()
	op, err := server.OpenOperator(ctx, cfg)
	if err != nil {
		return exitFor(err, stderr)
	}
	defer func() { _ = op.Close() }()
	if err := fn(ctx, op, args, stdout, stderr); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(stderr, "usage error in %s\n%s\n", name, usage)
			return exitConfig
		}
		return exitFor(err, stderr)
	}
	return exitOK
}

var errUsage = errors.New("usage")

func warnBaseURL(stderr io.Writer) {
	if os.Getenv("LITURGIST_BASE_URL") == "" {
		fmt.Fprintln(stderr, "warning: LITURGIST_BASE_URL is not set; the link may point to localhost.")
	}
}

func setupLink(ctx context.Context, op *server.Operator, args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errUsage
	}
	warnBaseURL(stderr)
	link, err := op.SetupLink(ctx)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, server.SetupLinkBlock("Setup link (any earlier link no longer works).", link))
	return nil
}

func resetPassword(ctx context.Context, op *server.Operator, args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errUsage
	}
	warnBaseURL(stderr)
	res, err := op.ResetPassword(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Password-reset link for %s (valid until %s):\n%s\n", res.UserName, res.ExpiresAt.Format("2006-01-02 15:04 MST"), res.Link)
	return nil
}

func listUsers(ctx context.Context, op *server.Operator, args []string, stdout, _ io.Writer) error {
	if len(args) != 0 {
		return errUsage
	}
	users, err := op.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		roles := "(not a member)"
		if u.Roles != nil {
			roles = "roles: " + strings.Join(u.Roles, ", ")
			if len(u.Roles) == 0 {
				roles = "team member"
			}
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", u.Name, orDash(u.Email), orDash(u.Phone), roles)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func grantAdmin(ctx context.Context, op *server.Operator, args []string, stdout, _ io.Writer) error {
	if len(args) != 1 {
		return errUsage
	}
	if err := op.GrantAdmin(ctx, args[0]); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "The member now holds the Church admin role.")
	return nil
}

func clearThrottle(ctx context.Context, op *server.Operator, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("auth clear-throttle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	identifier := fs.String("identifier", "", "email or phone number")
	ip := fs.String("ip", "", "client IP address")
	all := fs.Bool("all", false, "delete every counter")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errUsage
	}
	n := 0
	for _, set := range []bool{*identifier != "", *ip != "", *all} {
		if set {
			n++
		}
	}
	if n != 1 {
		return errUsage
	}
	if err := op.ClearThrottle(ctx, *identifier, *ip, *all); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Login-throttle counters deleted.")
	return nil
}

func reindexSongs(ctx context.Context, op *server.Operator, args []string, stdout, _ io.Writer) error {
	if len(args) != 0 {
		return errUsage
	}
	n, err := op.ReindexSongs(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Song search index rebuilt for %d church(es).\n", n)
	return nil
}

func setup(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	p := server.SetupParams{}
	fs.StringVar(&p.ChurchName, "church-name", "", "church name (required)")
	fs.StringVar(&p.AdminName, "admin-name", "", "first admin's name (required)")
	fs.StringVar(&p.AdminIdentifier, "admin-identifier", "", "first admin's email or phone (required)")
	fs.StringVar(&p.UILanguage, "ui-language", "en", "default UI language: en or id")
	fs.StringVar(&p.Language, "language", "id", "default content language: id, en, zh-Hans, zh-Hant")
	fs.StringVar(&p.Translation, "translation", "TB", "default Bible translation code")
	fs.StringVar(&p.TimeZone, "time-zone", "Asia/Jakarta", "IANA time zone")
	fs.StringVar(&p.KeyDisplay, "key-display", "do", "key display: do or letter")
	fromStdin := fs.Bool("password-stdin", false, "read the password from standard input")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	if fs.NArg() != 0 || p.ChurchName == "" || p.AdminName == "" || p.AdminIdentifier == "" {
		fmt.Fprintln(stderr, "setup needs --church-name, --admin-name and --admin-identifier")
		return exitConfig
	}
	pw, err := readPassword(stdin, stderr, *fromStdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	p.Password = pw
	return withOperator("setup", nil, stdout, stderr, func(ctx context.Context, op *server.Operator, _ []string, stdout, _ io.Writer) error {
		if err := op.Setup(ctx, p); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "The church is set up. Log in with the admin's email or phone and password.")
		return nil
	})
}

// readPassword reads one line from stdin, or prompts twice without echo on a terminal.
func readPassword(stdin io.Reader, stderr io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading the password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	f, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // file descriptors fit in int
		return "", errors.New("no terminal to ask for the password; use --password-stdin")
	}
	fmt.Fprint(stderr, "Admin password: ")
	first, err := term.ReadPassword(int(f.Fd())) //nolint:gosec // file descriptors fit in int
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(stderr, "Repeat the password: ")
	second, err := term.ReadPassword(int(f.Fd())) //nolint:gosec // file descriptors fit in int
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("the passwords don't match")
	}
	return string(first), nil
}

func backupCmd(args []string, stdout, stderr io.Writer) int {
	cfg, ok := loadToStderr(stderr)
	if !ok {
		return exitConfig
	}
	if len(args) > 1 {
		fmt.Fprintln(stderr, usage)
		return exitConfig
	}
	dest := ""
	if len(args) == 1 {
		dest = args[0]
	}
	res, err := server.Backup(context.Background(), cfg, dest)
	if err != nil {
		return exitFor(err, stderr)
	}
	fmt.Fprintf(stdout, "Backup written to %s (%.1f MB, schema version %d).\n", res.Path, float64(res.Size)/(1<<20), res.Schema)
	return exitOK
}

func restoreCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	// Flags may come after the file name: liturgist restore backup.zip --yes.
	var files []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return exitConfig
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(files) != 1 {
		fmt.Fprintln(stderr, usage)
		return exitConfig
	}
	cfg, ok := loadToStderr(stderr)
	if !ok {
		return exitConfig
	}
	var confirm func(server.RestoreInfo) bool
	if !*yes {
		f, isFile := stdin.(*os.File)
		if !isFile || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // file descriptors fit in int
			fmt.Fprintln(stderr, "no terminal to ask for confirmation; use --yes")
			return exitError
		}
		confirm = func(info server.RestoreInfo) bool {
			fmt.Fprintf(stderr, "Replace ALL current data with the backup made %s (Liturgist %s)?\nThe current data is kept in the backups folder. Type yes to continue: ",
				info.CreatedAt.Local().Format("2006-01-02 15:04"), info.Program)
			line, _ := bufio.NewReader(stdin).ReadString('\n')
			answer := strings.ToLower(strings.TrimSpace(line))
			return answer == "y" || answer == "yes"
		}
	}
	res, err := server.Restore(context.Background(), cfg, files[0], confirm)
	if err != nil {
		return exitFor(err, stderr)
	}
	fmt.Fprintf(stdout, "Restored (schema version %d). Start the server again.\n", res.Schema)
	if res.PreRestoreDB != "" {
		fmt.Fprintf(stdout, "The previous database is kept as %s\n", res.PreRestoreDB)
	}
	if res.PreRestoreFiles != "" {
		fmt.Fprintf(stdout, "The previous files are kept in %s\n", res.PreRestoreFiles)
	}
	return exitOK
}
