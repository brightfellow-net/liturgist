// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"encoding/json"
	"fmt"
)

// jsonValue scans a JSON column from TEXT (SQLite) or jsonb (PostgreSQL) into v.
type jsonValue struct{ v any }

func (j jsonValue) Scan(src any) error {
	switch b := src.(type) {
	case string:
		return json.Unmarshal([]byte(b), j.v)
	case []byte:
		return json.Unmarshal(b, j.v)
	case nil:
		return nil
	}
	return fmt.Errorf("cannot scan %T as JSON", src)
}

func jsonArg(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
