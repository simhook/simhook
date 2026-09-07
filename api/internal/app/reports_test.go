package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// TestStaleSweepTrustsAWorkingPhone: a phone that is still reporting is
// behind the model, not gone, so its overdue messages are left alone until
// a day has passed. A phone that has fallen silent gets no such credit.
func TestStaleSweepTrustsAWorkingPhone(t *testing.T) {
	h := startApp(t)
	web := h.signUp(t, "working@example.com")
	phone, deviceID := h.pairPhone(t, web, "hw-working-0001", "tok-working")
	ctx := context.Background()

	r := web.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155550401", "+14155550402", "+14155550403"}, "body": "behind"}, 202)
	raw := r.body["message_ids"].([]any)
	id1, id2, id3 := uuid.MustParse(raw[0].(string)), uuid.MustParse(raw[1].(string)), uuid.MustParse(raw[2].(string))
	waitFor(t, "wake-up push", func() bool { return len(h.pusher.sends()) == 1 })
	if r = phone.must("GET", "/v1/device/outbox", nil, 200); len(r.body["data"].([]any)) != 3 {
		t.Fatalf("outbox: %s", r.raw)
	}

	// The phone is slower than the model: every message is overdue, but it
	// has just reported on the first.
	execSQL(t, `update messages set dispatched_at = now() - interval '1 hour', expected_send_at = now() - interval '1 hour' where id = any($1::uuid[])`, []uuid.UUID{id1, id2, id3})
	phone.must("POST", "/v1/device/messages/"+id1.String()+"/status", map[string]any{"status": "sent"}, 200)
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, id := range []uuid.UUID{id2, id3} {
		if r = web.must("GET", "/v1/messages/"+id.String(), nil, 200); str(r.body, "message", "status") != "dispatched" {
			t.Fatalf("a phone that is reporting is working, not gone: %s", r.raw)
		}
	}

	// A day without a report on a fetched message is given up on, however
	// busy the phone.
	execSQL(t, `update messages set dispatched_at = now() - interval '25 hours', expected_send_at = now() - interval '25 hours' where id = $1`, id3)
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r = web.must("GET", "/v1/messages/"+id3.String(), nil, 200); str(r.body, "message", "status") != "unknown" {
		t.Fatalf("a day is the limit: %s", r.raw)
	}
	if r = web.must("GET", "/v1/messages/"+id2.String(), nil, 200); str(r.body, "message", "status") != "dispatched" {
		t.Fatalf("the newer message still waits for the working phone: %s", r.raw)
	}

	// Once the phone falls silent, the overdue message is swept.
	execSQL(t, `update devices set last_report_at = now() - interval '1 hour' where id = $1`, uuid.MustParse(deviceID))
	if err := h.app.Gateway.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r = web.must("GET", "/v1/messages/"+id2.String(), nil, 200); str(r.body, "message", "status") != "unknown" {
		t.Fatalf("a silent phone's overdue message is unknown: %s", r.raw)
	}
}

// TestRacingReportsKeepTheCountersStraight: a sent and a delivered report
// for the same message arriving together must not both read the same
// starting status, or the batch counters drift for good.
func TestRacingReportsKeepTheCountersStraight(t *testing.T) {
	h := startApp(t)
	web := h.signUp(t, "racing@example.com")
	phone, _ := h.pairPhone(t, web, "hw-racing-0001", "tok-racing")

	const n = 12
	to := make([]string, n)
	for i := range to {
		to[i] = fmt.Sprintf("+1415555%04d", 500+i)
	}
	r := web.must("POST", "/v1/messages", map[string]any{"to": to, "body": "racing"}, 202)
	batchID := str(r.body, "batch", "id")
	ids := r.body["message_ids"].([]any)
	waitFor(t, "wake-up push", func() bool { return len(h.pusher.sends()) == 1 })
	if r = phone.must("GET", "/v1/device/outbox", nil, 200); len(r.body["data"].([]any)) != n {
		t.Fatalf("outbox: %s", r.raw)
	}

	var wg sync.WaitGroup
	for _, raw := range ids {
		id := raw.(string)
		for _, status := range []string{"sent", "delivered"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				body, _ := json.Marshal(map[string]any{"status": status})
				req, _ := http.NewRequest("POST", h.srv.URL+"/v1/device/messages/"+id+"/status", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+phone.bearer)
				if res, err := http.DefaultClient.Do(req); err == nil {
					res.Body.Close()
				}
			}()
		}
	}
	wg.Wait()

	// Whichever report landed first, every message ends delivered and the
	// batch counts each exactly once.
	r = web.must("GET", "/v1/batches/"+batchID, nil, 200)
	if num(r.body, "batch", "dispatched_count") != 0 || num(r.body, "batch", "sent_count") != 0 ||
		num(r.body, "batch", "delivered_count") != n || str(r.body, "batch", "status") != "completed" {
		t.Fatalf("counters after racing reports: %s", r.raw)
	}
}
