// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
)

// OptString tells an omitted field apart from an explicit null (04 §6 PATCH semantics).
type OptString struct {
	Set   bool // the key was present
	Null  bool // the value was null
	Value string
}

// UnmarshalJSON is only called when the key is present.
func (o *OptString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

// Schema describes a nullable string for OpenAPI and request validation.
func (OptString) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Nullable: true}
}
