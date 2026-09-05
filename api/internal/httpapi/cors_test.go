package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simhook/simhook/internal/config"
)

func TestCORSAdmitsDashboardAndSite(t *testing.T) {
	cfg := &config.Config{WebURL: "https://app.example.com/", SiteURL: "https://example.com"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := cors(cfg)(next)

	cases := []struct {
		origin  string
		allowed bool
	}{
		{"https://app.example.com", true},
		{"https://example.com", true},
		{"https://evil.example.net", false},
		{"", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Get("Access-Control-Allow-Origin")
		if tc.allowed && got != tc.origin {
			t.Errorf("origin %q: want it echoed, got %q", tc.origin, got)
		}
		if !tc.allowed && got != "" {
			t.Errorf("origin %q: want no CORS header, got %q", tc.origin, got)
		}
	}

	// A preflight from the site is answered without reaching the handler.
	req := httptest.NewRequest(http.MethodOptions, "/v1/auth/logout", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight: got %d %q", rec.Code, rec.Header().Get("Access-Control-Allow-Credentials"))
	}
}

func TestCORSWithoutSiteURL(t *testing.T) {
	h := cors(&config.Config{WebURL: "https://app.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("site origin must not be allowed when unset, got %q", got)
	}
}
