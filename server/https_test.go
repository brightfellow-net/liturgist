// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// selfSigned is a certificate for the test domain and the pool that trusts it.
func selfSigned(t *testing.T, domain string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// A server with a Domain serves the app over HTTPS and, on the plain port,
// the challenge handler, /healthz and a redirect.
func TestBuiltInHTTPS(t *testing.T) {
	const domain = "liturgi.test"
	cert, pool := selfSigned(t, domain)
	httpPort, httpsPort := freePort(t), freePort(t)

	cfg := testConfig(t, "https://"+domain, nil)
	cfg.Domain, cfg.HTTPPort, cfg.HTTPSPort = domain, httpPort, httpsPort
	srv, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	var challenged bool
	srv.tlsSource = func(context.Context) (*tls.Config, func(http.Handler) http.Handler, error) {
		wrap := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/acme-challenge/tok" {
					challenged = true
					_, _ = w.Write([]byte("key-authorization"))
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}}, wrap, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(cancel)

	// The names resolve to the local machine through a custom dialer.
	dial := func(_ context.Context, network, addr string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(addr)
		return net.Dial(network, "127.0.0.1:"+port)
	}
	client := &http.Client{
		Transport:     &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{RootCAs: pool}, ForceAttemptHTTP2: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	get := func(url string) (*http.Response, string) {
		t.Helper()
		var res *http.Response
		var err error
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if res, err = client.Get(url); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	https := "https://" + domain + ":" + strconv.Itoa(httpsPort)
	plain := "http://" + domain + ":" + strconv.Itoa(httpPort)

	if res, body := get(https + "/healthz"); res.StatusCode != 200 || body != "ok" || res.ProtoMajor != 2 {
		t.Fatalf("https healthz: %d %q proto %d", res.StatusCode, body, res.ProtoMajor)
	}
	if res, _ := get(https + "/api/v1/system/status"); res.Header.Get("Strict-Transport-Security") == "" {
		t.Fatalf("no HSTS header on the HTTPS server: %v", res.Header)
	}
	if res, body := get(plain + "/healthz"); res.StatusCode != 200 || body != "ok" {
		t.Fatalf("plain healthz: %d %q", res.StatusCode, body)
	}
	res, _ := get(plain + "/songs?q=a%20b")
	if res.StatusCode != http.StatusPermanentRedirect || res.Header.Get("Location") != https+"/songs?q=a%20b" {
		t.Fatalf("redirect: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	if res, body := get(plain + "/.well-known/acme-challenge/tok"); res.StatusCode != 200 || body != "key-authorization" || !challenged {
		t.Fatalf("challenge: %d %q", res.StatusCode, body)
	}
	// The app is not served on the plain port.
	if res, body := get(plain + "/api/v1/system/status"); res.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("app on the plain port: %d %q", res.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestRedirectKeepsDefaultPortOut(t *testing.T) {
	h := redirectToHTTPS("liturgi.example.org", 443)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://liturgi.example.org/a?b=1", nil))
	if loc := rec.Header().Get("Location"); loc != "https://liturgi.example.org/a?b=1" {
		t.Fatalf("Location %q", loc)
	}
}

func TestNoWarningsWithBuiltInHTTPS(t *testing.T) {
	cfg := testConfig(t, "https://liturgi.example.org", nil)
	cfg.Domain = "liturgi.example.org"
	cfg.Listen = ":8080"
	if w := StartupWarnings(withDefaults(cfg)); len(w) != 0 {
		t.Fatalf("%v", w)
	}
	cfg.Domain = ""
	if w := StartupWarnings(withDefaults(cfg)); !strings.Contains(strings.Join(w, ""), "BASE_URL") {
		t.Fatalf("without a domain the proxy warning stays: %v", w)
	}
}

func TestSystemStatusModes(t *testing.T) {
	for _, tc := range []struct{ base, domain, want string }{
		{"http://localhost:8080", "", "plain_http"},
		{"http://localhost:8080", "liturgi.test", "built_in"}, // the harness serves the setup page over http
	} {
		var logs bytes.Buffer
		cfg := testConfig(t, tc.base, &logs)
		cfg.Domain = tc.domain
		cfg.Listen = ":8080" // reachable on the network: without a Domain this warns
		srv, err := New(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = srv.Close() })
		h := harness{t: t, srv: srv, h: srv.Handler(), log: &logs}
		rec := h.get("/api/v1/system/status", h.setupChurch())
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", tc.want, rec.Code, rec.Body.String())
		}
		hs := decode(t, rec)["https"].(map[string]any)
		if hs["mode"] != tc.want {
			t.Errorf("%s/%q: mode %v, want %s", tc.base, tc.domain, hs["mode"], tc.want)
		}
		if tc.want == "built_in" && (hs["plain_http_warning"] != false || hs["proxy_missing_warning"] != false) {
			t.Errorf("warnings with built-in HTTPS: %v", hs)
		}
	}
}

// Certificates are issued again when missing, so a backup does not carry them.
func TestBackupLeavesOutCertificates(t *testing.T) {
	cfg := seeded(t)
	write(t, filepath.Join(cfg.DataDir, "certs", "certificates", "x", "x.key"), "private key")
	res, err := Backup(context.Background(), cfg, filepath.Join(t.TempDir(), "church.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if strings.Contains(f.Name, "certs") || strings.Contains(f.Name, ".key") {
			t.Errorf("archive holds %q", f.Name)
		}
	}
}
