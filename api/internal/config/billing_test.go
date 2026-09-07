package config

import (
	"strings"
	"testing"
)

func TestBillingSwitches(t *testing.T) {
	c := &Config{
		BillingAllowlist:        cleanList([]string{" Owner@Example.com ", "", "other@example.com", "other@example.com"}, strings.ToLower),
		BillingBlockedCountries: cleanList([]string{"tr", " ", "Ru"}, strings.ToUpper),
	}
	if len(c.BillingAllowlist) != 2 || c.BillingAllowlist[0] != "owner@example.com" {
		t.Fatalf("allowlist: %v", c.BillingAllowlist)
	}
	if !c.BillingAllowed("OWNER@example.com") || c.BillingAllowed("stranger@example.com") {
		t.Fatal("allowlist does not decide by lowercase email")
	}
	if !c.CountryBlocked("tr") || !c.CountryBlocked("RU") || c.CountryBlocked("US") || c.CountryBlocked("") {
		t.Fatalf("blocked countries: %v", c.BillingBlockedCountries)
	}
	open := &Config{}
	if !open.BillingAllowed("anyone@example.com") || open.CountryBlocked("TR") {
		t.Fatal("an empty list must allow everyone")
	}
}

func TestPolarEnvironmentValidated(t *testing.T) {
	t.Setenv("SIMHOOK_DATABASE_URL", "postgres://x")
	t.Setenv("SIMHOOK_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("SIMHOOK_POLAR_ENVIRONMENT", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("an unknown Polar environment must be refused")
	}
	t.Setenv("SIMHOOK_POLAR_ENVIRONMENT", "production")
	t.Setenv("SIMHOOK_BILLING_ALLOWLIST", "A@b.c, ,d@e.f")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PolarEnvironment != "production" || len(cfg.BillingAllowlist) != 2 || cfg.BillingAllowlist[0] != "a@b.c" {
		t.Fatalf("got %q %v", cfg.PolarEnvironment, cfg.BillingAllowlist)
	}
}

// TestCommentReadAsListIsRefused: an .env line with a comment after an empty
// value reaches the process with the comment as the value. Loudly refusing
// it beats an allowlist of nobody that quietly turns every checkout away.
func TestCommentReadAsListIsRefused(t *testing.T) {
	t.Setenv("SIMHOOK_DATABASE_URL", "postgres://x")
	t.Setenv("SIMHOOK_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("SIMHOOK_BILLING_ALLOWLIST", "# comma-separated emails; set, only these accounts can buy")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "comment") {
		t.Fatalf("a comment read as the allowlist must be refused, got %v", err)
	}
	t.Setenv("SIMHOOK_BILLING_ALLOWLIST", "")
	t.Setenv("SIMHOOK_BILLING_BLOCKED_COUNTRIES", "# comma-separated ISO codes")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SIMHOOK_BILLING_BLOCKED_COUNTRIES") {
		t.Fatalf("a comment read as the blocked countries must be refused, got %v", err)
	}
}
