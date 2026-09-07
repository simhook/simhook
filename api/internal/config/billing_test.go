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
