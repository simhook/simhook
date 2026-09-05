package auth

import (
	"context"

	"github.com/riverqueue/river"
)

// SweepArgs deletes expired sessions and one-time codes.
type SweepArgs struct{}

// Kind implements river.JobArgs.
func (SweepArgs) Kind() string { return "sweep_credentials" }

// SweepWorker runs the credential sweep.
type SweepWorker struct {
	river.WorkerDefaults[SweepArgs]
	svc *Service
}

// NewSweepWorker builds the worker.
func NewSweepWorker(svc *Service) *SweepWorker { return &SweepWorker{svc: svc} }

// Work implements river.Worker.
func (w *SweepWorker) Work(ctx context.Context, _ *river.Job[SweepArgs]) error {
	return w.svc.SweepCredentials(ctx)
}
