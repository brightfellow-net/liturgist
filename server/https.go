// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/caddyserver/certmagic"
)

// tlsSource gives the TLS settings of the built-in HTTPS and the wrapper that
// answers certificate-authority challenges on the plain port. Tests replace it.
type tlsSource func(ctx context.Context) (*tls.Config, func(http.Handler) http.Handler, error)

// builtInHTTPS serves the app over HTTPS on HTTPSPort, and on HTTPPort only
// the certificate challenges, /healthz and a redirect to HTTPS (14 §19).
func (s *Server) builtInHTTPS(ctx context.Context) ([]listener, error) {
	src := s.tlsSource
	if src == nil {
		src = s.acmeTLS
	}
	tlsConf, challenge, err := src(ctx)
	if err != nil {
		return nil, fmt.Errorf("built-in HTTPS: %w", err)
	}
	httpsAddr := ":" + strconv.Itoa(s.cfg.HTTPSPort)
	httpAddr := ":" + strconv.Itoa(s.cfg.HTTPPort)

	secure := newHTTPServer(ctx, httpsAddr, s.handler)
	secure.TLSConfig = tlsConf
	front := newHTTPServer(ctx, httpAddr, challenge(redirectToHTTPS(s.cfg.Domain, s.cfg.HTTPSPort)))

	s.cfg.Logger.Info("listening", "https", httpsAddr, "http", httpAddr, "domain", s.cfg.Domain, "version", Version)
	return []listener{
		{secure, func() error { return secure.ListenAndServeTLS("", "") }},
		{front, front.ListenAndServe},
	}, nil
}

// redirectToHTTPS answers every request but /healthz and /readyz with a
// redirect to the same path on the domain. /healthz stays plain so that
// "liturgist healthcheck" and Docker need no certificate.
func redirectToHTTPS(domain string, httpsPort int) http.Handler {
	host := domain
	if httpsPort != 443 {
		host = net.JoinHostPort(domain, strconv.Itoa(httpsPort))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}

// acmeTLS gets and renews the certificate of cfg.Domain from Let's Encrypt (or
// cfg.ACMECA) with certmagic. Certificates are kept in <DataDir>/certs; they
// are not part of a backup because they can be issued again.
func (s *Server) acmeTLS(ctx context.Context) (*tls.Config, func(http.Handler) http.Handler, error) {
	magic := certmagic.NewDefault()
	magic.Storage = &certmagic.FileStorage{Path: filepath.Join(s.cfg.DataDir, "certs")}
	acme := certmagic.DefaultACME
	acme.Agreed = true // setting LITURGIST_DOMAIN accepts the CA's subscriber agreement (guide)
	acme.Email = s.cfg.ACMEEmail
	if s.cfg.ACMECA != "" {
		acme.CA = s.cfg.ACMECA
	}
	// The challenges are answered by our own servers on these ports, even when
	// they are not 80 and 443 (a port mapping in front, e.g. Docker); certmagic
	// then does not start listeners of its own.
	acme.AltHTTPPort, acme.AltTLSALPNPort = s.cfg.HTTPPort, s.cfg.HTTPSPort
	issuer := certmagic.NewACMEIssuer(magic, acme)
	magic.Issuers = []certmagic.Issuer{issuer}
	if err := magic.ManageAsync(ctx, []string{s.cfg.Domain}); err != nil {
		return nil, nil, err
	}
	return magic.TLSConfig(), issuer.HTTPChallengeHandler, nil
}
