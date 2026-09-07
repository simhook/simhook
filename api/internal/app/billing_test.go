package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/store"
)

// A secret in the shape Polar hands out: "whsec_" and base64.
const polarTestSecret = "whsec_MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func boolAt(m map[string]any, path ...string) bool {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		cur = mm[p]
	}
	b, _ := cur.(bool)
	return b
}

// polarDeliver posts one signed delivery the way Polar does.
func polarDeliver(t *testing.T, h *harness, secret, id string, event map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(event)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/v1/billing/webhooks/polar", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", ts)
	req.Header.Set("webhook-signature", billing.SignWebhook(secret, id, ts, body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// polarSubscription builds a subscription event as Polar sends it.
func polarSubscription(id, userID, productID, status string, modified time.Time, extra map[string]any) map[string]any {
	start := modified.Add(-24 * time.Hour)
	end := start.AddDate(0, 1, 0)
	data := map[string]any{
		"id": id, "created_at": start.Format(time.RFC3339), "modified_at": modified.Format(time.RFC3339Nano),
		"status": status, "recurring_interval": "month",
		"current_period_start": start.Format(time.RFC3339), "current_period_end": end.Format(time.RFC3339),
		"cancel_at_period_end": false, "canceled_at": nil, "ends_at": nil, "ended_at": nil,
		"customer_id": "cus_1", "product_id": productID, "amount": 1200, "currency": "usd",
		"metadata":       map[string]any{},
		"customer":       map[string]any{"id": "cus_1", "external_id": userID, "email": "buyer@example.com"},
		"pending_update": nil,
	}
	for k, v := range extra {
		data[k] = v
	}
	return map[string]any{"type": "subscription." + status, "timestamp": modified.Format(time.RFC3339), "data": data}
}

func seedProducts(t *testing.T, h *harness) {
	t.Helper()
	ctx := context.Background()
	for _, p := range []store.BillingProduct{
		{Provider: "polar", Environment: "sandbox", PlanID: "pro", Interval: "month", ProductID: "prod_pro_month", PriceID: "price_1", PriceCents: 1200},
		{Provider: "polar", Environment: "sandbox", PlanID: "scale", Interval: "year", ProductID: "prod_scale_year", PriceID: "price_2", PriceCents: 39000},
	} {
		if err := h.app.Store.UpsertBillingProduct(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBillingWebhooksDriveThePlan(t *testing.T) {
	t.Setenv("SIMHOOK_POLAR_ACCESS_TOKEN", "polar_oat_test")
	t.Setenv("SIMHOOK_POLAR_WEBHOOK_SECRET", polarTestSecret)
	h := startApp(t)
	seedProducts(t, h)
	c := h.signUp(t, "buyer@example.com")
	userID := str(c.must("GET", "/v1/auth/me", nil, 200).body, "user", "id")

	r := c.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "provider") != "polar" || !boolAt(r.body, "checkout", "available") || r.body["subscription"] != nil || boolAt(r.body, "portal_available") {
		t.Fatalf("fresh account: %v", r.body)
	}

	now := time.Now().UTC().Truncate(time.Second)
	active := polarSubscription("sub_1", userID, "prod_pro_month", "active", now, nil)

	// A delivery signed with another secret is refused and changes nothing.
	if status, body := polarDeliver(t, h, "whsec_bm90IHRoZSBzZWNyZXQgYXQgYWxsLCBub3QgZXZlbiBjbG9zZQ==", "wh_0", active); status != 403 || str(body, "code") != "invalid_signature" {
		t.Fatalf("forged delivery: %d %v", status, body)
	}
	if str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id") != "free" {
		t.Fatal("a forged delivery changed the plan")
	}

	if status, body := polarDeliver(t, h, polarTestSecret, "wh_1", active); status != 202 {
		t.Fatalf("delivery: %d %v", status, body)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "pro" {
		t.Fatalf("plan after subscription.active: %q", got)
	}
	r = c.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "subscription", "plan_id") != "pro" || str(r.body, "subscription", "interval") != "month" ||
		num(r.body, "subscription", "price_cents") != 1200 || !boolAt(r.body, "subscription", "managed") ||
		boolAt(r.body, "checkout", "available") || str(r.body, "checkout", "reason") != "subscribed" || !boolAt(r.body, "portal_available") {
		t.Fatalf("subscribed account: %v", r.body)
	}

	// The same delivery id again is acknowledged and ignored, whatever it says now.
	ended := now.Add(time.Minute)
	replay := polarSubscription("sub_1", userID, "prod_pro_month", "canceled", ended, map[string]any{"ended_at": ended.Format(time.RFC3339)})
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_1", replay); status != 202 {
		t.Fatalf("replay: %d", status)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "pro" {
		t.Fatalf("a replayed delivery changed the plan: %q", got)
	}

	// An older view arriving late is ignored too.
	stale := polarSubscription("sub_1", userID, "prod_pro_month", "canceled", now.Add(-time.Hour), map[string]any{"ended_at": now.Add(-time.Hour).Format(time.RFC3339)})
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_2", stale); status != 202 {
		t.Fatalf("stale: %d", status)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "pro" {
		t.Fatalf("a stale delivery changed the plan: %q", got)
	}

	// A product this environment does not sell is acknowledged and ignored.
	other := polarSubscription("sub_2", userID, "prod_from_elsewhere", "active", now.Add(2*time.Minute), nil)
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_3", other); status != 202 {
		t.Fatalf("unknown product: %d", status)
	}
	if got := str(c.must("GET", "/v1/billing", nil, 200).body, "subscription", "plan_id"); got != "pro" {
		t.Fatalf("an unknown product changed the subscription: %q", got)
	}

	// The cancellation, when it comes, ends the plan.
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_4", replay); status != 202 {
		t.Fatalf("cancel: %d", status)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "free" {
		t.Fatalf("plan after subscription.canceled: %q", got)
	}
	r = c.must("GET", "/v1/billing", nil, 200)
	if r.body["subscription"] != nil || !boolAt(r.body, "checkout", "available") || !boolAt(r.body, "portal_available") {
		t.Fatalf("after cancellation: %v", r.body)
	}

	// Nothing to manage on Free.
	for _, path := range []string{"/v1/billing/cancel", "/v1/billing/resume"} {
		if got := str(c.must("POST", path, nil, 409).body, "code"); got != "not_subscribed" {
			t.Fatalf("%s on Free: %q", path, got)
		}
	}
	if got := str(c.must("POST", "/v1/billing/change", map[string]any{"plan_id": "scale", "interval": "year"}, 409).body, "code"); got != "not_subscribed" {
		t.Fatalf("change on Free: %q", got)
	}
	// A plan that is not sold is refused before the provider is called.
	if got := str(c.must("POST", "/v1/billing/checkout", map[string]any{"plan_id": "pro", "interval": "year"}, 422).body, "code"); got != "validation_failed" {
		t.Fatalf("unsold plan: %q", got)
	}
	// Only a session may buy.
	key := str(c.must("POST", "/v1/api-keys", map[string]any{"name": "ci", "scopes": []string{"send", "read"}}, 201).body, "key")
	viaKey := &client{t: t, base: h.srv.URL, apiKey: key}
	viaKey.must("GET", "/v1/billing", nil, 401)
}

func TestBillingClosedWithoutProvider(t *testing.T) {
	h := startApp(t)
	c := h.signUp(t, "free@example.com")
	r := c.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "provider") != "" || boolAt(r.body, "checkout", "available") || str(r.body, "checkout", "reason") != "closed" {
		t.Fatalf("closed billing: %v", r.body)
	}
	if got := str(c.must("POST", "/v1/billing/checkout", map[string]any{"plan_id": "pro", "interval": "month"}, 403).body, "code"); got != "billing_closed" {
		t.Fatalf("checkout while closed: %q", got)
	}
	if got := str(c.must("POST", "/v1/billing/portal", nil, 403).body, "code"); got != "billing_closed" {
		t.Fatalf("portal while closed: %q", got)
	}
	// With no secret configured every delivery is refused.
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_1", polarSubscription("sub_1", "00000000-0000-0000-0000-000000000000", "prod_pro_month", "active", time.Now(), nil)); status != 403 {
		t.Fatalf("delivery without a secret: %d", status)
	}
}

func TestBillingAllowlistAndRegion(t *testing.T) {
	t.Setenv("SIMHOOK_POLAR_ACCESS_TOKEN", "polar_oat_test")
	t.Setenv("SIMHOOK_POLAR_WEBHOOK_SECRET", polarTestSecret)
	t.Setenv("SIMHOOK_BILLING_ALLOWLIST", "Owner@example.com")
	t.Setenv("SIMHOOK_BILLING_BLOCKED_COUNTRIES", "tr")
	t.Setenv("SIMHOOK_TRUST_PROXY", "true")
	h := startApp(t)
	seedProducts(t, h)

	stranger := h.signUp(t, "someone@example.com")
	r := stranger.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "provider") != "" || boolAt(r.body, "checkout", "available") || str(r.body, "checkout", "reason") != "closed" {
		t.Fatalf("account outside the allowlist: %v", r.body)
	}
	if got := str(stranger.must("POST", "/v1/billing/checkout", map[string]any{"plan_id": "pro", "interval": "month"}, 403).body, "code"); got != "billing_closed" {
		t.Fatalf("checkout outside the allowlist: %q", got)
	}

	owner := h.signUp(t, "owner@example.com")
	r = owner.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "provider") != "polar" || !boolAt(r.body, "checkout", "available") {
		t.Fatalf("allowlisted account: %v", r.body)
	}
	fromTurkey := owner.with(map[string]string{"CF-IPCountry": "TR"})
	r = fromTurkey.must("GET", "/v1/billing", nil, 200)
	if boolAt(r.body, "checkout", "available") || str(r.body, "checkout", "reason") != "region" {
		t.Fatalf("blocked country: %v", r.body)
	}
	if got := str(fromTurkey.must("POST", "/v1/billing/checkout", map[string]any{"plan_id": "pro", "interval": "month"}, 403).body, "code"); got != "billing_region" {
		t.Fatalf("checkout from a blocked country: %q", got)
	}
	if r = owner.with(map[string]string{"CF-IPCountry": "DE"}).must("GET", "/v1/billing", nil, 200); !boolAt(r.body, "checkout", "available") {
		t.Fatalf("other country: %v", r.body)
	}
}

// TestPendingPaymentBlocksASecondCheckout: while the bank is still
// confirming the first payment the subscription is incomplete. It grants
// nothing yet, and it must not be bought again from the same account.
func TestPendingPaymentBlocksASecondCheckout(t *testing.T) {
	t.Setenv("SIMHOOK_POLAR_ACCESS_TOKEN", "polar_oat_test")
	t.Setenv("SIMHOOK_POLAR_WEBHOOK_SECRET", polarTestSecret)
	h := startApp(t)
	seedProducts(t, h)
	c := h.signUp(t, "pending@example.com")
	userID := str(c.must("GET", "/v1/auth/me", nil, 200).body, "user", "id")
	now := time.Now().UTC().Truncate(time.Second)

	pending := polarSubscription("sub_p", userID, "prod_pro_month", "incomplete", now, nil)
	if status, body := polarDeliver(t, h, polarTestSecret, "wh_p1", pending); status != 202 {
		t.Fatalf("pending delivery: %d %v", status, body)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "free" {
		t.Fatalf("an incomplete subscription grants nothing: %q", got)
	}
	r := c.must("GET", "/v1/billing", nil, 200)
	if str(r.body, "subscription", "status") != "incomplete" || boolAt(r.body, "checkout", "available") || str(r.body, "checkout", "reason") != "subscribed" {
		t.Fatalf("pending payment: %v", r.body)
	}
	if got := str(c.must("POST", "/v1/billing/checkout", map[string]any{"plan_id": "pro", "interval": "month"}, 409).body, "code"); got != "already_subscribed" {
		t.Fatalf("a second checkout during a pending payment: %q", got)
	}

	// The bank confirms: the plan applies.
	active := polarSubscription("sub_p", userID, "prod_pro_month", "active", now.Add(time.Minute), nil)
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_p2", active); status != 202 {
		t.Fatalf("activation: %d", status)
	}
	if got := str(c.must("GET", "/v1/auth/me", nil, 200).body, "limits", "plan_id"); got != "pro" {
		t.Fatalf("plan after the payment settles: %q", got)
	}

	// Or it never does: the subscription expires and the account may buy again.
	d := h.signUp(t, "declined@example.com")
	dID := str(d.must("GET", "/v1/auth/me", nil, 200).body, "user", "id")
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_d1", polarSubscription("sub_d", dID, "prod_pro_month", "incomplete", now, nil)); status != 202 {
		t.Fatalf("pending delivery: %d", status)
	}
	if boolAt(d.must("GET", "/v1/billing", nil, 200).body, "checkout", "available") {
		t.Fatal("a pending payment must not be bought twice")
	}
	expired := polarSubscription("sub_d", dID, "prod_pro_month", "incomplete_expired", now.Add(time.Minute), map[string]any{"ended_at": now.Add(time.Minute).Format(time.RFC3339)})
	if status, _ := polarDeliver(t, h, polarTestSecret, "wh_d2", expired); status != 202 {
		t.Fatalf("expiry: %d", status)
	}
	if r = d.must("GET", "/v1/billing", nil, 200); r.body["subscription"] != nil || !boolAt(r.body, "checkout", "available") {
		t.Fatalf("after the payment failed for good: %v", r.body)
	}
}
