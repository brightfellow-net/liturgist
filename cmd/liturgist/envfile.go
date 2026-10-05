// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

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
