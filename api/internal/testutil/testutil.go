// Package testutil prepares a real Postgres database for integration tests.
// Tests need the dev compose database, or SIMHOOK_TEST_ADMIN_URL pointing at
// a server where the user may create databases.
package testutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
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
// database on first use. Every package gets a database of its own, named
// after its directory, because go test runs packages in parallel and Reset
// truncates: one shared database had packages emptying each other's rows
// mid-test. SIMHOOK_TEST_DATABASE_URL points every package at one database
// instead, which is only safe under go test -p 1. The test is skipped when
// Postgres is unreachable.
func DatabaseURL(t *testing.T) string {
	t.Helper()
	prepareOnce.Do(func() {
		adminURL := os.Getenv("SIMHOOK_TEST_ADMIN_URL")
		if adminURL == "" {
			adminURL = defaultAdminURL
		}
		testURL = os.Getenv("SIMHOOK_TEST_DATABASE_URL")
		if testURL == "" {
			testURL = strings.Replace(adminURL, "/postgres?", "/"+packageDatabase()+"?", 1)
		}
		name, err := databaseName(testURL)
		if err != nil {
			prepareErr = err
			return
		}
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			prepareErr = err
			return
		}
		defer conn.Close(ctx)
		var exists bool
		if err := conn.QueryRow(ctx, `select exists(select 1 from pg_database where datname = $1)`, name).Scan(&exists); err != nil {
			prepareErr = err
			return
		}
		if !exists {
			if _, err := conn.Exec(ctx, `create database `+pgx.Identifier{name}.Sanitize()); err != nil {
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

// packageDatabase names the database for the package under test after the
// directory go test runs it in: simhook_test_internal_app, and so on.
func packageDatabase() string {
	wd, err := os.Getwd()
	if err != nil {
		return "simhook_test"
	}
	return "simhook_test_" + identifier(filepath.Base(filepath.Dir(wd))) + "_" + identifier(filepath.Base(wd))
}

// identifier keeps what Postgres allows in an unquoted name.
func identifier(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func databaseName(dbURL string) (string, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("testutil: %q names no database", dbURL)
	}
	return name, nil
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
		truncate river_job;
		truncate billing_events;
		truncate billing_products;`)
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
