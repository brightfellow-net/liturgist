// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type entry struct {
	name string
	body string
	mode os.FileMode
}

// craft writes a zip with a valid manifest for db and the given extra entries.
func craft(t *testing.T, dbBody string, extra ...entry) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "src.db")
	writeFile(t, dbPath, dbBody)
	sum, err := sha256File(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mb, _ := json.Marshal(Manifest{Format: Format, Program: "test", Schema: 1, CreatedAt: time.Now(), Driver: "sqlite", SHA256: sum})
	out := filepath.Join(dir, "x.zip")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	all := append([]entry{{manifestName, string(mb), 0o600}, {dbName, dbBody, 0o600}}, extra...)
	for _, e := range all {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return out
}

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	db := filepath.Join(src, "snap.db")
	writeFile(t, db, "database bytes")
	files := filepath.Join(src, "files")
	writeFile(t, filepath.Join(files, "a.txt"), "aaa")
	writeFile(t, filepath.Join(files, "sub", "b.txt"), "bbb")

	dest := filepath.Join(t.TempDir(), "out.zip")
	m := Manifest{Program: "v1", Schema: 7, CreatedAt: time.Now().UTC(), Driver: "sqlite"}
	if err := CreateFile(dest, m, db, files, func(s string) { t.Error("warning:", s) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".partial"); err == nil {
		t.Error("partial file left behind")
	}
	a, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if a.Manifest.Schema != 7 || a.Manifest.Format != Format || a.Size != int64(len("database bytes")+6) {
		t.Errorf("manifest or size: %+v %d", a.Manifest, a.Size)
	}
	out := t.TempDir()
	if err := a.Extract(out); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{"liturgist.db": "database bytes", "files/a.txt": "aaa", "files/sub/b.txt": "bbb"} {
		got, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(p)))
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v", p, got, err)
		}
	}
	// An existing destination is never overwritten, and no partial file stays.
	if err := CreateFile(dest, m, db, files, nil); !errors.Is(err, os.ErrExist) {
		t.Errorf("existing destination: %v", err)
	}
	if _, err := os.Stat(dest + ".partial"); err == nil {
		t.Error("partial file left after the refusal")
	}
}

// TC-601
func TestUnsafeEntriesRefused(t *testing.T) {
	for name, e := range map[string]entry{
		"dot dot":   {"files/../../evil", "x", 0o600},
		"absolute":  {"files//etc/passwd", "x", 0o600},
		"backslash": {`files/..\evil`, "x", 0o600},
		"drive":     {"files/C:evil", "x", 0o600},
		"unknown":   {"other.txt", "x", 0o600},
		"symlink":   {"files/link", "/etc/passwd", os.ModeSymlink | 0o777},
		"dot":       {"files/./a", "x", 0o600},
	} {
		t.Run(name, func(t *testing.T) {
			a, err := Open(craft(t, "db", e))
			if err == nil {
				_ = a.Close()
				t.Fatal("opened")
			}
			if !errors.Is(err, ErrDamaged) {
				t.Errorf("error %v is not ErrDamaged", err)
			}
		})
	}
	t.Run("duplicate", func(t *testing.T) {
		if _, err := Open(craft(t, "db", entry{"files/a", "1", 0o600}, entry{"files/a", "2", 0o600})); !errors.Is(err, ErrDamaged) {
			t.Errorf("duplicate: %v", err)
		}
	})
	t.Run("not a zip", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "x.zip")
		writeFile(t, p, "plain text")
		if _, err := Open(p); !errors.Is(err, ErrDamaged) {
			t.Errorf("not a zip: %v", err)
		}
	})
}

// TC-602 (checksum part)
func TestChecksumMismatch(t *testing.T) {
	good := craft(t, "the real database")
	// Same manifest (checksum of "the real database"), different database.
	r, err := zip.OpenReader(good)
	if err != nil {
		t.Fatal(err)
	}
	var manifest []byte
	for _, f := range r.File {
		if f.Name == manifestName {
			rc, _ := f.Open()
			manifest = make([]byte, f.UncompressedSize64)
			_, _ = rc.Read(manifest)
			_ = rc.Close()
		}
	}
	_ = r.Close()
	bad := craft(t, "a swapped database")
	// Replace the manifest of bad by the one of good.
	var m Manifest
	_ = json.Unmarshal(manifest, &m)
	a, err := Open(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	a.Manifest.SHA256 = m.SHA256
	if err := a.Extract(t.TempDir()); !errors.Is(err, ErrDamaged) {
		t.Errorf("mismatch: %v", err)
	}
}

func TestVanishedFileSkipped(t *testing.T) {
	src := t.TempDir()
	db := filepath.Join(src, "snap.db")
	writeFile(t, db, "d")
	files := filepath.Join(src, "files")
	writeFile(t, filepath.Join(files, "a.txt"), "a")
	if err := os.Symlink("/etc/passwd", filepath.Join(files, "link")); err != nil {
		t.Skip("no symlinks:", err)
	}
	var warned []string
	dest := filepath.Join(t.TempDir(), "o.zip")
	if err := CreateFile(dest, Manifest{Driver: "sqlite"}, db, files, func(s string) { warned = append(warned, s) }); err != nil {
		t.Fatal(err)
	}
	if len(warned) != 1 {
		t.Errorf("warnings: %v", warned)
	}
	a, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if a.Size != 2 { // db "d" + a.txt "a"; the link is not in the archive
		t.Errorf("size %d", a.Size)
	}
}
