// Package auth resolves who is calling and manages the credentials they
// call with: dashboard sessions, developer API keys, and one-time codes.
// Device tokens are resolved here too but issued by the gateway at pairing.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/ids"
	mailer "github.com/simhook/simhook/internal/mail"
	"github.com/simhook/simhook/internal/ratelimit"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/validate"
)

// Errors surfaced to the HTTP layer.
var (
	ErrEmailTaken         = errors.New("an account with this email already exists")
	ErrInvalidCredentials = errors.New("wrong email or password")
	ErrWrongPassword      = errors.New("the current password is wrong")
	ErrTooManyAttempts    = errors.New("too many sign-in attempts for this account; wait a minute and try again")
	ErrInvalidCode        = errors.New("the code is wrong or has expired")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrForbidden          = errors.New("this credential cannot perform that action")
	ErrWeakPassword       = errors.New("password must be at least 10 characters")
	ErrInvalidEmail       = errors.New("email address is not valid")
	ErrNoPassword         = errors.New("this account signs in with Google")
	ErrBanned             = errors.New("this account is suspended")
)

// Scopes an API key can hold.
const (
	ScopeSend     = "send"
	ScopeRead     = "read"
	ScopeDevices  = "devices"
	ScopeWebhooks = "webhooks"
)

// AllScopes lists every valid scope.
var AllScopes = []string{ScopeSend, ScopeRead, ScopeDevices, ScopeWebhooks}

// Kind says which credential authenticated a request.
type Kind string

// Credential kinds.
const (
	KindSession Kind = "session"
	KindAPIKey  Kind = "api_key"
	KindDevice  Kind = "device"
)

// Principal is the authenticated caller.
type Principal struct {
	Kind   Kind
	User   *store.User
	APIKey *store.APIKey
	Device *store.Device
	// Session is set for KindSession. SessionRefreshed says its expiry moved
	// on this request, so the browser's cookies need re-issuing.
	Session          *store.Session
	SessionRefreshed bool
}

// HasScope reports whether the caller may perform a scoped action. Sessions
// carry every scope; devices carry none.
func (p *Principal) HasScope(scope string) bool {
	switch p.Kind {
	case KindSession:
		return true
	case KindAPIKey:
		return p.APIKey != nil && slices.Contains(p.APIKey.Scopes, scope)
	default:
		return false
	}
}

type ctxKey struct{}

// WithPrincipal attaches a principal to a context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal, or nil.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// Sign-in attempts per account, right or wrong, before the account is
// asked to wait. Guessing a password then costs a minute per five tries
// whatever the caller's address.
const (
	loginRate  = 12 * time.Second
	loginBurst = 5
)

// touchInterval bounds how often a credential's bookkeeping row is written.
const touchInterval = time.Minute

// Service implements credential operations.
type Service struct {
	st     *store.Store
	cfg    *config.Config
	mailer mailer.Mailer
	log    *slog.Logger
	logins *ratelimit.Keyed

	touchMu sync.Mutex
	touched map[uuid.UUID]time.Time
	pending map[uuid.UUID]int
}

// New builds the service.
func New(st *store.Store, cfg *config.Config, m mailer.Mailer, log *slog.Logger) *Service {
	return &Service{
		st: st, cfg: cfg, mailer: m, log: log,
		logins:  ratelimit.NewKeyed(rate.Every(loginRate), loginBurst),
		touched: map[uuid.UUID]time.Time{},
		pending: map[uuid.UUID]int{},
	}
}

// ---------------------------------------------------------------------------
// Accounts
// ---------------------------------------------------------------------------

// NormalizeEmail lowercases and trims, and validates the shape.
func NormalizeEmail(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return "", ErrInvalidEmail
	}
	return e, nil
}

func checkPassword(pw string) error {
	if len(pw) < 10 || len(pw) > 200 {
		return ErrWeakPassword
	}
	return nil
}

// Register creates an account and sends the verification code.
func (s *Service) Register(ctx context.Context, email, password, name string) (store.User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return store.User{}, err
	}
	if err := checkPassword(password); err != nil {
		return store.User{}, err
	}
	hash, err := secrets.HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	var namePtr *string
	if n := strings.TrimSpace(name); n != "" {
		namePtr = &n
	}
	u, err := s.st.CreateUser(ctx, store.CreateUserParams{
		ID: ids.New(), Email: email, Name: namePtr, PasswordHash: &hash,
		Verified: !s.cfg.RequireEmailVerification,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return store.User{}, ErrEmailTaken
		}
		return store.User{}, err
	}
	if s.cfg.RequireEmailVerification {
		if err := s.SendVerification(ctx, u); err != nil {
			s.log.Warn("verification email failed", "user", u.ID, "err", err)
		}
	}
	return u, nil
}

// Login checks a password and returns the account. Attempts are throttled
// per account as well as per address upstream.
func (s *Service) Login(ctx context.Context, email, password string) (store.User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return store.User{}, ErrInvalidCredentials
	}
	if !s.logins.Allow(email) {
		return store.User{}, ErrTooManyAttempts
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Burn the same time as a real check so timing does not reveal
			// whether the address exists.
			secrets.VerifyPassword("$argon2id$v=19$m=65536,t=2,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
			return store.User{}, ErrInvalidCredentials
		}
		return store.User{}, err
	}
	if u.PasswordHash == nil {
		return store.User{}, ErrNoPassword
	}
	if !secrets.VerifyPassword(*u.PasswordHash, password) {
		return store.User{}, ErrInvalidCredentials
	}
	if u.BannedAt != nil {
		return store.User{}, ErrBanned
	}
	_ = s.st.TouchLogin(ctx, u.ID)
	return u, nil
}

// CreateSession issues a session token for the browser.
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, userAgent, ip string) (string, time.Time, error) {
	token, hash, err := secrets.NewToken(secrets.PrefixSession)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(s.cfg.SessionTTL())
	if _, err := s.st.CreateSession(ctx, userID, hash, userAgent, ip, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Logout destroys a session.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.st.DeleteSession(ctx, secrets.Hash(token))
}

const codeTTL = 30 * time.Minute

// SendVerification emails a fresh verification code.
func (s *Service) SendVerification(ctx context.Context, u store.User) error {
	if u.EmailVerifiedAt != nil {
		return nil
	}
	code, hash, err := secrets.NewOneTimeCode()
	if err != nil {
		return err
	}
	if err := s.st.CreateUserToken(ctx, u.ID, "email_verify", hash, time.Now().Add(codeTTL)); err != nil {
		return err
	}
	name := ""
	if u.Name != nil {
		name = *u.Name
	}
	return s.mailer.Send(ctx, mailer.VerifyEmail(u.Email, name, code))
}

// VerifyEmail confirms a code.
func (s *Service) VerifyEmail(ctx context.Context, userID uuid.UUID, code string) error {
	err := s.st.ConsumeUserToken(ctx, userID, "email_verify", secrets.Hash(strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCode
		}
		return err
	}
	return s.st.SetEmailVerified(ctx, userID)
}

// RequestPasswordReset emails a code if the account exists. It never reveals
// whether it does.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	code, hash, err := secrets.NewOneTimeCode()
	if err != nil {
		return err
	}
	if err := s.st.CreateUserToken(ctx, u.ID, "password_reset", hash, time.Now().Add(codeTTL)); err != nil {
		return err
	}
	return s.mailer.Send(ctx, mailer.PasswordReset(u.Email, code))
}

// ResetPassword completes a reset and signs the user out everywhere.
func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	email, err := NormalizeEmail(email)
	if err != nil {
		return ErrInvalidCode
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCode
		}
		return err
	}
	if err := s.st.ConsumeUserToken(ctx, u.ID, "password_reset", secrets.Hash(strings.TrimSpace(code))); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCode
		}
		return err
	}
	hash, err := secrets.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.st.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	// A reset proves control of the inbox, which also verifies the address.
	_ = s.st.SetEmailVerified(ctx, u.ID)
	return s.st.DeleteUserSessions(ctx, u.ID)
}

// ChangePassword requires the current password. Every session but the one
// making the change is signed out: whoever else held the old password is
// no longer welcome.
func (s *Service) ChangePassword(ctx context.Context, u store.User, current, next string, keep uuid.UUID) error {
	if u.PasswordHash == nil {
		return ErrNoPassword
	}
	if !secrets.VerifyPassword(*u.PasswordHash, current) {
		return ErrWrongPassword
	}
	if err := checkPassword(next); err != nil {
		return err
	}
	hash, err := secrets.HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.st.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	return s.st.DeleteOtherUserSessions(ctx, u.ID, keep)
}

// ListSessions returns the account's live sessions, most recently used first.
func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]store.Session, error) {
	return s.st.ListUserSessions(ctx, userID)
}

// RevokeSession ends one of the account's sessions.
func (s *Service) RevokeSession(ctx context.Context, userID, id uuid.UUID) error {
	return s.st.DeleteUserSession(ctx, userID, id)
}

// RevokeOtherSessions ends every session of the account but the current one.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID, current uuid.UUID) error {
	return s.st.DeleteOtherUserSessions(ctx, userID, current)
}

// SweepCredentials deletes sessions and one-time codes that have expired.
// Nothing can present them any more; the rows are only weight.
func (s *Service) SweepCredentials(ctx context.Context) error {
	sessions, err := s.st.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
	codes, err := s.st.DeleteExpiredUserTokens(ctx)
	if err != nil {
		return err
	}
	if sessions+codes > 0 {
		s.log.Info("expired credentials swept", "sessions", sessions, "codes", codes)
	}
	return nil
}

// GetUser fetches an account.
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (store.User, error) {
	return s.st.GetUser(ctx, id)
}

// UpdateProfile changes display fields.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, name *string) (store.User, error) {
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			name = nil
		} else {
			name = &n
		}
	}
	return s.st.UpdateUserProfile(ctx, id, name, nil)
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

// ListAPIKeys returns a user's keys, masked.
func (s *Service) ListAPIKeys(ctx context.Context, userID uuid.UUID, includeRevoked bool) ([]store.APIKey, error) {
	return s.st.ListAPIKeys(ctx, userID, includeRevoked)
}

// RenameAPIKey changes a key's label.
func (s *Service) RenameAPIKey(ctx context.Context, userID, id uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return validate.Field("name", "required")
	}
	return s.st.RenameAPIKey(ctx, userID, id, name)
}

// RevokeAPIKey disables a key but keeps its record.
func (s *Service) RevokeAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	return s.st.RevokeAPIKey(ctx, userID, id)
}

// DeleteAPIKey removes a key entirely.
func (s *Service) DeleteAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	return s.st.DeleteAPIKey(ctx, userID, id)
}

// CreateAPIKey mints a key. The full key is returned once.
func (s *Service) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, scopes []string, expires *time.Time) (string, store.APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "API key"
	}
	if len(scopes) == 0 {
		scopes = slices.Clone(AllScopes)
	}
	for _, sc := range scopes {
		if !slices.Contains(AllScopes, sc) {
			return "", store.APIKey{}, validate.Field("scopes", "unknown scope "+sc)
		}
	}
	if expires != nil && expires.Before(time.Now()) {
		return "", store.APIKey{}, validate.Field("expires_at", "must be in the future")
	}
	token, hash, err := secrets.NewToken(secrets.PrefixAPIKey)
	if err != nil {
		return "", store.APIKey{}, err
	}
	key, err := s.st.CreateAPIKey(ctx, userID, name, secrets.DisplayPrefix(token), hash, scopes, expires)
	if err != nil {
		return "", store.APIKey{}, err
	}
	return token, key, nil
}

// ---------------------------------------------------------------------------
// Request authentication
// ---------------------------------------------------------------------------

// Authenticate resolves a principal from whichever credential is present.
// Precedence: device bearer, API key header, session cookie. Returns nil
// with no error when nothing is presented, or when only a session cookie
// is and it names no live session.
func (s *Service) Authenticate(ctx context.Context, sessionToken, apiKey, bearer string) (*Principal, error) {
	switch {
	case strings.HasPrefix(bearer, secrets.PrefixDevice):
		d, tok, err := s.st.GetDeviceByToken(ctx, secrets.Hash(bearer))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrUnauthenticated
			}
			return nil, err
		}
		s.touch(tok.ID, func(c context.Context, _ int) error { return s.st.TouchDeviceToken(c, tok.ID) })
		return &Principal{Kind: KindDevice, Device: &d}, nil

	case apiKey != "" || strings.HasPrefix(bearer, secrets.PrefixAPIKey):
		presented := apiKey
		if presented == "" {
			presented = bearer
		}
		k, err := s.st.GetLiveAPIKey(ctx, secrets.Hash(presented))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrUnauthenticated
			}
			return nil, err
		}
		u, err := s.st.GetUser(ctx, k.UserID)
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if u.BannedAt != nil {
			return nil, ErrBanned
		}
		s.touch(k.ID, func(c context.Context, n int) error { return s.st.TouchAPIKey(c, k.ID, n) })
		return &Principal{Kind: KindAPIKey, User: &u, APIKey: &k}, nil

	case sessionToken != "":
		return s.authenticateSession(ctx, sessionToken)
	}
	return nil, nil
}

// authenticateSession resolves a session cookie. A cookie that names no
// live session is not an error: the browser is simply not signed in, and
// the caller clears the cookie. Sessions slide: when less than half of the
// idle window remains, the expiry moves out by a full window, but never
// past the absolute cap counted from sign-in.
func (s *Service) authenticateSession(ctx context.Context, token string) (*Principal, error) {
	hash := secrets.Hash(token)
	sess, err := s.st.GetLiveSession(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now()
	limit := sess.CreatedAt.Add(s.cfg.SessionMax())
	if !now.Before(limit) {
		_ = s.st.DeleteSession(ctx, hash)
		return nil, nil
	}
	u, err := s.st.GetUser(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if u.BannedAt != nil {
		return nil, ErrBanned
	}
	p := &Principal{Kind: KindSession, User: &u}
	if sess.ExpiresAt.Sub(now) < s.cfg.SessionTTL()/2 {
		expires := now.Add(s.cfg.SessionTTL())
		if expires.After(limit) {
			expires = limit
		}
		if expires.After(sess.ExpiresAt) {
			if err := s.st.ExtendSession(ctx, sess.ID, expires); err != nil {
				return nil, err
			}
			sess.ExpiresAt = expires
			p.SessionRefreshed = true
		}
	}
	p.Session = &sess
	s.touch(sess.ID, func(c context.Context, _ int) error { return s.st.TouchSession(c, sess.ID) })
	return p, nil
}

// touch runs a credential's bookkeeping write off the request path, at most
// once a minute per credential. Uses in between are counted and handed to
// the next write as n, so a busy key neither spawns a goroutine per request
// nor loses its use count.
func (s *Service) touch(id uuid.UUID, fn func(ctx context.Context, n int) error) {
	s.touchMu.Lock()
	now := time.Now()
	s.pending[id]++
	if last, ok := s.touched[id]; ok && now.Sub(last) < touchInterval {
		s.touchMu.Unlock()
		return
	}
	if len(s.touched) > 10000 {
		for k, t := range s.touched {
			if now.Sub(t) > touchInterval {
				delete(s.touched, k)
				delete(s.pending, k)
			}
		}
	}
	n := s.pending[id]
	s.pending[id] = 0
	s.touched[id] = now
	s.touchMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fn(ctx, n); err != nil {
			s.log.Debug("credential touch failed", "err", err)
		}
	}()
}
