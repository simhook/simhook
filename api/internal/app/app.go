// Package app wires every component together. main uses it to serve; tests
// use it to assemble the real service with a few adapters swapped out.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/db"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/httpapi"
	"github.com/simhook/simhook/internal/mail"
	"github.com/simhook/simhook/internal/push"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/webhooks"
)

// Options override adapters that are otherwise chosen from configuration.
type Options struct {
	Mailer mail.Mailer
	Push   push.Sender
	// Migrate applies schema migrations during Build. Off in tests that
	// prepare the database themselves.
	Migrate bool
	// PeriodicJobs enables the scheduled sweeps. Tests turn them off so the
	// timing of state changes is deterministic.
	PeriodicJobs bool
}

// App is the assembled service.
type App struct {
	Cfg      *config.Config
	Log      *slog.Logger
	Pool     *pgxpool.Pool
	Store    *store.Store
	Auth     *auth.Service
	Billing  *billing.Service
	Webhooks *webhooks.Service
	Gateway  *gateway.Service
	Queue    *river.Client[pgx.Tx]
	HTTP     *httpapi.Server
}

// Build connects to the database and wires the services. Nothing runs until
// Start is called.
func Build(ctx context.Context, cfg *config.Config, log *slog.Logger, opts Options) (*App, error) {
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if opts.Migrate {
		if err := db.MigrateUp(ctx, pool, log); err != nil {
			pool.Close()
			return nil, err
		}
	}
	st := store.New(pool)
	box, err := secrets.NewBox(cfg.SecretKey())
	if err != nil {
		pool.Close()
		return nil, err
	}

	mailer := opts.Mailer
	if mailer == nil {
		mailer = &mail.Logger{Log: log}
		if cfg.SMTPHost != "" {
			mailer = mail.NewSMTP(mail.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Password: cfg.SMTPPassword, From: cfg.SMTPFrom})
		}
	}
	sender := opts.Push
	if sender == nil {
		sender = &push.Logger{Log: log}
		if cfg.FCMCredentialsFile != "" {
			sender, err = push.NewFCM(ctx, cfg.FCMCredentialsFile)
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("push: %w", err)
			}
			log.Info("push provider ready", "provider", "fcm")
		} else {
			log.Warn("no push credentials configured; pushes are logged, not sent")
		}
	}

	authSvc := auth.New(st, cfg, mailer, log)
	billingSvc := billing.New(st)
	hooksSvc := webhooks.New(st, cfg, box, mailer, log)
	gwSvc := gateway.New(st, cfg, sender, hooksSvc, billingSvc, log)

	workers := river.NewWorkers()
	river.AddWorker(workers, gateway.NewDispatchWorker(gwSvc))
	river.AddWorker(workers, gateway.NewReconcileWorker(gwSvc))
	river.AddWorker(workers, gateway.NewPresenceWorker(gwSvc))
	river.AddWorker(workers, webhooks.NewDeliverWorker(hooksSvc))
	river.AddWorker(workers, webhooks.NewAutoPauseWorker(hooksSvc))
	river.AddWorker(workers, auth.NewSweepWorker(authSvc))

	var periodic []*river.PeriodicJob
	if opts.PeriodicJobs {
		periodic = []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(5*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return gateway.ReconcileArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return gateway.PresenceArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return webhooks.AutoPauseArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return auth.SweepArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		}
	}
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 20},
			webhooks.QueueName: {MaxWorkers: 20},
		},
		Workers:      workers,
		PeriodicJobs: periodic,
		Logger:       log,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("queue: %w", err)
	}
	hooksSvc.SetQueue(queue)
	gwSvc.SetQueue(queue)

	api := httpapi.New(httpapi.Deps{Config: cfg, Log: log, Auth: authSvc, Billing: billingSvc, Gateway: gwSvc, Webhooks: hooksSvc})

	return &App{
		Cfg: cfg, Log: log, Pool: pool, Store: st, Auth: authSvc, Billing: billingSvc,
		Webhooks: hooksSvc, Gateway: gwSvc, Queue: queue, HTTP: api,
	}, nil
}

// Start runs the job workers.
func (a *App) Start(ctx context.Context) error {
	if err := a.Queue.Start(ctx); err != nil {
		return fmt.Errorf("queue start: %w", err)
	}
	return nil
}

// Stop drains the workers and closes the database.
func (a *App) Stop(ctx context.Context) error {
	err := a.Queue.Stop(ctx)
	a.Pool.Close()
	return err
}
