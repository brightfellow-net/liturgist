// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"io"

	"github.com/brightfellow-net/liturgist/domain"
)

// Extension points (04 §7). They are provisional [P-27]: signatures may change
// until the step that first uses them, and every change is listed under
// "Ports" in CHANGELOG.md. Types marked "placeholder" get their fields in
// that step. Entitlements is in entitlements.go.

// URLBuilder builds browser paths and shareable links (04 §4). route starts
// with "/" and has no church prefix.
type URLBuilder interface {
	AppPath(ctx context.Context, route string) string
	AppURL(ctx context.Context, route string) string
}

// AuthProvider authenticates a user. Password login stays in Auth.Login
// until a second provider (SSO, passkeys) arrives.
type AuthProvider interface {
	Authenticate(ctx context.Context, identifier, password string) (domain.UserID, error)
}

// Storage keeps files by key. Keys use only a-z, 0-9, '/', '_', '.' and '-',
// with no empty, "." or ".." segments; other keys are ErrInvalid. Open of a
// missing key is ErrNotFound; Delete of a missing key succeeds.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// EventBus passes messages between parts of the server (liturgy editor, step 3).
// Subscribe returns the message channel and a function that ends the
// subscription and closes the channel.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string) (<-chan []byte, func())
}

// NotifyEvent is what happened (placeholder; designed with publishing, step 5).
type NotifyEvent struct{}

// Message is one notification to deliver or copy (placeholder; step 5).
type Message struct{}

// Notifier turns an event into messages.
type Notifier interface {
	Compose(ctx context.Context, event NotifyEvent) ([]Message, error)
}

// BibleText is the text of a reading (placeholder; readings, step 2).
type BibleText struct{}

// ErrNotAvailable means no BibleTextProvider can supply the text.
var ErrNotAvailable = errors.New("bible text not available")

// BibleTextProvider looks up the text of a reading. None is registered in
// the community edition: use cases treat a nil provider as ErrNotAvailable.
type BibleTextProvider interface {
	Lookup(ctx context.Context, ref domain.Reference, translation string) (BibleText, error)
}

// PublishedVersion is a published liturgy (placeholder; step 5).
type PublishedVersion struct{}

// Exporter writes a published liturgy in a format (step 5).
type Exporter interface {
	Export(ctx context.Context, liturgy PublishedVersion, format string, w io.Writer) error
}

// ImportHint says what kind of document is imported (placeholder; step 2).
type ImportHint struct{}

// ImportCandidate is one item found in an imported document (placeholder; step 2).
type ImportCandidate struct{}

// Importer finds songs or readings in a document (step 2).
type Importer interface {
	Parse(ctx context.Context, r io.Reader, hint ImportHint) ([]ImportCandidate, error)
}
