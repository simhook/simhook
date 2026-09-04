// Package mail sends transactional email. Templates are deliberately plain:
// one short text body and a matching HTML body per message type.
package mail

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	gomail "github.com/wneessen/go-mail"
)

// Message is one outgoing email.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer delivers messages.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// SMTPConfig configures the SMTP mailer.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

type smtpMailer struct {
	cfg SMTPConfig
}

// NewSMTP returns a Mailer that talks to an SMTP relay. Authentication and
// TLS are used when credentials are configured; a bare host and port (a local
// catcher) is used as-is.
func NewSMTP(cfg SMTPConfig) Mailer {
	return &smtpMailer{cfg: cfg}
}

func (s *smtpMailer) Send(ctx context.Context, m Message) error {
	opts := []gomail.Option{gomail.WithPort(s.cfg.Port)}
	if s.cfg.User != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(s.cfg.User),
			gomail.WithPassword(s.cfg.Password),
			gomail.WithTLSPolicy(gomail.TLSMandatory),
		)
	} else {
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	}
	client, err := gomail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("mail: client: %w", err)
	}
	msg := gomail.NewMsg()
	if err := msg.From(s.cfg.From); err != nil {
		return fmt.Errorf("mail: from: %w", err)
	}
	if err := msg.To(m.To); err != nil {
		return fmt.Errorf("mail: to: %w", err)
	}
	msg.Subject(m.Subject)
	msg.SetBodyString(gomail.TypeTextPlain, m.Text)
	if m.HTML != "" {
		msg.AddAlternativeString(gomail.TypeTextHTML, m.HTML)
	}
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

// Logger is a Mailer that only logs. Used when no SMTP host is configured.
type Logger struct {
	Log *slog.Logger
}

// Send implements Mailer.
func (l *Logger) Send(_ context.Context, m Message) error {
	l.Log.Info("email (logged, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

// ---------------------------------------------------------------------------
// Message builders
// ---------------------------------------------------------------------------

func wrapHTML(title, body string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="font-family:system-ui,sans-serif;max-width:560px;margin:32px auto;padding:0 16px;color:#111">`)
	b.WriteString("<h2 style=\"margin:0 0 16px\">" + html.EscapeString(title) + "</h2>")
	b.WriteString(body)
	b.WriteString(`<p style="margin-top:32px;color:#666;font-size:13px">simhook</p></body></html>`)
	return b.String()
}

// VerifyEmail builds the verification message.
func VerifyEmail(to, name, code string) Message {
	greeting := "Hi"
	if name != "" {
		greeting = "Hi " + name
	}
	text := fmt.Sprintf("%s,\n\nYour simhook verification code is %s\n\nIt expires in 30 minutes. If you did not create an account, ignore this email.\n", greeting, code)
	htmlBody := fmt.Sprintf(`<p>%s,</p><p>Your verification code is</p><p style="font-size:28px;letter-spacing:4px;font-weight:600">%s</p><p>It expires in 30 minutes. If you did not create an account, ignore this email.</p>`,
		html.EscapeString(greeting), html.EscapeString(code))
	return Message{To: to, Subject: "Verify your email", Text: text, HTML: wrapHTML("Verify your email", htmlBody)}
}

// PasswordReset builds the reset message.
func PasswordReset(to, code string) Message {
	text := fmt.Sprintf("Your simhook password reset code is %s\n\nIt expires in 30 minutes. If you did not request a reset, ignore this email.\n", code)
	htmlBody := fmt.Sprintf(`<p>Your password reset code is</p><p style="font-size:28px;letter-spacing:4px;font-weight:600">%s</p><p>It expires in 30 minutes. If you did not request a reset, ignore this email.</p>`,
		html.EscapeString(code))
	return Message{To: to, Subject: "Reset your password", Text: text, HTML: wrapHTML("Reset your password", htmlBody)}
}

// WebhookPaused tells a user their endpoint was disabled for failing.
func WebhookPaused(to, webhookName, url, reason, dashboardURL string) Message {
	label := webhookName
	if label == "" {
		label = url
	}
	text := fmt.Sprintf("Your webhook %q was paused.\n\n%s\n\nRe-enable it from the dashboard when your endpoint is ready: %s\n", label, reason, dashboardURL)
	htmlBody := fmt.Sprintf(`<p>Your webhook <strong>%s</strong> was paused.</p><p>%s</p><p><a href="%s">Re-enable it from the dashboard</a> when your endpoint is ready.</p>`,
		html.EscapeString(label), html.EscapeString(reason), html.EscapeString(dashboardURL))
	return Message{To: to, Subject: "A webhook was paused", Text: text, HTML: wrapHTML("A webhook was paused", htmlBody)}
}
