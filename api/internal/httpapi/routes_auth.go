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

type registerInput struct {
	Body struct {
		Email    string `json:"email" format:"email" maxLength:"254" doc:"Sign-in email. A verification code is sent to it."`
		Password string `json:"password" minLength:"10" maxLength:"200" doc:"At least 10 characters."`
		Name     string `json:"name,omitempty" maxLength:"100" doc:"Display name."`
	}
}

type loginInput struct {
	Body struct {
		Email    string `json:"email" format:"email" maxLength:"254"`
		Password string `json:"password" maxLength:"200"`
	}
}

type sessionOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		User store.User `json:"user"`
	}
}

type emptyOutput struct{}

type clearCookieOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
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
		Email string `json:"email" format:"email" maxLength:"254"`
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

func (s *Server) sessionCookie(token string, expires time.Time) http.Cookie {
	return http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.deps.Config.IsProduction(), SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) registerAuth() {
	tags := []string{"auth"}

	huma.Register(s.api, huma.Operation{
		OperationID: "register", Method: http.MethodPost, Path: "/v1/auth/register",
		Summary: "Create an account", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "Creates the account, signs it in with a session cookie, and emails a verification code. Sending is blocked until the email is verified.",
	}, func(ctx context.Context, in *registerInput) (*sessionOutput, error) {
		u, err := s.deps.Auth.Register(ctx, in.Body.Email, in.Body.Password, in.Body.Name)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return s.issueSession(ctx, u)
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/v1/auth/login",
		Summary: "Sign in", Tags: tags,
		Description: "Checks the password and sets the session cookie used by the dashboard.",
	}, func(ctx context.Context, in *loginInput) (*sessionOutput, error) {
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
	}, func(ctx context.Context, _ *struct{}) (*clearCookieOutput, error) {
		if tok, ok := ctx.Value(sessionTokenKey{}).(string); ok && tok != "" {
			_ = s.deps.Auth.Logout(ctx, tok)
		}
		return &clearCookieOutput{SetCookie: http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode}}, nil
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
	}, func(ctx context.Context, in *changePasswordInput) (*emptyOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Auth.ChangePassword(ctx, *p.User, in.Body.CurrentPassword, in.Body.NewPassword); err != nil {
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
	token, expires, err := s.deps.Auth.CreateSession(ctx, u.ID, ua, ip)
	if err != nil {
		return nil, mapErr(ctx, s.deps.Log, err)
	}
	out := &sessionOutput{SetCookie: s.sessionCookie(token, expires)}
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
