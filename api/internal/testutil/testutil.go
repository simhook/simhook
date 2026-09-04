// Package testutil prepares a real Postgres database for integration tests.
// Tests need SIMHOOK_TEST_DATABASE_URL or the dev compose database.
package testutil

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/db"
)

const defaultAdminURL = "postgres://simhook:simhook@localhost:5432/postgres?sslmode=disable"

var (
	prepareOnce sync.Once
	prepareErr  error
	testURL     string
)

// DatabaseURL returns the test database URL, creating and migrating the
// database on first use. It skips the test when Postgres is unreachable.
func DatabaseURL(t *testing.T) string {
	t.Helper()
	prepareOnce.Do(func() {
		adminURL := os.Getenv("SIMHOOK_TEST_ADMIN_URL")
		if adminURL == "" {
			adminURL = defaultAdminURL
		}
		testURL = os.Getenv("SIMHOOK_TEST_DATABASE_URL")
		if testURL == "" {
			testURL = strings.Replace(adminURL, "/postgres?", "/simhook_test?", 1)
		}
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			prepareErr = err
			return
		}
		defer conn.Close(ctx)
		var exists bool
		if err := conn.QueryRow(ctx, `select exists(select 1 from pg_database where datname = 'simhook_test')`).Scan(&exists); err != nil {
			prepareErr = err
			return
		}
		if !exists {
			if _, err := conn.Exec(ctx, `create database simhook_test`); err != nil {
				prepareErr = err
				return
			}
		}
		pool, err := db.Connect(ctx, testURL)
		if err != nil {
			prepareErr = err
			return
		}
		defer pool.Close()
		prepareErr = db.MigrateUp(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	})
	if prepareErr != nil {
		t.Skipf("postgres not available for integration tests: %v", prepareErr)
	}
	return testURL
}

// Reset empties every table that tests write to. Plans stay seeded.
func Reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, DatabaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `
		truncate users cascade;
		truncate river_job;`)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// Config builds a configuration pointed at the test database, with private
// webhook hosts allowed so httptest servers can receive deliveries.
func Config(t *testing.T) *config.Config {
	t.Helper()
	url := DatabaseURL(t)
	t.Setenv("SIMHOOK_DATABASE_URL", url)
	t.Setenv("SIMHOOK_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("SIMHOOK_ENV", "test")
	t.Setenv("SIMHOOK_LOG_LEVEL", "warn")
	t.Setenv("SIMHOOK_WEBHOOK_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("SIMHOOK_WEBHOOK_TIMEOUT_SECONDS", "5")
	t.Setenv("SIMHOOK_DISPATCH_WAVE_SIZE", "40")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg
}
