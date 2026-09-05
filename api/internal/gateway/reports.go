package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/push"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/webhooks"
)

// ---------------------------------------------------------------------------
// What the phone reports
// ---------------------------------------------------------------------------

// allowedFrom lists which states a report may move a message out of. Reports
// that arrive out of order (a late "sent" after "delivered") are ignored
// rather than moving the message backwards.
var allowedFrom = map[string][]string{
	store.StatusSent:      {store.StatusQueued, store.StatusDispatched},
	store.StatusDelivered: {store.StatusQueued, store.StatusDispatched, store.StatusSent},
	store.StatusFailed:    {store.StatusQueued, store.StatusDispatched, store.StatusSent},
}

var statusEvent = map[string]string{
	store.StatusSent:      webhooks.EventMessageSent,
	store.StatusDelivered: webhooks.EventMessageDelivered,
	store.StatusFailed:    webhooks.EventMessageFailed,
	store.StatusUnknown:   webhooks.EventMessageUnknown,
}

// ReportStatus records what happened to an outbound message on the phone.
// It is idempotent: a repeated or out-of-order report returns the current
// message without changing it. The transition, the batch counters, and the
// webhook fan-out commit together.
func (s *Service) ReportStatus(ctx context.Context, device store.Device, messageID uuid.UUID, status string, at time.Time, errorCode, errorMessage *string) (store.Message, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	from, ok := allowedFrom[status]
	if !ok {
		return store.Message{}, &ValidationError{Field: "status", Message: "must be sent, delivered, or failed"}
	}
	if at.IsZero() {
		at = time.Now()
	}
	if status == store.StatusFailed && (errorCode == nil || *errorCode == "") {
		code := "send_failed"
		errorCode = &code
	}
	var out store.Message
	err := s.st.Tx(ctx, func(tx pgx.Tx, st *store.Store) error {
		t, err := st.TransitionMessage(ctx, messageID, device.ID, from, status, at, errorCode, errorMessage)
		if err != nil {
			return err
		}
		if t.BatchID != nil {
			if _, err := st.ApplyBatchTransition(ctx, *t.BatchID, t.PrevStatus, status, 1); err != nil {
				return err
			}
		}
		if status == store.StatusSent {
			// A message counts as sent by the phone once the carrier took it,
			// not when it was handed over.
			if err := st.AddDeviceCounts(ctx, device.ID, 1, 0); err != nil {
				return err
			}
		}
		s.emitMessage(ctx, tx, st, statusEvent[status], t.Message)
		out = t.Message
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Either the message is not on this device, or it is already past
			// this state. Only the first is an error.
			return s.st.GetDeviceMessage(ctx, device.ID, messageID)
		}
		return store.Message{}, err
	}
	return out, nil
}

// InboundInput is a received SMS.
type InboundInput struct {
	Sender            string
	Body              string
	ReceivedAt        time.Time
	Fingerprint       *string
	SimSubscriptionID *int32
}

// ReportInbound stores a received SMS. The second return is false when the
// same message was already stored.
func (s *Service) ReportInbound(ctx context.Context, device store.Device, in InboundInput) (store.Message, bool, error) {
	sender := strings.TrimSpace(in.Sender)
	if sender == "" {
		return store.Message{}, false, &ValidationError{Field: "sender", Message: "required"}
	}
	if in.Body == "" {
		return store.Message{}, false, &ValidationError{Field: "body", Message: "required"}
	}
	if in.ReceivedAt.IsZero() {
		in.ReceivedAt = time.Now()
	}
	var fp *string
	if in.Fingerprint != nil {
		if f := strings.TrimSpace(*in.Fingerprint); f != "" && len(f) <= 128 {
			fp = &f
		}
	}
	var msg store.Message
	var inserted bool
	err := s.st.Tx(ctx, func(tx pgx.Tx, st *store.Store) error {
		var err error
		msg, inserted, err = st.InsertInbound(ctx, store.InsertInboundParams{
			ID: ids.New(), UserID: device.UserID, DeviceID: device.ID,
			Sender: sender, Body: in.Body, ReceivedAt: in.ReceivedAt,
			Fingerprint: fp, SimSubscriptionID: in.SimSubscriptionID,
		})
		if err != nil || !inserted {
			return err
		}
		if err := st.AddReceived(ctx, device.UserID, time.Now()); err != nil {
			return err
		}
		if err := st.AddDeviceCounts(ctx, device.ID, 0, 1); err != nil {
			return err
		}
		s.emitMessage(ctx, tx, st, webhooks.EventMessageReceived, msg)
		return nil
	})
	if err != nil {
		return store.Message{}, false, err
	}
	return msg, inserted, nil
}

// HeartbeatInput is a check-in.
type HeartbeatInput struct {
	PushToken      *string
	AppVersionName *string
	AppVersionCode *int32
	OSVersion      *string
	OSAPILevel     *int32
	Telemetry      json.RawMessage
	Sims           json.RawMessage
}

// Heartbeat records a check-in and returns the device so the phone can sync
// settings. A device that was offline emits device.online, once, however
// many check-ins race.
func (s *Service) Heartbeat(ctx context.Context, device store.Device, in HeartbeatInput) (store.Device, error) {
	if in.PushToken != nil && strings.TrimSpace(*in.PushToken) == "" {
		in.PushToken = nil
	}
	d, wasOnline, err := s.st.RecordHeartbeat(ctx, device.ID, store.HeartbeatParams(in))
	if err != nil {
		return store.Device{}, err
	}
	if !wasOnline {
		s.emitDevice(ctx, webhooks.EventDeviceOnline, d)
	}
	return d, nil
}

// RefreshPushToken stores a new push registration.
func (s *Service) RefreshPushToken(ctx context.Context, device store.Device, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return &ValidationError{Field: "token", Message: "required"}
	}
	return s.st.SetPushToken(ctx, device.ID, token)
}

// ListDeviceMessages pages one phone's own history for the app.
func (s *Service) ListDeviceMessages(ctx context.Context, device store.Device, f store.MessageFilter) (Page[store.Message], error) {
	f.DeviceIDs = []uuid.UUID{device.ID}
	return s.ListMessages(ctx, device.UserID, f)
}

// ---------------------------------------------------------------------------
// Periodic sweeps
// ---------------------------------------------------------------------------

// ReconcileArgs flips silent in-flight messages to unknown.
type ReconcileArgs struct{}

// Kind implements river.JobArgs.
func (ReconcileArgs) Kind() string { return "reconcile_stale" }

// ReconcileWorker runs the stale sweep.
type ReconcileWorker struct {
	river.WorkerDefaults[ReconcileArgs]
	svc *Service
}

// NewReconcileWorker builds the worker.
func NewReconcileWorker(svc *Service) *ReconcileWorker { return &ReconcileWorker{svc: svc} }

// Work implements river.Worker.
func (w *ReconcileWorker) Work(ctx context.Context, _ *river.Job[ReconcileArgs]) error {
	return w.svc.reconcileStale(ctx)
}

// reconcileStale moves messages nobody reported on to unknown. The flip, the
// batch counters, and the webhook fan-out commit together so a crash in the
// middle cannot leave counters drifted.
func (s *Service) reconcileStale(ctx context.Context) error {
	var count int
	err := s.st.Tx(ctx, func(tx pgx.Tx, st *store.Store) error {
		stale, err := st.MarkStaleUnknown(ctx, time.Now().Add(-s.cfg.StaleAfter()))
		if err != nil {
			return err
		}
		count = len(stale)
		if count == 0 {
			return nil
		}
		type key struct {
			batch uuid.UUID
			prev  string
		}
		counts := map[key]int{}
		for _, m := range stale {
			if m.BatchID != nil {
				counts[key{*m.BatchID, m.PrevStatus}]++
			}
		}
		for k, n := range counts {
			if _, err := st.ApplyBatchTransition(ctx, k.batch, k.prev, store.StatusUnknown, n); err != nil {
				return err
			}
		}
		for _, sm := range stale {
			m, err := st.GetUserMessage(ctx, sm.UserID, sm.ID)
			if err != nil {
				return err
			}
			s.emitMessage(ctx, tx, st, webhooks.EventMessageUnknown, m)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if count > 0 {
		s.log.Info("stale messages reconciled", "count", count)
	}
	return nil
}

// PresenceArgs marks silent devices offline and probes them.
type PresenceArgs struct{}

// Kind implements river.JobArgs.
func (PresenceArgs) Kind() string { return "presence_sweep" }

// PresenceWorker runs the presence sweep.
type PresenceWorker struct {
	river.WorkerDefaults[PresenceArgs]
	svc *Service
}

// NewPresenceWorker builds the worker.
func NewPresenceWorker(svc *Service) *PresenceWorker { return &PresenceWorker{svc: svc} }

// Work implements river.Worker.
func (w *PresenceWorker) Work(ctx context.Context, _ *river.Job[PresenceArgs]) error {
	return w.svc.sweepPresence(ctx)
}

const probeTTL = 15 * time.Minute

func (s *Service) sweepPresence(ctx context.Context) error {
	now := time.Now()
	offline, err := s.st.MarkOfflineStale(ctx, now.Add(-s.cfg.OfflineAfter()))
	if err != nil {
		return err
	}
	for _, d := range offline {
		s.emitDevice(ctx, webhooks.EventDeviceOffline, d)
	}

	// Probe anything silent for longer than half the offline window so a
	// phone that merely dozed gets nudged before it is declared offline.
	probe, err := s.st.ListDevicesToProbe(ctx, now.Add(-s.cfg.OfflineAfter()/2))
	if err != nil {
		return err
	}
	if len(probe) == 0 {
		return nil
	}
	msgs := make([]push.Message, len(probe))
	for i, d := range probe {
		msgs[i] = push.Message{Token: *d.PushToken, Data: map[string]string{"type": "heartbeat"}, TTL: probeTTL, CollapseKey: "heartbeat"}
	}
	results, err := s.push.Send(ctx, msgs)
	if err != nil {
		return err
	}
	invalid := 0
	for i, r := range results {
		if r.TokenInvalid {
			invalid++
			_ = s.st.InvalidatePushToken(ctx, probe[i].ID, "rejected by push service during presence probe")
		}
	}
	s.log.Info("presence sweep", "offline", len(offline), "probed", len(probe), "invalid_tokens", invalid)
	return nil
}
