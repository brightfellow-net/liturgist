// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package web embeds the built frontend (web/dist). Only dist/.keep is committed;
// without a build the server serves a "frontend not built" page.
package web

import "embed"

// Dist holds the built frontend under "dist/".
//
//go:embed all:dist
var Dist embed.FS
