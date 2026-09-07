package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simhook/simhook/internal/db"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/testutil"
)

// fakePolar answers the two calls a return from checkout makes: the
// checkout by id, and the account's subscriptions. Like the real one, the
// checkout does not name the subscription it produced; the subscription
// names the checkout.
func fakePolar(t *testing.T, userID uuid.UUID, checkoutID, productID string) *httptest.Server {
	t.Helper()
	now := time.Now().UTC()
	sub := map[string]any{
		"id": "sub_from_checkout", "created_at": now, "modified_at": now, "status": "active", "recurring_interval": "month",
		"current_period_start": now, "current_period_end": now.AddDate(0, 1, 0), "cancel_at_period_end": false,
		"customer_id": "cus_1", "product_id": productID, "checkout_id": checkoutID, "amount": 3900, "currency": "usd",
		"metadata": map[string]any{}, "customer": map[string]any{"id": "cus_1", "external_id": userID.String()},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/checkouts/"+checkoutID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": checkoutID, "status": "succeeded", "url": "https://sandbox.polar.sh/checkout/x",
				"customer_id": "cus_1", "external_customer_id": userID.String(), "subscription_id": nil, "product_id": productID,
			})
		case r.URL.Path == "/v1/checkouts/ck_someone_elses":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ck_someone_elses", "status": "succeeded", "external_customer_id": uuid.NewString()})
		case r.URL.Path == "/v1/subscriptions/":
			if r.URL.Query().Get("external_customer_id") != userID.String() {
				t.Errorf("subscriptions listed for %q", r.URL.Query().Get("external_customer_id"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{sub}, "pagination": map[string]any{"total_count": 1, "max_page": 1}})
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCheckoutStateFindsTheSubscriptionByCheckout(t *testing.T) {
	cfg := testutil.Config(t)
	testutil.Reset(t)
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	st := store.New(pool)
	user, err := st.CreateUser(ctx, store.CreateUserParams{ID: uuid.New(), Email: "buyer@example.com", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertBillingProduct(ctx, store.BillingProduct{Provider: ProviderPolar, Environment: "sandbox", PlanID: "scale", Interval: "month", ProductID: "prod_scale_month", PriceID: "price_1", PriceCents: 3900}); err != nil {
		t.Fatal(err)
	}
	polar := fakePolar(t, user.ID, "ck_1", "prod_scale_month")
	defer polar.Close()
	cfg.PolarWebhookSecret = "whsec_dGVzdA=="
	svc := New(st, cfg, nil)
	svc.polar = &Polar{base: strings.TrimRight(polar.URL, "/"), token: "test-token", http: polar.Client()}

	res, err := svc.CheckoutState(ctx, user, "ck_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "succeeded" || !res.Applied {
		t.Fatalf("checkout state: %+v", res)
	}
	limits, err := st.EffectiveLimits(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if limits.PlanID != "scale" {
		t.Fatalf("plan after return: %q", limits.PlanID)
	}
	live, err := st.GetLiveSubscription(ctx, user.ID)
	if err != nil || live.ProviderSubscriptionID == nil || *live.ProviderSubscriptionID != "sub_from_checkout" {
		t.Fatalf("live subscription: %+v %v", live, err)
	}

	// Asking again is idempotent and still says applied.
	if res, err = svc.CheckoutState(ctx, user, "ck_1"); err != nil || !res.Applied {
		t.Fatalf("second read: %+v %v", res, err)
	}

	// Another account's checkout is not this account's business.
	if _, err := svc.CheckoutState(ctx, user, "ck_someone_elses"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign checkout: %v", err)
	}
}
