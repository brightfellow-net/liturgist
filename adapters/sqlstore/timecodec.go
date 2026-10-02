// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"fmt"
	"time"
)

// Time scans timestamps from either dialect: SQLite returns fixed-format text,
// PostgreSQL returns time.Time. Values are always UTC (02 §4). Bind timestamps
// with Dialect.TimeArg, never directly.
type Time struct{ time.Time }

// Scan implements sql.Scanner.
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		t.Time = v.UTC()
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	default:
		return fmt.Errorf("cannot scan %T into Time", src)
	}
	return nil
}

func (t *Time) parse(s string) error {
	p, err := time.Parse(sqliteTimeLayout, s)
	if err != nil {
		return fmt.Errorf("bad timestamp %q: %w", s, err)
	}
	t.Time = p
	return nil
}

// NullTime is a Time that may be NULL.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements sql.Scanner.
func (n *NullTime) Scan(src any) error {
	if src == nil {
		n.Time, n.Valid = time.Time{}, false
		return nil
	}
	var t Time
	if err := t.Scan(src); err != nil {
		return err
	}
	n.Time, n.Valid = t.Time, true
	return nil
}
