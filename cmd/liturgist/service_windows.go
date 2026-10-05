// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/brightfellow-net/liturgist/internal/envconfig"
	"github.com/brightfellow-net/liturgist/internal/logging"
)

const (
	serviceName = "Liturgist"
	logFileName = "liturgist.log"
)

// serviceDir is where the service keeps its settings, data and log:
// %ProgramData%\Liturgist.
func serviceDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "Liturgist")
}

const sampleEnv = `# Settings for the Liturgist Windows service, one KEY=VALUE per line.
# Restart the service after a change: liturgist service stop, then start.
# See docs/self-host/configuration.md for every setting.
#
# LITURGIST_BASE_URL=https://liturgi.example.org
# LITURGIST_LISTEN=127.0.0.1:8080
`

func serviceCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: liturgist service install|uninstall|start|stop")
		return exitConfig
	}
	var err error
	switch args[0] {
	case "install":
		err = installService(stdout)
	case "uninstall":
		err = withService(func(s *mgr.Service) error {
			if st, _ := s.Query(); st.State != svc.Stopped {
				_, _ = s.Control(svc.Stop)
			}
			return s.Delete()
		})
		if err == nil {
			fmt.Fprintf(stdout, "service removed; the data in %s was left alone\n", serviceDir())
		}
	case "start":
		err = withService(func(s *mgr.Service) error { return s.Start() })
	case "stop":
		err = withService(func(s *mgr.Service) error {
			_, err := s.Control(svc.Stop)
			return err
		})
	case "run": // started by the service manager
		return runService()
	default:
		fmt.Fprintln(stderr, "usage: liturgist service install|uninstall|start|stop")
		return exitConfig
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	return exitOK
}

func withService(f func(*mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("cannot reach the service manager (run as Administrator): %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %q is not installed: %w", serviceName, err)
	}
	defer func() { _ = s.Close() }()
	return f(s)
}

func installService(stdout io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := serviceDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	envPath := filepath.Join(dir, envFileName)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := os.WriteFile(envPath, []byte(sampleEnv), 0o600); err != nil {
			return err
		}
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("cannot reach the service manager (run as Administrator): %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: "Liturgist",
		Description: "Liturgist, a web app for planning church services",
		StartType:   mgr.StartAutomatic,
	}, "service", "run")
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 24*60*60)
	fmt.Fprintf(stdout, "service installed. Settings: %s\nData: %s\nStart it with: liturgist service start\n",
		envPath, filepath.Join(dir, "data"))
	return nil
}

// runService is what the service manager starts: it reads the settings file,
// sends the log to a file and runs the server until told to stop.
func runService() int {
	dir := serviceDir()
	logf, err := os.OpenFile(filepath.Join(dir, logFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return exitError
	}
	defer func() { _ = logf.Close() }()
	if _, err := applySettings(dir, os.LookupEnv, os.Setenv); err != nil {
		fmt.Fprintln(logf, err)
		return exitConfig
	}
	_ = defaultDataDir(dir, os.LookupEnv, os.Setenv)
	cfg, opts, err := envconfig.Load(os.Getenv)
	if err != nil {
		fmt.Fprintf(logf, "invalid configuration:\n%v\n", err)
		return exitConfig
	}
	cfg.Logger = logging.New(logf, opts.Format, opts.Level)

	code := exitOK
	err = svc.Run(serviceName, &handler{run: func(ctx context.Context) { code = serveUntil(ctx, cfg, logf) }})
	if err != nil {
		fmt.Fprintf(logf, "service: %v\n", err)
		return exitError
	}
	return code
}

type handler struct{ run func(context.Context) }

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); h.run(ctx) }()
	status <- svc.Status{State: svc.StartPending}
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case <-done: // the server ended by itself: report failure so the manager restarts it
			status <- svc.Status{State: svc.StopPending}
			return false, 1
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 30000}
				cancel()
				<-done
				return false, 0
			}
		}
	}
}
