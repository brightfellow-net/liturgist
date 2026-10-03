// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

// BookKey exposes the comparison form of a book spelling to the tests.
func BookKey(s string) string { return bookKey(s) }
