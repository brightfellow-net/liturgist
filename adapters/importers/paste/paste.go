// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package paste

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// Importer splits pasted lyrics; hint.Name is the title.
type Importer struct{}

// ReadText reads r as UTF-8 and removes a byte order mark (08 §4).
func ReadText(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", &app.ImportUnreadableError{Reason: app.ImportNotUTF8}
	}
	return strings.TrimPrefix(string(b), "\xEF\xBB\xBF"), nil
}

// Parse implements app.Importer.
func (Importer) Parse(ctx context.Context, r io.Reader, hint app.ImportHint) ([]app.ImportCandidate, error) {
	text, err := ReadText(r)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res := Build(Blocks(text), hint.Language)
	switch {
	case res.TooMany:
		return []app.ImportCandidate{{Reject: app.ImportTooManySections}}, nil
	case len(res.Sections) == 0:
		return nil, &app.ImportUnreadableError{Reason: app.ImportNoSong}
	}
	d := domain.SongDraft{Language: hint.Language, Title: strings.TrimSpace(hint.Name), AltTitles: []string{},
		Sections: res.Sections, DefaultArrangement: res.Arrangement}
	if d.DefaultArrangement == nil {
		d.DefaultArrangement = []int{}
	}
	return []app.ImportCandidate{{Draft: d, Warnings: res.Warnings}}, nil
}
