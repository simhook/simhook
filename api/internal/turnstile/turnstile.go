// Package turnstile checks Cloudflare Turnstile tokens, the bot check on
// the sign-in forms.
package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrFailed is returned when the token does not pass.
var ErrFailed = errors.New("the bot check did not pass")

// Verifier checks a token a browser obtained from the widget.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// Endpoint is Cloudflare's verification endpoint.
const Endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Client verifies against Cloudflare.
type Client struct {
	Secret   string
	Endpoint string
	HTTP     *http.Client
}

// New builds a client for the widget's secret key.
func New(secret string) *Client {
	return &Client{Secret: secret, Endpoint: Endpoint, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Verify implements Verifier. A token is single-use and bound to the
// visitor's address when one is given.
func (c *Client) Verify(ctx context.Context, token, remoteIP string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 2048 {
		return ErrFailed
	}
	form := url.Values{"secret": {c.Secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("turnstile: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("turnstile: %w", err)
	}
	var out struct {
		Success bool     `json:"success"`
		Errors  []string `json:"error-codes"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("turnstile: bad answer: %w", err)
	}
	if !out.Success {
		return fmt.Errorf("%w (%s)", ErrFailed, strings.Join(out.Errors, ", "))
	}
	return nil
}

// Fake accepts one token. For tests.
type Fake struct {
	Accept string
}

// Verify implements Verifier.
func (f *Fake) Verify(_ context.Context, token, _ string) error {
	if token != f.Accept {
		return ErrFailed
	}
	return nil
}
