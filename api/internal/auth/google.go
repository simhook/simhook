package auth

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"

	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
)

// ErrGoogleEmailUnverified is returned when Google vouches for an identity
// but not for its email address, and that address belongs to an account.
var ErrGoogleEmailUnverified = errors.New("Google has not verified this email address; sign in with your password instead")

// GoogleIdentity is what Google says about the person who just signed in.
type GoogleIdentity struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// GoogleExchanger runs the authorization-code flow with Google. The API
// holds the client secret and the PKCE verifier; the dashboard only ever
// sees a redirect.
type GoogleExchanger interface {
	// AuthURL is where to send the browser.
	AuthURL(state, verifier string) string
	// Exchange turns the code Google sent back into a verified identity.
	Exchange(ctx context.Context, code, verifier string) (GoogleIdentity, error)
}

type googleOAuth struct {
	cfg *oauth2.Config
}

// NewGoogle builds the exchanger for an OAuth client whose redirect URI is
// redirectURL.
func NewGoogle(clientID, clientSecret, redirectURL string) GoogleExchanger {
	return &googleOAuth{cfg: &oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		Scopes: []string{"openid", "email", "profile"}, Endpoint: google.Endpoint,
	}}
}

func (g *googleOAuth) AuthURL(state, verifier string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(verifier))
}

func (g *googleOAuth) Exchange(ctx context.Context, code, verifier string) (GoogleIdentity, error) {
	tok, err := g.cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("google exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return GoogleIdentity{}, errors.New("google exchange: no id token")
	}
	payload, err := idtoken.Validate(ctx, raw, g.cfg.ClientID)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("google id token: %w", err)
	}
	id := GoogleIdentity{Sub: payload.Subject}
	id.Email, _ = payload.Claims["email"].(string)
	id.EmailVerified, _ = payload.Claims["email_verified"].(bool)
	id.Name, _ = payload.Claims["name"].(string)
	id.Picture, _ = payload.Claims["picture"].(string)
	if id.Sub == "" || id.Email == "" {
		return GoogleIdentity{}, errors.New("google id token: no subject or email")
	}
	return id, nil
}

// SignInWithGoogle finds or makes the account behind a Google identity.
//
// An account already linked to the Google id signs in. Otherwise an account
// with the same address is linked to it, provided Google vouches for the
// address; if that account never verified its email, whoever set its
// password did not own the inbox Google just vouched for, so the password
// is dropped and its sessions are ended. Otherwise a new account is made,
// verified when Google says the address is.
func (s *Service) SignInWithGoogle(ctx context.Context, id GoogleIdentity) (store.User, error) {
	if id.Sub == "" {
		return store.User{}, ErrInvalidCredentials
	}
	email, err := NormalizeEmail(id.Email)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.st.GetUserByGoogleSub(ctx, id.Sub)
	if err == nil {
		if u.BannedAt != nil {
			return store.User{}, ErrBanned
		}
		_ = s.st.TouchLogin(ctx, u.ID)
		return u, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}

	u, err = s.st.GetUserByEmail(ctx, email)
	if err == nil {
		if !id.EmailVerified {
			return store.User{}, ErrGoogleEmailUnverified
		}
		if u.BannedAt != nil {
			return store.User{}, ErrBanned
		}
		fresh := u.EmailVerifiedAt == nil
		u, err = s.st.LinkGoogle(ctx, u.ID, id.Sub, id.Picture, fresh)
		if err != nil {
			return store.User{}, err
		}
		if fresh {
			if err := s.st.DeleteUserSessions(ctx, u.ID); err != nil {
				return store.User{}, err
			}
		}
		_ = s.st.TouchLogin(ctx, u.ID)
		return u, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}

	var name, picture *string
	if id.Name != "" {
		name = &id.Name
	}
	if id.Picture != "" {
		picture = &id.Picture
	}
	verified := id.EmailVerified || !s.cfg.RequireEmailVerification
	u, err = s.st.CreateUser(ctx, store.CreateUserParams{
		ID: ids.New(), Email: email, Name: name, GoogleSub: &id.Sub, AvatarURL: picture, Verified: verified,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return store.User{}, ErrEmailTaken
		}
		return store.User{}, err
	}
	if !verified {
		if err := s.SendVerification(ctx, u); err != nil {
			s.log.Warn("verification email failed", "user", u.ID, "err", err)
		}
	}
	_ = s.st.TouchLogin(ctx, u.ID)
	return u, nil
}
