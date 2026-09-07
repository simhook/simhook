package billing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/simhook/simhook/internal/store"
)

// webhookEvents are the deliveries the API needs: every subscription
// change. Orders, refunds, and benefits are Polar's business.
var webhookEvents = []string{
	"subscription.created", "subscription.updated", "subscription.active", "subscription.canceled",
	"subscription.uncanceled", "subscription.cycled", "subscription.revoked", "subscription.past_due",
	"subscription.paused", "subscription.resumed",
}

// SyncedProduct is one plan and interval after a sync.
type SyncedProduct struct {
	PlanID     string
	Interval   string
	ProductID  string
	PriceCents int32
	Action     string // created, updated, unchanged
}

// SyncReport is what `simhook billing sync` did.
type SyncReport struct {
	Environment    string
	Products       []SyncedProduct
	Endpoint       PolarWebhookEndpoint
	EndpointAction string // created, updated, unchanged
}

// Sync makes the provider match the plans table: one product per paid
// plan and interval, found by metadata so a rerun changes nothing, and
// one webhook endpoint at the given address. It is safe to run again at
// any time, against either environment.
func (s *Service) Sync(ctx context.Context, webhookURL string) (SyncReport, error) {
	if s.polar == nil {
		return SyncReport{}, errors.New("SIMHOOK_POLAR_ACCESS_TOKEN is not set")
	}
	rep := SyncReport{Environment: s.cfg.PolarEnvironment}
	plans, err := s.st.ListPlans(ctx)
	if err != nil {
		return rep, err
	}
	for _, plan := range plans {
		for _, interval := range []string{"month", "year"} {
			cents := priceFor(plan, interval)
			if cents <= 0 {
				continue
			}
			sp, err := s.syncProduct(ctx, plan, interval, cents)
			if err != nil {
				return rep, fmt.Errorf("%s %s: %w", plan.ID, interval, err)
			}
			rep.Products = append(rep.Products, sp)
		}
	}
	ep, action, err := s.syncEndpoint(ctx, webhookURL)
	if err != nil {
		return rep, fmt.Errorf("webhook endpoint: %w", err)
	}
	rep.Endpoint, rep.EndpointAction = ep, action
	return rep, nil
}

func productName(plan store.Plan, interval string) string {
	if interval == "year" {
		return plan.Name + " (yearly)"
	}
	return plan.Name + " (monthly)"
}

func productDescription(plan store.Plan) string {
	count := func(n int32) string {
		if n < 0 {
			return "unlimited"
		}
		return fmt.Sprintf("%d", n)
	}
	phones := "phones"
	if plan.DeviceLimit == 1 {
		phones = "phone"
	}
	return fmt.Sprintf("simhook %s: %s messages a month, %s %s, %s recipients per send.",
		plan.Name, count(plan.MonthlyLimit), count(plan.DeviceLimit), phones, count(plan.BatchLimit))
}

func (s *Service) syncProduct(ctx context.Context, plan store.Plan, interval string, cents int32) (SyncedProduct, error) {
	meta := map[string]string{"simhook_plan": plan.ID, "simhook_interval": interval}
	name, desc := productName(plan, interval), productDescription(plan)
	found, err := s.polar.ListProducts(ctx, meta)
	if err != nil {
		return SyncedProduct{}, err
	}
	var product PolarProduct
	action := "unchanged"
	switch {
	case len(found) == 0:
		product, err = s.polar.CreateProduct(ctx, ProductCreate{
			Name: name, Description: desc, RecurringInterval: interval, Visibility: "private", Metadata: meta,
			Prices: []PriceCreate{{AmountType: "fixed", PriceAmount: int64(cents), PriceCurrency: "usd"}},
		})
		if err != nil {
			return SyncedProduct{}, err
		}
		action = "created"
	default:
		product = found[0]
		patch := map[string]any{}
		if product.Name != name {
			patch["name"] = name
		}
		if product.Description == nil || *product.Description != desc {
			patch["description"] = desc
		}
		if price, ok := product.ActivePrice(); !ok || price.PriceAmount != int64(cents) || price.PriceCurrency != "usd" {
			patch["prices"] = []PriceCreate{{AmountType: "fixed", PriceAmount: int64(cents), PriceCurrency: "usd"}}
		}
		if len(patch) > 0 {
			product, err = s.polar.UpdateProduct(ctx, product.ID, patch)
			if err != nil {
				return SyncedProduct{}, err
			}
			action = "updated"
		}
	}
	price, ok := product.ActivePrice()
	if !ok {
		return SyncedProduct{}, fmt.Errorf("product %s has no live fixed price", product.ID)
	}
	if err := s.st.UpsertBillingProduct(ctx, store.BillingProduct{
		Provider: ProviderPolar, Environment: s.cfg.PolarEnvironment, PlanID: plan.ID, Interval: interval,
		ProductID: product.ID, PriceID: price.ID, PriceCents: int32(price.PriceAmount),
	}); err != nil {
		return SyncedProduct{}, err
	}
	return SyncedProduct{PlanID: plan.ID, Interval: interval, ProductID: product.ID, PriceCents: int32(price.PriceAmount), Action: action}, nil
}

func (s *Service) syncEndpoint(ctx context.Context, address string) (PolarWebhookEndpoint, string, error) {
	endpoints, err := s.polar.ListWebhookEndpoints(ctx)
	if err != nil {
		return PolarWebhookEndpoint{}, "", err
	}
	for _, ep := range endpoints {
		if strings.TrimRight(ep.URL, "/") != strings.TrimRight(address, "/") {
			continue
		}
		patch := map[string]any{}
		if !ep.Enabled {
			patch["enabled"] = true
		}
		missing := false
		for _, ev := range webhookEvents {
			if !slices.Contains(ep.Events, ev) {
				missing = true
			}
		}
		if missing || ep.Format != "raw" {
			events := append([]string{}, ep.Events...)
			for _, ev := range webhookEvents {
				if !slices.Contains(events, ev) {
					events = append(events, ev)
				}
			}
			patch["events"] = events
			patch["format"] = "raw"
		}
		if len(patch) == 0 {
			return ep, "unchanged", nil
		}
		updated, err := s.polar.UpdateWebhookEndpoint(ctx, ep.ID, patch)
		if err != nil {
			return ep, "", err
		}
		if updated.Secret == "" {
			updated.Secret = ep.Secret
		}
		return updated, "updated", nil
	}
	created, err := s.polar.CreateWebhookEndpoint(ctx, address, "simhook API", webhookEvents)
	if err != nil {
		return PolarWebhookEndpoint{}, "", err
	}
	return created, "created", nil
}

// Report describes the setup for `simhook billing status`.
type Report struct {
	Environment  string
	TokenSet     bool
	SecretSet    bool
	Products     []store.BillingProduct
	Endpoints    []PolarWebhookEndpoint
	Allowlist    []string
	BlockedCodes []string
}

// Report reads the setup without changing anything.
func (s *Service) Report(ctx context.Context) (Report, error) {
	r := Report{Environment: s.cfg.PolarEnvironment, TokenSet: s.polar != nil, SecretSet: s.cfg.PolarWebhookSecret != "",
		Allowlist: s.cfg.BillingAllowlist, BlockedCodes: s.cfg.BillingBlockedCountries}
	products, err := s.st.ListBillingProducts(ctx, ProviderPolar, s.cfg.PolarEnvironment)
	if err != nil {
		return r, err
	}
	r.Products = products
	if s.polar != nil {
		eps, err := s.polar.ListWebhookEndpoints(ctx)
		if err != nil {
			return r, err
		}
		r.Endpoints = eps
	}
	return r, nil
}
