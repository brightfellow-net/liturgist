// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package localfs_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/storage/localfs"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// TC-T-006: keys are validated before any file is touched.
func TestKeys(t *testing.T) {
	for key, want := range map[string]bool{
		"logo.png": true, "church/01jabc/logo_v2.png": true, "a-b.c": true,
		"": false, "../etc/passwd": false, "a/../b": false, "./a": false, "/abs": false,
		"a//b": false, "a/": false, "Logo.png": false, `a\b`: false, "a b": false, "é": false,
		strings.Repeat("a", 513): false,
	} {
		if got := localfs.ValidKey(key); got != want {
			t.Errorf("ValidKey(%q) = %v", key, got)
		}
	}
	s := localfs.Storage{Dir: t.TempDir()}
	ctx := context.Background()
	if err := s.Put(ctx, "../etc/passwd", strings.NewReader("x")); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Put outside the folder: %v", err)
	}
	if _, err := s.Open(ctx, "Logo.png"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Open with upper case: %v", err)
	}
	if err := s.Delete(ctx, "a/../../b"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Delete outside the folder: %v", err)
	}
}

func TestPutOpenDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "files")
	s := localfs.Storage{Dir: dir}
	ctx := context.Background()
	if _, err := s.Open(ctx, "church/logo.png"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("open before put: %v", err)
	}
	for _, content := range []string{"first", "second"} {
		if err := s.Put(ctx, "church/logo.png", strings.NewReader(content)); err != nil {
			t.Fatal(err)
		}
	}
	f, err := s.Open(ctx, "church/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "second" {
		t.Errorf("content %q", b)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "church"))
	if len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}
	if err := s.Delete(ctx, "church/logo.png"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "church/logo.png"); err != nil {
		t.Errorf("delete again: %v", err)
	}
	if _, err := s.Open(ctx, "church/logo.png"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("open after delete: %v", err)
	}
	failing := io.MultiReader(strings.NewReader("part"), errReader{})
	if err := s.Put(ctx, "church/broken.png", failing); err == nil {
		t.Error("a failed copy must fail Put")
	}
	if _, err := s.Open(ctx, "church/broken.png"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("a failed Put must leave no file: %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("disk on fire") }

// TC-619: the key of a church's logo (the ID is an upper-case ULID) is a valid key.
func TestLogoKeyIsValid(t *testing.T) {
	key := domain.LogoKey("01M487A8PBK1DEN9Q93N6VB05D", "0123456789abcdef")
	if key != "church/01m487a8pbk1den9q93n6vb05d/logo-0123456789abcdef.png" || !localfs.ValidKey(key) {
		t.Errorf("key %q", key)
	}
}
