// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package localfs is the community Storage: files in a local folder,
// <DataDir>/files (04 §7).
package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/brightfellow-net/liturgist/app"
)

// Storage keeps files under Dir, which is created on the first Put.
type Storage struct{ Dir string }

var _ app.Storage = Storage{}

// ValidKey reports whether key may be used: only a-z, 0-9, '/', '_', '.' and
// '-', at most 512 bytes, and no empty, "." or ".." segments (so no leading or
// trailing '/', no "//" and no way out of Dir).
func ValidKey(key string) bool {
	if key == "" || len(key) > 512 {
		return false
	}
	for _, c := range key {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && !strings.ContainsRune("/_.-", c) {
			return false
		}
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func (s Storage) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("storage key %q: %w", key, app.ErrInvalid)
	}
	return filepath.Join(s.Dir, filepath.FromSlash(key)), nil
}

// Put writes r to key, replacing any earlier file. The file appears only
// when it is complete (written to a temporary file, then renamed).
func (s Storage) Put(ctx context.Context, key string, r io.Reader) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after the rename
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return fullDisk(err)
	}
	if err := tmp.Close(); err != nil {
		return fullDisk(err)
	}
	return fullDisk(os.Rename(tmp.Name(), p))
}

// fullDisk marks a "no space left" error as app.ErrStorageFull (14 §4); any
// other error is returned as it is.
func fullDisk(err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == syscall.ENOSPC || (runtime.GOOS == "windows" && (errno == 39 || errno == 112))) {
		return fmt.Errorf("%w: %w", app.ErrStorageFull, err)
	}
	return err
}

// Open returns the file at key; app.ErrNotFound if there is none.
func (s Storage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // p is Dir joined with a validated key
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("storage key %q: %w", key, app.ErrNotFound)
	}
	return f, err
}

// Delete removes the file at key; a missing file is not an error.
func (s Storage) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
