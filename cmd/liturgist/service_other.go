// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"fmt"
	"io"
)

func serviceCmd(_ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "liturgist service is only supported on Windows; on Linux use the systemd unit (deploy/liturgist.service)")
	return exitError
}

// serviceDir is empty where there is no Windows service to read settings from.
func serviceDir() string { return "" }
