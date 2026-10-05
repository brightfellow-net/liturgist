// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package backup writes and reads the backup archive of 14 §3: one zip with
// manifest.json, liturgist.db and files/….
package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Format is the archive format this package writes and reads.
const Format = 1

const (
	manifestName = "manifest.json"
	dbName       = "liturgist.db"
	filesPrefix  = "files/"
)

// Manifest describes an archive.
type Manifest struct {
	Format    int       `json:"format"`
	Program   string    `json:"program"` // version of the program that wrote it
	Schema    int64     `json:"schema"`  // schema version of the database inside
	CreatedAt time.Time `json:"created_at"`
	Driver    string    `json:"driver"`
	SHA256    string    `json:"sha256"` // of liturgist.db
}

// ErrDamaged means the archive is unreadable, has unexpected content or fails
// its checksum.
var ErrDamaged = errors.New("this backup is damaged")

func damaged(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrDamaged, fmt.Sprintf(format, a...))
}

// Write writes the archive to w: the manifest (with the SHA-256 of the
// database file at dbPath filled in), the database, then every regular file
// below filesDir. A file that disappears while it is read is skipped and
// reported to warn; so are links and other irregular files.
func Write(w io.Writer, m Manifest, dbPath, filesDir string, warn func(string)) error {
	sum, err := sha256File(dbPath)
	if err != nil {
		return err
	}
	m.Format, m.SHA256 = Format, sum
	zw := zip.NewWriter(w)
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: manifestName, Method: zip.Deflate, Modified: m.CreatedAt})
	if err != nil {
		return err
	}
	if _, err := mw.Write(mb); err != nil {
		return err
	}
	if err := addFile(zw, dbName, dbPath, m.CreatedAt); err != nil {
		return err
	}
	if filesDir != "" {
		err = filepath.WalkDir(filesDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil // the folder is optional, or the entry vanished
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(filesDir, p)
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				warn("skipped " + rel + ": not a regular file")
				return nil
			}
			err = addFile(zw, filesPrefix+filepath.ToSlash(rel), p, m.CreatedAt)
			if errors.Is(err, fs.ErrNotExist) {
				warn("skipped " + rel + ": it was deleted during the backup")
				return nil
			}
			return err
		})
		if err != nil {
			return err
		}
	}
	return zw.Close()
}

func addFile(zw *zip.Writer, name, src string, modified time.Time) error {
	f, err := os.Open(src) //nolint:gosec // the caller's own data folder
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zf, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
	if err != nil {
		return err
	}
	_, err = io.Copy(zf, f)
	return err
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p) //nolint:gosec // the caller's own data folder
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CreateFile writes the archive to dest, which must not exist: first to
// dest+".partial", synced, then renamed. A failure removes the partial file.
func CreateFile(dest string, m Manifest, dbPath, filesDir string, warn func(string)) (err error) {
	tmp := dest + ".partial"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // chosen by the operator
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if err = Write(f, m, dbPath, filesDir, warn); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if _, statErr := os.Lstat(dest); statErr == nil {
		return fs.ErrExist
	}
	return os.Rename(tmp, dest)
}

// Archive is an opened archive that passed the checks of Open.
type Archive struct {
	zr       *zip.ReadCloser
	Manifest Manifest
	// Size is the total uncompressed size of the database and the files.
	Size int64
}

// Close closes the archive.
func (a *Archive) Close() error { return a.zr.Close() }

// Open opens the archive at path and checks its structure: every entry is the
// manifest, the database or a regular file under files/ with a local, plain
// name (no "..", absolute paths, backslashes, links or duplicates), and the
// manifest is complete. The database's checksum is checked by Extract.
func Open(p string) (*Archive, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, damaged("%v", err)
	}
	a := &Archive{zr: zr}
	if err := a.check(); err != nil {
		_ = zr.Close()
		return nil, err
	}
	return a, nil
}

func (a *Archive) check() error {
	seen := map[string]bool{}
	var manifest, database *zip.File
	for _, f := range a.zr.File {
		name := f.Name
		if seen[name] {
			return damaged("duplicate entry %q", name)
		}
		seen[name] = true
		if f.Mode()&fs.ModeType != 0 && !f.FileInfo().IsDir() {
			return damaged("entry %q is not a regular file", name)
		}
		switch {
		case name == manifestName:
			manifest = f
		case name == dbName:
			database = f
			a.Size += int64(f.UncompressedSize64) //nolint:gosec // bounded by the free-space check
		case strings.HasPrefix(name, filesPrefix):
			if f.FileInfo().IsDir() {
				continue
			}
			if !localName(strings.TrimPrefix(name, filesPrefix)) {
				return damaged("unsafe entry name %q", name)
			}
			a.Size += int64(f.UncompressedSize64) //nolint:gosec // bounded by the free-space check
		default:
			return damaged("unexpected entry %q", name)
		}
	}
	if manifest == nil || database == nil {
		return damaged("it has no manifest or no database")
	}
	rc, err := manifest.Open()
	if err != nil {
		return damaged("%v", err)
	}
	defer func() { _ = rc.Close() }()
	if err := json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&a.Manifest); err != nil {
		return damaged("manifest: %v", err)
	}
	if a.Manifest.SHA256 == "" {
		return damaged("manifest has no checksum")
	}
	return nil
}

// localName reports whether name is a relative slash path that stays inside
// its folder and has no backslash, drive letter or empty or dot element.
func localName(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || path.Clean(name) != name {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(name))
}

// Extract writes the database to dir/liturgist.db and the files to dir/files/,
// and checks the database's SHA-256 against the manifest. dir must exist.
func (a *Archive) Extract(dir string) error {
	for _, f := range a.zr.File {
		var dest string
		switch {
		case f.Name == dbName:
			dest = filepath.Join(dir, dbName)
		case strings.HasPrefix(f.Name, filesPrefix) && !f.FileInfo().IsDir():
			dest = filepath.Join(dir, filepath.FromSlash(f.Name))
		default:
			continue
		}
		if err := extractOne(f, dest); err != nil {
			return err
		}
	}
	sum, err := sha256File(filepath.Join(dir, dbName))
	if err != nil {
		return err
	}
	if sum != a.Manifest.SHA256 {
		return damaged("the database does not match its checksum")
	}
	return nil
}

func extractOne(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return damaged("%v", err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // inside the restore folder: names were checked
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, readErrors{rc}); err != nil {
		_ = out.Close()
		var re *readError
		if errors.As(err, &re) {
			return damaged("%v", re.err)
		}
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// readError marks an error from reading the archive, as against writing the
// extracted file: a bad checksum, a corrupt compressed stream or a short entry.
type readError struct{ err error }

func (e *readError) Error() string { return e.err.Error() }

type readErrors struct{ r io.Reader }

func (r readErrors) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF { //nolint:errorlint // io.EOF is returned as is
		err = &readError{err}
	}
	return n, err
}

// TreeSize is the total size of the regular files below dir (0 if it does not exist).
func TreeSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
