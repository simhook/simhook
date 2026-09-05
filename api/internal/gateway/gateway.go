// Package gateway is the core: pairing phones, accepting sends, dispatching
// them to devices, and ingesting what devices report back.
package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/push"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/validate"
	"github.com/simhook/simhook/internal/webhooks"
)

// Errors surfaced to the HTTP layer.
var (
	ErrInvalidPairingCode = errors.New("the pairing code is wrong or has expired")
	ErrEmailUnverified    = errors.New("verify your email address before sending")
	ErrNoDevice           = errors.New("no enabled device to send from; pair a phone or pass device_id")
	ErrDeviceDisabled     = errors.New("the device is disabled")
	ErrQueueNotReady      = errors.New("job queue not configured")
)

// ValidationError is a request problem tied to one field.
type ValidationError = validate.Error

// Enqueuer is the subset of the job client the service needs.
type Enqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service implements the gateway.
type Service struct {
	st      *store.Store
	cfg     *config.Config
	push    push.Sender
	hooks   *webhooks.Service
	billing *billing.Service
	log     *slog.Logger
	jobs    Enqueuer
}

// New builds the service. Call SetQueue before sending.
func New(st *store.Store, cfg *config.Config, p push.Sender, h *webhooks.Service, b *billing.Service, log *slog.Logger) *Service {
	return &Service{st: st, cfg: cfg, push: p, hooks: h, billing: b, log: log}
}

// SetQueue wires the job client once it exists.
func (s *Service) SetQueue(q Enqueuer) { s.jobs = q }

// ---------------------------------------------------------------------------
// Pairing
// ---------------------------------------------------------------------------

const pairingCodeTTL = 10 * time.Minute

// PairingCode is a minted code, returned once with its text.
type PairingCode struct {
	ID        uuid.UUID
	Code      string
	ExpiresAt time.Time
}

// CreatePairingCode mints a code the phone exchanges for a device token.
func (s *Service) CreatePairingCode(ctx context.Context, userID uuid.UUID) (PairingCode, error) {
	code, hash, err := secrets.NewPairingCode()
	if err != nil {
		return PairingCode{}, err
	}
	expires := time.Now().Add(pairingCodeTTL)
	id, err := s.st.CreatePairingCode(ctx, userID, hash, expires)
	if err != nil {
		return PairingCode{}, err
	}
	return PairingCode{ID: id, Code: code, ExpiresAt: expires}, nil
}

// PairingStatus reports whether a code has been used and, if so, by which
// phone, so the dashboard can wait for a pairing without guessing from the
// device list.
func (s *Service) PairingStatus(ctx context.Context, userID, id uuid.UUID) (store.PairingCode, *store.Device, error) {
	pc, err := s.st.GetUserPairingCode(ctx, userID, id)
	if err != nil {
		return store.PairingCode{}, nil, err
	}
	if pc.ConsumedByDeviceID == nil {
		return pc, nil, nil
	}
	d, err := s.st.GetUserDevice(ctx, userID, *pc.ConsumedByDeviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Paired, then unpaired again before anyone looked.
			return pc, nil, nil
		}
		return store.PairingCode{}, nil, err
	}
	return pc, &d, nil
}

// PairInput is what the phone sends with a code.
type PairInput struct {
	Code           string
	HardwareKey    string
	Name           *string
	Manufacturer   *string
	Brand          *string
	Model          *string
	BuildID        *string
	OSVersion      *string
	OSAPILevel     *int32
	AppVersionName *string
	AppVersionCode *int32
	PushToken      *string
}

// Pair exchanges a code for a device and a device token. The same hardware
// on the same account re-pairs to its existing device row.
func (s *Service) Pair(ctx context.Context, in PairInput) (store.Device, string, error) {
	if strings.TrimSpace(in.HardwareKey) == "" {
		return store.Device{}, "", &ValidationError{Field: "hardware_key", Message: "required"}
	}
	hash := secrets.Hash(secrets.NormalizePairingCode(in.Code))
	var device store.Device
	var token string
	err := s.st.Tx(ctx, func(_ pgx.Tx, st *store.Store) error {
		pc, err := st.GetLivePairingCode(ctx, hash)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrInvalidPairingCode
			}
			return err
		}
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		if name == "" {
			name = strings.TrimSpace(deref(in.Brand) + " " + deref(in.Model))
		}
		if name == "" {
			name = "Android phone"
		}
		hasDefault, err := st.HasDefaultDevice(ctx, pc.UserID)
		if err != nil {
			return err
		}
		device, err = st.UpsertDevice(ctx, store.UpsertDeviceParams{
			ID: ids.New(), UserID: pc.UserID, Name: name, HardwareKey: in.HardwareKey,
			Manufacturer: in.Manufacturer, Brand: in.Brand, Model: in.Model, BuildID: in.BuildID,
			OSVersion: in.OSVersion, OSAPILevel: in.OSAPILevel,
			AppVersionName: in.AppVersionName, AppVersionCode: in.AppVersionCode,
			PushToken: in.PushToken, HeartbeatIntervalMinutes: int32(s.cfg.HeartbeatIntervalMinutes),
			MakeDefault: !hasDefault,
		})
		if err != nil {
			return err
		}
		// The same handset paired to another account earlier must not keep
		// acting for it.
		if err := st.RetireOtherPairings(ctx, in.HardwareKey, device.ID); err != nil {
			return err
		}
		limits, err := st.EffectiveLimits(ctx, pc.UserID)
		if err != nil {
			return err
		}
		if limits.DeviceLimit >= 0 {
			n, err := st.CountLiveDevices(ctx, pc.UserID)
			if err != nil {
				return err
			}
			if int32(n) > limits.DeviceLimit {
				return &billing.LimitError{Kind: "devices", Limit: limits.DeviceLimit, Used: int32(n)}
			}
		}
		if _, err := st.ConsumePairingCode(ctx, hash, device.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrInvalidPairingCode
			}
			return err
		}
		tok, tokHash, err := secrets.NewToken(secrets.PrefixDevice)
		if err != nil {
			return err
		}
		if _, err := st.CreateDeviceToken(ctx, device.ID, tokHash); err != nil {
			return err
		}
		token = tok
		return nil
	})
	if err != nil {
		return store.Device{}, "", err
	}
	return device, token, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// ListDevices returns a user's paired phones.
func (s *Service) ListDevices(ctx context.Context, userID uuid.UUID) ([]store.Device, error) {
	return s.st.ListDevices(ctx, userID)
}

// GetDevice returns one of a user's phones.
func (s *Service) GetDevice(ctx context.Context, userID, id uuid.UUID) (store.Device, error) {
	return s.st.GetUserDevice(ctx, userID, id)
}

// DevicePatch is a settings change from the dashboard or the phone.
type DevicePatch struct {
	Name                       *string
	Enabled                    *bool
	ReceiveEnabled             *bool
	SendDelaySeconds           *int32
	HeartbeatIntervalMinutes   *int32
	PreferredSimSubscriptionID *int32
	ClearPreferredSim          bool
}

func (p DevicePatch) validate() error {
	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 64 {
			return &ValidationError{Field: "name", Message: "must be 1 to 64 characters"}
		}
		*p.Name = n
	}
	if p.SendDelaySeconds != nil && (*p.SendDelaySeconds < 0 || *p.SendDelaySeconds > 3600) {
		return &ValidationError{Field: "send_delay_seconds", Message: "must be between 0 and 3600"}
	}
	if p.HeartbeatIntervalMinutes != nil && (*p.HeartbeatIntervalMinutes < 15 || *p.HeartbeatIntervalMinutes > 1440) {
		return &ValidationError{Field: "heartbeat_interval_minutes", Message: "must be between 15 and 1440"}
	}
	return nil
}

// UpdateDevice applies settings to one of the user's phones.
func (s *Service) UpdateDevice(ctx context.Context, userID, id uuid.UUID, p DevicePatch) (store.Device, error) {
	if err := p.validate(); err != nil {
		return store.Device{}, err
	}
	if _, err := s.st.GetUserDevice(ctx, userID, id); err != nil {
		return store.Device{}, err
	}
	d, err := s.st.UpdateDeviceSettings(ctx, id, store.DeviceSettingsPatch(p))
	if err != nil {
		return store.Device{}, err
	}
	s.nudge(ctx, d)
	return d, nil
}

// UpdateOwnDevice applies the settings a phone may change about itself.
func (s *Service) UpdateOwnDevice(ctx context.Context, device store.Device, p DevicePatch) (store.Device, error) {
	p.HeartbeatIntervalMinutes = nil
	if err := p.validate(); err != nil {
		return store.Device{}, err
	}
	return s.st.UpdateDeviceSettings(ctx, device.ID, store.DeviceSettingsPatch(p))
}

// SetDefaultDevice picks the phone used when a send names none.
func (s *Service) SetDefaultDevice(ctx context.Context, userID, id uuid.UUID) (store.Device, error) {
	if err := s.st.SetDefaultDevice(ctx, userID, id); err != nil {
		return store.Device{}, err
	}
	d, err := s.st.GetUserDevice(ctx, userID, id)
	if err != nil {
		return store.Device{}, err
	}
	s.nudge(ctx, d)
	return d, nil
}

// UnpairDevice removes a phone. Its message history is kept. The phone is
// nudged so it notices within seconds that its token is gone.
func (s *Service) UnpairDevice(ctx context.Context, userID, id uuid.UUID) error {
	d, err := s.st.GetUserDevice(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.st.SoftDeleteDevice(ctx, userID, id); err != nil {
		return err
	}
	s.nudge(ctx, d)
	return nil
}

// nudge asks a phone to check in now, so a change made in the dashboard or
// through the API reaches it within seconds rather than at its next scheduled
// heartbeat. Best effort: a phone that is offline picks the change up later.
func (s *Service) nudge(ctx context.Context, d store.Device) {
	if d.PushToken == nil || *d.PushToken == "" || d.PushTokenInvalidatedAt != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	results, err := s.push.Send(ctx, []push.Message{{Token: *d.PushToken, Data: map[string]string{"type": "heartbeat"}, TTL: 5 * time.Minute, CollapseKey: "heartbeat"}})
	if err != nil {
		s.log.Warn("device nudge failed", "device", d.ID, "err", err)
		return
	}
	if len(results) == 1 && results[0].TokenInvalid {
		_ = s.st.InvalidatePushToken(ctx, d.ID, "rejected by push service during nudge")
	}
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// Page is a keyset-paged result.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

// EncodeCursor turns a position into an opaque string.
func EncodeCursor(c store.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", c.CreatedAt.UnixNano(), c.ID)))
}

// DecodeCursor parses an opaque cursor.
func DecodeCursor(s string) (*store.Cursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, &ValidationError{Field: "cursor", Message: "invalid"}
	}
	var nanos int64
	var idStr string
	if _, err := fmt.Sscanf(string(raw), "%d:%s", &nanos, &idStr); err != nil {
		return nil, &ValidationError{Field: "cursor", Message: "invalid"}
	}
	id, ok := ids.Parse(idStr)
	if !ok {
		return nil, &ValidationError{Field: "cursor", Message: "invalid"}
	}
	return &store.Cursor{CreatedAt: time.Unix(0, nanos), ID: id}, nil
}

// ListMessages pages account history.
func (s *Service) ListMessages(ctx context.Context, userID uuid.UUID, f store.MessageFilter) (Page[store.Message], error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	rows, err := s.st.ListMessages(ctx, userID, f)
	if err != nil {
		return Page[store.Message]{}, err
	}
	page := Page[store.Message]{Items: rows}
	if len(rows) > f.Limit {
		page.Items = rows[:f.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = EncodeCursor(store.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return page, nil
}

// GetMessage returns one message on the account.
func (s *Service) GetMessage(ctx context.Context, userID, id uuid.UUID) (store.Message, error) {
	return s.st.GetUserMessage(ctx, userID, id)
}

// ListBatches pages sends, newest first.
func (s *Service) ListBatches(ctx context.Context, userID uuid.UUID, cursor *store.Cursor, limit int) (Page[store.Batch], error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.st.ListBatches(ctx, userID, cursor, limit)
	if err != nil {
		return Page[store.Batch]{}, err
	}
	page := Page[store.Batch]{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = EncodeCursor(store.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return page, nil
}

// GetBatch returns a send and its messages.
func (s *Service) GetBatch(ctx context.Context, userID, id uuid.UUID) (store.Batch, []store.Message, error) {
	b, err := s.st.GetUserBatch(ctx, userID, id)
	if err != nil {
		return store.Batch{}, nil, err
	}
	msgs, err := s.st.ListBatchMessages(ctx, userID, id)
	return b, msgs, err
}

// Stats are account totals for the dashboard.
type Stats struct {
	Sent     int64 `json:"sent"`
	Received int64 `json:"received"`
	Devices  int   `json:"devices"`
}

// GetStats sums the account.
func (s *Service) GetStats(ctx context.Context, userID uuid.UUID) (Stats, error) {
	sent, received, err := s.st.UserMessageTotals(ctx, userID)
	if err != nil {
		return Stats{}, err
	}
	n, err := s.st.CountLiveDevices(ctx, userID)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Sent: sent, Received: received, Devices: n}, nil
}
