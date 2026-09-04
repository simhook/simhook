// Command simhook runs the API server and its workers, applies migrations,
// and exports the OpenAPI document.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simhook/simhook/internal/app"
	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/db"
	"github.com/simhook/simhook/internal/httpapi"
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
  openapi                 print the OpenAPI document as JSON`)
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
