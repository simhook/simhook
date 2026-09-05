package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/simhook/simhook/internal/app"
	"github.com/simhook/simhook/internal/mail"
	"github.com/simhook/simhook/internal/push"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/testutil"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type captureMailer struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (m *captureMailer) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *captureMailer) lastCode(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no email was sent")
	}
	code := regexp.MustCompile(`\b(\d{6})\b`).FindString(m.sent[len(m.sent)-1].Text)
	if code == "" {
		t.Fatalf("no code in email: %q", m.sent[len(m.sent)-1].Text)
	}
	return code
}

type recordingPush struct {
	mu     sync.Mutex
	pushes []push.Message
	reject map[string]bool // tokens to reject as invalid
}

func (p *recordingPush) Send(_ context.Context, msgs []push.Message) ([]push.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]push.Result, len(msgs))
	for i, m := range msgs {
		p.pushes = append(p.pushes, m)
		if p.reject[m.Token] {
			out[i] = push.Result{TokenInvalid: true, Err: fmt.Errorf("unregistered")}
			continue
		}
		out[i] = push.Result{OK: true}
	}
	return out, nil
}

func (p *recordingPush) sends() []push.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []push.Message
	for _, m := range p.pushes {
		if m.Data["type"] == "send" {
			out = append(out, m)
		}
	}
	return out
}

type hookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type hookServer struct {
	*httptest.Server
	mu     sync.Mutex
	secret string
	events []hookEvent
	badSig int
}

func newHookServer(t *testing.T) *hookServer {
	h := &hookServer{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		defer h.mu.Unlock()
		if !secrets.VerifyWebhook(h.secret, r.Header.Get("X-Simhook-Signature"), body, time.Now().Unix(), 300) {
			h.badSig++
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var ev hookEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.events = append(h.events, ev)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *hookServer) count(event string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, e := range h.events {
		if e.Event == event {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

type client struct {
	t      *testing.T
	base   string
	cookie *http.Cookie
	apiKey string
	bearer string
}

type resp struct {
	status int
	body   map[string]any
	raw    []byte
	cookie *http.Cookie
}

func (c *client) do(method, path string, in any) resp {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, body)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	for _, ck := range res.Cookies() {
		if ck.Name == "simhook_session" {
			out.cookie = ck
		}
	}
	return out
}

func (c *client) must(method, path string, in any, want int) resp {
	c.t.Helper()
	r := c.do(method, path, in)
	if r.status != want {
		c.t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, r.status, r.raw)
	}
	return r
}

func str(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	s, _ := cur.(string)
	return s
}

func num(m map[string]any, path ...string) float64 {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return -1
		}
		cur = mm[p]
	}
	f, _ := cur.(float64)
	return f
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ---------------------------------------------------------------------------
// The flow
// ---------------------------------------------------------------------------

func TestEndToEnd(t *testing.T) {
	cfg := testutil.Config(t)
	testutil.Reset(t)
	mailer := &captureMailer{}
	pusher := &recordingPush{reject: map[string]bool{}}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := app.Build(ctx, cfg, log, app.Options{Mailer: mailer, Push: pusher})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		stopCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = a.Stop(stopCtx)
	}()
	srv := httptest.NewServer(a.HTTP.Handler())
	defer srv.Close()

	web := &client{t: t, base: srv.URL}

	// Register, verify email, get an API key.
	r := web.must("POST", "/v1/auth/register", map[string]any{"email": "Dev@Example.com", "password": "hunter2hunter2", "name": "Dev"}, 201)
	if r.cookie == nil {
		t.Fatal("no session cookie on register")
	}
	web.cookie = r.cookie
	if str(r.body, "user", "email") != "dev@example.com" {
		t.Fatalf("email not normalized: %s", r.raw)
	}
	web.must("POST", "/v1/messages", map[string]any{"to": []string{"+15550001"}, "body": "x"}, 403) // unverified
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": "000000"}, 400)
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": mailer.lastCode(t)}, 200)

	r = web.must("POST", "/v1/api-keys", map[string]any{"name": "ci"}, 201)
	apiKey := str(r.body, "key")
	if apiKey == "" {
		t.Fatalf("no key: %s", r.raw)
	}
	dev := &client{t: t, base: srv.URL, apiKey: apiKey}
	dev.must("GET", "/v1/api-keys", nil, 401)                                                       // keys cannot manage keys
	dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+15550001"}, "body": "x"}, 400) // no device yet

	// Pair a phone.
	r = dev.must("POST", "/v1/devices/pairing-codes", nil, 201)
	code := str(r.body, "code")
	phone := &client{t: t, base: srv.URL}
	phone.must("POST", "/v1/device/pair", map[string]any{"code": "AAAA-BBBB", "hardware_key": "hw-pixel-0001"}, 400)
	r = phone.must("POST", "/v1/device/pair", map[string]any{
		"code": code, "hardware_key": "hw-pixel-0001", "brand": "Pixel", "model": "8", "push_token": "tok-1",
	}, 201)
	phone.bearer = str(r.body, "device_token")
	deviceID := str(r.body, "device", "id")
	if phone.bearer == "" || deviceID == "" {
		t.Fatalf("pair response: %s", r.raw)
	}
	if r.body["device"].(map[string]any)["is_default"] != true {
		t.Fatalf("first device should be default: %s", r.raw)
	}
	if r.body["device"].(map[string]any)["receive_enabled"] != true {
		t.Fatalf("a new device should forward incoming SMS: %s", r.raw)
	}
	phone.must("POST", "/v1/device/pair", map[string]any{"code": code, "hardware_key": "hw-pixel-0002"}, 400) // code consumed
	phone.must("GET", "/v1/devices", nil, 401)                                                                // device token is not a user

	// Subscribe a webhook.
	hooks := newHookServer(t)
	r = dev.must("POST", "/v1/webhooks", map[string]any{
		"url": hooks.URL, "events": []string{"message.sent", "message.delivered", "message.failed", "message.received", "device.online", "ping"},
	}, 201)
	hooks.secret = str(r.body, "secret")
	webhookID := str(r.body, "webhook", "id")
	dev.must("POST", "/v1/webhooks", map[string]any{"url": "ftp://x", "events": []string{"ping"}}, 422)
	dev.must("POST", "/v1/webhooks/"+webhookID+"/test", nil, 202)
	waitFor(t, "ping delivery", func() bool { return hooks.count("ping") == 1 })

	// Heartbeat brings the device online.
	r = phone.must("POST", "/v1/device/heartbeat", map[string]any{"telemetry": map[string]any{"battery": 80}, "sims": []any{map[string]any{"subscription_id": 1}}}, 200)
	if r.body["device"].(map[string]any)["online"] != true {
		t.Fatalf("device should be online: %s", r.raw)
	}
	waitFor(t, "device.online", func() bool { return hooks.count("device.online") == 1 })

	// Send to two recipients; the dispatcher pushes both.
	r = dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+1 (415) 555-0123", "+14155550124", "+14155550124"}, "body": "hello"}, 202)
	batchID := str(r.body, "batch", "id")
	msgIDs := r.body["message_ids"].([]any)
	if len(msgIDs) != 2 {
		t.Fatalf("duplicates should collapse to 2 ids: %s", r.raw)
	}
	waitFor(t, "2 pushes", func() bool { return len(pusher.sends()) == 2 })
	waitFor(t, "batch processing", func() bool {
		b := dev.must("GET", "/v1/batches/"+batchID, nil, 200)
		return str(b.body, "batch", "status") == "processing" && num(b.body, "batch", "dispatched_count") == 2
	})
	if got := pusher.sends()[0].Data["to"]; got != "+14155550123" {
		t.Fatalf("push recipient: %s", got)
	}

	// The phone reports outcomes.
	id1, id2 := msgIDs[0].(string), msgIDs[1].(string)
	phone.must("POST", "/v1/device/messages/"+id1+"/status", map[string]any{"status": "sent"}, 200)
	phone.must("POST", "/v1/device/messages/"+id1+"/status", map[string]any{"status": "delivered"}, 200)
	r = phone.must("POST", "/v1/device/messages/"+id1+"/status", map[string]any{"status": "sent"}, 200) // late, ignored
	if str(r.body, "message", "status") != "delivered" {
		t.Fatalf("out-of-order report moved the message backwards: %s", r.raw)
	}
	phone.must("POST", "/v1/device/messages/"+id2+"/status", map[string]any{"status": "failed", "error_code": "no_service", "error_message": "No signal"}, 200)
	phone.must("POST", "/v1/device/messages/00000000-0000-0000-0000-000000000000/status", map[string]any{"status": "sent"}, 404)

	r = dev.must("GET", "/v1/batches/"+batchID, nil, 200)
	if str(r.body, "batch", "status") != "partial" || num(r.body, "batch", "delivered_count") != 1 || num(r.body, "batch", "failed_count") != 1 {
		t.Fatalf("batch after reports: %s", r.raw)
	}
	waitFor(t, "status webhooks", func() bool {
		return hooks.count("message.sent") == 1 && hooks.count("message.delivered") == 1 && hooks.count("message.failed") == 1
	})

	// Inbound, with de-duplication by fingerprint.
	in := map[string]any{"sender": "+19995550100", "body": "reply", "received_at": time.Now().UTC().Format(time.RFC3339), "fingerprint": "fp-1"}
	phone.must("POST", "/v1/device/inbound", in, 201)
	r = phone.must("POST", "/v1/device/inbound", in, 200)
	if r.body["inserted"] != false {
		t.Fatalf("duplicate inbound should not insert: %s", r.raw)
	}
	waitFor(t, "message.received", func() bool { return hooks.count("message.received") == 1 })
	r = dev.must("GET", "/v1/messages?direction=inbound", nil, 200)
	if n := len(r.body["data"].([]any)); n != 1 {
		t.Fatalf("want 1 inbound message, got %d", n)
	}
	r = dev.must("GET", "/v1/messages?q=hello&limit=1", nil, 200)
	if n := len(r.body["data"].([]any)); n != 1 || str(r.body, "next_cursor") == "" {
		t.Fatalf("search + paging: %s", r.raw)
	}
	r = dev.must("GET", "/v1/messages?q=hello&limit=1&cursor="+str(r.body, "next_cursor"), nil, 200)
	if n := len(r.body["data"].([]any)); n != 1 || str(r.body, "next_cursor") != "" {
		t.Fatalf("second page: %s", r.raw)
	}

	// Usage and limits.
	r = web.must("GET", "/v1/auth/me", nil, 200)
	if num(r.body, "usage", "sent_today") != 2 || str(r.body, "limits", "plan_id") != "free" {
		t.Fatalf("usage: %s", r.raw)
	}
	many := make([]string, 26)
	for i := range many {
		many[i] = fmt.Sprintf("+1415555%04d", i)
	}
	r = dev.do("POST", "/v1/messages", map[string]any{"to": many, "body": "bulk"})
	if r.status != 429 || str(r.body, "code") != "plan_limit_batch" {
		t.Fatalf("batch limit: %d %s", r.status, r.raw)
	}
	dev.must("POST", "/v1/messages", map[string]any{"to": many[:25], "body": "bulk"}, 202) // 2 + 25 = 27 of 30
	r = dev.do("POST", "/v1/messages", map[string]any{"to": many[:4], "body": "bulk"})
	if r.status != 429 || str(r.body, "code") != "plan_limit_daily" {
		t.Fatalf("daily limit: %d %s", r.status, r.raw)
	}
	r = web.must("GET", "/v1/auth/me", nil, 200)
	if num(r.body, "usage", "sent_today") != 27 {
		t.Fatalf("a rejected send must not count: %s", r.raw)
	}

	// Invalid push token fails the message and invalidates the device token.
	pusher.reject["tok-1"] = true
	r = dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155551000"}, "body": "x"}, 202)
	waitFor(t, "push rejection recorded", func() bool {
		b := dev.must("GET", "/v1/batches/"+str(r.body, "batch", "id"), nil, 200)
		return str(b.body, "batch", "status") == "failed"
	})
	r = dev.must("GET", "/v1/devices/"+deviceID, nil, 200)
	if r.body["device"].(map[string]any)["push_token_invalidated_at"] == nil {
		t.Fatalf("push token should be invalidated: %s", r.raw)
	}

	// The phone can unpair itself, which revokes its token; the dashboard
	// then no longer finds it.
	phone.must("DELETE", "/v1/device", nil, 204)
	phone.must("GET", "/v1/device", nil, 401)
	dev.must("DELETE", "/v1/devices/"+deviceID, nil, 404)
	r = dev.must("GET", "/v1/devices", nil, 200)
	if n := len(r.body["data"].([]any)); n != 0 {
		t.Fatalf("device list after unpair: %s", r.raw)
	}

	// Sessions: logout kills the cookie.
	web.must("POST", "/v1/auth/logout", nil, 204)
	web.must("GET", "/v1/auth/me", nil, 401)

	if hooks.badSig != 0 {
		t.Fatalf("%d deliveries had bad signatures", hooks.badSig)
	}
}
