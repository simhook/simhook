package httpapi

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// The API writes two cookies and nothing else ever does.
//
// The session cookie carries the token. It is httpOnly and host-only, so no
// script and no other host can read it; the API is its only reader and its
// only judge.
//
// The flag cookie carries no secret. It says "this browser is probably
// signed in" and is set on the cookie domain, so the site and the dashboard,
// which live on other hosts under that domain, can read it: the site paints
// the right bar before its first frame and the dashboard's proxy routes a
// signed-out visitor to sign-in before any page loads. Probably is the word:
// only a request to the API says for certain, and the API keeps the two
// cookies in step by clearing both whenever a session is gone.
const (
	sessionCookie  = "simhook_session"
	signedInCookie = "simhook_signed_in"
)

func (s *Server) sessionCookieFor(token string, expires time.Time) http.Cookie {
	return http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) signedInCookieFor(expires time.Time) http.Cookie {
	return http.Cookie{
		Name: signedInCookie, Value: "1", Path: "/", Domain: s.deps.Config.CookieDomain, Expires: expires,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteLaxMode,
	}
}

// issueCookies is the pair a signed-in browser holds, expiring together.
func (s *Server) issueCookies(token string, expires time.Time) []http.Cookie {
	return []http.Cookie{s.sessionCookieFor(token, expires), s.signedInCookieFor(expires)}
}

// clearCookies expires both, with the attributes they were set with so the
// browser matches them.
func (s *Server) clearCookies() []http.Cookie {
	session := s.sessionCookieFor("", time.Time{})
	session.MaxAge = -1
	flag := s.signedInCookieFor(time.Time{})
	flag.Value = ""
	flag.MaxAge = -1
	return []http.Cookie{session, flag}
}

// setCookies adds Set-Cookie headers from middleware, before the handler
// writes its own.
func setCookies(ctx huma.Context, cookies []http.Cookie) {
	for i := range cookies {
		ctx.AppendHeader("Set-Cookie", cookies[i].String())
	}
}
