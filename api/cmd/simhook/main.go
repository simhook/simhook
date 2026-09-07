// Command simhook runs the API server and its workers, applies migrations,
// and exports the OpenAPI document.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/simhook/simhook/internal/app"
	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/db"
	"github.com/simhook/simhook/internal/httpapi"
	"github.com/simhook/simhook/internal/store"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "migrate":
		err = migrate(os.Args[2:])
	case "openapi":
		err = openapi()
	case "billing":
		err = billingCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: simhook <command>

  serve                   run the API server and job workers
  migrate up|down|status  manage the database schema
  openapi                 print the OpenAPI document as JSON
  billing sync            create or update the paid plans and the webhook endpoint at Polar
                          (--webhook-url overrides <SIMHOOK_PUBLIC_URL>/v1/billing/webhooks/polar)
  billing status          show the billing setup for this environment`)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func migrate(args []string) error {
	if len(args) == 0 {
		return errors.New("migrate needs up, down, or status")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	switch args[0] {
	case "up":
		return db.MigrateUp(ctx, pool, log)
	case "down":
		return db.MigrateDown(ctx, pool, log)
	case "status":
		statuses, err := db.Status(ctx, pool)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			fmt.Printf("%-8s %s\n", s.State, s.Source.Path)
		}
		return nil
	}
	return fmt.Errorf("unknown migrate action %q", args[0])
}

// billingCmd runs the provider setup: `sync` makes Polar match the plans
// table and registers the webhook endpoint, `status` shows the result.
func billingCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("billing needs sync or status")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc := billing.New(store.New(pool), cfg, log)
	switch args[0] {
	case "sync":
		fs := flag.NewFlagSet("billing sync", flag.ContinueOnError)
		address := fs.String("webhook-url", strings.TrimRight(cfg.PublicURL, "/")+"/v1/billing/webhooks/polar", "where Polar delivers events")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rep, err := svc.Sync(ctx, *address)
		if err != nil {
			return err
		}
		fmt.Printf("environment: %s\n", rep.Environment)
		for _, p := range rep.Products {
			fmt.Printf("product %-9s %-6s %s  $%d.%02d  %s\n", p.PlanID, p.Interval, p.ProductID, p.PriceCents/100, p.PriceCents%100, p.Action)
		}
		fmt.Printf("webhook  %s  %s (%s)\n", rep.Endpoint.URL, rep.Endpoint.ID, rep.EndpointAction)
		fmt.Printf("\nSet SIMHOOK_POLAR_WEBHOOK_SECRET=%s in the API's environment and restart it.\n", rep.Endpoint.Secret)
		return nil
	case "status":
		rep, err := svc.Report(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("environment: %s\naccess token: %v\nwebhook secret: %v\n", rep.Environment, rep.TokenSet, rep.SecretSet)
		if len(rep.Allowlist) > 0 {
			fmt.Printf("allowlist: %s\n", strings.Join(rep.Allowlist, ", "))
		}
		if len(rep.BlockedCodes) > 0 {
			fmt.Printf("blocked countries: %s\n", strings.Join(rep.BlockedCodes, ", "))
		}
		if len(rep.Products) == 0 {
			fmt.Println("products: none synced (run `simhook billing sync`)")
		}
		for _, p := range rep.Products {
			fmt.Printf("product %-9s %-6s %s  $%d.%02d  synced %s\n", p.PlanID, p.Interval, p.ProductID, p.PriceCents/100, p.PriceCents%100, p.SyncedAt.Format(time.RFC3339))
		}
		for _, ep := range rep.Endpoints {
			fmt.Printf("webhook  %s  enabled=%v  events=%d\n", ep.URL, ep.Enabled, len(ep.Events))
		}
		fmt.Printf("paid plans open: %v\n", svc.Enabled(ctx))
		return nil
	}
	return fmt.Errorf("unknown billing action %q", args[0])
}

func openapi() error {
	srv := httpapi.New(httpapi.Deps{Config: &config.Config{PublicURL: "https://api.simhook.dev", WebURL: "https://app.simhook.dev"}})
	out, err := json.MarshalIndent(srv.OpenAPI(), "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(out, '\n'))
	return err
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.Build(ctx, cfg, log, app.Options{Migrate: true, PeriodicJobs: true})
	if err != nil {
		return err
	}
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           a.HTTP.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := a.Start(ctx); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "docs", cfg.PublicURL+"/docs")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		log.Error("http server failed", "err", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	return a.Stop(shutdownCtx)
}
