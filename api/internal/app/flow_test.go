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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
// The assembled service
// ---------------------------------------------------------------------------

type harness struct {
	app    *app.App
	srv    *httptest.Server
	mailer *captureMailer
	pusher *recordingPush
}

// execSQL runs one statement on the test database, for the states the API
// only reaches with time.
func execSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.DatabaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

// startApp assembles the real service on the test database, with the mailer
// and the push provider swapped for recorders.
func startApp(t *testing.T) *harness {
	t.Helper()
	cfg := testutil.Config(t)
	testutil.Reset(t)
	mailer := &captureMailer{}
	pusher := &recordingPush{reject: map[string]bool{}}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(context.Background())
	a, err := app.Build(ctx, cfg, log, app.Options{Mailer: mailer, Push: pusher})
	if err != nil {
		cancel()
		t.Fatalf("build: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start: %v", err)
	}
	srv := httptest.NewServer(a.HTTP.Handler())
	t.Cleanup(func() {
		srv.Close()
		stopCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = a.Stop(stopCtx)
		cancel()
	})
	return &harness{app: a, srv: srv, mailer: mailer, pusher: pusher}
}

// signUp registers and verifies an account and returns a signed-in client.
func (h *harness) signUp(t *testing.T, email string) *client {
	t.Helper()
	c := &client{t: t, base: h.srv.URL}
	r := c.must("POST", "/v1/auth/register", map[string]any{"email": email, "password": "hunter2hunter2"}, 201)
	c.signIn(r)
	c.must("POST", "/v1/auth/verify-email", map[string]any{"code": h.mailer.lastCode(t)}, 200)
	return c
}

// login signs an existing account in on a fresh client.
func (h *harness) login(t *testing.T, email, password string) *client {
	t.Helper()
	c := &client{t: t, base: h.srv.URL}
	c.signIn(c.must("POST", "/v1/auth/login", map[string]any{"email": email, "password": password}, 200))
	return c
}

// pairPhone mints a code on the account and pairs a phone with it.
func (h *harness) pairPhone(t *testing.T, account *client, hardwareKey, pushToken string) (phone *client, deviceID string) {
	t.Helper()
	r := account.must("POST", "/v1/devices/pairing-codes", nil, 201)
	phone = &client{t: t, base: h.srv.URL}
	r = phone.must("POST", "/v1/device/pair", map[string]any{"code": str(r.body, "code"), "hardware_key": hardwareKey, "push_token": pushToken}, 201)
	phone.bearer = str(r.body, "device_token")
	return phone, str(r.body, "device", "id")
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

type client struct {
	t       *testing.T
	base    string
	cookie  *http.Cookie // the session cookie
	flag    *http.Cookie // the readable signed-in flag
	apiKey  string
	bearer  string
	headers map[string]string
}

type resp struct {
	status     int
	body       map[string]any
	raw        []byte
	cookie     *http.Cookie            // the session cookie, if set
	cookies    map[string]*http.Cookie // every cookie set, by name
	setCookies []*http.Cookie          // every Set-Cookie header, in order
}

// signIn takes the cookies a sign-in response set.
func (c *client) signIn(r resp) {
	c.t.Helper()
	if r.cookies["simhook_session"] == nil || r.cookies["simhook_signed_in"] == nil {
		c.t.Fatalf("sign-in must set both cookies, got %v", r.setCookies)
	}
	c.cookie, c.flag = r.cookies["simhook_session"], r.cookies["simhook_signed_in"]
}

// cleared reports whether a response expires both cookies.
func cleared(r resp) bool {
	for _, name := range []string{"simhook_session", "simhook_signed_in"} {
		c := r.cookies[name]
		if c == nil || c.Value != "" || c.MaxAge >= 0 {
			return false
		}
	}
	return true
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
		req.AddCookie(&http.Cookie{Name: c.cookie.Name, Value: c.cookie.Value})
	}
	if c.flag != nil {
		req.AddCookie(&http.Cookie{Name: c.flag.Name, Value: c.flag.Value})
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw, cookies: map[string]*http.Cookie{}}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	for _, ck := range res.Cookies() {
		out.setCookies = append(out.setCookies, ck)
		out.cookies[ck.Name] = ck
		if ck.Name == "simhook_session" {
			out.cookie = ck
		}
	}
	return out
}

// with returns a copy of the client that sends extra request headers.
func (c *client) with(headers map[string]string) *client {
	cp := *c
	cp.headers = headers
	return &cp
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
	h := startApp(t)
	web := &client{t: t, base: h.srv.URL}

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
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": h.mailer.lastCode(t)}, 200)

	r = web.must("POST", "/v1/api-keys", map[string]any{"name": "ci"}, 201)
	apiKey := str(r.body, "key")
	keyID := str(r.body, "api_key", "id")
	if apiKey == "" || keyID == "" {
		t.Fatalf("no key: %s", r.raw)
	}
	web.must("POST", "/v1/api-keys", map[string]any{"name": "bad", "scopes": []string{"bogus"}}, 422)
	web.must("PATCH", "/v1/api-keys/"+keyID, map[string]any{"name": "   "}, 422)
	dev := &client{t: t, base: h.srv.URL, apiKey: apiKey}
	dev.must("GET", "/v1/api-keys", nil, 401)                                                       // keys cannot manage keys
	dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+15550001"}, "body": "x"}, 400) // no device yet

	// Pair a phone. The dashboard watches the code, not the device list.
	r = dev.must("POST", "/v1/devices/pairing-codes", nil, 201)
	code := str(r.body, "code")
	codeID := str(r.body, "id")
	r = dev.must("GET", "/v1/devices/pairing-codes/"+codeID, nil, 200)
	if r.body["consumed"] != false || r.body["device"] != nil {
		t.Fatalf("fresh code: %s", r.raw)
	}
	phone := &client{t: t, base: h.srv.URL}
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
	r = dev.must("GET", "/v1/devices/pairing-codes/"+codeID, nil, 200)
	if r.body["consumed"] != true || str(r.body, "device", "id") != deviceID {
		t.Fatalf("used code should name the phone: %s", r.raw)
	}

	// Subscribe a webhook.
	hooks := newHookServer(t)
	r = dev.must("POST", "/v1/webhooks", map[string]any{
		"url": hooks.URL, "events": []string{"message.sent", "message.delivered", "message.failed", "message.received", "device.online", "ping"},
	}, 201)
	hooks.secret = str(r.body, "secret")
	webhookID := str(r.body, "webhook", "id")
	dev.must("POST", "/v1/webhooks", map[string]any{"url": "ftp://x", "events": []string{"ping"}}, 422)
	dev.must("GET", "/v1/webhooks/deliveries?event=foo", nil, 422)
	dev.must("POST", "/v1/webhooks/"+webhookID+"/test", nil, 202)
	waitFor(t, "ping delivery", func() bool { return hooks.count("ping") == 1 })

	// Heartbeat brings the device online.
	r = phone.must("POST", "/v1/device/heartbeat", map[string]any{"telemetry": map[string]any{"battery": 80}, "sims": []any{map[string]any{"subscription_id": 1}}}, 200)
	if r.body["device"].(map[string]any)["online"] != true {
		t.Fatalf("device should be online: %s", r.raw)
	}
	waitFor(t, "device.online", func() bool { return hooks.count("device.online") == 1 })

	// Send to two recipients. The dispatcher wakes the phone with one push
	// that names the phone and carries no content; the phone pulls its outbox.
	r = dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+1 (415) 555-0123", "+14155550124", "+14155550124"}, "body": "hello"}, 202)
	batchID := str(r.body, "batch", "id")
	msgIDs := r.body["message_ids"].([]any)
	if len(msgIDs) != 2 {
		t.Fatalf("duplicates should collapse to 2 ids: %s", r.raw)
	}
	waitFor(t, "wake-up push", func() bool { return len(h.pusher.sends()) == 1 })
	if wake := h.pusher.sends()[0]; wake.Data["device_id"] != deviceID || wake.Data["to"] != "" || wake.Data["body"] != "" {
		t.Fatalf("the push must name the phone and carry no content: %v", wake.Data)
	}
	r = phone.must("GET", "/v1/device/outbox", nil, 200)
	outbox := r.body["data"].([]any)
	if len(outbox) != 2 || outbox[0].(map[string]any)["to"] != "+14155550123" || outbox[0].(map[string]any)["body"] != "hello" {
		t.Fatalf("outbox: %s", r.raw)
	}
	r = dev.must("GET", "/v1/batches/"+batchID, nil, 200)
	if str(r.body, "batch", "status") != "processing" || num(r.body, "batch", "dispatched_count") != 2 {
		t.Fatalf("batch after the fetch: %s", r.raw)
	}
	r = phone.must("GET", "/v1/device/outbox", nil, 200)
	if n := len(r.body["data"].([]any)); n != 2 {
		t.Fatalf("unreported messages must be offered again, got %d", n)
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
	r = dev.must("GET", "/v1/stats", nil, 200)
	if num(r.body, "sent") != 1 {
		t.Fatalf("only the message the carrier took counts as sent: %s", r.raw)
	}
	r = phone.must("GET", "/v1/device/outbox", nil, 200)
	if n := len(r.body["data"].([]any)); n != 0 {
		t.Fatalf("reported messages leave the outbox, got %d", n)
	}

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

	// A send scheduled for tomorrow counts against tomorrow, not today.
	tomorrow := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	r = dev.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155552000"}, "body": "later", "scheduled_at": tomorrow}, 202)
	if str(r.body, "batch", "scheduled_at") == "" || str(r.body, "batch", "status") != "queued" {
		t.Fatalf("scheduled send: %s", r.raw)
	}
	r = web.must("GET", "/v1/auth/me", nil, 200)
	if num(r.body, "usage", "sent_today") != 27 {
		t.Fatalf("a scheduled send must not count today: %s", r.raw)
	}

	// Invalid push token fails the message and invalidates the device token.
	h.pusher.reject["tok-1"] = true
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

	// Sign-out clears both cookies.
	if r = web.must("POST", "/v1/auth/logout", nil, 204); !cleared(r) {
		t.Fatalf("logout should clear both cookies: %v", r.setCookies)
	}
	web.must("GET", "/v1/auth/me", nil, 401)

	if hooks.badSig != 0 {
		t.Fatalf("%d deliveries had bad signatures", hooks.badSig)
	}
}

// TestPairingMovesThePhone: a handset paired to a second account stops
// acting for the first, even though the first never unpaired it.
func TestPairingMovesThePhone(t *testing.T) {
	h := startApp(t)
	first := h.signUp(t, "first@example.com")
	phone, firstDevice := h.pairPhone(t, first, "hw-shared-0001", "tok-shared")
	oldToken := phone.bearer

	second := h.signUp(t, "second@example.com")
	_, secondDevice := h.pairPhone(t, second, "hw-shared-0001", "tok-shared")
	if secondDevice == firstDevice {
		t.Fatal("the second account gets its own device row")
	}

	// The first account's row can no longer be woken, and its token is dead.
	r := first.must("GET", "/v1/devices/"+firstDevice, nil, 200)
	if r.body["device"].(map[string]any)["push_token_invalidated_at"] == nil {
		t.Fatalf("the first pairing should have lost its push registration: %s", r.raw)
	}
	phone.bearer = oldToken
	phone.must("GET", "/v1/device", nil, 401)
}

// TestStaleSweepRespectsPacing: a fetched message is only overdue once the
// phone should have sent it, a late report resolves an overdue message, and
// a queued message waits a day for the phone before it is given up on.
func TestStaleSweepRespectsPacing(t *testing.T) {
	h := startApp(t)
	web := h.signUp(t, "sweep@example.com")
	phone, _ := h.pairPhone(t, web, "hw-sweep-0001", "tok-sweep")
	ctx := context.Background()

	r := web.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155550301", "+14155550302"}, "body": "paced"}, 202)
	batchID := str(r.body, "batch", "id")
	ids := r.body["message_ids"].([]any)
	id1, id2 := uuid.MustParse(ids[0].(string)), uuid.MustParse(ids[1].(string))
	waitFor(t, "wake-up push", func() bool { return len(h.pusher.sends()) == 1 })
	r = phone.must("GET", "/v1/device/outbox", nil, 200)
	if n := len(r.body["data"].([]any)); n != 2 {
		t.Fatalf("outbox: %s", r.raw)
	}

	// Both were fetched an hour ago; only the first was expected to be sent by now.
	execSQL(t, `update messages set dispatched_at = now() - interval '1 hour', expected_send_at = now() - interval '1 hour' where id = $1`, id1)
	execSQL(t, `update messages set dispatched_at = now() - interval '1 hour', expected_send_at = now() + interval '1 hour' where id = $1`, id2)
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r = web.must("GET", "/v1/messages/"+id1.String(), nil, 200); str(r.body, "message", "status") != "unknown" {
		t.Fatalf("the overdue message should be unknown: %s", r.raw)
	}
	if r = web.must("GET", "/v1/messages/"+id2.String(), nil, 200); str(r.body, "message", "status") != "dispatched" {
		t.Fatalf("a message the phone has not reached yet must be left alone: %s", r.raw)
	}
	r = web.must("GET", "/v1/batches/"+batchID, nil, 200)
	if num(r.body, "batch", "unknown_count") != 1 || num(r.body, "batch", "dispatched_count") != 1 || str(r.body, "batch", "status") != "processing" {
		t.Fatalf("batch after the sweep: %s", r.raw)
	}

	// The phone does send it in the end; the truth wins.
	r = phone.must("POST", "/v1/device/messages/"+id1.String()+"/status", map[string]any{"status": "sent"}, 200)
	if str(r.body, "message", "status") != "sent" {
		t.Fatalf("a late report should resolve an unknown message: %s", r.raw)
	}
	r = web.must("GET", "/v1/batches/"+batchID, nil, 200)
	if num(r.body, "batch", "unknown_count") != 0 || num(r.body, "batch", "sent_count") != 1 {
		t.Fatalf("batch after the late report: %s", r.raw)
	}

	// A queued message is not given up on until a day after it was due.
	r = web.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155550303"}, "body": "waiting"}, 202)
	waitingBatch := str(r.body, "batch", "id")
	id3 := uuid.MustParse(r.body["message_ids"].([]any)[0].(string))
	execSQL(t, `update messages set dispatch_due_at = now() - interval '2 hours' where id = $1`, id3)
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r = web.must("GET", "/v1/messages/"+id3.String(), nil, 200); str(r.body, "message", "status") != "queued" {
		t.Fatalf("two hours is not a day: %s", r.raw)
	}
	execSQL(t, `update messages set dispatch_due_at = now() - interval '25 hours' where id = $1`, id3)
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	r = web.must("GET", "/v1/messages/"+id3.String(), nil, 200)
	if str(r.body, "message", "status") != "unknown" || str(r.body, "message", "error_code") != "not_dispatched" {
		t.Fatalf("a day later the message is given up on: %s", r.raw)
	}
	if r = web.must("GET", "/v1/batches/"+waitingBatch, nil, 200); str(r.body, "batch", "status") != "unknown" {
		t.Fatalf("a batch with every message unknown is unknown: %s", r.raw)
	}
}

// TestCredentialHardening covers the two ways a credential could be guessed:
// a six-digit code, and a password for a known address.
func TestCredentialHardening(t *testing.T) {
	h := startApp(t)
	web := &client{t: t, base: h.srv.URL}

	r := web.must("POST", "/v1/auth/register", map[string]any{"email": "lock@example.com", "password": "hunter2hunter2"}, 201)
	web.cookie = r.cookie
	real := h.mailer.lastCode(t)
	wrong := "000000"
	if wrong == real {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		web.must("POST", "/v1/auth/verify-email", map[string]any{"code": wrong}, 400)
	}
	// Five wrong guesses burn the code; even the right one no longer works.
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": real}, 400)
	web.must("POST", "/v1/auth/verify-email/send", nil, 202)
	fresh := h.mailer.lastCode(t)
	if fresh == real {
		t.Fatal("a resend must mint a new code")
	}
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": fresh}, 200)

	// Sign-in is throttled per account, right or wrong, whatever the caller's address.
	other := &client{t: t, base: h.srv.URL}
	for i := 0; i < 5; i++ {
		other.must("POST", "/v1/auth/login", map[string]any{"email": "lock@example.com", "password": "not-the-password"}, 401)
	}
	r = other.do("POST", "/v1/auth/login", map[string]any{"email": "lock@example.com", "password": "hunter2hunter2"})
	if r.status != 429 || str(r.body, "code") != "rate_limited" {
		t.Fatalf("sixth attempt within a minute should be refused: %d %s", r.status, r.raw)
	}
	// Another account is unaffected.
	other.must("POST", "/v1/auth/login", map[string]any{"email": "nobody@example.com", "password": "hunter2hunter2"}, 401)
}

// TestSessions is the cookie contract with the browser: what sign-in sets,
// what a dead cookie gets back, expiry that slides under a cap, the origin
// check on cookie-authenticated writes, and seeing and ending sessions.
func TestSessions(t *testing.T) {
	h := startApp(t)
	const email, password = "cookie@example.com", "hunter2hunter2"
	web := &client{t: t, base: h.srv.URL}

	// Sign-in sets two cookies: the httpOnly session and the readable flag,
	// expiring together.
	r := web.must("POST", "/v1/auth/register", map[string]any{"email": email, "password": password}, 201)
	web.signIn(r)
	sess, flag := web.cookie, web.flag
	if !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || sess.Path != "/" || sess.Expires.IsZero() || sess.Domain != "" {
		t.Fatalf("session cookie: %s", sess)
	}
	if flag.HttpOnly || flag.Value != "1" || flag.SameSite != http.SameSiteLaxMode || flag.Expires.Sub(sess.Expires).Abs() > time.Second {
		t.Fatalf("flag cookie: %s", flag)
	}
	web.must("POST", "/v1/auth/verify-email", map[string]any{"code": h.mailer.lastCode(t)}, 200)

	// A healthy session is left alone; a missing flag is put back on its own.
	if r = web.must("GET", "/v1/auth/me", nil, 200); len(r.setCookies) != 0 {
		t.Fatalf("a healthy session should not be rewritten: %v", r.setCookies)
	}
	web.flag = nil
	r = web.must("GET", "/v1/auth/me", nil, 200)
	if c := r.cookies["simhook_signed_in"]; c == nil || c.Value != "1" || len(r.setCookies) != 1 {
		t.Fatalf("a missing flag should come back alone: %v", r.setCookies)
	}
	web.flag = flag

	// The session slides: once under half of the idle window remains, both
	// cookies are re-issued with the new expiry and the same token.
	execSQL(t, `update sessions set expires_at = now() + interval '1 hour'`)
	r = web.must("GET", "/v1/auth/me", nil, 200)
	fresh := r.cookies["simhook_session"]
	if fresh == nil || fresh.Value != sess.Value || fresh.Expires.Before(time.Now().Add(29*24*time.Hour)) || r.cookies["simhook_signed_in"] == nil {
		t.Fatalf("sliding refresh: %v", r.setCookies)
	}
	if r = web.must("GET", "/v1/auth/me", nil, 200); len(r.setCookies) != 0 {
		t.Fatalf("a refreshed session should not be refreshed again at once: %v", r.setCookies)
	}
	// Never past the cap: an old session ends however active it is.
	execSQL(t, `update sessions set created_at = now() - interval '181 days'`)
	r = web.do("GET", "/v1/auth/me", nil)
	if r.status != 401 || str(r.body, "code") != "unauthenticated" || !cleared(r) {
		t.Fatalf("a capped session should end and clear the cookies: %d %s %v", r.status, r.raw, r.setCookies)
	}

	// Garbage in the cookie is not an error, but both cookies are cleared;
	// so is a flag with no session behind it, even on a public request.
	stale := &client{t: t, base: h.srv.URL, cookie: &http.Cookie{Name: "simhook_session", Value: "shs_nonsense"}}
	if r = stale.do("GET", "/v1/auth/me", nil); r.status != 401 || !cleared(r) {
		t.Fatalf("dead cookie: %d %v", r.status, r.setCookies)
	}
	flagOnly := &client{t: t, base: h.srv.URL, flag: &http.Cookie{Name: "simhook_signed_in", Value: "1"}}
	if r = flagOnly.must("GET", "/v1/plans", nil, 200); !cleared(r) {
		t.Fatalf("a stale flag should be cleared on any request: %v", r.setCookies)
	}
	// A dead cookie does not stop the browser signing in again, and sign-in
	// writes only its own cookies.
	r = stale.must("POST", "/v1/auth/login", map[string]any{"email": email, "password": password}, 200)
	if len(r.setCookies) != 2 {
		t.Fatalf("sign-in should set exactly the two cookies: %v", r.setCookies)
	}
	web.signIn(r)

	// A wrong current password is a 401, not the end of the session.
	r = web.do("POST", "/v1/auth/password", map[string]any{"current_password": "not-the-password", "new_password": "hunter3hunter3"})
	if r.status != 401 || str(r.body, "code") != "invalid_credentials" || len(r.setCookies) != 0 {
		t.Fatalf("wrong current password: %d %s %v", r.status, r.raw, r.setCookies)
	}
	web.must("GET", "/v1/auth/me", nil, 200)

	// Cookie-authenticated writes must come from our own pages.
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no source", nil, 201},
		{"the dashboard", map[string]string{"Origin": "http://localhost:3000"}, 201},
		{"a dashboard page as referer", map[string]string{"Referer": "http://localhost:3000/devices?x=1"}, 201},
		{"a foreign origin", map[string]string{"Origin": "https://evil.example"}, 403},
		{"a foreign referer", map[string]string{"Referer": "https://evil.example/"}, 403},
		{"an opaque origin", map[string]string{"Origin": "null"}, 403},
		{"a cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
	} {
		r = web.with(tc.headers).do("POST", "/v1/devices/pairing-codes", nil)
		if r.status != tc.want {
			t.Fatalf("%s: want %d, got %d %s", tc.name, tc.want, r.status, r.raw)
		}
		if tc.want == 403 && (str(r.body, "code") != "csrf_rejected" || len(r.setCookies) != 0) {
			t.Fatalf("%s: a refused write keeps its code and the session: %s %v", tc.name, r.raw, r.setCookies)
		}
	}
	web.with(map[string]string{"Origin": "https://evil.example"}).must("GET", "/v1/devices", nil, 200) // reads are not checked
	foreign := &client{t: t, base: h.srv.URL, headers: map[string]string{"Origin": "https://evil.example"}}
	if r = foreign.do("POST", "/v1/auth/login", map[string]any{"email": email, "password": password}); r.status != 403 {
		t.Fatalf("sign-in from a foreign page: %d %s", r.status, r.raw)
	}
	// Browsers never add an API key on their own, so a key is not checked.
	r = web.must("POST", "/v1/api-keys", map[string]any{"name": "csrf"}, 201)
	dev := &client{t: t, base: h.srv.URL, apiKey: str(r.body, "key"), headers: map[string]string{"Origin": "https://evil.example"}}
	dev.must("POST", "/v1/devices/pairing-codes", nil, 201)

	// Sessions can be seen and ended.
	other := h.login(t, email, password)
	r = web.must("GET", "/v1/auth/sessions", nil, 200)
	list := r.body["data"].([]any)
	if len(list) != 2 {
		t.Fatalf("two sessions: %s", r.raw)
	}
	var otherID string
	for _, item := range list {
		m := item.(map[string]any)
		if m["current"] == true {
			if m["ip"] == nil || m["user_agent"] == nil {
				t.Fatalf("a session knows where it came from: %v", m)
			}
			continue
		}
		otherID = m["id"].(string)
	}
	if otherID == "" {
		t.Fatalf("one session is current, the other is not: %s", r.raw)
	}
	web.must("DELETE", "/v1/auth/sessions/"+otherID, nil, 204)
	other.must("GET", "/v1/auth/me", nil, 401)
	web.must("DELETE", "/v1/auth/sessions/"+otherID, nil, 404)
	// Ending your own session clears the cookies.
	third := h.login(t, email, password)
	r = third.must("GET", "/v1/auth/sessions", nil, 200)
	var thirdID string
	for _, item := range r.body["data"].([]any) {
		if m := item.(map[string]any); m["current"] == true {
			thirdID = m["id"].(string)
		}
	}
	if r = third.must("DELETE", "/v1/auth/sessions/"+thirdID, nil, 204); !cleared(r) {
		t.Fatalf("ending the current session should clear the cookies: %v", r.setCookies)
	}
	third.must("GET", "/v1/auth/me", nil, 401)

	// Changing the password keeps this session and ends every other one.
	fourth := h.login(t, email, password)
	web.must("POST", "/v1/auth/password", map[string]any{"current_password": password, "new_password": "hunter3hunter3"}, 204)
	web.must("GET", "/v1/auth/me", nil, 200)
	fourth.must("GET", "/v1/auth/me", nil, 401)
	// So does "sign out everywhere else".
	fifth := h.login(t, email, "hunter3hunter3")
	web.must("POST", "/v1/auth/sessions/revoke-others", nil, 204)
	fifth.must("GET", "/v1/auth/me", nil, 401)
	web.must("GET", "/v1/auth/me", nil, 200)

	// A session cannot be found by an API key.
	dev.must("GET", "/v1/auth/sessions", nil, 401)

	// Sign-out clears both cookies, and works again on the dead cookie.
	if r = web.must("POST", "/v1/auth/logout", nil, 204); !cleared(r) {
		t.Fatalf("logout: %v", r.setCookies)
	}
	if r = web.must("POST", "/v1/auth/logout", nil, 204); !cleared(r) {
		t.Fatalf("logout with a dead cookie: %v", r.setCookies)
	}
	web.must("GET", "/v1/auth/me", nil, 401)
}
