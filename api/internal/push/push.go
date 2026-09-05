// Package push wakes phones. The only implementation that reaches a real
// device is Firebase Cloud Messaging; the logging sender exists so the whole
// pipeline runs in development without credentials.
package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// Message is one push to one device. Data is delivered as-is; the phone
// never sees a notification, only a wake-up with a payload.
type Message struct {
	Token       string
	Data        map[string]string
	TTL         time.Duration
	CollapseKey string
}

// Result is the outcome of one Message.
type Result struct {
	OK bool
	// TokenInvalid means the provider will never deliver to this token again
	// and the device must re-register.
	TokenInvalid bool
	Err          error
}

// Sender delivers pushes. Implementations must return exactly one Result per
// Message, in order, unless the whole call fails.
type Sender interface {
	Send(ctx context.Context, msgs []Message) ([]Result, error)
}

// ErrNoToken is returned in a Result when a message had no token.
var ErrNoToken = errors.New("push: device has no push token")

// fcmBatchLimit is the provider's maximum per call.
const fcmBatchLimit = 500

type fcmSender struct {
	client *messaging.Client
}

// NewFCM builds a Sender backed by Firebase Cloud Messaging using a
// service-account credentials file.
func NewFCM(ctx context.Context, credentialsFile string) (Sender, error) {
	// The SDK falls back to ambient credentials when the file cannot be read,
	// which surfaces later as a confusing "project ID is required" error.
	if _, err := os.ReadFile(credentialsFile); err != nil {
		return nil, fmt.Errorf("push: credentials file: %w", err)
	}
	app, err := firebase.NewApp(ctx, nil, option.WithAuthCredentialsFile(option.ServiceAccount, credentialsFile))
	if err != nil {
		return nil, err
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	return &fcmSender{client: client}, nil
}

func (s *fcmSender) Send(ctx context.Context, msgs []Message) ([]Result, error) {
	results := make([]Result, len(msgs))
	for start := 0; start < len(msgs); start += fcmBatchLimit {
		end := min(start+fcmBatchLimit, len(msgs))
		chunk := msgs[start:end]
		out := make([]*messaging.Message, 0, len(chunk))
		index := make([]int, 0, len(chunk))
		for i, m := range chunk {
			if m.Token == "" {
				results[start+i] = Result{Err: ErrNoToken, TokenInvalid: true}
				continue
			}
			ttl := m.TTL
			out = append(out, &messaging.Message{
				// The phone registers with an FCM registration token, which is what
				// the server holds; an installation id is a different thing.
				//lint:ignore SA1019 registration tokens are still the delivery address
				Token: m.Token,
				Data:  m.Data,
				Android: &messaging.AndroidConfig{
					Priority:    "high",
					TTL:         &ttl,
					CollapseKey: m.CollapseKey,
				},
			})
			index = append(index, start+i)
		}
		if len(out) == 0 {
			continue
		}
		resp, err := s.client.SendEach(ctx, out)
		if err != nil {
			return nil, err
		}
		for j, r := range resp.Responses {
			i := index[j]
			if r.Success {
				results[i] = Result{OK: true}
				continue
			}
			results[i] = Result{
				Err:          r.Error,
				TokenInvalid: messaging.IsUnregistered(r.Error) || messaging.IsSenderIDMismatch(r.Error),
			}
		}
	}
	return results, nil
}

// Logger is a Sender that reports success without contacting anything. It
// logs each push so the dispatch pipeline is observable in development.
type Logger struct {
	Log *slog.Logger
}

// Send implements Sender.
func (l *Logger) Send(_ context.Context, msgs []Message) ([]Result, error) {
	results := make([]Result, len(msgs))
	for i, m := range msgs {
		if m.Token == "" {
			results[i] = Result{Err: ErrNoToken, TokenInvalid: true}
			continue
		}
		l.Log.Info("push (logged, not sent)", "token", m.Token[:min(8, len(m.Token))]+"…", "data", m.Data, "ttl", m.TTL)
		results[i] = Result{OK: true}
	}
	return results, nil
}
