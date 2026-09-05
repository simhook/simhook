// Package config loads the service configuration from the environment.
// Every variable is prefixed SIMHOOK_. See api/.env.example for the contract.
package config

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the complete runtime configuration.
type Config struct {
	Env        string `env:"SIMHOOK_ENV" envDefault:"development"`
	HTTPAddr   string `env:"SIMHOOK_HTTP_ADDR" envDefault:":8080"`
	PublicURL  string `env:"SIMHOOK_PUBLIC_URL" envDefault:"http://localhost:8080"`
	WebURL     string `env:"SIMHOOK_WEB_URL" envDefault:"http://localhost:3000"`
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
	AdminEmail   string `env:"SIMHOOK_ADMIN_EMAIL"`

	DispatchWaveSize         int `env:"SIMHOOK_DISPATCH_WAVE_SIZE" envDefault:"40"`
	PushTTLSeconds           int `env:"SIMHOOK_PUSH_TTL_SECONDS" envDefault:"86400"`
	StaleAfterMinutes        int `env:"SIMHOOK_STALE_AFTER_MINUTES" envDefault:"15"`
	OfflineAfterMinutes      int `env:"SIMHOOK_OFFLINE_AFTER_MINUTES" envDefault:"45"`
	HeartbeatIntervalMinutes int `env:"SIMHOOK_HEARTBEAT_INTERVAL_MINUTES" envDefault:"20"`

	WebhookTimeoutSeconds    int  `env:"SIMHOOK_WEBHOOK_TIMEOUT_SECONDS" envDefault:"30"`
	WebhookMaxPerUser        int  `env:"SIMHOOK_WEBHOOK_MAX_PER_USER" envDefault:"5"`
	WebhookAllowPrivateHosts bool `env:"SIMHOOK_WEBHOOK_ALLOW_PRIVATE_HOSTS" envDefault:"false"`

	SessionTTLHours          int    `env:"SIMHOOK_SESSION_TTL_HOURS" envDefault:"720"`
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
	key, err := base64.StdEncoding.DecodeString(cfg.SecretKeyB64)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("config: SIMHOOK_SECRET_KEY must be 32 random bytes, base64 encoded")
	}
	cfg.secretKey = key
	if cfg.DispatchWaveSize < 1 {
		cfg.DispatchWaveSize = 1
	}
	return &cfg, nil
}

// SecretKey returns the decoded server key.
func (c *Config) SecretKey() []byte { return c.secretKey }

// IsProduction reports whether the service runs with production semantics.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// PushTTL is how long the push provider may hold an undelivered push.
func (c *Config) PushTTL() time.Duration { return time.Duration(c.PushTTLSeconds) * time.Second }

// StaleAfter is how long an in-flight message may go without a report.
func (c *Config) StaleAfter() time.Duration { return time.Duration(c.StaleAfterMinutes) * time.Minute }

// OfflineAfter is how long a device may go without a heartbeat.
func (c *Config) OfflineAfter() time.Duration {
	return time.Duration(c.OfflineAfterMinutes) * time.Minute
}

// SessionTTL is the lifetime of a dashboard session.
func (c *Config) SessionTTL() time.Duration { return time.Duration(c.SessionTTLHours) * time.Hour }

// WebhookTimeout bounds a single webhook delivery attempt.
func (c *Config) WebhookTimeout() time.Duration {
	return time.Duration(c.WebhookTimeoutSeconds) * time.Second
}
