package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Batch is one API send call: one body, many recipients.
type Batch struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	UserID                uuid.UUID  `db:"user_id" json:"-"`
	DeviceID              uuid.UUID  `db:"device_id" json:"device_id"`
	APIKeyID              *uuid.UUID `db:"api_key_id" json:"-"`
	Body                  string     `db:"body" json:"body"`
	RecipientCount        int32      `db:"recipient_count" json:"recipient_count"`
	RecipientPreview      string     `db:"recipient_preview" json:"recipient_preview"`
	DispatchedCount       int32      `db:"dispatched_count" json:"dispatched_count"`
	SentCount             int32      `db:"sent_count" json:"sent_count"`
	DeliveredCount        int32      `db:"delivered_count" json:"delivered_count"`
	FailedCount           int32      `db:"failed_count" json:"failed_count"`
	UnknownCount          int32      `db:"unknown_count" json:"unknown_count"`
	Status                string     `db:"status" json:"status"`
	ScheduledAt           *time.Time `db:"scheduled_at" json:"scheduled_at"`
	EstimatedCompletionAt *time.Time `db:"estimated_completion_at" json:"estimated_completion_at"`
	CompletedAt           *time.Time `db:"completed_at" json:"completed_at"`
	Error                 *string    `db:"error" json:"error"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at" json:"updated_at"`
}

const batchCols = `id, user_id, device_id, api_key_id, body, recipient_count, recipient_preview, dispatched_count,
	sent_count, delivered_count, failed_count, unknown_count, status, scheduled_at, estimated_completion_at,
	completed_at, error, created_at, updated_at`

// Message is one SMS in either direction.
type Message struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	UserID            uuid.UUID  `db:"user_id" json:"-"`
	DeviceID          uuid.UUID  `db:"device_id" json:"device_id"`
	BatchID           *uuid.UUID `db:"batch_id" json:"batch_id"`
	Direction         string     `db:"direction" json:"direction"`
	Status            string     `db:"status" json:"status"`
	Body              string     `db:"body" json:"body"`
	Recipient         *string    `db:"recipient" json:"recipient"`
	SimSubscriptionID *int32     `db:"sim_subscription_id" json:"sim_subscription_id"`
	DispatchDueAt     *time.Time `db:"dispatch_due_at" json:"-"`
	DispatchedAt      *time.Time `db:"dispatched_at" json:"dispatched_at"`
	SentAt            *time.Time `db:"sent_at" json:"sent_at"`
	DeliveredAt       *time.Time `db:"delivered_at" json:"delivered_at"`
	FailedAt          *time.Time `db:"failed_at" json:"failed_at"`
	ErrorCode         *string    `db:"error_code" json:"error_code"`
	ErrorMessage      *string    `db:"error_message" json:"error_message"`
	Sender            *string    `db:"sender" json:"sender"`
	ReceivedAt        *time.Time `db:"received_at" json:"received_at"`
	Fingerprint       *string    `db:"fingerprint" json:"-"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

const messageCols = `id, user_id, device_id, batch_id, direction, status, body, recipient, sim_subscription_id,
	dispatch_due_at, dispatched_at, sent_at, delivered_at, failed_at, error_code, error_message, sender,
	received_at, fingerprint, created_at, updated_at`

// Message statuses.
const (
	StatusQueued     = "queued"
	StatusDispatched = "dispatched"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusFailed     = "failed"
	StatusUnknown    = "unknown"
	StatusReceived   = "received"
)

// ---------------------------------------------------------------------------
// Batches and outbound messages
// ---------------------------------------------------------------------------

// CreateBatchParams describes a send.
type CreateBatchParams struct {
	ID                    uuid.UUID
	UserID                uuid.UUID
	DeviceID              uuid.UUID
	APIKeyID              *uuid.UUID
	Body                  string
	RecipientCount        int
	RecipientPreview      string
	ScheduledAt           *time.Time
	EstimatedCompletionAt *time.Time
}

// CreateBatch inserts a batch in the queued state.
func (s *Store) CreateBatch(ctx context.Context, p CreateBatchParams) (Batch, error) {
	return one[Batch](s.q.Query(ctx, `
		insert into batches (id, user_id, device_id, api_key_id, body, recipient_count, recipient_preview,
			scheduled_at, estimated_completion_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning `+batchCols,
		p.ID, p.UserID, p.DeviceID, p.APIKeyID, p.Body, p.RecipientCount, p.RecipientPreview,
		p.ScheduledAt, p.EstimatedCompletionAt))
}

// InsertOutbound inserts one queued message per recipient in a single
// statement. ids and recipients must be the same length.
func (s *Store) InsertOutbound(ctx context.Context, batchID, userID, deviceID uuid.UUID, body string, ids []uuid.UUID, recipients []string, sim *int32) error {
	if len(ids) != len(recipients) {
		return fmt.Errorf("store: ids and recipients differ in length")
	}
	_, err := s.q.Exec(ctx, `
		insert into messages (id, user_id, device_id, batch_id, direction, status, body, recipient, sim_subscription_id)
		select unnest($1::uuid[]), $2, $3, $4, 'outbound', 'queued', $5, unnest($6::text[]), $7`,
		ids, userID, deviceID, batchID, body, recipients, sim)
	return err
}

// StampDispatchDue records when a set of messages is due to be pushed.
func (s *Store) StampDispatchDue(ctx context.Context, ids []uuid.UUID, at time.Time) error {
	_, err := s.q.Exec(ctx, `update messages set dispatch_due_at = $2 where id = any($1::uuid[])`, ids, at)
	return err
}

// DispatchRow is what the dispatcher needs per message.
type DispatchRow struct {
	ID                uuid.UUID  `db:"id"`
	BatchID           *uuid.UUID `db:"batch_id"`
	DeviceID          uuid.UUID  `db:"device_id"`
	UserID            uuid.UUID  `db:"user_id"`
	Body              string     `db:"body"`
	Recipient         string     `db:"recipient"`
	SimSubscriptionID *int32     `db:"sim_subscription_id"`
}

// LoadQueuedForDispatch returns the still-queued messages among ids.
func (s *Store) LoadQueuedForDispatch(ctx context.Context, ids []uuid.UUID) ([]DispatchRow, error) {
	return many[DispatchRow](s.q.Query(ctx, `
		select id, batch_id, device_id, user_id, body, recipient, sim_subscription_id
		from messages where id = any($1::uuid[]) and status = 'queued' and direction = 'outbound'
		order by created_at, id`, ids))
}

// MarkDispatched moves queued messages to dispatched. Returns how many moved.
func (s *Store) MarkDispatched(ctx context.Context, ids []uuid.UUID, at time.Time) (int, error) {
	tag, err := s.q.Exec(ctx, `
		update messages set status = 'dispatched', dispatched_at = $2
		where id = any($1::uuid[]) and status = 'queued'`, ids, at)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// MarkFailedFromQueued fails queued messages that could not be pushed.
func (s *Store) MarkFailedFromQueued(ctx context.Context, ids []uuid.UUID, code, message string, at time.Time) ([]Message, error) {
	return many[Message](s.q.Query(ctx, `
		update messages set status = 'failed', failed_at = $2, error_code = $3, error_message = $4
		where id = any($1::uuid[]) and status = 'queued'
		returning `+messageCols, ids, at, code, message))
}

// MessageTransition is a message after a state change, with the state it
// left.
type MessageTransition struct {
	Message
	PrevStatus string `db:"prev_status"`
}

// TransitionMessage moves one message from any of the allowed statuses to
// the new one. Returns ErrNotFound when the message is not on the device or
// is not in an allowed state.
func (s *Store) TransitionMessage(ctx context.Context, id, deviceID uuid.UUID, from []string, to string, at time.Time, errorCode, errorMessage *string) (MessageTransition, error) {
	return one[MessageTransition](s.q.Query(ctx, `
		with prev as (
			select status as prev_status from messages where id = $1 and device_id = $2
		)
		update messages m set
			status = $3::text::message_status,
			sent_at = case when $3 = 'sent' then $4 else m.sent_at end,
			delivered_at = case when $3 = 'delivered' then $4 else m.delivered_at end,
			failed_at = case when $3 = 'failed' then $4 else m.failed_at end,
			error_code = case when $3 = 'failed' then $5 else m.error_code end,
			error_message = case when $3 = 'failed' then $6 else m.error_message end
		from prev
		where m.id = $1 and m.device_id = $2 and m.status::text = any($7::text[])
		returning `+prefixCols("m.", messageCols)+`, prev.prev_status`,
		id, deviceID, to, at, errorCode, errorMessage, from))
}

// StaleMessage is an in-flight message that went silent.
type StaleMessage struct {
	ID         uuid.UUID  `db:"id"`
	BatchID    *uuid.UUID `db:"batch_id"`
	DeviceID   uuid.UUID  `db:"device_id"`
	UserID     uuid.UUID  `db:"user_id"`
	PrevStatus string     `db:"prev_status"`
}

// MarkStaleUnknown flips queued messages that were due before cutoff and
// dispatched messages pushed before cutoff to unknown.
func (s *Store) MarkStaleUnknown(ctx context.Context, cutoff time.Time) ([]StaleMessage, error) {
	return many[StaleMessage](s.q.Query(ctx, `
		with c as (
			select id, status::text as prev_status from messages
			where direction = 'outbound' and (
				(status = 'queued' and coalesce(dispatch_due_at, created_at) < $1)
				or (status = 'dispatched' and dispatched_at < $1)
			)
			limit 1000
		)
		update messages m set
			status = 'unknown',
			error_code = case when c.prev_status = 'queued' then 'not_dispatched' else 'no_report' end,
			error_message = case when c.prev_status = 'queued'
				then 'The message was never handed to the phone.'
				else 'The phone accepted the message but never reported a result.' end
		from c where m.id = c.id
		returning m.id, m.batch_id, m.device_id, m.user_id, c.prev_status`, cutoff))
}

// ---------------------------------------------------------------------------
// Batch counters
// ---------------------------------------------------------------------------

var statusCounterColumn = map[string]string{
	StatusDispatched: "dispatched_count",
	StatusSent:       "sent_count",
	StatusDelivered:  "delivered_count",
	StatusFailed:     "failed_count",
	StatusUnknown:    "unknown_count",
}

// ApplyBatchTransition moves n messages worth of counters from one status to
// another and recomputes the batch status.
func (s *Store) ApplyBatchTransition(ctx context.Context, batchID uuid.UUID, from, to string, n int) (Batch, error) {
	var sets []string
	if col, ok := statusCounterColumn[from]; ok {
		sets = append(sets, fmt.Sprintf("%s = greatest(0, %s - $2)", col, col))
	}
	if col, ok := statusCounterColumn[to]; ok {
		sets = append(sets, fmt.Sprintf("%s = %s + $2", col, col))
	}
	if len(sets) > 0 {
		if _, err := s.q.Exec(ctx, `update batches set `+strings.Join(sets, ", ")+` where id = $1`, batchID, n); err != nil {
			return Batch{}, err
		}
	}
	return one[Batch](s.q.Query(ctx, `
		update batches set
			status = (case
				when recipient_count - (dispatched_count + sent_count + delivered_count + failed_count + unknown_count) = recipient_count then 'queued'
				when recipient_count - (sent_count + delivered_count + failed_count + unknown_count) > 0 then 'processing'
				when failed_count = recipient_count then 'failed'
				when unknown_count = recipient_count then 'unknown'
				when failed_count > 0 or unknown_count > 0 then 'partial'
				else 'completed' end)::batch_status,
			completed_at = case
				when recipient_count - (sent_count + delivered_count + failed_count + unknown_count) = 0 then coalesce(completed_at, now())
				else null end
		where id = $1
		returning `+batchCols, batchID))
}

// ---------------------------------------------------------------------------
// Inbound
// ---------------------------------------------------------------------------

// InsertInboundParams describes a received SMS.
type InsertInboundParams struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	DeviceID          uuid.UUID
	Sender            string
	Body              string
	ReceivedAt        time.Time
	Fingerprint       *string
	SimSubscriptionID *int32
}

// InsertInbound stores a received message. The second return is false when
// the fingerprint was already seen, in which case the existing row is
// returned.
func (s *Store) InsertInbound(ctx context.Context, p InsertInboundParams) (Message, bool, error) {
	m, err := one[Message](s.q.Query(ctx, `
		insert into messages (id, user_id, device_id, direction, status, body, sender, received_at, fingerprint, sim_subscription_id)
		values ($1, $2, $3, 'inbound', 'received', $4, $5, $6, $7, $8)
		on conflict (device_id, fingerprint) where fingerprint is not null do nothing
		returning `+messageCols,
		p.ID, p.UserID, p.DeviceID, p.Body, p.Sender, p.ReceivedAt, p.Fingerprint, p.SimSubscriptionID))
	if err == nil {
		return m, true, nil
	}
	if err != ErrNotFound || p.Fingerprint == nil {
		return Message{}, false, err
	}
	existing, err := one[Message](s.q.Query(ctx, `
		select `+messageCols+` from messages where device_id = $1 and fingerprint = $2`, p.DeviceID, *p.Fingerprint))
	return existing, false, err
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// GetUserMessage fetches a message on the user's account.
func (s *Store) GetUserMessage(ctx context.Context, userID, id uuid.UUID) (Message, error) {
	return one[Message](s.q.Query(ctx, `select `+messageCols+` from messages where id = $1 and user_id = $2`, id, userID))
}

// GetDeviceMessage fetches a message that belongs to a device.
func (s *Store) GetDeviceMessage(ctx context.Context, deviceID, id uuid.UUID) (Message, error) {
	return one[Message](s.q.Query(ctx, `select `+messageCols+` from messages where id = $1 and device_id = $2`, id, deviceID))
}

// GetUserBatch fetches a batch on the user's account.
func (s *Store) GetUserBatch(ctx context.Context, userID, id uuid.UUID) (Batch, error) {
	return one[Batch](s.q.Query(ctx, `select `+batchCols+` from batches where id = $1 and user_id = $2`, id, userID))
}

// ListBatchMessages returns every message in a batch, in insertion order.
func (s *Store) ListBatchMessages(ctx context.Context, userID, batchID uuid.UUID) ([]Message, error) {
	return many[Message](s.q.Query(ctx, `
		select `+messageCols+` from messages where batch_id = $1 and user_id = $2 order by created_at, id`, batchID, userID))
}

// ListBatches returns a user's batches, newest first, keyset paged.
func (s *Store) ListBatches(ctx context.Context, userID uuid.UUID, cursor *Cursor, limit int) ([]Batch, error) {
	args := []any{userID, limit + 1}
	where := `user_id = $1`
	if cursor != nil {
		args = append(args, cursor.CreatedAt, cursor.ID)
		where += ` and (created_at, id) < ($3, $4)`
	}
	return many[Batch](s.q.Query(ctx, `
		select `+batchCols+` from batches where `+where+` order by created_at desc, id desc limit $2`, args...))
}

// Cursor is a keyset position in a created_at, id ordering.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// MessageFilter narrows a history listing.
type MessageFilter struct {
	DeviceIDs []uuid.UUID
	Direction string // "", "outbound", "inbound"
	Status    string
	BatchID   *uuid.UUID
	Search    string
	From      *time.Time // inclusive, on created_at
	To        *time.Time // exclusive
	Ascending bool
	Cursor    *Cursor
	Limit     int
}

// ListMessages returns up to Limit+1 rows so callers can detect another page.
// Messages from unpaired devices are excluded.
func (s *Store) ListMessages(ctx context.Context, userID uuid.UUID, f MessageFilter) ([]Message, error) {
	args := []any{userID}
	conds := []string{`m.user_id = $1`,
		`not exists (select 1 from devices d where d.id = m.device_id and d.deleted_at is not null)`}
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if len(f.DeviceIDs) > 0 {
		conds = append(conds, `m.device_id = any(`+add(f.DeviceIDs)+`::uuid[])`)
	}
	if f.Direction != "" {
		conds = append(conds, `m.direction = `+add(f.Direction)+`::text::message_direction`)
	}
	if f.Status != "" {
		conds = append(conds, `m.status = `+add(f.Status)+`::text::message_status`)
	}
	if f.BatchID != nil {
		conds = append(conds, `m.batch_id = `+add(*f.BatchID))
	}
	if f.Search != "" {
		pattern := "%" + escapeLike(f.Search) + "%"
		p := add(pattern)
		conds = append(conds, `(m.body ilike `+p+` or m.recipient ilike `+p+` or m.sender ilike `+p+`)`)
	}
	if f.From != nil {
		conds = append(conds, `m.created_at >= `+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, `m.created_at < `+add(*f.To))
	}
	order := "desc"
	cmp := "<"
	if f.Ascending {
		order = "asc"
		cmp = ">"
	}
	if f.Cursor != nil {
		conds = append(conds, `(m.created_at, m.id) `+cmp+` (`+add(f.Cursor.CreatedAt)+`, `+add(f.Cursor.ID)+`)`)
	}
	limit := add(f.Limit + 1)
	return many[Message](s.q.Query(ctx, `
		select `+prefixCols("m.", messageCols)+` from messages m
		where `+strings.Join(conds, " and ")+`
		order by m.created_at `+order+`, m.id `+order+`
		limit `+limit, args...))
}

// UserMessageTotals sums lifetime counters across live devices.
func (s *Store) UserMessageTotals(ctx context.Context, userID uuid.UUID) (sent, received int64, err error) {
	err = s.q.QueryRow(ctx, `
		select coalesce(sum(sent_count), 0), coalesce(sum(received_count), 0)
		from devices where user_id = $1 and deleted_at is null`, userID).Scan(&sent, &received)
	return
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// prefixCols qualifies a comma-separated column list with a table alias.
func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
