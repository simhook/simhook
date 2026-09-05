package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simhook/simhook/internal/config"
)

func TestCookiePair(t *testing.T) {
	s := New(Deps{Config: &config.Config{PublicURL: "https://api.simhook.dev", WebURL: "https://app.simhook.dev", CookieDomain: "simhook.dev"}})
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	pair := s.issueCookies("shs_token", expires)
	if len(pair) != 2 {
		t.Fatalf("two cookies, got %d", len(pair))
	}
	session, flag := pair[0], pair[1]
	if session.Name != sessionCookie || session.Value != "shs_token" || !session.HttpOnly || !session.Secure || session.Domain != "" || session.Path != "/" || session.SameSite != http.SameSiteLaxMode || !session.Expires.Equal(expires) {
		t.Fatalf("session cookie: %s", session.String())
	}
	if flag.Name != signedInCookie || flag.Value != "1" || flag.HttpOnly || !flag.Secure || flag.Domain != "simhook.dev" || flag.SameSite != http.SameSiteLaxMode || !flag.Expires.Equal(expires) {
		t.Fatalf("flag cookie: %s", flag.String())
	}

	for _, c := range s.clearCookies() {
		h := c.String()
		if c.Value != "" || !strings.Contains(h, "Max-Age=0") || strings.Contains(h, "Expires=") {
			t.Fatalf("a clear must be empty and expire at once: %s", h)
		}
		if (c.Name == signedInCookie) != (c.Domain == "simhook.dev") {
			t.Fatalf("a clear must carry the attributes the cookie was set with: %s", h)
		}
	}
}

func TestPlainCookiesOnLocalhost(t *testing.T) {
	s := New(Deps{Config: &config.Config{PublicURL: "http://localhost:8080", WebURL: "http://localhost:3000"}})
	for _, c := range s.issueCookies("shs_token", time.Now().Add(time.Hour)) {
		if c.Secure || c.Domain != "" {
			t.Fatalf("localhost gets host-only, plain cookies: %s", c.String())
		}
	}
}
