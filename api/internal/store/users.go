package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// User is an account.
type User struct {
	ID                  uuid.UUID       `db:"id" json:"id"`
	Email               string          `db:"email" json:"email"`
	Name                *string         `db:"name" json:"name"`
	PasswordHash        *string         `db:"password_hash" json:"-"`
	GoogleSub           *string         `db:"google_sub" json:"-"`
	AvatarURL           *string         `db:"avatar_url" json:"avatar_url"`
	Role                string          `db:"role" json:"role"`
	EmailVerifiedAt     *time.Time      `db:"email_verified_at" json:"email_verified_at"`
	BannedAt            *time.Time      `db:"banned_at" json:"-"`
	DeletionRequestedAt *time.Time      `db:"deletion_requested_at" json:"deletion_requested_at"`
	DeletionReason      *string         `db:"deletion_reason" json:"-"`
	Onboarding          json.RawMessage `db:"onboarding" json:"onboarding"`
	LastLoginAt         *time.Time      `db:"last_login_at" json:"last_login_at"`
	CreatedAt           time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time       `db:"updated_at" json:"updated_at"`
}

const userCols = `id, email, name, password_hash, google_sub, avatar_url, role, email_verified_at,
	banned_at, deletion_requested_at, deletion_reason, onboarding, last_login_at, created_at, updated_at`

// CreateUserParams are the fields a new account starts with.
type CreateUserParams struct {
	ID           uuid.UUID
	Email        string
	Name         *string
	PasswordHash *string
	GoogleSub    *string
	AvatarURL    *string
	Verified     bool
}

// CreateUser inserts an account. Returns ErrConflict if the email exists.
func (s *Store) CreateUser(ctx context.Context, p CreateUserParams) (User, error) {
	var verifiedAt *time.Time
	if p.Verified {
		now := time.Now()
		verifiedAt = &now
	}
	u, err := one[User](s.q.Query(ctx, `
		insert into users (id, email, name, password_hash, google_sub, avatar_url, email_verified_at)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning `+userCols,
		p.ID, p.Email, p.Name, p.PasswordHash, p.GoogleSub, p.AvatarURL, verifiedAt))
	return u, wrapWrite(err)
}

// GetUser fetches by id.
func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (User, error) {
	return one[User](s.q.Query(ctx, `select `+userCols+` from users where id = $1`, id))
}

// GetUserByEmail fetches by email, case-insensitively.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	return one[User](s.q.Query(ctx, `select `+userCols+` from users where email = $1`, email))
}

// GetUserByGoogleSub fetches by the Google subject id.
func (s *Store) GetUserByGoogleSub(ctx context.Context, sub string) (User, error) {
	return one[User](s.q.Query(ctx, `select `+userCols+` from users where google_sub = $1`, sub))
}

// SetEmailVerified marks the address verified.
func (s *Store) SetEmailVerified(ctx context.Context, id uuid.UUID) error {
	_, err := s.q.Exec(ctx, `update users set email_verified_at = coalesce(email_verified_at, now()) where id = $1`, id)
	return err
}

// UpdateUserProfile changes display fields. Nil leaves a field alone.
func (s *Store) UpdateUserProfile(ctx context.Context, id uuid.UUID, name, avatarURL *string) (User, error) {
	return one[User](s.q.Query(ctx, `
		update users set name = coalesce($2, name), avatar_url = coalesce($3, avatar_url)
		where id = $1 returning `+userCols, id, name, avatarURL))
}

// SetPassword replaces the password hash.
func (s *Store) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := s.q.Exec(ctx, `update users set password_hash = $2 where id = $1`, id, hash)
	return err
}

// TouchLogin records a successful sign-in.
func (s *Store) TouchLogin(ctx context.Context, id uuid.UUID) error {
	_, err := s.q.Exec(ctx, `update users set last_login_at = now() where id = $1`, id)
	return err
}

// SetOnboarding stores dashboard onboarding progress.
func (s *Store) SetOnboarding(ctx context.Context, id uuid.UUID, state json.RawMessage) error {
	_, err := s.q.Exec(ctx, `update users set onboarding = $2 where id = $1`, id, state)
	return err
}

// RequestDeletion records a deletion request.
func (s *Store) RequestDeletion(ctx context.Context, id uuid.UUID, reason *string) error {
	_, err := s.q.Exec(ctx, `update users set deletion_requested_at = now(), deletion_reason = $2 where id = $1`, id, reason)
	return err
}

// ---------------------------------------------------------------------------
// One-time codes
// ---------------------------------------------------------------------------

// CreateUserToken stores a hashed one-time code, replacing any live code of
// the same kind so only the newest one works.
func (s *Store) CreateUserToken(ctx context.Context, userID uuid.UUID, kind string, hash []byte, expires time.Time) error {
	if _, err := s.q.Exec(ctx, `
		update user_tokens set consumed_at = now()
		where user_id = $1 and kind = $2 and consumed_at is null`, userID, kind); err != nil {
		return err
	}
	_, err := s.q.Exec(ctx, `
		insert into user_tokens (user_id, kind, token_hash, expires_at) values ($1, $2, $3, $4)`,
		userID, kind, hash, expires)
	return err
}

// ConsumeUserToken burns a code and returns its owner. Returns ErrNotFound if
// the code is unknown, used, or expired.
func (s *Store) ConsumeUserToken(ctx context.Context, userID uuid.UUID, kind string, hash []byte) error {
	tag, err := s.q.Exec(ctx, `
		update user_tokens set consumed_at = now()
		where user_id = $1 and kind = $2 and token_hash = $3 and consumed_at is null and expires_at > now()`,
		userID, kind, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// Session is a dashboard login.
type Session struct {
	ID         uuid.UUID `db:"id"`
	UserID     uuid.UUID `db:"user_id"`
	TokenHash  []byte    `db:"token_hash"`
	UserAgent  *string   `db:"user_agent"`
	IP         *string   `db:"ip"`
	ExpiresAt  time.Time `db:"expires_at"`
	LastSeenAt time.Time `db:"last_seen_at"`
	CreatedAt  time.Time `db:"created_at"`
}

const sessionCols = `id, user_id, token_hash, user_agent, ip, expires_at, last_seen_at, created_at`

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, hash []byte, ua, ip string, expires time.Time) (Session, error) {
	return one[Session](s.q.Query(ctx, `
		insert into sessions (user_id, token_hash, user_agent, ip, expires_at)
		values ($1, $2, nullif($3, ''), nullif($4, ''), $5) returning `+sessionCols,
		userID, hash, ua, ip, expires))
}

// GetLiveSession returns an unexpired session by token hash.
func (s *Store) GetLiveSession(ctx context.Context, hash []byte) (Session, error) {
	return one[Session](s.q.Query(ctx, `
		select `+sessionCols+` from sessions where token_hash = $1 and expires_at > now()`, hash))
}

// TouchSession bumps last_seen_at, at most once a minute to keep writes low.
func (s *Store) TouchSession(ctx context.Context, id uuid.UUID) error {
	_, err := s.q.Exec(ctx, `
		update sessions set last_seen_at = now() where id = $1 and last_seen_at < now() - interval '1 minute'`, id)
	return err
}

// DeleteSession removes one session.
func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.q.Exec(ctx, `delete from sessions where token_hash = $1`, hash)
	return err
}

// DeleteUserSessions signs a user out everywhere.
func (s *Store) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.q.Exec(ctx, `delete from sessions where user_id = $1`, userID)
	return err
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

// APIKey is a developer credential. The key itself is never stored.
type APIKey struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	UserID     uuid.UUID  `db:"user_id" json:"-"`
	Name       string     `db:"name" json:"name"`
	Prefix     string     `db:"prefix" json:"prefix"`
	KeyHash    []byte     `db:"key_hash" json:"-"`
	Scopes     []string   `db:"scopes" json:"scopes"`
	ExpiresAt  *time.Time `db:"expires_at" json:"expires_at"`
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at"`
	UseCount   int64      `db:"use_count" json:"use_count"`
	RevokedAt  *time.Time `db:"revoked_at" json:"revoked_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
}

const apiKeyCols = `id, user_id, name, prefix, key_hash, scopes, expires_at, last_used_at, use_count, revoked_at, created_at`

// CreateAPIKey stores a key's hash and metadata.
func (s *Store) CreateAPIKey(ctx context.Context, userID uuid.UUID, name, prefix string, hash []byte, scopes []string, expires *time.Time) (APIKey, error) {
	return one[APIKey](s.q.Query(ctx, `
		insert into api_keys (user_id, name, prefix, key_hash, scopes, expires_at)
		values ($1, $2, $3, $4, $5, $6) returning `+apiKeyCols,
		userID, name, prefix, hash, scopes, expires))
}

// GetLiveAPIKey resolves a presented key. Revoked and expired keys are not found.
func (s *Store) GetLiveAPIKey(ctx context.Context, hash []byte) (APIKey, error) {
	return one[APIKey](s.q.Query(ctx, `
		select `+apiKeyCols+` from api_keys
		where key_hash = $1 and revoked_at is null and (expires_at is null or expires_at > now())`, hash))
}

// ListAPIKeys returns a user's keys, newest first. includeRevoked adds
// revoked ones.
func (s *Store) ListAPIKeys(ctx context.Context, userID uuid.UUID, includeRevoked bool) ([]APIKey, error) {
	return many[APIKey](s.q.Query(ctx, `
		select `+apiKeyCols+` from api_keys
		where user_id = $1 and ($2 or revoked_at is null)
		order by created_at desc`, userID, includeRevoked))
}

// GetUserAPIKey fetches one of the user's keys.
func (s *Store) GetUserAPIKey(ctx context.Context, userID, id uuid.UUID) (APIKey, error) {
	return one[APIKey](s.q.Query(ctx, `select `+apiKeyCols+` from api_keys where id = $1 and user_id = $2`, id, userID))
}

// RevokeAPIKey disables a key but keeps its record.
func (s *Store) RevokeAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.q.Exec(ctx, `
		update api_keys set revoked_at = now() where id = $1 and user_id = $2 and revoked_at is null`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameAPIKey changes the label.
func (s *Store) RenameAPIKey(ctx context.Context, userID, id uuid.UUID, name string) error {
	tag, err := s.q.Exec(ctx, `update api_keys set name = $3 where id = $1 and user_id = $2`, id, userID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAPIKey removes a key and its record.
func (s *Store) DeleteAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.q.Exec(ctx, `delete from api_keys where id = $1 and user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchAPIKey records a use. Called asynchronously; failures are ignored.
func (s *Store) TouchAPIKey(ctx context.Context, id uuid.UUID) error {
	_, err := s.q.Exec(ctx, `update api_keys set last_used_at = now(), use_count = use_count + 1 where id = $1`, id)
	return err
}
