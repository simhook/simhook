package config

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

func load(t *testing.T, vars map[string]string) (*Config, error) {
	t.Helper()
	t.Setenv("SIMHOOK_DATABASE_URL", "postgres://simhook@localhost/simhook")
	t.Setenv("SIMHOOK_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("SIMHOOK_COOKIE_DOMAIN", "")
	t.Setenv("SIMHOOK_SITE_URL", "")
	for k, v := range vars {
		t.Setenv(k, v)
	}
	return Load()
}

func TestCookieDomainMustCoverEveryHost(t *testing.T) {
	prod := map[string]string{
		"SIMHOOK_PUBLIC_URL": "https://api.simhook.dev",
		"SIMHOOK_WEB_URL":    "https://app.simhook.dev",
		"SIMHOOK_SITE_URL":   "https://simhook.dev",
	}
	with := func(domain string) map[string]string {
		m := map[string]string{"SIMHOOK_COOKIE_DOMAIN": domain}
		for k, v := range prod {
			m[k] = v
		}
		return m
	}

	cfg, err := load(t, with("simhook.dev"))
	if err != nil {
		t.Fatalf("the parent domain should pass: %v", err)
	}
	if cfg.CookieDomain != "simhook.dev" || !cfg.SecureCookies() {
		t.Fatalf("domain %q secure %v", cfg.CookieDomain, cfg.SecureCookies())
	}
	if got := cfg.BrowserOrigins(); !slices.Equal(got, []string{"https://app.simhook.dev", "https://simhook.dev", "https://api.simhook.dev"}) {
		t.Fatalf("origins: %v", got)
	}
	if cfg, err = load(t, with(".Simhook.dev")); err != nil || cfg.CookieDomain != "simhook.dev" {
		t.Fatalf("a leading dot and case should be forgiven: %v %q", err, cfg.CookieDomain)
	}
	if _, err = load(t, with("hook.dev")); err == nil || !strings.Contains(err.Error(), "SIMHOOK_COOKIE_DOMAIN") {
		t.Fatalf("a suffix that is not a label should be refused: %v", err)
	}
	if _, err = load(t, with("app.simhook.dev")); err == nil {
		t.Fatal("a domain the API is not under should be refused")
	}
	m := with("simhook.dev")
	m["SIMHOOK_WEB_URL"] = "https://dashboard.example.com"
	if _, err = load(t, m); err == nil {
		t.Fatal("a dashboard on another domain cannot see the flag")
	}
}

func TestDefaultsAreLocal(t *testing.T) {
	cfg, err := load(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CookieDomain != "" || cfg.SecureCookies() {
		t.Fatalf("localhost gets host-only, plain cookies: %q %v", cfg.CookieDomain, cfg.SecureCookies())
	}
	if got := cfg.BrowserOrigins(); !slices.Equal(got, []string{"http://localhost:3000", "http://localhost:8080"}) {
		t.Fatalf("origins: %v", got)
	}
	if cfg.SessionMax() < cfg.SessionTTL() {
		t.Fatal("the cap must cover the idle window")
	}
}

func TestSessionCapCoversIdleWindow(t *testing.T) {
	if _, err := load(t, map[string]string{"SIMHOOK_SESSION_TTL_HOURS": "48", "SIMHOOK_SESSION_MAX_HOURS": "24"}); err == nil {
		t.Fatal("a cap under the idle window should be refused")
	}
	if _, err := load(t, map[string]string{"SIMHOOK_SESSION_TTL_HOURS": "0"}); err == nil {
		t.Fatal("a zero idle window should be refused")
	}
}
