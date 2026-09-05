package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
)

// ---------------------------------------------------------------------------
// Shapes
// ---------------------------------------------------------------------------

// turnstileDoc explains the bot-check token where a form accepts one.
const turnstileDoc = "Cloudflare Turnstile token. Required when GET /v1/auth/config reports a turnstile_site_key."

type registerInput struct {
	Body struct {
		Email          string `json:"email" format:"email" maxLength:"254" doc:"Sign-in email. A verification code is sent to it."`
		Password       string `json:"password" minLength:"10" maxLength:"200" doc:"At least 10 characters."`
		Name           string `json:"name,omitempty" maxLength:"100" doc:"Display name."`
		TurnstileToken string `json:"turnstile_token,omitempty" maxLength:"2048" doc:"Cloudflare Turnstile token. Required when GET /v1/auth/config reports a turnstile_site_key."`
	}
}

type loginInput struct {
	Body struct {
		Email          string `json:"email" format:"email" maxLength:"254"`
		Password       string `json:"password" maxLength:"200"`
		TurnstileToken string `json:"turnstile_token,omitempty" maxLength:"2048" doc:"Cloudflare Turnstile token. Required when GET /v1/auth/config reports a turnstile_site_key."`
	}
}

type authConfigOutput struct {
	Body struct {
		GoogleSignIn     bool   `json:"google_sign_in" doc:"Whether Continue with Google is available at GET /v1/auth/google/start."`
		TurnstileSiteKey string `json:"turnstile_site_key" doc:"Site key for the Cloudflare Turnstile widget, or empty when no bot check is needed."`
	}
}

type sessionOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      struct {
		User store.User `json:"user"`
	}
}

type emptyOutput struct{}

type clearCookieOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type sessionView struct {
	ID         uuid.UUID `json:"id"`
	UserAgent  *string   `json:"user_agent" doc:"The browser that signed in, as it described itself."`
	IP         *string   `json:"ip" doc:"The address it signed in from."`
	CreatedAt  time.Time `json:"created_at" doc:"When it signed in."`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at" doc:"When it ends if it stays idle."`
	Current    bool      `json:"current" doc:"True for the session making this request."`
}

type listSessionsOutput struct {
	Body struct {
		Data []sessionView `json:"data"`
	}
}

type sessionIDInput struct {
	ID string `path:"id" doc:"Session id."`
}

type meOutput struct {
	Body struct {
		User   store.User   `json:"user"`
		Limits store.Limits `json:"limits"`
		Usage  usageView    `json:"usage"`
	}
}

type usageView struct {
	SentToday     int32 `json:"sent_today"`
	SentThisMonth int32 `json:"sent_this_month"`
	ReceivedToday int32 `json:"received_today"`
	ReceivedMonth int32 `json:"received_this_month"`
}

type verifyEmailInput struct {
	Body struct {
		Code string `json:"code" minLength:"6" maxLength:"6" doc:"The 6-digit code from the email."`
	}
}

type resetRequestInput struct {
	Body struct {
		Email          string `json:"email" format:"email" maxLength:"254"`
		TurnstileToken string `json:"turnstile_token,omitempty" maxLength:"2048" doc:"Cloudflare Turnstile token. Required when GET /v1/auth/config reports a turnstile_site_key."`
	}
}

type resetInput struct {
	Body struct {
		Email       string `json:"email" format:"email" maxLength:"254"`
		Code        string `json:"code" minLength:"6" maxLength:"6"`
		NewPassword string `json:"new_password" minLength:"10" maxLength:"200"`
	}
}

type changePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" maxLength:"200"`
		NewPassword     string `json:"new_password" minLength:"10" maxLength:"200"`
	}
}

type profileInput struct {
	Body struct {
		Name *string `json:"name,omitempty" maxLength:"100"`
	}
}

type userOutput struct {
	Body struct {
		User store.User `json:"user"`
	}
}

type createKeyInput struct {
	Body struct {
		Name      string     `json:"name,omitempty" maxLength:"64" doc:"Label shown in the dashboard."`
		Scopes    []string   `json:"scopes,omitempty" doc:"Any of send, read, devices, webhooks. Defaults to all."`
		ExpiresAt *time.Time `json:"expires_at,omitempty" doc:"Optional expiry."`
	}
}

type createKeyOutput struct {
	Body struct {
		Key    string       `json:"key" doc:"The full key. It is shown once and never stored."`
		APIKey store.APIKey `json:"api_key"`
	}
}

type listKeysInput struct {
	IncludeRevoked bool `query:"include_revoked" doc:"Also return revoked keys."`
}

type listKeysOutput struct {
	Body struct {
		Data []store.APIKey `json:"data"`
	}
}

type keyIDInput struct {
	ID string `path:"id" doc:"API key id."`
}

type renameKeyInput struct {
	ID   string `path:"id"`
	Body struct {
		Name string `json:"name" minLength:"1" maxLength:"64"`
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// checkTurnstile verifies the bot token when the check is on.
func (s *Server) checkTurnstile(ctx context.Context, token string) error {
	if s.deps.Turnstile == nil {
		return nil
	}
	ip, _ := ctx.Value(remoteAddrKey{}).(string)
	if err := s.deps.Turnstile.Verify(ctx, token, hostOf(ip)); err != nil {
		return mapErr(ctx, s.deps.Log, err)
	}
	return nil
}

func (s *Server) registerAuth() {
	tags := []string{"auth"}

	huma.Register(s.api, huma.Operation{
		OperationID: "auth-config", Method: http.MethodGet, Path: "/v1/auth/config",
		Summary: "Sign-in options", Tags: tags,
		Description: "What the sign-in page needs to know: whether Google sign-in is on, and the Turnstile site key when a bot check is required on the sign-in, sign-up, and password reset forms.",
	}, func(ctx context.Context, _ *struct{}) (*authConfigOutput, error) {
		out := &authConfigOutput{}
		out.Body.GoogleSignIn = s.deps.Google != nil
		if s.deps.Turnstile != nil {
			out.Body.TurnstileSiteKey = s.deps.Config.TurnstileSiteKey
		}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "register", Method: http.MethodPost, Path: "/v1/auth/register",
		Summary: "Create an account", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "Creates the account, signs it in with the session cookies, and emails a verification code. Sending is blocked until the email is verified.",
	}, func(ctx context.Context, in *registerInput) (*sessionOutput, error) {
		if err := s.checkTurnstile(ctx, in.Body.TurnstileToken); err != nil {
			return nil, err
		}
		u, err := s.deps.Auth.Register(ctx, in.Body.Email, in.Body.Password, in.Body.Name)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return s.issueSession(ctx, u)
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/v1/auth/login",
		Summary: "Sign in", Tags: tags,
		Description: "Checks the password and sets the session cookies the dashboard uses. A session lives 30 days without use, longer while it is used, and 180 days at most.",
	}, func(ctx context.Context, in *loginInput) (*sessionOutput, error) {
		if err := s.checkTurnstile(ctx, in.Body.TurnstileToken); err != nil {
			return nil, err
		}
		u, err := s.deps.Auth.Login(ctx, in.Body.Email, in.Body.Password)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return s.issueSession(ctx, u)
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/v1/auth/logout",
		Extensions: scoped(scopeSession),
		Summary:    "Sign out", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityUser,
		Description: "Ends this session and clears the cookies. Always succeeds, so a browser with a dead cookie can still clean up.",
	}, func(ctx context.Context, _ *struct{}) (*clearCookieOutput, error) {
		if tok, ok := ctx.Value(sessionTokenKey{}).(string); ok && tok != "" {
			_ = s.deps.Auth.Logout(ctx, tok)
		}
		return &clearCookieOutput{SetCookie: s.clearCookies()}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-sessions", Method: http.MethodGet, Path: "/v1/auth/sessions",
		Extensions: scoped(scopeSession),
		Summary:    "List sessions", Tags: tags, Security: securityUser,
		Description: "Every browser signed in to this account, most recently used first. The one making the request is marked current.",
	}, func(ctx context.Context, _ *struct{}) (*listSessionsOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		sessions, err := s.deps.Auth.ListSessions(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &listSessionsOutput{}
		out.Body.Data = make([]sessionView, 0, len(sessions))
		for _, sess := range sessions {
			out.Body.Data = append(out.Body.Data, sessionView{
				ID: sess.ID, UserAgent: sess.UserAgent, IP: sess.IP, CreatedAt: sess.CreatedAt,
				LastSeenAt: sess.LastSeenAt, ExpiresAt: sess.ExpiresAt, Current: sess.ID == p.Session.ID,
			})
		}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "revoke-session", Method: http.MethodDelete, Path: "/v1/auth/sessions/{id}",
		Extensions: scoped(scopeSession),
		Summary:    "End a session", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityUser,
		Description: "Signs that browser out. Ending the current session also clears its cookies.",
	}, func(ctx context.Context, in *sessionIDInput) (*clearCookieOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such session.")
		}
		if err := s.deps.Auth.RevokeSession(ctx, p.User.ID, id); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &clearCookieOutput{}
		if id == p.Session.ID {
			out.SetCookie = s.clearCookies()
		}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "revoke-other-sessions", Method: http.MethodPost, Path: "/v1/auth/sessions/revoke-others",
		Extensions: scoped(scopeSession),
		Summary:    "End every other session", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityUser,
		Description: "Signs out every browser but this one.",
	}, func(ctx context.Context, _ *struct{}) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Auth.RevokeOtherSessions(ctx, p.User.ID, p.Session.ID); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/v1/auth/me",
		Summary: "Current account", Tags: tags, Security: securityUser,
		Description: "The account behind the credentials, with its plan limits and usage this period.",
	}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
		p, err := requireUser(ctx, "")
		if err != nil {
			return nil, err
		}
		limits, err := s.deps.Billing.Limits(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		usage, err := s.deps.Billing.Usage(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &meOutput{}
		out.Body.User = *p.User
		out.Body.Limits = limits
		out.Body.Usage = usageView{SentToday: usage.SentToday, SentThisMonth: usage.SentThisMonth, ReceivedToday: usage.ReceivedToday, ReceivedMonth: usage.ReceivedMonth}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "send-verification", Method: http.MethodPost, Path: "/v1/auth/verify-email/send",
		Extensions: scoped(scopeSession),
		Summary:    "Resend the verification code", Tags: tags, DefaultStatus: http.StatusAccepted, Security: securityUser,
	}, func(ctx context.Context, _ *struct{}) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Auth.SendVerification(ctx, *p.User); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "verify-email", Method: http.MethodPost, Path: "/v1/auth/verify-email",
		Extensions: scoped(scopeSession),
		Summary:    "Verify the email address", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *verifyEmailInput) (*userOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Auth.VerifyEmail(ctx, p.User.ID, in.Body.Code); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return s.userOut(ctx, p.User.ID)
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "request-password-reset", Method: http.MethodPost, Path: "/v1/auth/password-reset/request",
		Summary: "Request a password reset code", Tags: tags, DefaultStatus: http.StatusAccepted,
		Description: "Emails a code if the address has an account. The response is the same either way.",
	}, func(ctx context.Context, in *resetRequestInput) (*emptyOutput, error) {
		if err := s.checkTurnstile(ctx, in.Body.TurnstileToken); err != nil {
			return nil, err
		}
		if err := s.deps.Auth.RequestPasswordReset(ctx, in.Body.Email); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "reset-password", Method: http.MethodPost, Path: "/v1/auth/password-reset",
		Summary: "Set a new password with a reset code", Tags: tags,
		Description: "Completes a reset. Every existing session is signed out.",
	}, func(ctx context.Context, in *resetInput) (*emptyOutput, error) {
		if err := s.deps.Auth.ResetPassword(ctx, in.Body.Email, in.Body.Code, in.Body.NewPassword); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "change-password", Method: http.MethodPost, Path: "/v1/auth/password",
		Extensions: scoped(scopeSession),
		Summary:    "Change password", Tags: tags, Security: securityUser,
		Description: "Requires the current password. Every other session is signed out.",
	}, func(ctx context.Context, in *changePasswordInput) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Auth.ChangePassword(ctx, *p.User, in.Body.CurrentPassword, in.Body.NewPassword, p.Session.ID); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-profile", Method: http.MethodPatch, Path: "/v1/auth/profile",
		Extensions: scoped(scopeSession),
		Summary:    "Update profile", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *profileInput) (*userOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		u, err := s.deps.Auth.UpdateProfile(ctx, p.User.ID, in.Body.Name)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &userOutput{}
		out.Body.User = u
		return out, nil
	})

	// ---- API keys ----
	ktags := []string{"api-keys"}

	huma.Register(s.api, huma.Operation{
		OperationID: "create-api-key", Method: http.MethodPost, Path: "/v1/api-keys",
		Extensions: scoped(scopeSession),
		Summary:    "Create an API key", Tags: ktags, DefaultStatus: http.StatusCreated, Security: securityUser,
		Description: "The full key is returned once. Only its hash is stored.",
	}, func(ctx context.Context, in *createKeyInput) (*createKeyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		key, rec, err := s.deps.Auth.CreateAPIKey(ctx, p.User.ID, in.Body.Name, in.Body.Scopes, in.Body.ExpiresAt)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &createKeyOutput{}
		out.Body.Key = key
		out.Body.APIKey = rec
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-api-keys", Method: http.MethodGet, Path: "/v1/api-keys",
		Extensions: scoped(scopeSession),
		Summary:    "List API keys", Tags: ktags, Security: securityUser,
	}, func(ctx context.Context, in *listKeysInput) (*listKeysOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		keys, err := s.deps.Auth.ListAPIKeys(ctx, p.User.ID, in.IncludeRevoked)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &listKeysOutput{}
		out.Body.Data = keys
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "rename-api-key", Method: http.MethodPatch, Path: "/v1/api-keys/{id}",
		Extensions: scoped(scopeSession),
		Summary:    "Rename an API key", Tags: ktags, DefaultStatus: http.StatusNoContent, Security: securityUser,
	}, func(ctx context.Context, in *renameKeyInput) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such API key.")
		}
		if err := s.deps.Auth.RenameAPIKey(ctx, p.User.ID, id, in.Body.Name); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "revoke-api-key", Method: http.MethodPost, Path: "/v1/api-keys/{id}/revoke",
		Extensions: scoped(scopeSession),
		Summary:    "Revoke an API key", Tags: ktags, DefaultStatus: http.StatusNoContent, Security: securityUser,
		Description: "Stops the key working immediately while keeping its record.",
	}, func(ctx context.Context, in *keyIDInput) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such API key.")
		}
		if err := s.deps.Auth.RevokeAPIKey(ctx, p.User.ID, id); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-api-key", Method: http.MethodDelete, Path: "/v1/api-keys/{id}",
		Extensions: scoped(scopeSession),
		Summary:    "Delete an API key", Tags: ktags, DefaultStatus: http.StatusNoContent, Security: securityUser,
	}, func(ctx context.Context, in *keyIDInput) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such API key.")
		}
		if err := s.deps.Auth.DeleteAPIKey(ctx, p.User.ID, id); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})
}

type sessionTokenKey struct{}

func (s *Server) issueSession(ctx context.Context, u store.User) (*sessionOutput, error) {
	ua, _ := ctx.Value(userAgentKey{}).(string)
	ip, _ := ctx.Value(remoteAddrKey{}).(string)
	token, expires, err := s.deps.Auth.CreateSession(ctx, u.ID, ua, hostOf(ip))
	if err != nil {
		return nil, mapErr(ctx, s.deps.Log, err)
	}
	out := &sessionOutput{SetCookie: s.issueCookies(token, expires)}
	out.Body.User = u
	return out, nil
}

func (s *Server) userOut(ctx context.Context, id uuid.UUID) (*userOutput, error) {
	u, err := s.deps.Auth.GetUser(ctx, id)
	if err != nil {
		return nil, mapErr(ctx, s.deps.Log, err)
	}
	out := &userOutput{}
	out.Body.User = u
	return out, nil
}

type userAgentKey struct{}
type remoteAddrKey struct{}
