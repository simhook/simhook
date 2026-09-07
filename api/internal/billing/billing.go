// Package billing answers "may this account do that" using plan limits and
// usage counters, and runs the paid plans through the payment provider:
// checkouts, plan changes, the customer portal, and the webhooks that keep
// the subscriptions table true (decision 021).
package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/store"
)

// LimitError says which limit blocked an action.
type LimitError struct {
	Kind  string // daily, monthly, batch, devices
	Limit int32
	Used  int32
}

func (e *LimitError) Error() string {
	switch e.Kind {
	case "batch":
		return fmt.Sprintf("a single send may include at most %d recipients on your plan", e.Limit)
	case "devices":
		return fmt.Sprintf("your plan allows %d paired device(s)", e.Limit)
	default:
		return fmt.Sprintf("%s message limit reached (%d of %d used)", e.Kind, e.Used, e.Limit)
	}
}

// Service checks limits and talks to the provider.
type Service struct {
	st  *store.Store
	cfg *config.Config
	log *slog.Logger
	// polar is nil while no access token is configured; every provider
	// operation then answers that paid plans are closed.
	polar *Polar
}

// New builds the service. The provider is on when the configuration names
// an access token; the rest of the setup (synced products, the webhook
// secret) is checked per call, so an incomplete setup reads as closed
// rather than broken.
func New(st *store.Store, cfg *config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{st: st, cfg: cfg, log: log}
	if cfg != nil && cfg.PolarAccessToken != "" {
		s.polar = NewPolar(cfg.PolarEnvironment, cfg.PolarAccessToken)
	}
	return s
}

// Plans returns the public plan catalogue.
func (s *Service) Plans(ctx context.Context) ([]store.Plan, error) {
	return s.st.ListPlans(ctx)
}

// Limits returns the effective limits for a user.
func (s *Service) Limits(ctx context.Context, userID uuid.UUID) (store.Limits, error) {
	return s.st.EffectiveLimits(ctx, userID)
}

// Usage returns the current period counters.
func (s *Service) Usage(ctx context.Context, userID uuid.UUID) (store.Usage, error) {
	return s.st.GetUsage(ctx, userID, time.Now())
}

// CheckBatchSize rejects a send with too many recipients for the plan.
func (s *Service) CheckBatchSize(limits store.Limits, n int) error {
	if limits.BatchLimit >= 0 && int32(n) > limits.BatchLimit {
		return &LimitError{Kind: "batch", Limit: limits.BatchLimit, Used: int32(n)}
	}
	return nil
}

// ReserveSends counts n sends against the day and month that contain at
// (the scheduled time, or now) and returns a LimitError if either limit is
// exceeded. Run it inside the send transaction so a rejected reservation
// rolls back.
func (s *Service) ReserveSends(ctx context.Context, tx *store.Store, userID uuid.UUID, limits store.Limits, n int, at time.Time) error {
	day, month, err := tx.ReserveSends(ctx, userID, n, at)
	if err != nil {
		return err
	}
	if limits.DailyLimit >= 0 && day > limits.DailyLimit {
		return &LimitError{Kind: "daily", Limit: limits.DailyLimit, Used: day}
	}
	if limits.MonthlyLimit >= 0 && month > limits.MonthlyLimit {
		return &LimitError{Kind: "monthly", Limit: limits.MonthlyLimit, Used: month}
	}
	return nil
}
