package turnstile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerify(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = map[string]string{"secret": r.Form.Get("secret"), "response": r.Form.Get("response"), "remoteip": r.Form.Get("remoteip")}
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("response") == "good" {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	}))
	defer srv.Close()

	c := New("s3cret")
	c.Endpoint = srv.URL
	ctx := context.Background()
	if err := c.Verify(ctx, "good", "203.0.113.9"); err != nil {
		t.Fatalf("a good token should pass: %v", err)
	}
	if got["secret"] != "s3cret" || got["response"] != "good" || got["remoteip"] != "203.0.113.9" {
		t.Fatalf("form sent: %v", got)
	}
	if err := c.Verify(ctx, "bad", ""); !errors.Is(err, ErrFailed) {
		t.Fatalf("a bad token should fail with ErrFailed: %v", err)
	}
	if err := c.Verify(ctx, "  ", ""); !errors.Is(err, ErrFailed) || got["response"] != "bad" {
		t.Fatalf("an empty token should fail without a round trip: %v", err)
	}
}
