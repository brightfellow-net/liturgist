// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package migrations embeds the SQL migrations; both folders must hold the same versions (02 §5).
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed all:sqlite all:postgres
var files embed.FS

// For returns the migrations for a dialect ("sqlite" | "postgres").
func For(dialect string) fs.FS {
	sub, err := fs.Sub(files, dialect)
	if err != nil {
		panic(err) // the folder names are fixed
	}
	return sub
}
