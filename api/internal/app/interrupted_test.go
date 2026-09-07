package app_test

import (
	"testing"

	"github.com/google/uuid"
)

// TestALateResultOverturnsAnInterruptedFailure: a phone that handed a
// message to the radio and never heard back reports it failed, honestly
// labelled; when the radio's answer comes after all, the truth wins. A
// failure with any other label is final.
func TestALateResultOverturnsAnInterruptedFailure(t *testing.T) {
	h := startApp(t)
	web := h.signUp(t, "interrupted@example.com")
	phone, _ := h.pairPhone(t, web, "hw-interrupted-0001", "tok-interrupted")

	r := web.must("POST", "/v1/messages", map[string]any{"to": []string{"+14155550601", "+14155550602"}, "body": "quiet radio"}, 202)
	batchID := str(r.body, "batch", "id")
	raw := r.body["message_ids"].([]any)
	id1, id2 := uuid.MustParse(raw[0].(string)), uuid.MustParse(raw[1].(string))
	waitFor(t, "wake-up push", func() bool { return len(h.pusher.sends()) == 1 })
	phone.must("GET", "/v1/device/outbox", nil, 200)

	report := func(id uuid.UUID, body map[string]any) resp {
		return phone.must("POST", "/v1/device/messages/"+id.String()+"/status", body, 200)
	}
	if r = report(id1, map[string]any{"status": "failed", "error_code": "interrupted", "error_message": "No answer from the radio."}); str(r.body, "message", "status") != "failed" {
		t.Fatalf("interrupted report: %s", r.raw)
	}
	if r = report(id1, map[string]any{"status": "sent"}); str(r.body, "message", "status") != "sent" || r.body["message"].(map[string]any)["error_code"] != nil {
		t.Fatalf("a late sent must overturn an interrupted failure and clear its error: %s", r.raw)
	}
	if r = report(id1, map[string]any{"status": "delivered"}); str(r.body, "message", "status") != "delivered" {
		t.Fatalf("delivered after the late sent: %s", r.raw)
	}

	// A refusal the carrier actually gave stays a failure.
	report(id2, map[string]any{"status": "failed", "error_code": "generic_failure", "error_message": "The carrier refused it."})
	if r = report(id2, map[string]any{"status": "sent"}); str(r.body, "message", "status") != "failed" {
		t.Fatalf("a real failure is final: %s", r.raw)
	}

	r = web.must("GET", "/v1/batches/"+batchID, nil, 200)
	if num(r.body, "batch", "delivered_count") != 1 || num(r.body, "batch", "failed_count") != 1 || num(r.body, "batch", "sent_count") != 0 || str(r.body, "batch", "status") != "partial" {
		t.Fatalf("batch counters after the overturned failure: %s", r.raw)
	}
}
