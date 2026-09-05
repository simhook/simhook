// Package config loads the service configuration from the environment.
// Every variable is prefixed SIMHOOK_. See api/.env.example for the contract.
package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the complete runtime configuration.
type Config struct {
	Env       string `env:"SIMHOOK_ENV" envDefault:"development"`
	HTTPAddr  string `env:"SIMHOOK_HTTP_ADDR" envDefault:":8080"`
	PublicURL string `env:"SIMHOOK_PUBLIC_URL" envDefault:"http://localhost:8080"`
	WebURL    string `env:"SIMHOOK_WEB_URL" envDefault:"http://localhost:3000"`
	// The public site, when it lives on another origin than the dashboard. It
	// signs visitors out and is told who is signed in, so its origin is
	// allowed alongside the dashboard's.
	SiteURL    string `env:"SIMHOOK_SITE_URL"`
	LogLevel   string `env:"SIMHOOK_LOG_LEVEL" envDefault:"info"`
	TrustProxy bool   `env:"SIMHOOK_TRUST_PROXY" envDefault:"false"`

	DatabaseURL string `env:"SIMHOOK_DATABASE_URL,required"`

	// SecretKeyB64 is 32 random bytes, base64 encoded. It encrypts webhook
	// signing secrets at rest and signs nothing else; tokens are hashed.
	SecretKeyB64 string `env:"SIMHOOK_SECRET_KEY,required"`

	FCMCredentialsFile string `env:"SIMHOOK_FCM_CREDENTIALS_FILE"`

	SMTPHost     string `env:"SIMHOOK_SMTP_HOST"`
	SMTPPort     int    `env:"SIMHOOK_SMTP_PORT" envDefault:"1025"`
	SMTPUser     string `env:"SIMHOOK_SMTP_USER"`
	SMTPPassword string `env:"SIMHOOK_SMTP_PASSWORD"`
	SMTPFrom     string `env:"SIMHOOK_SMTP_FROM" envDefault:"simhook <noreply@localhost>"`

	DispatchWaveSize         int `env:"SIMHOOK_DISPATCH_WAVE_SIZE" envDefault:"40"`
	PushTTLSeconds           int `env:"SIMHOOK_PUSH_TTL_SECONDS" envDefault:"86400"`
	StaleAfterMinutes        int `env:"SIMHOOK_STALE_AFTER_MINUTES" envDefault:"15"`
	OfflineAfterMinutes      int `env:"SIMHOOK_OFFLINE_AFTER_MINUTES" envDefault:"45"`
	HeartbeatIntervalMinutes int `env:"SIMHOOK_HEARTBEAT_INTERVAL_MINUTES" envDefault:"20"`

	WebhookTimeoutSeconds    int  `env:"SIMHOOK_WEBHOOK_TIMEOUT_SECONDS" envDefault:"30"`
	WebhookMaxPerUser        int  `env:"SIMHOOK_WEBHOOK_MAX_PER_USER" envDefault:"5"`
	WebhookAllowPrivateHosts bool `env:"SIMHOOK_WEBHOOK_ALLOW_PRIVATE_HOSTS" envDefault:"false"`

	// CookieDomain is the domain the readable signed-in flag cookie is set
	// on, so the site and the dashboard on hosts under it can read it. It
	// must be a parent of the API, dashboard, and site hosts. Empty means
	// the API's own host, which is right for localhost.
	CookieDomain string `env:"SIMHOOK_COOKIE_DOMAIN"`
	// SessionTTLHours is how long a session lives without being used; use
	// extends it. SessionMaxHours caps its life however active it is.
	SessionTTLHours          int    `env:"SIMHOOK_SESSION_TTL_HOURS" envDefault:"720"`
	SessionMaxHours          int    `env:"SIMHOOK_SESSION_MAX_HOURS" envDefault:"4320"`
	RequireEmailVerification bool   `env:"SIMHOOK_REQUIRE_EMAIL_VERIFICATION" envDefault:"true"`
	GoogleClientID           string `env:"SIMHOOK_GOOGLE_CLIENT_ID"`
	TurnstileSecretKey       string `env:"SIMHOOK_TURNSTILE_SECRET_KEY"`

	secretKey []byte
}

// Load reads an optional .env file from the working directory, then the
// process environment, and validates the result.
func Load() (*Config, error) {
	_ = godotenv.Load()
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	key, err := base64.StdEncoding.DecodeString(c.SecretKeyB64)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("config: SIMHOOK_SECRET_KEY must be 32 random bytes, base64 encoded")
	}
	c.secretKey = key
	if c.DispatchWaveSize < 1 {
		c.DispatchWaveSize = 1
	}
	if c.SessionTTLHours < 1 {
		return fmt.Errorf("config: SIMHOOK_SESSION_TTL_HOURS must be at least 1")
	}
	if c.SessionMaxHours < c.SessionTTLHours {
		return fmt.Errorf("config: SIMHOOK_SESSION_MAX_HOURS (%d) must be at least SIMHOOK_SESSION_TTL_HOURS (%d)", c.SessionMaxHours, c.SessionTTLHours)
	}
	c.CookieDomain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(c.CookieDomain), "."))
	if c.CookieDomain != "" {
		// A flag set on a domain the dashboard is not under would never reach
		// it, and the dashboard would bounce every visit to sign-in.
		for _, raw := range []string{c.PublicURL, c.WebURL, c.SiteURL} {
			if raw == "" {
				continue
			}
			host := hostOf(raw)
			if host == "" || !UnderDomain(host, c.CookieDomain) {
				return fmt.Errorf("config: SIMHOOK_COOKIE_DOMAIN %q is not a parent domain of %s", c.CookieDomain, raw)
			}
		}
	}
	return nil
}

// UnderDomain reports whether host is domain itself or a name under it.
func UnderDomain(host, domain string) bool {
	host, domain = strings.ToLower(host), strings.ToLower(domain)
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// originOf reduces a URL to its origin: scheme, host, and port.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// BrowserOrigins lists the origins whose pages may make cookie-authenticated
// requests: the dashboard, the site, and the API's own reference page.
func (c *Config) BrowserOrigins() []string {
	var out []string
	for _, raw := range []string{c.WebURL, c.SiteURL, c.PublicURL} {
		if o := originOf(raw); o != "" && !slices.Contains(out, o) {
			out = append(out, o)
		}
	}
	return out
}

// SecretKey returns the decoded server key.
func (c *Config) SecretKey() []byte { return c.secretKey }

// IsProduction reports whether the service runs with production semantics.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// SecureCookies reports whether cookies carry the Secure attribute: they do
// whenever the API is reached over https. A browser refuses a Secure cookie
// from a plain http origin, so the scheme decides, not the environment.
func (c *Config) SecureCookies() bool {
	return strings.HasPrefix(strings.ToLower(c.PublicURL), "https://")
}

// PushTTL is how long the push provider may hold an undelivered push.
func (c *Config) PushTTL() time.Duration { return time.Duration(c.PushTTLSeconds) * time.Second }

// StaleAfter is how long an in-flight message may go without a report.
func (c *Config) StaleAfter() time.Duration { return time.Duration(c.StaleAfterMinutes) * time.Minute }

// OfflineAfter is how long a device may go without a heartbeat.
func (c *Config) OfflineAfter() time.Duration {
	return time.Duration(c.OfflineAfterMinutes) * time.Minute
}

// SessionTTL is how long a dashboard session lives without being used.
func (c *Config) SessionTTL() time.Duration { return time.Duration(c.SessionTTLHours) * time.Hour }

// SessionMax is the longest a dashboard session lives, however active.
func (c *Config) SessionMax() time.Duration { return time.Duration(c.SessionMaxHours) * time.Hour }

// WebhookTimeout bounds a single webhook delivery attempt.
func (c *Config) WebhookTimeout() time.Duration {
	return time.Duration(c.WebhookTimeoutSeconds) * time.Second
}
