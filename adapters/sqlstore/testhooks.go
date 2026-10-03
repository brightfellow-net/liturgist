// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sqlstore

import "context"

// ExecForTest runs a statement outside any Tx. It is for tests (DDL, and
// states that no use case can reach yet); production code does not use it.
func (db *DB) ExecForTest(ctx context.Context, q string, args ...any) error {
	_, err := db.writer.ExecContext(ctx, db.d.Rebind(q), args...)
	return db.d.MapError(err)
}
