package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Device is a paired phone.
type Device struct {
	ID                         uuid.UUID       `db:"id" json:"id"`
	UserID                     uuid.UUID       `db:"user_id" json:"-"`
	Name                       string          `db:"name" json:"name"`
	Enabled                    bool            `db:"enabled" json:"enabled"`
	IsDefault                  bool            `db:"is_default" json:"is_default"`
	HardwareKey                string          `db:"hardware_key" json:"-"`
	Manufacturer               *string         `db:"manufacturer" json:"manufacturer"`
	Brand                      *string         `db:"brand" json:"brand"`
	Model                      *string         `db:"model" json:"model"`
	BuildID                    *string         `db:"build_id" json:"-"`
	OSVersion                  *string         `db:"os_version" json:"os_version"`
	OSAPILevel                 *int32          `db:"os_api_level" json:"os_api_level"`
	AppVersionName             *string         `db:"app_version_name" json:"app_version_name"`
	AppVersionCode             *int32          `db:"app_version_code" json:"app_version_code"`
	PushToken                  *string         `db:"push_token" json:"-"`
	PushTokenUpdatedAt         *time.Time      `db:"push_token_updated_at" json:"-"`
	PushTokenInvalidatedAt     *time.Time      `db:"push_token_invalidated_at" json:"push_token_invalidated_at"`
	PushTokenInvalidReason     *string         `db:"push_token_invalid_reason" json:"push_token_invalid_reason"`
	ReceiveEnabled             bool            `db:"receive_enabled" json:"receive_enabled"`
	SendDelaySeconds           int32           `db:"send_delay_seconds" json:"send_delay_seconds"`
	HeartbeatIntervalMinutes   int32           `db:"heartbeat_interval_minutes" json:"heartbeat_interval_minutes"`
	PreferredSimSubscriptionID *int32          `db:"preferred_sim_subscription_id" json:"preferred_sim_subscription_id"`
	LastHeartbeatAt            *time.Time      `db:"last_heartbeat_at" json:"last_heartbeat_at"`
	Online                     bool            `db:"online" json:"online"`
	Telemetry                  json.RawMessage `db:"telemetry" json:"telemetry"`
	Sims                       json.RawMessage `db:"sims" json:"sims"`
	SentCount                  int64           `db:"sent_count" json:"sent_count"`
	ReceivedCount              int64           `db:"received_count" json:"received_count"`
	DeletedAt                  *time.Time      `db:"deleted_at" json:"-"`
	CreatedAt                  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt                  time.Time       `db:"updated_at" json:"updated_at"`
}

const deviceCols = `id, user_id, name, enabled, is_default, hardware_key, manufacturer, brand, model, build_id,
	os_version, os_api_level, app_version_name, app_version_code, push_token, push_token_updated_at,
	push_token_invalidated_at, push_token_invalid_reason, receive_enabled, send_delay_seconds,
	heartbeat_interval_minutes, preferred_sim_subscription_id, last_heartbeat_at, online, telemetry, sims,
	sent_count, received_count, deleted_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Pairing
// ---------------------------------------------------------------------------

// PairingCode is a short-lived code a phone exchanges for a device token.
type PairingCode struct {
	ID                 uuid.UUID  `db:"id"`
	UserID             uuid.UUID  `db:"user_id"`
	CodeHash           []byte     `db:"code_hash"`
	ExpiresAt          time.Time  `db:"expires_at"`
	ConsumedAt         *time.Time `db:"consumed_at"`
	ConsumedByDeviceID *uuid.UUID `db:"consumed_by_device_id"`
	CreatedAt          time.Time  `db:"created_at"`
}

// CreatePairingCode stores a hashed code and returns its id.
func (s *Store) CreatePairingCode(ctx context.Context, userID uuid.UUID, hash []byte, expires time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.q.QueryRow(ctx, `insert into pairing_codes (user_id, code_hash, expires_at) values ($1, $2, $3) returning id`,
		userID, hash, expires).Scan(&id)
	return id, err
}

// GetUserPairingCode fetches one of the user's codes, used or not.
func (s *Store) GetUserPairingCode(ctx context.Context, userID, id uuid.UUID) (PairingCode, error) {
	return one[PairingCode](s.q.Query(ctx, `
		select id, user_id, code_hash, expires_at, consumed_at, consumed_by_device_id, created_at
		from pairing_codes where id = $1 and user_id = $2`, id, userID))
}

// GetLivePairingCode looks up an unconsumed, unexpired code.
func (s *Store) GetLivePairingCode(ctx context.Context, hash []byte) (PairingCode, error) {
	return one[PairingCode](s.q.Query(ctx, `
		select id, user_id, code_hash, expires_at, consumed_at, consumed_by_device_id, created_at
		from pairing_codes where code_hash = $1 and consumed_at is null and expires_at > now()`, hash))
}

// ConsumePairingCode burns a live code and returns it. Returns ErrNotFound
// when the code is unknown, used, or expired.
func (s *Store) ConsumePairingCode(ctx context.Context, hash []byte, deviceID uuid.UUID) (PairingCode, error) {
	return one[PairingCode](s.q.Query(ctx, `
		update pairing_codes set consumed_at = now(), consumed_by_device_id = $2
		where code_hash = $1 and consumed_at is null and expires_at > now()
		returning id, user_id, code_hash, expires_at, consumed_at, consumed_by_device_id, created_at`,
		hash, deviceID))
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// UpsertDeviceParams describes a phone at pairing time.
type UpsertDeviceParams struct {
	ID                       uuid.UUID
	UserID                   uuid.UUID
	Name                     string
	HardwareKey              string
	Manufacturer             *string
	Brand                    *string
	Model                    *string
	BuildID                  *string
	OSVersion                *string
	OSAPILevel               *int32
	AppVersionName           *string
	AppVersionCode           *int32
	PushToken                *string
	HeartbeatIntervalMinutes int32
	MakeDefault              bool
}

// UpsertDevice creates a device, or revives and updates the row for the
// same hardware on the same account. A revived row starts over with the
// settings a new phone gets; a live row keeps its name and settings.
func (s *Store) UpsertDevice(ctx context.Context, p UpsertDeviceParams) (Device, error) {
	return one[Device](s.q.Query(ctx, `
		insert into devices (id, user_id, name, hardware_key, manufacturer, brand, model, build_id,
			os_version, os_api_level, app_version_name, app_version_code, push_token, push_token_updated_at,
			heartbeat_interval_minutes, is_default)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			case when $13::text is null then null else now() end, $14, $15)
		on conflict (user_id, hardware_key) do update set
			name = case when devices.deleted_at is null then devices.name else excluded.name end,
			manufacturer = excluded.manufacturer,
			brand = excluded.brand,
			model = excluded.model,
			build_id = excluded.build_id,
			os_version = excluded.os_version,
			os_api_level = excluded.os_api_level,
			app_version_name = excluded.app_version_name,
			app_version_code = excluded.app_version_code,
			push_token = coalesce(excluded.push_token, devices.push_token),
			push_token_updated_at = case when excluded.push_token is null then devices.push_token_updated_at else now() end,
			push_token_invalidated_at = null,
			push_token_invalid_reason = null,
			enabled = true,
			receive_enabled = case when devices.deleted_at is null then devices.receive_enabled else true end,
			send_delay_seconds = case when devices.deleted_at is null then devices.send_delay_seconds else 5 end,
			heartbeat_interval_minutes = case when devices.deleted_at is null then devices.heartbeat_interval_minutes else excluded.heartbeat_interval_minutes end,
			preferred_sim_subscription_id = case when devices.deleted_at is null then devices.preferred_sim_subscription_id else null end,
			deleted_at = null,
			is_default = devices.is_default or excluded.is_default
		returning `+deviceCols,
		p.ID, p.UserID, p.Name, p.HardwareKey, p.Manufacturer, p.Brand, p.Model, p.BuildID,
		p.OSVersion, p.OSAPILevel, p.AppVersionName, p.AppVersionCode, p.PushToken,
		p.HeartbeatIntervalMinutes, p.MakeDefault))
}

// CountLiveDevices counts a user's paired, undeleted devices.
func (s *Store) CountLiveDevices(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `select count(*) from devices where user_id = $1 and deleted_at is null`, userID).Scan(&n)
	return n, err
}

// HasDefaultDevice reports whether the user has a live default device.
func (s *Store) HasDefaultDevice(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.q.QueryRow(ctx, `
		select exists(select 1 from devices where user_id = $1 and is_default and deleted_at is null)`, userID).Scan(&ok)
	return ok, err
}

// GetDevice fetches a live device by id, regardless of owner.
func (s *Store) GetDevice(ctx context.Context, id uuid.UUID) (Device, error) {
	return one[Device](s.q.Query(ctx, `select `+deviceCols+` from devices where id = $1 and deleted_at is null`, id))
}

// GetUserDevice fetches a live device owned by the user.
func (s *Store) GetUserDevice(ctx context.Context, userID, id uuid.UUID) (Device, error) {
	return one[Device](s.q.Query(ctx, `
		select `+deviceCols+` from devices where id = $1 and user_id = $2 and deleted_at is null`, id, userID))
}

// ListDevices returns a user's live devices, default first, then by name.
func (s *Store) ListDevices(ctx context.Context, userID uuid.UUID) ([]Device, error) {
	return many[Device](s.q.Query(ctx, `
		select `+deviceCols+` from devices where user_id = $1 and deleted_at is null
		order by is_default desc, name asc, created_at asc`, userID))
}

// DeviceSettingsPatch updates settings. Nil fields are left alone.
type DeviceSettingsPatch struct {
	Name                       *string
	Enabled                    *bool
	ReceiveEnabled             *bool
	SendDelaySeconds           *int32
	HeartbeatIntervalMinutes   *int32
	PreferredSimSubscriptionID *int32
	ClearPreferredSim          bool
}

// UpdateDeviceSettings applies a patch and returns the device.
func (s *Store) UpdateDeviceSettings(ctx context.Context, id uuid.UUID, p DeviceSettingsPatch) (Device, error) {
	return one[Device](s.q.Query(ctx, `
		update devices set
			name = coalesce($2, name),
			enabled = coalesce($3, enabled),
			receive_enabled = coalesce($4, receive_enabled),
			send_delay_seconds = coalesce($5, send_delay_seconds),
			heartbeat_interval_minutes = coalesce($6, heartbeat_interval_minutes),
			preferred_sim_subscription_id = case when $8 then null else coalesce($7, preferred_sim_subscription_id) end
		where id = $1 and deleted_at is null
		returning `+deviceCols,
		id, p.Name, p.Enabled, p.ReceiveEnabled, p.SendDelaySeconds, p.HeartbeatIntervalMinutes,
		p.PreferredSimSubscriptionID, p.ClearPreferredSim))
}

// SetDefaultDevice makes one device the default and clears the others.
func (s *Store) SetDefaultDevice(ctx context.Context, userID, id uuid.UUID) error {
	if _, err := s.q.Exec(ctx, `update devices set is_default = false where user_id = $1 and is_default and id <> $2`, userID, id); err != nil {
		return err
	}
	tag, err := s.q.Exec(ctx, `update devices set is_default = true where id = $1 and user_id = $2 and deleted_at is null`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteDevice unpairs a phone. History is kept; tokens are revoked.
func (s *Store) SoftDeleteDevice(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.q.Exec(ctx, `
		update devices set deleted_at = now(), enabled = false, is_default = false, online = false, push_token = null
		where id = $1 and user_id = $2 and deleted_at is null`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.q.Exec(ctx, `update device_tokens set revoked_at = now() where device_id = $1 and revoked_at is null`, id)
	return err
}

// HeartbeatParams is what a phone reports on each check-in.
type HeartbeatParams struct {
	PushToken      *string
	AppVersionName *string
	AppVersionCode *int32
	OSVersion      *string
	OSAPILevel     *int32
	Telemetry      json.RawMessage
	Sims           json.RawMessage
}

type heartbeatRow struct {
	Device
	WasOnline bool `db:"was_online"`
}

// RecordHeartbeat stores a check-in and returns the device plus whether it
// was online before this beat. One statement with a row lock, so two
// check-ins that race see each other and only the first reports "was
// offline".
func (s *Store) RecordHeartbeat(ctx context.Context, id uuid.UUID, p HeartbeatParams) (Device, bool, error) {
	row, err := one[heartbeatRow](s.q.Query(ctx, `
		with prev as (
			select online as was_online from devices where id = $1 and deleted_at is null for update
		)
		update devices d set
			last_heartbeat_at = now(),
			online = true,
			push_token = coalesce($2, d.push_token),
			push_token_updated_at = case when $2::text is null or $2 = d.push_token then d.push_token_updated_at else now() end,
			push_token_invalidated_at = case when $2::text is null then d.push_token_invalidated_at else null end,
			push_token_invalid_reason = case when $2::text is null then d.push_token_invalid_reason else null end,
			app_version_name = coalesce($3, d.app_version_name),
			app_version_code = coalesce($4, d.app_version_code),
			os_version = coalesce($5, d.os_version),
			os_api_level = coalesce($6, d.os_api_level),
			telemetry = coalesce($7, d.telemetry),
			sims = coalesce($8, d.sims)
		from prev
		where d.id = $1
		returning `+prefixCols("d.", deviceCols)+`, prev.was_online`,
		id, p.PushToken, p.AppVersionName, p.AppVersionCode, p.OSVersion, p.OSAPILevel, p.Telemetry, p.Sims))
	if err != nil {
		return Device{}, false, err
	}
	return row.Device, row.WasOnline, nil
}

// SetPushToken records a fresh token for a device.
func (s *Store) SetPushToken(ctx context.Context, id uuid.UUID, token string) error {
	_, err := s.q.Exec(ctx, `
		update devices set push_token = $2, push_token_updated_at = now(),
			push_token_invalidated_at = null, push_token_invalid_reason = null
		where id = $1 and deleted_at is null`, id, token)
	return err
}

// InvalidatePushToken marks a token dead so the device is skipped until it
// re-registers.
func (s *Store) InvalidatePushToken(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := s.q.Exec(ctx, `
		update devices set push_token_invalidated_at = now(), push_token_invalid_reason = $2 where id = $1`, id, reason)
	return err
}

// MarkOfflineStale flips devices that have not checked in since cutoff and
// returns them so the caller can notify.
func (s *Store) MarkOfflineStale(ctx context.Context, cutoff time.Time) ([]Device, error) {
	return many[Device](s.q.Query(ctx, `
		update devices set online = false
		where online and deleted_at is null and (last_heartbeat_at is null or last_heartbeat_at < $1)
		returning `+deviceCols, cutoff))
}

// ListDevicesToProbe returns enabled devices with a usable push token that
// have been silent since cutoff.
func (s *Store) ListDevicesToProbe(ctx context.Context, cutoff time.Time) ([]Device, error) {
	return many[Device](s.q.Query(ctx, `
		select `+deviceCols+` from devices
		where deleted_at is null and enabled and push_token is not null and push_token_invalidated_at is null
			and (last_heartbeat_at is null or last_heartbeat_at < $1)
		order by last_heartbeat_at nulls first
		limit 2000`, cutoff))
}

// AddDeviceCounts bumps the lifetime counters.
func (s *Store) AddDeviceCounts(ctx context.Context, id uuid.UUID, sent, received int) error {
	_, err := s.q.Exec(ctx, `update devices set sent_count = sent_count + $2, received_count = received_count + $3 where id = $1`,
		id, sent, received)
	return err
}

// ResolveSenderDevice picks the device a send goes out from: the explicit id
// if given (owner-checked), else the default enabled device, else the enabled
// device that checked in most recently.
func (s *Store) ResolveSenderDevice(ctx context.Context, userID uuid.UUID, explicit *uuid.UUID) (Device, error) {
	if explicit != nil {
		return s.GetUserDevice(ctx, userID, *explicit)
	}
	return one[Device](s.q.Query(ctx, `
		select `+deviceCols+` from devices
		where user_id = $1 and deleted_at is null and enabled
		order by is_default desc, last_heartbeat_at desc nulls last, created_at desc
		limit 1`, userID))
}

// ---------------------------------------------------------------------------
// Device tokens
// ---------------------------------------------------------------------------

// DeviceToken is a phone credential.
type DeviceToken struct {
	ID         uuid.UUID  `db:"id"`
	DeviceID   uuid.UUID  `db:"device_id"`
	TokenHash  []byte     `db:"token_hash"`
	LastUsedAt *time.Time `db:"last_used_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

// CreateDeviceToken stores a token hash, revoking any earlier tokens for the
// device so exactly one is live.
func (s *Store) CreateDeviceToken(ctx context.Context, deviceID uuid.UUID, hash []byte) (DeviceToken, error) {
	if _, err := s.q.Exec(ctx, `update device_tokens set revoked_at = now() where device_id = $1 and revoked_at is null`, deviceID); err != nil {
		return DeviceToken{}, err
	}
	return one[DeviceToken](s.q.Query(ctx, `
		insert into device_tokens (device_id, token_hash) values ($1, $2)
		returning id, device_id, token_hash, last_used_at, revoked_at, created_at`, deviceID, hash))
}

// GetDeviceByToken resolves a presented device token to its live device.
func (s *Store) GetDeviceByToken(ctx context.Context, hash []byte) (Device, DeviceToken, error) {
	tok, err := one[DeviceToken](s.q.Query(ctx, `
		select id, device_id, token_hash, last_used_at, revoked_at, created_at
		from device_tokens where token_hash = $1 and revoked_at is null`, hash))
	if err != nil {
		return Device{}, DeviceToken{}, err
	}
	d, err := s.GetDevice(ctx, tok.DeviceID)
	if err != nil {
		return Device{}, DeviceToken{}, err
	}
	return d, tok, nil
}

// TouchDeviceToken records a use.
func (s *Store) TouchDeviceToken(ctx context.Context, id uuid.UUID) error {
	_, err := s.q.Exec(ctx, `update device_tokens set last_used_at = now() where id = $1`, id)
	return err
}
