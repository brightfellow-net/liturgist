// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build race

package app_test

// raceEnabled is true when the tests run under the race detector, which makes
// the sequential model tests about three times slower and finds nothing in them.
const raceEnabled = true
