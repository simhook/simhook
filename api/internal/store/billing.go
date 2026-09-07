package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	// What the provider last said, and a change it will apply next period.
	ProviderUpdatedAt *time.Time `db:"provider_updated_at" json:"-"`
	PendingPlanID     *string    `db:"pending_plan_id" json:"-"`
	PendingInterval   *string    `db:"pending_interval" json:"-"`
	PendingAt         *time.Time `db:"pending_at" json:"-"`
}

const subscriptionCols = `id, user_id, plan_id, status, provider, provider_subscription_id, provider_customer_id,
	billing_interval, current_period_start, current_period_end, cancel_at_period_end, ended_at, limit_overrides,
	created_at, updated_at, provider_updated_at, pending_plan_id, pending_interval, pending_at`

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

// SubscriptionLive reports whether a status grants the plan. past_due is
// the provider retrying a payment; the plan stays until it gives up.
func SubscriptionLive(status string) bool {
	return status == "active" || status == "trialing" || status == "past_due"
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
		if SubscriptionLive(sub.Status) {
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

// ---------------------------------------------------------------------------
// The payment provider's side: its products, its deliveries, its view of a
// subscription (decision 021)
// ---------------------------------------------------------------------------

// BillingProduct is a paid plan and interval as the provider sells it.
type BillingProduct struct {
	Provider    string    `db:"provider" json:"-"`
	Environment string    `db:"environment" json:"-"`
	PlanID      string    `db:"plan_id" json:"plan_id"`
	Interval    string    `db:"interval" json:"interval"`
	ProductID   string    `db:"product_id" json:"product_id"`
	PriceID     string    `db:"price_id" json:"-"`
	PriceCents  int32     `db:"price_cents" json:"price_cents"`
	SyncedAt    time.Time `db:"synced_at" json:"-"`
}

const billingProductCols = `provider, environment, plan_id, interval, product_id, price_id, price_cents, synced_at`

// UpsertBillingProduct records the provider's product for a plan and interval.
func (s *Store) UpsertBillingProduct(ctx context.Context, p BillingProduct) error {
	_, err := s.q.Exec(ctx, `
		insert into billing_products (provider, environment, plan_id, interval, product_id, price_id, price_cents, synced_at)
		values ($1, $2, $3, $4, $5, $6, $7, now())
		on conflict (provider, environment, plan_id, interval) do update
			set product_id = excluded.product_id, price_id = excluded.price_id, price_cents = excluded.price_cents, synced_at = now()`,
		p.Provider, p.Environment, p.PlanID, p.Interval, p.ProductID, p.PriceID, p.PriceCents)
	return err
}

// ListBillingProducts returns the products synced for one environment.
func (s *Store) ListBillingProducts(ctx context.Context, provider, environment string) ([]BillingProduct, error) {
	return many[BillingProduct](s.q.Query(ctx, `
		select `+billingProductCols+` from billing_products
		where provider = $1 and environment = $2 order by plan_id, interval`, provider, environment))
}

// RecordBillingEvent notes a provider delivery by its id and reports
// whether it is new. A redelivery is not.
func (s *Store) RecordBillingEvent(ctx context.Context, provider, id, kind string) (bool, error) {
	tag, err := s.q.Exec(ctx, `insert into billing_events (id, provider, type) values ($1, $2, $3) on conflict (id) do nothing`,
		id, provider, kind)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// HasProviderCustomer reports whether the user ever had a subscription
// with the provider, which is when the provider knows them as a customer.
func (s *Store) HasProviderCustomer(ctx context.Context, userID uuid.UUID, provider string) (bool, error) {
	var has bool
	err := s.q.QueryRow(ctx, `select exists (select 1 from subscriptions where user_id = $1 and provider = $2)`, userID, provider).Scan(&has)
	return has, err
}

// ProviderSubscription is the provider's view of one subscription.
type ProviderSubscription struct {
	Provider           string
	SubscriptionID     string
	CustomerID         string
	UserID             uuid.UUID
	PlanID             string
	Interval           string
	Status             string
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	CancelAtPeriodEnd  bool
	EndedAt            *time.Time
	// UpdatedAt is when the provider last changed it; an older view than
	// the one stored is not applied.
	UpdatedAt       time.Time
	PendingPlanID   *string
	PendingInterval *string
	PendingAt       *time.Time
}

// ApplyProviderSubscription writes the provider's view: the row with that
// provider id is updated, or created. A row that is not ended is the one
// live row the user may have, so any other live row ends first. It
// reports whether anything was written; a view older than the stored one
// is skipped. Call it inside a transaction.
func (s *Store) ApplyProviderSubscription(ctx context.Context, ps ProviderSubscription) (bool, error) {
	var stored *time.Time
	err := s.q.QueryRow(ctx, `
		select provider_updated_at from subscriptions where provider = $1 and provider_subscription_id = $2`,
		ps.Provider, ps.SubscriptionID).Scan(&stored)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if stored != nil && stored.After(ps.UpdatedAt) {
		return false, nil
	}
	if ps.EndedAt == nil {
		if _, err := s.q.Exec(ctx, `
			update subscriptions set ended_at = now(), updated_at = now()
			where user_id = $1 and ended_at is null
			  and (provider is distinct from $2 or provider_subscription_id is distinct from $3)`,
			ps.UserID, ps.Provider, ps.SubscriptionID); err != nil {
			return false, err
		}
	}
	_, err = s.q.Exec(ctx, `
		insert into subscriptions (user_id, plan_id, status, provider, provider_subscription_id, provider_customer_id,
			billing_interval, current_period_start, current_period_end, cancel_at_period_end, ended_at,
			provider_updated_at, pending_plan_id, pending_interval, pending_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		on conflict (provider, provider_subscription_id) where provider_subscription_id is not null do update set
			plan_id = excluded.plan_id, status = excluded.status, provider_customer_id = excluded.provider_customer_id,
			billing_interval = excluded.billing_interval, current_period_start = excluded.current_period_start,
			current_period_end = excluded.current_period_end, cancel_at_period_end = excluded.cancel_at_period_end,
			ended_at = excluded.ended_at, provider_updated_at = excluded.provider_updated_at,
			pending_plan_id = excluded.pending_plan_id, pending_interval = excluded.pending_interval,
			pending_at = excluded.pending_at, updated_at = now()`,
		ps.UserID, ps.PlanID, ps.Status, ps.Provider, ps.SubscriptionID, ps.CustomerID,
		ps.Interval, ps.CurrentPeriodStart, ps.CurrentPeriodEnd, ps.CancelAtPeriodEnd, ps.EndedAt,
		ps.UpdatedAt, ps.PendingPlanID, ps.PendingInterval, ps.PendingAt)
	return true, wrapWrite(err)
}
