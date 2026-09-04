// Package webhooks manages event subscriptions and delivers events to them
// with signed requests, retries, and automatic pausing of dead endpoints.
package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/mail"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/store"
)

// Event names.
const (
	EventMessageReceived  = "message.received"
	EventMessageSent      = "message.sent"
	EventMessageDelivered = "message.delivered"
	EventMessageFailed    = "message.failed"
	EventMessageUnknown   = "message.unknown"
	EventDeviceOnline     = "device.online"
	EventDeviceOffline    = "device.offline"
	EventPing             = "ping"
)

// AllEvents lists every subscribable event.
var AllEvents = []string{
	EventMessageReceived, EventMessageSent, EventMessageDelivered, EventMessageFailed,
	EventMessageUnknown, EventDeviceOnline, EventDeviceOffline, EventPing,
}

// Errors surfaced to the HTTP layer.
var (
	ErrInvalidURL    = errors.New("delivery URL must be an http or https URL with a public host")
	ErrInvalidEvents = errors.New("choose at least one known event")
	ErrTooMany       = errors.New("webhook limit reached for this account")
	ErrQueueNotReady = errors.New("job queue not configured")
)

// Enqueuer is the subset of the job client the service needs.
type Enqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service manages subscriptions and deliveries.
type Service struct {
	st     *store.Store
	cfg    *config.Config
	box    *secrets.Box
	mailer mail.Mailer
	log    *slog.Logger
	client *http.Client
	jobs   Enqueuer
}

// New builds the service. Call SetQueue before emitting events.
func New(st *store.Store, cfg *config.Config, box *secrets.Box, m mail.Mailer, log *slog.Logger) *Service {
	return &Service{
		st: st, cfg: cfg, box: box, mailer: m, log: log,
		client: newHTTPClient(cfg.WebhookTimeout(), cfg.WebhookAllowPrivateHosts),
	}
}

// SetQueue wires the job client once it exists.
func (s *Service) SetQueue(q Enqueuer) { s.jobs = q }

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

// CreateInput describes a new subscription.
type CreateInput struct {
	Name   *string
	URL    string
	Events []string
}

// Create stores a subscription and returns the signing secret once.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (store.Webhook, string, error) {
	if err := s.ValidateURL(in.URL); err != nil {
		return store.Webhook{}, "", err
	}
	events, err := normalizeEvents(in.Events)
	if err != nil {
		return store.Webhook{}, "", err
	}
	n, err := s.st.CountWebhooks(ctx, userID)
	if err != nil {
		return store.Webhook{}, "", err
	}
	if n >= s.cfg.WebhookMaxPerUser {
		return store.Webhook{}, "", ErrTooMany
	}
	secret, err := secrets.NewWebhookSecret()
	if err != nil {
		return store.Webhook{}, "", err
	}
	enc, err := s.box.Seal([]byte(secret))
	if err != nil {
		return store.Webhook{}, "", err
	}
	w, err := s.st.CreateWebhook(ctx, ids.New(), userID, trimName(in.Name), in.URL, enc, events)
	if err != nil {
		return store.Webhook{}, "", err
	}
	return w, secret, nil
}

// List returns a user's subscriptions.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]store.Webhook, error) {
	return s.st.ListWebhooks(ctx, userID)
}

// Get returns one subscription.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (store.Webhook, error) {
	return s.st.GetUserWebhook(ctx, userID, id)
}

// UpdateInput patches a subscription. Nil leaves a field alone.
type UpdateInput struct {
	Name    *string
	URL     *string
	Events  []string
	Enabled *bool
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, in UpdateInput) (store.Webhook, error) {
	patch := store.WebhookPatch{Name: trimName(in.Name), URL: in.URL, Enabled: in.Enabled}
	if in.URL != nil {
		if err := s.ValidateURL(*in.URL); err != nil {
			return store.Webhook{}, err
		}
	}
	if in.Events != nil {
		events, err := normalizeEvents(in.Events)
		if err != nil {
			return store.Webhook{}, err
		}
		patch.Events = events
	}
	return s.st.UpdateWebhook(ctx, userID, id, patch)
}

// RotateSecret replaces the signing secret and returns the new one once.
func (s *Service) RotateSecret(ctx context.Context, userID, id uuid.UUID) (string, error) {
	secret, err := secrets.NewWebhookSecret()
	if err != nil {
		return "", err
	}
	enc, err := s.box.Seal([]byte(secret))
	if err != nil {
		return "", err
	}
	if _, err := s.st.UpdateWebhook(ctx, userID, id, store.WebhookPatch{SecretEnc: enc}); err != nil {
		return "", err
	}
	return secret, nil
}

// Delete removes a subscription; its delivery history stays.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	return s.st.SoftDeleteWebhook(ctx, userID, id)
}

// SendTest queues a ping delivery to one subscription regardless of its
// event list.
func (s *Service) SendTest(ctx context.Context, userID, id uuid.UUID) (store.Delivery, error) {
	w, err := s.st.GetUserWebhook(ctx, userID, id)
	if err != nil {
		return store.Delivery{}, err
	}
	data := map[string]any{"webhook_id": w.ID, "message": "This is a test delivery from simhook."}
	deliveries, err := s.deliverTo(ctx, nil, s.st, []store.Webhook{w}, EventPing, nil, nil, data)
	if err != nil {
		return store.Delivery{}, err
	}
	return deliveries[0], nil
}

// ListDeliveries pages a user's delivery history.
func (s *Service) ListDeliveries(ctx context.Context, userID uuid.UUID, f store.DeliveryFilter) ([]store.Delivery, error) {
	return s.st.ListDeliveries(ctx, userID, f)
}

// GetDelivery returns one delivery on the user's account.
func (s *Service) GetDelivery(ctx context.Context, userID, id uuid.UUID) (store.Delivery, error) {
	return s.st.GetUserDelivery(ctx, userID, id)
}

func trimName(n *string) *string {
	if n == nil {
		return nil
	}
	t := strings.TrimSpace(*n)
	if len(t) > 64 {
		t = t[:64]
	}
	return &t
}

func normalizeEvents(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, e := range in {
		e = strings.ToLower(strings.TrimSpace(e))
		if !slices.Contains(AllEvents, e) {
			return nil, ErrInvalidEvents
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, ErrInvalidEvents
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// URL safety
// ---------------------------------------------------------------------------

// ValidateURL rejects URLs that are not http(s) or that point at private
// address space, unless the deployment allows private hosts.
func (s *Service) ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ErrInvalidURL
	}
	if s.cfg.WebhookAllowPrivateHosts {
		return nil
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return ErrInvalidURL
	}
	if ip, err := netip.ParseAddr(host); err == nil && isPrivateIP(ip) {
		return ErrInvalidURL
	}
	return nil
}

func isPrivateIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if ip.Is4() {
		b := ip.As4()
		// 0.0.0.0/8, 100.64.0.0/10 (carrier NAT), 192.0.0.0/24, 198.18.0.0/15, 240.0.0.0/4
		switch {
		case b[0] == 0,
			b[0] == 100 && b[1] >= 64 && b[1] <= 127,
			b[0] == 192 && b[1] == 0 && b[2] == 0,
			b[0] == 198 && (b[1] == 18 || b[1] == 19),
			b[0] >= 240:
			return true
		}
	}
	return false
}

// newHTTPClient builds a client that resolves the destination itself and
// refuses private addresses at connect time, which also defeats DNS
// rebinding. Redirects are not followed.
func newHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy:               nil,
		MaxIdleConns:        50,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range addrs {
				if !allowPrivate && isPrivateIP(ip) {
					lastErr = fmt.Errorf("refusing to connect to private address %s", ip)
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no addresses for %s", host)
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ---------------------------------------------------------------------------
// Emitting events
// ---------------------------------------------------------------------------

// Event is something that happened on an account.
type Event struct {
	UserID    uuid.UUID
	Name      string
	MessageID *uuid.UUID
	DeviceID  *uuid.UUID
	Data      any
}

// Emit fans an event out to every enabled subscription. When tx is non-nil
// the delivery rows and jobs are written inside it.
func (s *Service) Emit(ctx context.Context, tx pgx.Tx, st *store.Store, ev Event) error {
	subs, err := st.ListWebhooksForEvent(ctx, ev.UserID, ev.Name)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	_, err = s.deliverTo(ctx, tx, st, subs, ev.Name, ev.MessageID, ev.DeviceID, ev.Data)
	return err
}

type payload struct {
	ID        uuid.UUID `json:"id"`
	Event     string    `json:"event"`
	CreatedAt time.Time `json:"created_at"`
	Data      any       `json:"data"`
}

func (s *Service) deliverTo(ctx context.Context, tx pgx.Tx, st *store.Store, subs []store.Webhook, event string, messageID, deviceID *uuid.UUID, data any) ([]store.Delivery, error) {
	if s.jobs == nil {
		return nil, ErrQueueNotReady
	}
	body, err := json.Marshal(payload{ID: ids.New(), Event: event, CreatedAt: time.Now().UTC(), Data: data})
	if err != nil {
		return nil, err
	}
	out := make([]store.Delivery, 0, len(subs))
	for _, w := range subs {
		d, err := st.CreateDelivery(ctx, ids.New(), w.ID, messageID, deviceID, event, body, w.URL)
		if err != nil {
			return nil, err
		}
		args := DeliverArgs{DeliveryID: d.ID}
		if tx != nil {
			_, err = s.jobs.InsertTx(ctx, tx, args, nil)
		} else {
			_, err = s.jobs.Insert(ctx, args, nil)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Delivery worker
// ---------------------------------------------------------------------------

// DeliverArgs identifies one delivery to attempt.
type DeliverArgs struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
}

// Kind implements river.JobArgs.
func (DeliverArgs) Kind() string { return "webhook_deliver" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (DeliverArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Queue: QueueName}
}

// QueueName isolates webhook traffic from message dispatch.
const QueueName = "webhooks"

// DeliverWorker performs delivery attempts.
type DeliverWorker struct {
	river.WorkerDefaults[DeliverArgs]
	svc *Service
}

// NewDeliverWorker builds the worker.
func NewDeliverWorker(svc *Service) *DeliverWorker { return &DeliverWorker{svc: svc} }

// Timeout bounds one attempt including the HTTP call.
func (w *DeliverWorker) Timeout(*river.Job[DeliverArgs]) time.Duration {
	return w.svc.cfg.WebhookTimeout() + 15*time.Second
}

// Work implements river.Worker.
func (w *DeliverWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	return w.svc.attempt(ctx, job.Args.DeliveryID)
}

// Backoff between attempts. The first retry is quick to absorb a blip; the
// rest spread over a day.
var retryDelays = [...]time.Duration{
	1 * time.Minute, 5 * time.Minute, 15 * time.Minute, 1 * time.Hour,
	3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

const (
	maxAttempts             = len(retryDelays) + 1
	maxAttemptsNonRetryable = 3
	responseExcerptLimit    = 1000
)

func (s *Service) attempt(ctx context.Context, deliveryID uuid.UUID) error {
	d, err := s.st.GetDelivery(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return river.JobCancel(err)
		}
		return err
	}
	if d.Status == "delivered" || d.Status == "failed" {
		return nil
	}
	w, err := s.st.GetWebhook(ctx, d.WebhookID)
	if err != nil {
		return err
	}
	if !w.Enabled || w.DeletedAt != nil {
		reason := "webhook is disabled"
		_, err := s.st.RecordDeliveryAttempt(ctx, d.ID, store.DeliveryOutcome{Abandoned: true, Error: &reason})
		return err
	}
	if err := s.ValidateURL(w.URL); err != nil {
		reason := err.Error()
		_, err := s.st.RecordDeliveryAttempt(ctx, d.ID, store.DeliveryOutcome{Abandoned: true, Error: &reason})
		return err
	}
	secretBytes, err := s.box.Open(w.SecretEnc)
	if err != nil {
		return fmt.Errorf("webhooks: decrypt secret: %w", err)
	}

	now := time.Now()
	body := []byte(d.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		reason := err.Error()
		_, err := s.st.RecordDeliveryAttempt(ctx, d.ID, store.DeliveryOutcome{Abandoned: true, Error: &reason})
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "simhook-webhooks/1.0")
	req.Header.Set("X-Simhook-Event", d.Event)
	req.Header.Set("X-Simhook-Delivery", d.ID.String())
	req.Header.Set("X-Simhook-Signature", secrets.SignWebhook(string(secretBytes), now.Unix(), body))

	outcome := store.DeliveryOutcome{}
	retryable := false
	resp, err := s.client.Do(req)
	if err != nil {
		msg := trimErr(err)
		outcome.Error = &msg
		retryable = true
	} else {
		excerpt := readExcerpt(resp.Body)
		resp.Body.Close()
		status := int32(resp.StatusCode)
		outcome.HTTPStatus = &status
		outcome.ResponseExcerpt = &excerpt
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			outcome.Delivered = true
		case resp.StatusCode == 408 || resp.StatusCode == 425 || resp.StatusCode == 429 || resp.StatusCode >= 500:
			retryable = true
			msg := fmt.Sprintf("endpoint returned HTTP %d", resp.StatusCode)
			outcome.Error = &msg
		default:
			msg := fmt.Sprintf("endpoint rejected the request with HTTP %d", resp.StatusCode)
			outcome.Error = &msg
		}
	}

	attempts := int(d.AttemptCount) + 1
	var snooze time.Duration
	if !outcome.Delivered {
		limit := maxAttemptsNonRetryable
		if retryable {
			limit = maxAttempts
		}
		if attempts >= limit {
			outcome.Abandoned = true
		} else {
			snooze = retryDelays[min(attempts-1, len(retryDelays)-1)]
			next := now.Add(snooze)
			outcome.NextAttemptAt = &next
		}
	}
	if _, err := s.st.RecordDeliveryAttempt(ctx, d.ID, outcome); err != nil {
		return err
	}
	if snooze > 0 {
		return river.JobSnooze(snooze)
	}
	return nil
}

func readExcerpt(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, responseExcerptLimit))
	return string(b)
}

func trimErr(err error) string {
	s := err.Error()
	if len(s) > responseExcerptLimit {
		s = s[:responseExcerptLimit]
	}
	return s
}

// ---------------------------------------------------------------------------
// Auto-pause
// ---------------------------------------------------------------------------

// AutoPauseArgs runs the dead-endpoint sweep.
type AutoPauseArgs struct{}

// Kind implements river.JobArgs.
func (AutoPauseArgs) Kind() string { return "webhook_autopause" }

// AutoPauseWorker pauses webhooks that keep failing.
type AutoPauseWorker struct {
	river.WorkerDefaults[AutoPauseArgs]
	svc *Service
}

// NewAutoPauseWorker builds the worker.
func NewAutoPauseWorker(svc *Service) *AutoPauseWorker { return &AutoPauseWorker{svc: svc} }

const (
	autoPauseWindow      = 7 * 24 * time.Hour
	autoPauseMinFailures = 20
	autoPauseRecentOK    = 24 * time.Hour
)

// Work implements river.Worker.
func (w *AutoPauseWorker) Work(ctx context.Context, _ *river.Job[AutoPauseArgs]) error {
	return w.svc.autoPause(ctx)
}

func (s *Service) autoPause(ctx context.Context) error {
	now := time.Now()
	stats, err := s.st.WebhookFailureStatsSince(ctx, now.Add(-autoPauseWindow))
	if err != nil {
		return err
	}
	for _, st := range stats {
		total := st.Failed + st.Delivered
		if st.Failed < autoPauseMinFailures || st.Failed*2 < total {
			continue
		}
		w, err := s.st.GetWebhook(ctx, st.WebhookID)
		if err != nil {
			continue
		}
		if w.LastSuccessAt != nil && w.LastSuccessAt.After(now.Add(-autoPauseRecentOK)) {
			continue
		}
		reason := fmt.Sprintf("Paused automatically: %d of %d deliveries in the last 7 days failed.", st.Failed, total)
		if err := s.st.DisableWebhook(ctx, w.ID, reason); err != nil {
			s.log.Warn("auto-pause failed", "webhook", w.ID, "err", err)
			continue
		}
		s.log.Info("webhook auto-paused", "webhook", w.ID, "failed", st.Failed, "total", total)
		u, err := s.st.GetUser(ctx, w.UserID)
		if err != nil {
			continue
		}
		name := ""
		if w.Name != nil {
			name = *w.Name
		}
		if err := s.mailer.Send(ctx, mail.WebhookPaused(u.Email, name, w.URL, reason, s.cfg.WebURL+"/webhooks")); err != nil {
			s.log.Warn("auto-pause email failed", "webhook", w.ID, "err", err)
		}
	}
	return nil
}
