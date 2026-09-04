package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Webhook is a subscription to events on an account.
type Webhook struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	UserID         uuid.UUID  `db:"user_id" json:"-"`
	Name           *string    `db:"name" json:"name"`
	URL            string     `db:"url" json:"url"`
	SecretEnc      []byte     `db:"secret_enc" json:"-"`
	Events         []string   `db:"events" json:"events"`
	Enabled        bool       `db:"enabled" json:"enabled"`
	SuccessCount   int64      `db:"success_count" json:"success_count"`
	FailureCount   int64      `db:"failure_count" json:"failure_count"`
	LastAttemptAt  *time.Time `db:"last_attempt_at" json:"last_attempt_at"`
	LastSuccessAt  *time.Time `db:"last_success_at" json:"last_success_at"`
	LastFailureAt  *time.Time `db:"last_failure_at" json:"last_failure_at"`
	LastEnabledAt  *time.Time `db:"last_enabled_at" json:"-"`
	DisabledReason *string    `db:"disabled_reason" json:"disabled_reason"`
	DeletedAt      *time.Time `db:"deleted_at" json:"-"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
}

const webhookCols = `id, user_id, name, url, secret_enc, events, enabled, success_count, failure_count,
	last_attempt_at, last_success_at, last_failure_at, last_enabled_at, disabled_reason, deleted_at,
	created_at, updated_at`

// Delivery is one attempt series to deliver one event to one webhook.
type Delivery struct {
	ID              uuid.UUID       `db:"id" json:"id"`
	WebhookID       uuid.UUID       `db:"webhook_id" json:"webhook_id"`
	MessageID       *uuid.UUID      `db:"message_id" json:"message_id"`
	DeviceID        *uuid.UUID      `db:"device_id" json:"device_id"`
	Event           string          `db:"event" json:"event"`
	Payload         json.RawMessage `db:"payload" json:"payload"`
	URL             string          `db:"url" json:"url"`
	Status          string          `db:"status" json:"status"`
	AttemptCount    int32           `db:"attempt_count" json:"attempt_count"`
	NextAttemptAt   *time.Time      `db:"next_attempt_at" json:"next_attempt_at"`
	LastAttemptAt   *time.Time      `db:"last_attempt_at" json:"last_attempt_at"`
	DeliveredAt     *time.Time      `db:"delivered_at" json:"delivered_at"`
	AbandonedAt     *time.Time      `db:"abandoned_at" json:"abandoned_at"`
	HTTPStatus      *int32          `db:"http_status" json:"http_status"`
	ResponseExcerpt *string         `db:"response_excerpt" json:"response_excerpt"`
	Error           *string         `db:"error" json:"error"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
}

const deliveryCols = `id, webhook_id, message_id, device_id, event, payload, url, status, attempt_count,
	next_attempt_at, last_attempt_at, delivered_at, abandoned_at, http_status, response_excerpt, error, created_at`

// CreateWebhook stores a subscription.
func (s *Store) CreateWebhook(ctx context.Context, id, userID uuid.UUID, name *string, url string, secretEnc []byte, events []string) (Webhook, error) {
	return one[Webhook](s.q.Query(ctx, `
		insert into webhooks (id, user_id, name, url, secret_enc, events, last_enabled_at)
		values ($1, $2, $3, $4, $5, $6, now()) returning `+webhookCols,
		id, userID, name, url, secretEnc, events))
}

// CountWebhooks counts a user's live subscriptions.
func (s *Store) CountWebhooks(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `select count(*) from webhooks where user_id = $1 and deleted_at is null`, userID).Scan(&n)
	return n, err
}

// GetUserWebhook fetches a live subscription owned by the user.
func (s *Store) GetUserWebhook(ctx context.Context, userID, id uuid.UUID) (Webhook, error) {
	return one[Webhook](s.q.Query(ctx, `
		select `+webhookCols+` from webhooks where id = $1 and user_id = $2 and deleted_at is null`, id, userID))
}

// GetWebhook fetches any subscription, including deleted, for delivery
// bookkeeping.
func (s *Store) GetWebhook(ctx context.Context, id uuid.UUID) (Webhook, error) {
	return one[Webhook](s.q.Query(ctx, `select `+webhookCols+` from webhooks where id = $1`, id))
}

// ListWebhooks returns a user's live subscriptions.
func (s *Store) ListWebhooks(ctx context.Context, userID uuid.UUID) ([]Webhook, error) {
	return many[Webhook](s.q.Query(ctx, `
		select `+webhookCols+` from webhooks where user_id = $1 and deleted_at is null order by created_at asc`, userID))
}

// ListWebhooksForEvent returns the enabled subscriptions to an event.
func (s *Store) ListWebhooksForEvent(ctx context.Context, userID uuid.UUID, event string) ([]Webhook, error) {
	return many[Webhook](s.q.Query(ctx, `
		select `+webhookCols+` from webhooks
		where user_id = $1 and deleted_at is null and enabled and $2 = any(events)`, userID, event))
}

// WebhookPatch updates a subscription. Nil fields are left alone.
type WebhookPatch struct {
	Name      *string
	URL       *string
	SecretEnc []byte
	Events    []string
	Enabled   *bool
}

// UpdateWebhook applies a patch.
func (s *Store) UpdateWebhook(ctx context.Context, userID, id uuid.UUID, p WebhookPatch) (Webhook, error) {
	return one[Webhook](s.q.Query(ctx, `
		update webhooks set
			name = coalesce($3, name),
			url = coalesce($4, url),
			secret_enc = coalesce($5, secret_enc),
			events = coalesce($6, events),
			enabled = coalesce($7, enabled),
			last_enabled_at = case when $7 is true and not enabled then now() else last_enabled_at end,
			disabled_reason = case when $7 is true then null else disabled_reason end
		where id = $1 and user_id = $2 and deleted_at is null
		returning `+webhookCols,
		id, userID, p.Name, p.URL, p.SecretEnc, p.Events, p.Enabled))
}

// DisableWebhook pauses a subscription with a reason.
func (s *Store) DisableWebhook(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := s.q.Exec(ctx, `update webhooks set enabled = false, disabled_reason = $2 where id = $1 and enabled`, id, reason)
	return err
}

// SoftDeleteWebhook removes a subscription but keeps its delivery history.
func (s *Store) SoftDeleteWebhook(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.q.Exec(ctx, `
		update webhooks set deleted_at = now(), enabled = false where id = $1 and user_id = $2 and deleted_at is null`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Deliveries
// ---------------------------------------------------------------------------

// CreateDelivery records a pending delivery.
func (s *Store) CreateDelivery(ctx context.Context, id, webhookID uuid.UUID, messageID, deviceID *uuid.UUID, event string, payload json.RawMessage, url string) (Delivery, error) {
	return one[Delivery](s.q.Query(ctx, `
		insert into webhook_deliveries (id, webhook_id, message_id, device_id, event, payload, url, next_attempt_at)
		values ($1, $2, $3, $4, $5::text::webhook_event, $6, $7, now()) returning `+deliveryCols,
		id, webhookID, messageID, deviceID, event, payload, url))
}

// GetDelivery fetches a delivery.
func (s *Store) GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	return one[Delivery](s.q.Query(ctx, `select `+deliveryCols+` from webhook_deliveries where id = $1`, id))
}

// GetUserDelivery fetches a delivery on one of the user's webhooks.
func (s *Store) GetUserDelivery(ctx context.Context, userID, id uuid.UUID) (Delivery, error) {
	return one[Delivery](s.q.Query(ctx, `
		select `+prefixCols("d.", deliveryCols)+` from webhook_deliveries d
		join webhooks w on w.id = d.webhook_id
		where d.id = $1 and w.user_id = $2`, id, userID))
}

// DeliveryOutcome records the result of one attempt.
type DeliveryOutcome struct {
	Delivered       bool
	Abandoned       bool
	NextAttemptAt   *time.Time
	HTTPStatus      *int32
	ResponseExcerpt *string
	Error           *string
}

// RecordDeliveryAttempt updates a delivery and the webhook's counters.
func (s *Store) RecordDeliveryAttempt(ctx context.Context, id uuid.UUID, o DeliveryOutcome) (Delivery, error) {
	status := "retrying"
	switch {
	case o.Delivered:
		status = "delivered"
	case o.Abandoned:
		status = "failed"
	}
	d, err := one[Delivery](s.q.Query(ctx, `
		update webhook_deliveries set
			status = $2::text::delivery_status,
			attempt_count = attempt_count + 1,
			last_attempt_at = now(),
			next_attempt_at = $3,
			delivered_at = case when $2 = 'delivered' then now() else delivered_at end,
			abandoned_at = case when $2 = 'failed' then now() else abandoned_at end,
			http_status = $4,
			response_excerpt = $5,
			error = $6
		where id = $1 returning `+deliveryCols,
		id, status, o.NextAttemptAt, o.HTTPStatus, o.ResponseExcerpt, o.Error))
	if err != nil {
		return Delivery{}, err
	}
	_, err = s.q.Exec(ctx, `
		update webhooks set
			last_attempt_at = now(),
			success_count = success_count + case when $2 then 1 else 0 end,
			failure_count = failure_count + case when $2 then 0 else 1 end,
			last_success_at = case when $2 then now() else last_success_at end,
			last_failure_at = case when $2 then last_failure_at else now() end
		where id = $1`, d.WebhookID, o.Delivered)
	return d, err
}

// DeliveryFilter narrows a delivery listing.
type DeliveryFilter struct {
	WebhookID *uuid.UUID
	Status    string
	Event     string
	From      *time.Time
	To        *time.Time
	Cursor    *Cursor
	Limit     int
}

// ListDeliveries returns up to Limit+1 deliveries across the user's webhooks,
// newest first, including webhooks that were since deleted.
func (s *Store) ListDeliveries(ctx context.Context, userID uuid.UUID, f DeliveryFilter) ([]Delivery, error) {
	args := []any{userID}
	conds := []string{`w.user_id = $1`}
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.WebhookID != nil {
		conds = append(conds, `d.webhook_id = `+add(*f.WebhookID))
	}
	if f.Status != "" {
		conds = append(conds, `d.status = `+add(f.Status)+`::text::delivery_status`)
	}
	if f.Event != "" {
		conds = append(conds, `d.event = `+add(f.Event)+`::text::webhook_event`)
	}
	if f.From != nil {
		conds = append(conds, `d.created_at >= `+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, `d.created_at < `+add(*f.To))
	}
	if f.Cursor != nil {
		conds = append(conds, `(d.created_at, d.id) < (`+add(f.Cursor.CreatedAt)+`, `+add(f.Cursor.ID)+`)`)
	}
	limit := add(f.Limit + 1)
	return many[Delivery](s.q.Query(ctx, `
		select `+prefixCols("d.", deliveryCols)+` from webhook_deliveries d
		join webhooks w on w.id = d.webhook_id
		where `+strings.Join(conds, " and ")+`
		order by d.created_at desc, d.id desc
		limit `+limit, args...))
}

// WebhookFailureStats summarises recent outcomes for the auto-pause check.
type WebhookFailureStats struct {
	WebhookID uuid.UUID `db:"webhook_id"`
	Failed    int64     `db:"failed"`
	Delivered int64     `db:"delivered"`
}

// WebhookFailureStatsSince aggregates finished deliveries per enabled webhook
// over a window.
func (s *Store) WebhookFailureStatsSince(ctx context.Context, since time.Time) ([]WebhookFailureStats, error) {
	return many[WebhookFailureStats](s.q.Query(ctx, `
		select d.webhook_id,
			count(*) filter (where d.status = 'failed') as failed,
			count(*) filter (where d.status = 'delivered') as delivered
		from webhook_deliveries d
		join webhooks w on w.id = d.webhook_id
		where w.enabled and w.deleted_at is null and d.created_at >= $1
			and (w.last_enabled_at is null or w.last_enabled_at < $1)
		group by d.webhook_id`, since))
}
