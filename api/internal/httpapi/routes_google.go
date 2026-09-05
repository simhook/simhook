package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/oauth2"

	"github.com/simhook/simhook/internal/auth"
)

// Google sign-in is the authorization-code flow with PKCE, run by the API:
// the dashboard links to /start, Google sends the browser back to
// /callback, and the browser ends up on the dashboard with the session
// cookies. What the flow needs to remember between the two (the state, the
// PKCE verifier, and where to go afterwards) travels in a sealed, short-lived
// cookie of its own, so the API keeps no state and a callback can only
// complete a flow the same browser started.

const (
	googleCookie = "simhook_google"
	googleTTL    = 10 * time.Minute
)

type googleFlow struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
	Expires  int64  `json:"e"`
}

type googleStartInput struct {
	Next string `query:"next" maxLength:"512" doc:"Dashboard path to land on afterwards."`
}

type googleCallbackInput struct {
	Code  string `query:"code" maxLength:"2048"`
	State string `query:"state" maxLength:"128"`
	Error string `query:"error" maxLength:"128"`
	Flow  string `cookie:"simhook_google" maxLength:"4096"`
}

// redirectOutput sends the browser somewhere, possibly with cookies.
type redirectOutput struct {
	Status    int
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

func (s *Server) registerGoogle() {
	tags := []string{"auth"}

	huma.Register(s.api, huma.Operation{
		OperationID: "google-start", Method: http.MethodGet, Path: "/v1/auth/google/start",
		Summary: "Start Google sign-in", Tags: tags, DefaultStatus: http.StatusFound, Hidden: true,
	}, func(ctx context.Context, in *googleStartInput) (*redirectOutput, error) {
		if s.deps.Google == nil {
			return nil, apiErr(http.StatusNotFound, "google_off", "Google sign-in is not enabled on this server.")
		}
		state, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		verifier := oauth2.GenerateVerifier()
		sealed, err := s.sealFlow(googleFlow{State: state, Verifier: verifier, Next: safeNext(in.Next), Expires: time.Now().Add(googleTTL).Unix()})
		if err != nil {
			return nil, err
		}
		return &redirectOutput{
			Status: http.StatusFound, Location: s.deps.Google.AuthURL(state, verifier),
			SetCookie: []http.Cookie{s.googleCookieFor(sealed, googleTTL)},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "google-callback", Method: http.MethodGet, Path: "/v1/auth/google/callback",
		Summary: "Finish Google sign-in", Tags: tags, DefaultStatus: http.StatusFound, Hidden: true,
	}, func(ctx context.Context, in *googleCallbackInput) (*redirectOutput, error) {
		if s.deps.Google == nil {
			return nil, apiErr(http.StatusNotFound, "google_off", "Google sign-in is not enabled on this server.")
		}
		// Whatever happens, the flow cookie is spent.
		fail := func(code string) (*redirectOutput, error) {
			return &redirectOutput{
				Status: http.StatusFound, Location: s.deps.Config.WebURL + "/login?error=" + url.QueryEscape(code),
				SetCookie: []http.Cookie{s.googleCookieFor("", -1)},
			}, nil
		}
		flow, ok := s.openFlow(in.Flow)
		if !ok || in.State == "" || flow.State != in.State {
			return fail("google_state")
		}
		if in.Error != "" || in.Code == "" {
			return fail("google_cancelled")
		}
		id, err := s.deps.Google.Exchange(ctx, in.Code, flow.Verifier)
		if err != nil {
			s.deps.Log.Warn("google exchange failed", "err", err)
			return fail("google_failed")
		}
		u, err := s.deps.Auth.SignInWithGoogle(ctx, id)
		if err != nil {
			return fail(googleErrorCode(ctx, s, err))
		}
		session, err := s.issueSession(ctx, u)
		if err != nil {
			return nil, err
		}
		return &redirectOutput{
			Status: http.StatusFound, Location: s.deps.Config.WebURL + flow.Next,
			SetCookie: append(session.SetCookie, s.googleCookieFor("", -1)),
		}, nil
	})
}

// googleErrorCode names a sign-in failure for the dashboard's sign-in page.
func googleErrorCode(ctx context.Context, s *Server, err error) string {
	switch {
	case errors.Is(err, auth.ErrGoogleEmailUnverified):
		return "google_email_unverified"
	case errors.Is(err, auth.ErrBanned):
		return "account_suspended"
	case errors.Is(err, auth.ErrInvalidEmail):
		return "google_failed"
	}
	s.deps.Log.ErrorContext(ctx, "google sign-in failed", "err", err)
	return "google_failed"
}

func (s *Server) googleCookieFor(value string, maxAge time.Duration) http.Cookie {
	c := http.Cookie{
		Name: googleCookie, Value: value, Path: "/v1/auth/google", HttpOnly: true,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteLaxMode,
	}
	if maxAge < 0 {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(maxAge / time.Second)
	}
	return c
}

func (s *Server) sealFlow(f googleFlow) (string, error) {
	plain, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sealed, err := s.deps.Box.Seal(plain)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Server) openFlow(value string) (googleFlow, bool) {
	if value == "" || s.deps.Box == nil {
		return googleFlow{}, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return googleFlow{}, false
	}
	plain, err := s.deps.Box.Open(sealed)
	if err != nil {
		return googleFlow{}, false
	}
	var f googleFlow
	if err := json.Unmarshal(plain, &f); err != nil || f.State == "" || f.Verifier == "" || time.Now().Unix() > f.Expires {
		return googleFlow{}, false
	}
	return f, true
}

// safeNext keeps a post-sign-in destination on the dashboard: a path, not
// an address, and not a sign-in page.
func safeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/dashboard"
	}
	path := next
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	switch path {
	case "/", "/login", "/register", "/reset-password":
		return "/dashboard"
	}
	return next
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
