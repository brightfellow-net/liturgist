// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package tenancy holds the community tenant resolver and URL builder (04 §3, §4).
package tenancy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// SingleChurch resolves every request to the install's only church [P-25].
// The ID is read from the database and cached after the first success; it is
// never taken from configuration.
type SingleChurch struct {
	Tx app.Tx

	mu sync.RWMutex
	id domain.ChurchID
}

// TooManyChurchesError reports a database with more than one church.
type TooManyChurchesError struct{ N int }

func (e *TooManyChurchesError) Error() string {
	return fmt.Sprintf("This database contains %d churches. The community edition serves exactly one. "+
		"Use one install per church, or the hosted edition.", e.N)
}

// Is makes errors.Is(err, app.ErrTooManyChurches) true.
func (e *TooManyChurchesError) Is(target error) bool { return target == app.ErrTooManyChurches }

// Resolve returns the church, or app.ErrNotSetUp before setup.
func (s *SingleChurch) Resolve(r *http.Request) (domain.ChurchID, error) {
	return s.ChurchID(r.Context())
}

// ChurchID is Resolve without a request (CLI commands).
func (s *SingleChurch) ChurchID(ctx context.Context) (domain.ChurchID, error) {
	s.mu.RLock()
	id := s.id
	s.mu.RUnlock()
	if id != "" {
		return id, nil
	}
	return s.load(ctx)
}

// Refresh forgets the cache and reloads (called after setup created the church).
func (s *SingleChurch) Refresh(ctx context.Context) error {
	s.mu.Lock()
	s.id = ""
	s.mu.Unlock()
	_, err := s.load(ctx)
	return err
}

// Check returns a TooManyChurchesError if the database holds more than one
// church (serve refuses to start, exit 7).
func (s *SingleChurch) Check(ctx context.Context) error {
	_, err := s.load(ctx)
	if errors.Is(err, app.ErrNotSetUp) {
		return nil
	}
	return err
}

func (s *SingleChurch) load(ctx context.Context) (domain.ChurchID, error) {
	var (
		ids []domain.ChurchID
		n   int
	)
	err := s.Tx.Read(ctx, func(st app.Store) error {
		var err error
		if ids, err = st.Churches().IDs(ctx, 2); err != nil || len(ids) < 2 {
			return err
		}
		n, err = st.Churches().Count(ctx)
		return err
	})
	switch {
	case err != nil:
		return "", err
	case len(ids) == 0:
		return "", app.ErrNotSetUp
	case len(ids) > 1:
		return "", &TooManyChurchesError{N: n}
	}
	s.mu.Lock()
	s.id = ids[0]
	s.mu.Unlock()
	return ids[0], nil
}

// NoPrefix is the community URLBuilder: routes have no church prefix.
type NoPrefix struct{ BaseURL *url.URL }

// AppPath returns route unchanged.
func (NoPrefix) AppPath(_ context.Context, route string) string { return route }

// AppURL returns the base URL followed by route.
func (b NoPrefix) AppURL(_ context.Context, route string) string {
	return strings.TrimSuffix(b.BaseURL.String(), "/") + route
}
