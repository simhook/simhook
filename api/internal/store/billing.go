package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Plan is a tier with its limits. -1 means unlimited.
type Plan struct {
	ID                string `db:"id" json:"id"`
	Name              string `db:"name" json:"name"`
	DailyLimit        int32  `db:"daily_limit" json:"daily_limit"`
	MonthlyLimit      int32  `db:"monthly_limit" json:"monthly_limit"`
	BatchLimit        int32  `db:"batch_limit" json:"batch_limit"`
	DeviceLimit       int32  `db:"device_limit" json:"device_limit"`
	MonthlyPriceCents int32  `db:"monthly_price_cents" json:"monthly_price_cents"`
	YearlyPriceCents  int32  `db:"yearly_price_cents" json:"yearly_price_cents"`
	Active            bool   `db:"active" json:"active"`
	SortOrder         int32  `db:"sort_order" json:"-"`
}

const planCols = `id, name, daily_limit, monthly_limit, batch_limit, device_limit, monthly_price_cents,
	yearly_price_cents, active, sort_order`

// Subscription binds a user to a plan.
type Subscription struct {
	ID                     uuid.UUID       `db:"id" json:"id"`
	UserID                 uuid.UUID       `db:"user_id" json:"-"`
	PlanID                 string          `db:"plan_id" json:"plan_id"`
	Status                 string          `db:"status" json:"status"`
	Provider               *string         `db:"provider" json:"provider"`
	ProviderSubscriptionID *string         `db:"provider_subscription_id" json:"-"`
	ProviderCustomerID     *string         `db:"provider_customer_id" json:"-"`
	BillingInterval        *string         `db:"billing_interval" json:"billing_interval"`
	CurrentPeriodStart     *time.Time      `db:"current_period_start" json:"current_period_start"`
	CurrentPeriodEnd       *time.Time      `db:"current_period_end" json:"current_period_end"`
	CancelAtPeriodEnd      bool            `db:"cancel_at_period_end" json:"cancel_at_period_end"`
	EndedAt                *time.Time      `db:"ended_at" json:"-"`
	LimitOverrides         json.RawMessage `db:"limit_overrides" json:"-"`
	CreatedAt              time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time       `db:"updated_at" json:"updated_at"`
}

const subscriptionCols = `id, user_id, plan_id, status, provider, provider_subscription_id, provider_customer_id,
	billing_interval, current_period_start, current_period_end, cancel_at_period_end, ended_at, limit_overrides,
	created_at, updated_at`

// ListPlans returns the active plans in display order.
func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	return many[Plan](s.q.Query(ctx, `select `+planCols+` from plans where active order by sort_order`))
}

// GetPlan fetches one plan.
func (s *Store) GetPlan(ctx context.Context, id string) (Plan, error) {
	return one[Plan](s.q.Query(ctx, `select `+planCols+` from plans where id = $1`, id))
}

// GetLiveSubscription returns the user's current subscription, or ErrNotFound
// when they are on the free plan with no row.
func (s *Store) GetLiveSubscription(ctx context.Context, userID uuid.UUID) (Subscription, error) {
	return one[Subscription](s.q.Query(ctx, `
		select `+subscriptionCols+` from subscriptions where user_id = $1 and ended_at is null`, userID))
}

// Limits are the effective limits for a user after overrides.
type Limits struct {
	PlanID       string `json:"plan_id"`
	PlanName     string `json:"plan_name"`
	DailyLimit   int32  `json:"daily_limit"`
	MonthlyLimit int32  `json:"monthly_limit"`
	BatchLimit   int32  `json:"batch_limit"`
	DeviceLimit  int32  `json:"device_limit"`
}

// EffectiveLimits resolves plan plus overrides for a user.
func (s *Store) EffectiveLimits(ctx context.Context, userID uuid.UUID) (Limits, error) {
	planID := "free"
	var overrides json.RawMessage
	sub, err := s.GetLiveSubscription(ctx, userID)
	switch {
	case err == nil:
		if sub.Status == "active" || sub.Status == "trialing" || sub.Status == "past_due" {
			planID = sub.PlanID
			overrides = sub.LimitOverrides
		}
	case !errors.Is(err, ErrNotFound):
		return Limits{}, err
	}
	plan, err := s.GetPlan(ctx, planID)
	if err != nil {
		return Limits{}, err
	}
	l := Limits{
		PlanID: plan.ID, PlanName: plan.Name,
		DailyLimit: plan.DailyLimit, MonthlyLimit: plan.MonthlyLimit,
		BatchLimit: plan.BatchLimit, DeviceLimit: plan.DeviceLimit,
	}
	if len(overrides) > 0 {
		var o struct {
			DailyLimit   *int32 `json:"daily_limit"`
			MonthlyLimit *int32 `json:"monthly_limit"`
			BatchLimit   *int32 `json:"batch_limit"`
			DeviceLimit  *int32 `json:"device_limit"`
		}
		if json.Unmarshal(overrides, &o) == nil {
			if o.DailyLimit != nil {
				l.DailyLimit = *o.DailyLimit
			}
			if o.MonthlyLimit != nil {
				l.MonthlyLimit = *o.MonthlyLimit
			}
			if o.BatchLimit != nil {
				l.BatchLimit = *o.BatchLimit
			}
			if o.DeviceLimit != nil {
				l.DeviceLimit = *o.DeviceLimit
			}
		}
	}
	return l, nil
}

// Usage is what a user has consumed in the current periods.
type Usage struct {
	SentToday     int32 `json:"sent_today"`
	SentThisMonth int32 `json:"sent_this_month"`
	ReceivedToday int32 `json:"received_today"`
	ReceivedMonth int32 `json:"received_this_month"`
	DayStartsAt   time.Time
	MonthStartsAt time.Time
}

func periodStarts(now time.Time) (day, month time.Time) {
	y, m, d := now.UTC().Date()
	day = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	month = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return
}

// ReserveSends adds n to the day and month counters and returns the new
// totals. Callers compare against limits and roll back the transaction when
// over, which undoes the reservation.
func (s *Store) ReserveSends(ctx context.Context, userID uuid.UUID, n int, now time.Time) (day, month int32, err error) {
	dayStart, monthStart := periodStarts(now)
	if err = s.q.QueryRow(ctx, `
		insert into usage_counters (user_id, period_kind, period_start, sent) values ($1, 'day', $2, $3)
		on conflict (user_id, period_kind, period_start) do update set sent = usage_counters.sent + $3
		returning sent`, userID, dayStart, n).Scan(&day); err != nil {
		return
	}
	err = s.q.QueryRow(ctx, `
		insert into usage_counters (user_id, period_kind, period_start, sent) values ($1, 'month', $2, $3)
		on conflict (user_id, period_kind, period_start) do update set sent = usage_counters.sent + $3
		returning sent`, userID, monthStart, n).Scan(&month)
	return
}

// AddReceived counts an inbound message.
func (s *Store) AddReceived(ctx context.Context, userID uuid.UUID, now time.Time) error {
	dayStart, monthStart := periodStarts(now)
	_, err := s.q.Exec(ctx, `
		insert into usage_counters (user_id, period_kind, period_start, received) values
			($1, 'day', $2, 1), ($1, 'month', $3, 1)
		on conflict (user_id, period_kind, period_start) do update set received = usage_counters.received + 1`,
		userID, dayStart, monthStart)
	return err
}

// GetUsage reads the current period counters.
func (s *Store) GetUsage(ctx context.Context, userID uuid.UUID, now time.Time) (Usage, error) {
	dayStart, monthStart := periodStarts(now)
	u := Usage{DayStartsAt: dayStart, MonthStartsAt: monthStart}
	rows, err := s.q.Query(ctx, `
		select period_kind, sent, received from usage_counters
		where user_id = $1 and ((period_kind = 'day' and period_start = $2) or (period_kind = 'month' and period_start = $3))`,
		userID, dayStart, monthStart)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var sent, received int32
		if err := rows.Scan(&kind, &sent, &received); err != nil {
			return u, err
		}
		if kind == "day" {
			u.SentToday, u.ReceivedToday = sent, received
		} else {
			u.SentThisMonth, u.ReceivedMonth = sent, received
		}
	}
	return u, rows.Err()
}
