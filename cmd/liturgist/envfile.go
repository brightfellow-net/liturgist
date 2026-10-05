// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// envFileName is the settings file of the Windows service, in its folder.
const envFileName = "liturgist.env"

// parseEnvFile reads KEY=VALUE lines. Blank lines and lines starting with #
// are skipped, and one pair of surrounding quotes is removed. The Windows
// service has no shell to set variables, so it reads them from liturgist.env.
func parseEnvFile(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf") // Notepad's byte order mark
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out, sc.Err()
}

// applyEnv sets each variable of vars that the process does not already have,
// so a variable set in the environment wins over the file.
func applyEnv(vars map[string]string, lookup func(string) (string, bool), set func(k, v string) error) error {
	for k, v := range vars {
		if _, ok := lookup(k); ok {
			continue
		}
		if err := set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// applySettings reads dir/liturgist.env into the environment (variables that
// are already set win). found is false when the file does not exist.
func applySettings(dir string, lookup func(string) (string, bool), set func(k, v string) error) (found bool, err error) {
	f, err := os.Open(filepath.Join(dir, envFileName)) //nolint:gosec // G304: the settings folder is %ProgramData%\Liturgist, not user input
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	vars, err := parseEnvFile(f)
	if err != nil {
		return true, fmt.Errorf("%s: %w", envFileName, err)
	}
	return true, applyEnv(vars, lookup, set)
}

// defaultDataDir points LITURGIST_DATA_DIR at dir/data unless it is set.
func defaultDataDir(dir string, lookup func(string) (string, bool), set func(k, v string) error) error {
	if _, ok := lookup("LITURGIST_DATA_DIR"); ok {
		return nil
	}
	return set("LITURGIST_DATA_DIR", filepath.Join(dir, "data"))
}

// loadServiceSettings lets a command typed by hand use the settings and the
// data folder of an installed Windows service, so "liturgist backup" backs up
// the service's data. It does nothing when no service settings file exists,
// for commands that need no settings, and where there is no service (dir "").
func loadServiceSettings(args []string, stderr io.Writer, dir string, lookup func(string) (string, bool), set func(k, v string) error) bool {
	if dir == "" || len(args) == 0 {
		return true
	}
	switch args[0] {
	case "service", "version", "openapi":
		return true
	}
	found, err := applySettings(dir, lookup, set)
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return false
	}
	if found {
		_ = defaultDataDir(dir, lookup, set)
	}
	return true
}
