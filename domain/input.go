// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

// InvalidInputError reports a field that failed a domain rule; it maps to
// 422 validation_failed with the field in errors[].location.
type InvalidInputError struct {
	Field   string // JSON path below the body, e.g. "name" or "preferences.text_size"
	Message string
}

func (e *InvalidInputError) Error() string { return e.Field + ": " + e.Message }
