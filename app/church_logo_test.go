// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"errors"
	stdimage "image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/images"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// memStorage is an app.Storage in memory whose calls can be made to fail.
type memStorage struct {
	files   map[string][]byte
	putErr  error
	delErr  error
	deleted []string
}

func newMemStorage() *memStorage { return &memStorage{files: map[string][]byte{}} }

func (m *memStorage) Put(_ context.Context, key string, r io.Reader) error {
	if m.putErr != nil {
		return m.putErr
	}
	b, err := io.ReadAll(r)
	m.files[key] = b
	return err
}

func (m *memStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.files[key]
	if !ok {
		return nil, app.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.deleted = append(m.deleted, key)
	delete(m.files, key)
	return nil
}

// logoPNG is a w × h PNG of one colour.
func logoPNG(t testing.TB, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := stdimage.NewNRGBA(stdimage.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func logosOf(e cenv, st app.Storage, log *slog.Logger) *app.ChurchLogos {
	return &app.ChurchLogos{Tx: e.db, Clock: e.clock, Storage: st, Images: images.Normalizer{}, Log: log}
}

func churchLogo(t *testing.T, e cenv) *domain.ChurchLogo {
	t.Helper()
	res, err := e.churches.Get(e.ctx, e.admin)
	if err != nil {
		t.Fatal(err)
	}
	return res.Church.Settings.Logo
}

// TC-617: set, replace, remove, permission, and other settings keep the logo.
func TestChurchLogoUseCases(t *testing.T) {
	sqlstoretest.ForEachDialect(t, func(t *testing.T, db *sqlstore.DB) {
		e := newChurch(t, db, nil)
		st := newMemStorage()
		logos := logosOf(e, st, nil)
		red := logoPNG(t, 1000, 500, color.NRGBA{R: 200, A: 255})
		blue := logoPNG(t, 200, 200, color.NRGBA{B: 200, A: 255})

		if _, err := logos.Open(e.ctx, e.admin); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("open without a logo: %v", err)
		}
		res, err := logos.Set(e.ctx, e.admin, red)
		if err != nil {
			t.Fatal(err)
		}
		first := res.Church.Settings.Logo
		if first == nil || first.Width != 512 || first.Height != 256 || len(first.Version) != 16 {
			t.Fatalf("logo after set: %+v", first)
		}
		key := domain.LogoKey(e.church, first.Version)
		if len(st.files) != 1 || st.files[key] == nil {
			t.Fatalf("files %v, want only %s", st.files, key)
		}

		f, err := logos.Open(e.ctx, e.admin)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(f)
		_ = f.Close()
		if f.Version != first.Version || !bytes.Equal(body, st.files[key]) {
			t.Errorf("open gave version %s and %d bytes", f.Version, len(body))
		}

		// Other settings do not lose the logo.
		name := "GKY Uji"
		if _, err := e.churches.Update(e.ctx, e.admin, app.ChurchChange{Name: &name}); err != nil {
			t.Fatal(err)
		}
		if l := churchLogo(t, e); l == nil || l.Version != first.Version {
			t.Errorf("logo after a PATCH of the name: %+v", l)
		}

		// Replacing writes a new file and deletes the old one.
		res, err = logos.Set(e.ctx, e.admin, blue)
		if err != nil {
			t.Fatal(err)
		}
		second := res.Church.Settings.Logo
		if second.Version == first.Version || second.Width != 200 {
			t.Fatalf("replaced logo: %+v", second)
		}
		if len(st.files) != 1 || st.files[domain.LogoKey(e.church, second.Version)] == nil || len(st.deleted) != 1 {
			t.Errorf("after replacing: files %d, deleted %v", len(st.files), st.deleted)
		}

		// The same image again changes nothing and deletes nothing.
		st.deleted = nil
		if _, err := logos.Set(e.ctx, e.admin, blue); err != nil {
			t.Fatal(err)
		}
		if len(st.files) != 1 || len(st.deleted) != 0 || churchLogo(t, e).Version != second.Version {
			t.Errorf("same image again: files %d, deleted %v", len(st.files), st.deleted)
		}

		// Remove, then remove again.
		if res, err = logos.Remove(e.ctx, e.admin); err != nil || res.Church.Settings.Logo != nil || len(st.files) != 0 {
			t.Fatalf("remove: %v, logo %+v, files %d", err, res.Church.Settings.Logo, len(st.files))
		}
		updated := res.Church.UpdatedAt
		if res, err = logos.Remove(e.ctx, e.admin); err != nil || !res.Church.UpdatedAt.Equal(updated) {
			t.Errorf("second remove: %v, updated_at changed %v -> %v", err, updated, res.Church.UpdatedAt)
		}
	})
}

// TC-617: only church.settings changes the logo; every member can read it.
func TestChurchLogoPermission(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	st := newMemStorage()
	logos := logosOf(e, st, nil)
	img := logoPNG(t, 100, 100, color.NRGBA{G: 200, A: 255})
	if _, err := logos.Set(e.ctx, e.admin, img); err != nil {
		t.Fatal(err)
	}
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID)

	if _, err := logos.Set(e.ctx, editor, img); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("set by an editor: %v", err)
	}
	if _, err := logos.Remove(e.ctx, editor); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("remove by an editor: %v", err)
	}
	if f, err := logos.Open(e.ctx, editor); err != nil {
		t.Errorf("open by an editor: %v", err)
	} else {
		_ = f.Close()
	}
	if _, err := logos.Open(e.ctx, nil); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("open without a session: %v", err)
	}
	// A refused sender must not cost a decode: junk gives forbidden, not invalid input.
	if _, err := logos.Set(e.ctx, editor, []byte("junk")); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("junk by an editor: %v", err)
	}
}

// TC-617: what the sender can fix is invalid input and changes nothing.
func TestChurchLogoInvalid(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	st := newMemStorage()
	logos := logosOf(e, st, nil)
	// A real PNG with padding after it (the decoder ignores that), so only the size limit can refuse it.
	big := append(logoPNG(t, 64, 64, color.NRGBA{R: 9, A: 255}), make([]byte, domain.LogoMaxBytes)...)
	for name, c := range map[string]struct {
		data []byte
		msg  string
	}{
		"empty":        {nil, "Choose an image."},
		"too big":      {big, "Use an image under 2 MB."},
		"not an image": {[]byte("hello"), "Use a PNG, JPEG or WebP image."},
	} {
		_, err := logos.Set(e.ctx, e.admin, c.data)
		var in *domain.InvalidInputError
		if !errors.As(err, &in) || in.Field != "image" || in.Message != c.msg {
			t.Errorf("%s: %v, want %q", name, err, c.msg)
		}
	}
	if len(st.files) != 0 || churchLogo(t, e) != nil {
		t.Errorf("a refused image left files %d or a logo", len(st.files))
	}
}

// TC-618: a failing Storage.
func TestChurchLogoStorageFailures(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	st := newMemStorage()
	var logbuf strings.Builder
	logos := logosOf(e, st, slog.New(slog.NewTextHandler(&logbuf, nil)))
	one := logoPNG(t, 64, 64, color.NRGBA{R: 1, A: 255})
	two := logoPNG(t, 64, 64, color.NRGBA{R: 2, A: 255})
	if _, err := logos.Set(e.ctx, e.admin, one); err != nil {
		t.Fatal(err)
	}
	before := churchLogo(t, e).Version

	// Put fails: the setting stays.
	st.putErr = app.ErrStorageFull
	if _, err := logos.Set(e.ctx, e.admin, two); !errors.Is(err, app.ErrStorageFull) {
		t.Fatalf("put failing: %v", err)
	}
	if churchLogo(t, e).Version != before {
		t.Error("the setting changed although the file was not written")
	}
	st.putErr = nil

	// Delete of the old file fails: the request succeeds, a warning is logged, the file stays.
	st.delErr = errors.New("disk on fire")
	res, err := logos.Set(e.ctx, e.admin, two)
	if err != nil {
		t.Fatalf("set with a failing delete: %v", err)
	}
	if res.Church.Settings.Logo.Version == before || len(st.files) != 2 || !strings.Contains(logbuf.String(), "logo_file_not_deleted") {
		t.Errorf("version %s, files %d, log %q", res.Church.Settings.Logo.Version, len(st.files), logbuf.String())
	}

	// The file is gone: open says not found.
	for k := range st.files {
		delete(st.files, k)
	}
	if _, err := logos.Open(e.ctx, e.admin); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("open with a missing file: %v", err)
	}
}
