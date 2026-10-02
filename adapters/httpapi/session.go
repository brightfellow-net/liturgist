// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

const (
	cookieSecure   = "__Host-liturgist_session"
	cookieInsecure = "liturgist_session"
)

// Cookies builds session cookies (03 §4). Secure follows the BaseURL scheme.
type Cookies struct{ Secure bool }

func (c Cookies) name() string {
	if c.Secure {
		return cookieSecure
	}
	return cookieInsecure
}

func (c Cookies) other() string {
	if c.Secure {
		return cookieInsecure
	}
	return cookieSecure
}

// Set returns the session cookie plus a clearing cookie for the other name.
func (c Cookies) Set(token string, expires, now time.Time) []http.Cookie {
	return []http.Cookie{
		//nolint:gosec // Secure follows BaseURL: plain-HTTP LAN installs need a non-Secure cookie (P-19, 01 §5.1)
		{Name: c.name(), Value: token, Path: "/", HttpOnly: true, Secure: c.Secure,
			SameSite: http.SameSiteLaxMode, MaxAge: int(expires.Sub(now).Seconds())},
		c.clear(c.other()),
	}
}

// Clear expires both names.
func (c Cookies) Clear() []http.Cookie { return []http.Cookie{c.clear(c.name()), c.clear(c.other())} }

func (c Cookies) clear(name string) http.Cookie {
	//nolint:gosec // clearing cookie; Secure must match the name's scheme
	return http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: name == cookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1}
}

// SessionMiddleware authenticates the session cookie (03 §4): only the current
// scheme's cookie is used; an invalid one clears both names; a refreshed
// session re-sends the cookie (Authenticate has committed by then).
func SessionMiddleware(auth *app.Auth, c Cookies, clock app.Clock, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := RequestInfoFrom(r.Context())
			if ck, err := r.Cookie(c.name()); err == nil && ck.Value != "" {
				info.PresentedTokenHash = domain.HashToken(ck.Value)
				sess, refreshed, err := auth.Authenticate(r.Context(), ck.Value)
				switch {
				case err == nil:
					info.Session, info.UserID = &sess, sess.UserID
					if refreshed {
						setCookies(w, c.Set(ck.Value, sess.ExpiresAt, clock.Now()))
					}
				case errors.Is(err, app.ErrUnauthenticated):
					setCookies(w, c.Clear())
				default:
					var p *Problem
					if !errors.As(MapError(r.Context(), err, log), &p) {
						p = problem(http.StatusServiceUnavailable, "unavailable", "")
					}
					WriteProblem(w, p)
					return
				}
			} else if _, err := r.Cookie(c.other()); err == nil {
				setCookies(w, []http.Cookie{c.clear(c.other())}) // stale cookie from the other scheme
			}
			next.ServeHTTP(w, r)
		})
	}
}

func setCookies(w http.ResponseWriter, cs []http.Cookie) {
	for i := range cs {
		http.SetCookie(w, &cs[i])
	}
}
