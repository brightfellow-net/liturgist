// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package localfs

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/brightfellow-net/liturgist/app"
)

// A full disk is app.ErrStorageFull (the problem is 507 storage_full); other errors are unchanged.
func TestFullDisk(t *testing.T) {
	full := &os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC}
	if err := fullDisk(full); !errors.Is(err, app.ErrStorageFull) || !errors.Is(err, syscall.ENOSPC) {
		t.Errorf("ENOSPC: %v", err)
	}
	other := &os.PathError{Op: "write", Path: "x", Err: syscall.EACCES}
	if err := fullDisk(other); !errors.Is(err, other) || errors.Is(err, app.ErrStorageFull) {
		t.Errorf("other error changed: %v", err)
	}
	if err := fullDisk(nil); err != nil {
		t.Errorf("nil: %v", err)
	}
}
