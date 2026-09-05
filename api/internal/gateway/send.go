package gateway

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/push"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/webhooks"
)

const (
	maxBodyChars      = 1600
	maxScheduleAhead  = 7 * 24 * time.Hour
	maxRecipientsHard = 5000
)

var recipientPattern = regexp.MustCompile(`^\+?[0-9]{3,15}$`)

// NormalizeRecipient strips formatting and validates a phone number. It
// accepts E.164 and common local forms; the carrier decides what routes.
func NormalizeRecipient(raw string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
			// formatting, dropped
		default:
			return "", false
		}
	}
	n := b.String()
	if strings.HasPrefix(n, "00") {
		n = "+" + n[2:]
	}
	if !recipientPattern.MatchString(n) {
		return "", false
	}
	return n, true
}

// SendInput is a send request.
type SendInput struct {
	To                []string
	Body              string
	DeviceID          *uuid.UUID
	SimSubscriptionID *int32
	ScheduledAt       *time.Time
}

// SendResult is what the caller gets back.
type SendResult struct {
	Batch      store.Batch
	MessageIDs []uuid.UUID
}

// Send validates, reserves quota, records the batch, and enqueues dispatch,
// all in one transaction.
func (s *Service) Send(ctx context.Context, user store.User, apiKeyID *uuid.UUID, in SendInput) (SendResult, error) {
	if s.jobs == nil {
		return SendResult{}, ErrQueueNotReady
	}
	body := in.Body
	if strings.TrimSpace(body) == "" {
		return SendResult{}, &ValidationError{Field: "body", Message: "required"}
	}
	if len([]rune(body)) > maxBodyChars {
		return SendResult{}, &ValidationError{Field: "body", Message: fmt.Sprintf("at most %d characters", maxBodyChars)}
	}
	if len(in.To) == 0 {
		return SendResult{}, &ValidationError{Field: "to", Message: "at least one recipient"}
	}
	if len(in.To) > maxRecipientsHard {
		return SendResult{}, &ValidationError{Field: "to", Message: fmt.Sprintf("at most %d recipients per send", maxRecipientsHard)}
	}
	recipients := make([]string, 0, len(in.To))
	seen := make(map[string]struct{}, len(in.To))
	for i, raw := range in.To {
		n, ok := NormalizeRecipient(raw)
		if !ok {
			return SendResult{}, &ValidationError{Field: "to[" + strconv.Itoa(i) + "]", Message: "not a phone number"}
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		recipients = append(recipients, n)
	}
	now := time.Now()
	base := now
	if in.ScheduledAt != nil {
		at := in.ScheduledAt.UTC()
		if at.Before(now.Add(-time.Minute)) {
			return SendResult{}, &ValidationError{Field: "scheduled_at", Message: "must be in the future"}
		}
		if at.After(now.Add(maxScheduleAhead)) {
			return SendResult{}, &ValidationError{Field: "scheduled_at", Message: "at most 7 days ahead"}
		}
		if at.After(now) {
			base = at
		}
	}
	if s.cfg.RequireEmailVerification && user.EmailVerifiedAt == nil {
		return SendResult{}, ErrEmailUnverified
	}

	device, err := s.st.ResolveSenderDevice(ctx, user.ID, in.DeviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if in.DeviceID != nil {
				return SendResult{}, err
			}
			return SendResult{}, ErrNoDevice
		}
		return SendResult{}, err
	}
	if !device.Enabled {
		return SendResult{}, ErrDeviceDisabled
	}
	limits, err := s.billing.Limits(ctx, user.ID)
	if err != nil {
		return SendResult{}, err
	}
	if err := s.billing.CheckBatchSize(limits, len(recipients)); err != nil {
		return SendResult{}, err
	}

	sendDelay := time.Duration(device.SendDelaySeconds) * time.Second
	waves := planWaves(len(recipients), s.cfg.DispatchWaveSize, sendDelay, base)
	// When the phone should be done, given its pacing. Only omitted when the
	// send is already expected to be over, which a zero delay makes possible.
	var estimated *time.Time
	last := waves[len(waves)-1]
	if finish := last.due.Add(time.Duration(last.end-last.start) * sendDelay); finish.After(now) {
		estimated = &finish
	}
	var scheduled *time.Time
	if base.After(now) {
		scheduled = &base
	}

	msgIDs := make([]uuid.UUID, len(recipients))
	for i := range msgIDs {
		msgIDs[i] = ids.New()
	}
	var batch store.Batch
	err = s.st.Tx(ctx, func(tx pgx.Tx, st *store.Store) error {
		// Quota belongs to the day the messages go out, so a send scheduled
		// for tomorrow counts against tomorrow.
		if err := s.billing.ReserveSends(ctx, st, user.ID, limits, len(recipients), base); err != nil {
			return err
		}
		var err error
		batch, err = st.CreateBatch(ctx, store.CreateBatchParams{
			ID: ids.New(), UserID: user.ID, DeviceID: device.ID, APIKeyID: apiKeyID,
			Body: body, RecipientCount: len(recipients), RecipientPreview: preview(recipients),
			ScheduledAt: scheduled, EstimatedCompletionAt: estimated,
		})
		if err != nil {
			return err
		}
		if err := st.InsertOutbound(ctx, batch.ID, user.ID, device.ID, body, msgIDs, recipients, in.SimSubscriptionID); err != nil {
			return err
		}
		for _, w := range waves {
			waveIDs := msgIDs[w.start:w.end]
			// The wave is released at once; the phone works through it at
			// its pace, so each message has its own expected send time.
			expected := make([]time.Time, len(waveIDs))
			for i := range waveIDs {
				expected[i] = w.due.Add(time.Duration(i) * sendDelay)
			}
			if err := st.StampDispatch(ctx, waveIDs, w.due, expected); err != nil {
				return err
			}
			opts := &river.InsertOpts{}
			if w.due.After(now) {
				opts.ScheduledAt = w.due
			}
			if _, err := s.jobs.InsertTx(ctx, tx, DispatchArgs{BatchID: batch.ID, MessageIDs: waveIDs}, opts); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Batch: batch, MessageIDs: msgIDs}, nil
}

type wave struct {
	start, end int
	due        time.Time
}

// planWaves releases a batch to one phone in slices spaced by how long the
// phone needs to send the previous slice, so pushes never pile up ahead of
// what the phone can actually send.
func planWaves(n, size int, sendDelay time.Duration, base time.Time) []wave {
	if size < 1 {
		size = 1
	}
	spacing := time.Duration(size) * sendDelay
	var waves []wave
	for start, i := 0, 0; start < n; start, i = start+size, i+1 {
		waves = append(waves, wave{start: start, end: min(start+size, n), due: base.Add(time.Duration(i) * spacing)})
	}
	return waves
}

func preview(recipients []string) string {
	if len(recipients) <= 3 {
		return strings.Join(recipients, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(recipients[:3], ", "), len(recipients)-3)
}

// ---------------------------------------------------------------------------
// Dispatch worker
// ---------------------------------------------------------------------------

// DispatchArgs is one wave of a batch.
type DispatchArgs struct {
	BatchID    uuid.UUID   `json:"batch_id"`
	MessageIDs []uuid.UUID `json:"message_ids"`
}

// Kind implements river.JobArgs.
func (DispatchArgs) Kind() string { return "dispatch" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (DispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 1}
}

// DispatchWorker wakes the phone for a wave of queued messages.
type DispatchWorker struct {
	river.WorkerDefaults[DispatchArgs]
	svc *Service
}

// NewDispatchWorker builds the worker.
func NewDispatchWorker(svc *Service) *DispatchWorker { return &DispatchWorker{svc: svc} }

// Work implements river.Worker.
func (w *DispatchWorker) Work(ctx context.Context, job *river.Job[DispatchArgs]) error {
	return w.svc.dispatch(ctx, job.Args)
}

// Failure codes recorded on messages.
const (
	CodeDeviceUnpaired = "device_unpaired"
	CodeDeviceDisabled = "device_disabled"
	CodeNoPushToken    = "no_push_token"
	CodePushRejected   = "push_rejected"
)

// dispatch wakes the phone for one wave. The push carries no message
// content, only which phone should fetch its outbox; the messages stay
// queued until the phone does. Whatever still cannot reach a phone fails
// here with a reason.
func (s *Service) dispatch(ctx context.Context, args DispatchArgs) error {
	rows, err := s.st.LoadQueuedForDispatch(ctx, args.MessageIDs)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		// Already fetched by the phone, or failed.
		return nil
	}
	now := time.Now()
	device, err := s.st.GetDevice(ctx, rows[0].DeviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return s.failQueued(ctx, args.BatchID, idsOf(rows), CodeDeviceUnpaired, "The phone was unpaired before the message could be sent.", now)
		}
		return err
	}
	switch {
	case !device.Enabled:
		return s.failQueued(ctx, args.BatchID, idsOf(rows), CodeDeviceDisabled, "The phone is disabled. Enable it in the dashboard or the app.", now)
	case device.PushToken == nil || device.PushTokenInvalidatedAt != nil:
		return s.failQueued(ctx, args.BatchID, idsOf(rows), CodeNoPushToken, "The phone has no valid push registration. Open the app to reconnect it.", now)
	}

	results, err := s.push.Send(ctx, []push.Message{{
		Token:       *device.PushToken,
		Data:        map[string]string{"type": "send", "device_id": device.ID.String()},
		TTL:         s.cfg.PushTTL(),
		CollapseKey: "send",
	}})
	if err != nil {
		return fmt.Errorf("gateway: push: %w", err)
	}
	r := results[0]
	if r.OK {
		return nil
	}
	if r.TokenInvalid {
		// The device is marked before the messages fail, so anyone who sees
		// a failed batch also sees why on the device.
		_ = s.st.InvalidatePushToken(ctx, device.ID, "rejected by push service")
		return s.failQueued(ctx, args.BatchID, idsOf(rows), CodePushRejected, "The phone's push registration is no longer valid. Open the app to reconnect it.", now)
	}
	// A passing refusal: the job retries, and the phone fetches its outbox
	// on its next check-in regardless.
	if r.Err != nil {
		return fmt.Errorf("gateway: push: %w", r.Err)
	}
	return errors.New("gateway: push refused")
}

func idsOf(rows []store.DispatchRow) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// failQueued fails queued messages, fixes batch counters, and emits events.
func (s *Service) failQueued(ctx context.Context, batchID uuid.UUID, msgIDs []uuid.UUID, code, message string, at time.Time) error {
	failed, err := s.st.MarkFailedFromQueued(ctx, msgIDs, code, message, at)
	if err != nil {
		return err
	}
	if len(failed) == 0 {
		return nil
	}
	if _, err := s.st.ApplyBatchTransition(ctx, batchID, store.StatusQueued, store.StatusFailed, len(failed)); err != nil {
		return err
	}
	for _, m := range failed {
		s.emitMessage(ctx, nil, s.st, webhooks.EventMessageFailed, m)
	}
	return nil
}

// emitMessage fans a message event out; failures are logged, never fatal.
func (s *Service) emitMessage(ctx context.Context, tx pgx.Tx, st *store.Store, event string, m store.Message) {
	mid, did := m.ID, m.DeviceID
	if err := s.hooks.Emit(ctx, tx, st, webhooks.Event{UserID: m.UserID, Name: event, MessageID: &mid, DeviceID: &did, Data: m}); err != nil {
		s.log.Warn("webhook emit failed", "event", event, "message", m.ID, "err", err)
	}
}

// emitDevice fans a device event out.
func (s *Service) emitDevice(ctx context.Context, event string, d store.Device) {
	did := d.ID
	if err := s.hooks.Emit(ctx, nil, s.st, webhooks.Event{UserID: d.UserID, Name: event, DeviceID: &did, Data: d}); err != nil {
		s.log.Warn("webhook emit failed", "event", event, "device", d.ID, "err", err)
	}
}

// IsLimitError reports whether err is a plan limit rejection.
func IsLimitError(err error) (*billing.LimitError, bool) {
	var le *billing.LimitError
	if errors.As(err, &le) {
		return le, true
	}
	return nil, false
}
