package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simhook/simhook/internal/store"
)

// ProviderPolar names the provider in the subscriptions table.
const ProviderPolar = "polar"

// Errors the API maps to responses.
var (
	ErrClosed        = errors.New("paid plans are not open yet")
	ErrRegion        = errors.New("paid plans are not offered where you are")
	ErrUnverified    = errors.New("verify your email address before choosing a paid plan")
	ErrSubscribed    = errors.New("this account already has a subscription; change it instead")
	ErrNotSubscribed = errors.New("this account has no subscription to change")
	ErrNoPlan        = errors.New("no such paid plan")
	ErrNoCustomer    = errors.New("this account has not bought anything yet")
	ErrProvider      = errors.New("the payment provider could not be reached")
)

// Checkout reasons, for the dashboard to explain a missing button.
const (
	ReasonClosed      = "closed"
	ReasonRegion      = "region"
	ReasonVerifyEmail = "verify_email"
	ReasonSubscribed  = "subscribed"
)

// BillingStatus is what the dashboard shows about billing.
type BillingStatus struct {
	Provider     string        `json:"provider" doc:"Payment provider, or empty while paid plans are closed for this account."`
	Subscription *Subscription `json:"subscription" doc:"The live subscription, or null on Free."`
	Checkout     CheckoutState `json:"checkout"`
	Portal       bool          `json:"portal_available" doc:"Whether the provider's portal (invoices, payment method) can be opened for this account."`
}

// CheckoutState says whether a paid plan can be bought right now.
type CheckoutState struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty" enum:"closed,region,verify_email,subscribed" doc:"Why not, when it cannot: paid plans are closed, not offered in the visitor's country, the email is unverified, or the account already has a subscription to change instead."`
}

// Subscription is the live subscription as the dashboard sees it.
type Subscription struct {
	PlanID            string         `json:"plan_id"`
	PlanName          string         `json:"plan_name"`
	Status            string         `json:"status" doc:"active, trialing, past_due, canceled, incomplete, unpaid, or paused, as the provider reports it."`
	Interval          string         `json:"interval,omitempty" enum:"month,year"`
	PriceCents        int32          `json:"price_cents" doc:"What the current period costs, in US cents."`
	CurrentPeriodEnd  *time.Time     `json:"current_period_end" doc:"When the paid period ends; the renewal date unless the subscription is set to cancel."`
	CancelAtPeriodEnd bool           `json:"cancel_at_period_end" doc:"True when the subscription ends at the period end instead of renewing."`
	Managed           bool           `json:"managed" doc:"True when the provider runs it; false for a plan granted by hand."`
	Pending           *PendingChange `json:"pending" doc:"A plan change the provider applies at the next period, or null."`
}

// PendingChange is a scheduled plan change.
type PendingChange struct {
	PlanID   string    `json:"plan_id"`
	PlanName string    `json:"plan_name"`
	Interval string    `json:"interval" enum:"month,year"`
	At       time.Time `json:"at"`
}

// CheckoutResult is what the dashboard learns when it returns from a
// checkout page.
type CheckoutResult struct {
	Status  string `json:"status" enum:"open,expired,confirmed,succeeded,failed" doc:"The checkout's state at the provider."`
	Applied bool   `json:"applied" doc:"True once the subscription it produced is on the account."`
}

// Enabled reports whether paid plans can be sold at all: a provider,
// its webhook secret, and synced products for this environment.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.polar == nil || s.cfg == nil || s.cfg.PolarWebhookSecret == "" {
		return false
	}
	products, err := s.st.ListBillingProducts(ctx, ProviderPolar, s.cfg.PolarEnvironment)
	return err == nil && len(products) > 0
}

type catalogue struct {
	byProduct map[string]store.BillingProduct
	byPlan    map[string]store.BillingProduct // "pro/month"
}

func planKey(planID, interval string) string { return planID + "/" + interval }

func (s *Service) catalogue(ctx context.Context, st *store.Store) (catalogue, error) {
	env := ""
	if s.cfg != nil {
		env = s.cfg.PolarEnvironment
	}
	products, err := st.ListBillingProducts(ctx, ProviderPolar, env)
	if err != nil {
		return catalogue{}, err
	}
	c := catalogue{byProduct: map[string]store.BillingProduct{}, byPlan: map[string]store.BillingProduct{}}
	for _, p := range products {
		c.byProduct[p.ProductID] = p
		c.byPlan[planKey(p.PlanID, p.Interval)] = p
	}
	return c, nil
}

// checkoutReason says why the user cannot buy a plan now, or "" when they can.
func (s *Service) checkoutReason(ctx context.Context, user store.User, country string, live *store.Subscription) string {
	if !s.Enabled(ctx) || !s.cfg.BillingAllowed(user.Email) {
		return ReasonClosed
	}
	if s.cfg.CountryBlocked(country) {
		return ReasonRegion
	}
	if s.cfg.RequireEmailVerification && user.EmailVerifiedAt == nil {
		return ReasonVerifyEmail
	}
	if live != nil && live.Provider != nil && store.SubscriptionLive(live.Status) {
		return ReasonSubscribed
	}
	return ""
}

func (s *Service) liveSubscription(ctx context.Context, userID uuid.UUID) (*store.Subscription, error) {
	sub, err := s.st.GetLiveSubscription(ctx, userID)
	switch {
	case err == nil:
		return &sub, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, nil
	default:
		return nil, err
	}
}

func (s *Service) view(ctx context.Context, sub *store.Subscription) (*Subscription, error) {
	if sub == nil {
		return nil, nil
	}
	plan, err := s.st.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return nil, err
	}
	v := &Subscription{
		PlanID: plan.ID, PlanName: plan.Name, Status: sub.Status,
		CurrentPeriodEnd: sub.CurrentPeriodEnd, CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
		Managed: sub.Provider != nil,
	}
	if sub.BillingInterval != nil {
		v.Interval = *sub.BillingInterval
		v.PriceCents = priceFor(plan, v.Interval)
	}
	if sub.PendingPlanID != nil && sub.PendingInterval != nil && sub.PendingAt != nil {
		if p, err := s.st.GetPlan(ctx, *sub.PendingPlanID); err == nil {
			v.Pending = &PendingChange{PlanID: p.ID, PlanName: p.Name, Interval: *sub.PendingInterval, At: *sub.PendingAt}
		}
	}
	return v, nil
}

func priceFor(plan store.Plan, interval string) int32 {
	if interval == "year" {
		return plan.YearlyPriceCents
	}
	return plan.MonthlyPriceCents
}

// Status describes billing for one account. country is the visitor's
// country as the edge reports it, or empty.
func (s *Service) Status(ctx context.Context, user store.User, country string) (BillingStatus, error) {
	live, err := s.liveSubscription(ctx, user.ID)
	if err != nil {
		return BillingStatus{}, err
	}
	view, err := s.view(ctx, live)
	if err != nil {
		return BillingStatus{}, err
	}
	st := BillingStatus{Subscription: view}
	reason := s.checkoutReason(ctx, user, country, live)
	st.Checkout = CheckoutState{Available: reason == "", Reason: reason}
	managed := live != nil && live.Provider != nil
	if s.polar != nil && (reason != ReasonClosed || managed) {
		st.Provider = ProviderPolar
	}
	if s.polar != nil {
		has, err := s.st.HasProviderCustomer(ctx, user.ID, ProviderPolar)
		if err != nil {
			return BillingStatus{}, err
		}
		st.Portal = has
	}
	return st, nil
}

func reasonError(reason string) error {
	switch reason {
	case ReasonRegion:
		return ErrRegion
	case ReasonVerifyEmail:
		return ErrUnverified
	case ReasonSubscribed:
		return ErrSubscribed
	default:
		return ErrClosed
	}
}

func providerErr(err error) error {
	var pe *PolarError
	if errors.As(err, &pe) && pe.Status == 404 {
		return fmt.Errorf("%w: %v", store.ErrNotFound, err)
	}
	return fmt.Errorf("%w: %v", ErrProvider, err)
}

// Checkout opens a hosted checkout for a plan and interval and returns
// the page to send the browser to. The account is bound to the checkout
// as the external customer id, so the subscription it produces lands on
// this account whatever email the customer types.
func (s *Service) Checkout(ctx context.Context, user store.User, planID, interval, country, ip string) (string, error) {
	live, err := s.liveSubscription(ctx, user.ID)
	if err != nil {
		return "", err
	}
	if reason := s.checkoutReason(ctx, user, country, live); reason != "" {
		return "", reasonError(reason)
	}
	cat, err := s.catalogue(ctx, s.st)
	if err != nil {
		return "", err
	}
	product, ok := cat.byPlan[planKey(planID, interval)]
	if !ok {
		return "", ErrNoPlan
	}
	name := ""
	if user.Name != nil {
		name = *user.Name
	}
	id := user.ID.String()
	ck, err := s.polar.CreateCheckout(ctx, CheckoutCreate{
		Products:           []string{product.ProductID},
		ExternalCustomerID: id,
		CustomerEmail:      user.Email,
		CustomerName:       name,
		CustomerIPAddress:  ip,
		SuccessURL:         s.cfg.WebURL + "/billing?checkout={CHECKOUT_ID}",
		ReturnURL:          s.cfg.WebURL + "/billing",
		Metadata:           map[string]string{"simhook_user_id": id, "simhook_plan": planID, "simhook_interval": interval},
	})
	if err != nil {
		s.log.ErrorContext(ctx, "checkout failed", "user", id, "plan", planID, "err", err)
		return "", providerErr(err)
	}
	s.log.InfoContext(ctx, "checkout opened", "user", id, "plan", planID, "interval", interval, "checkout", ck.ID)
	return ck.URL, nil
}

// CheckoutState reads a checkout the browser came back from. When it
// succeeded, the subscription it produced is fetched and applied, so the
// dashboard does not have to wait for the webhook.
func (s *Service) CheckoutState(ctx context.Context, user store.User, checkoutID string) (CheckoutResult, error) {
	if s.polar == nil {
		return CheckoutResult{}, ErrClosed
	}
	ck, err := s.polar.GetCheckout(ctx, checkoutID)
	if err != nil {
		return CheckoutResult{}, providerErr(err)
	}
	if ck.ExternalCustomerID == nil || *ck.ExternalCustomerID != user.ID.String() {
		return CheckoutResult{}, store.ErrNotFound
	}
	out := CheckoutResult{Status: ck.Status}
	if ck.Status != "succeeded" {
		return out, nil
	}
	sub, err := s.subscriptionFromCheckout(ctx, user, ck)
	if err != nil {
		return out, err
	}
	if sub != nil {
		if err := s.applySubscription(ctx, *sub); err != nil {
			return out, err
		}
	}
	live, err := s.liveSubscription(ctx, user.ID)
	if err != nil {
		return out, err
	}
	managed := live != nil && live.Provider != nil && store.SubscriptionLive(live.Status)
	out.Applied = managed && (sub == nil || (live.ProviderSubscriptionID != nil && *live.ProviderSubscriptionID == sub.ID))
	return out, nil
}

// subscriptionFromCheckout finds the subscription a paid checkout
// produced. Polar does not write it on the checkout (that field names a
// subscription a checkout upgrades), but every subscription names the
// checkout that created it, so the account's subscriptions are listed
// and matched. Nil when Polar has not created it yet.
func (s *Service) subscriptionFromCheckout(ctx context.Context, user store.User, ck PolarCheckout) (*PolarSubscription, error) {
	if ck.SubscriptionID != nil {
		sub, err := s.polar.GetSubscription(ctx, *ck.SubscriptionID)
		if err != nil {
			return nil, providerErr(err)
		}
		return &sub, nil
	}
	subs, err := s.polar.ListSubscriptions(ctx, user.ID.String())
	if err != nil {
		return nil, providerErr(err)
	}
	for i := range subs {
		if subs[i].CheckoutID != nil && *subs[i].CheckoutID == ck.ID {
			return &subs[i], nil
		}
	}
	return nil, nil
}

// ChangePlan moves a live subscription to another plan or interval. A
// change that costs more per month is invoiced now and takes effect now;
// one that costs less takes effect at the next period, with nothing
// refunded, so a month already paid for is kept.
func (s *Service) ChangePlan(ctx context.Context, user store.User, planID, interval string) (*Subscription, error) {
	live, err := s.managed(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	cat, err := s.catalogue(ctx, s.st)
	if err != nil {
		return nil, err
	}
	product, ok := cat.byPlan[planKey(planID, interval)]
	if !ok {
		return nil, ErrNoPlan
	}
	current := ""
	if live.BillingInterval != nil {
		current = *live.BillingInterval
	}
	if live.PlanID == planID && current == interval {
		return s.view(ctx, live)
	}
	from, err := s.st.GetPlan(ctx, live.PlanID)
	if err != nil {
		return nil, err
	}
	to, err := s.st.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	proration := "invoice"
	if perMonth(to, interval) < perMonth(from, current) {
		proration = "next_period"
	}
	sub, err := s.polar.UpdateSubscription(ctx, *live.ProviderSubscriptionID, map[string]any{
		"product_id": product.ProductID, "proration_behavior": proration,
	})
	if err != nil {
		s.log.ErrorContext(ctx, "plan change failed", "user", user.ID, "plan", planID, "err", err)
		return nil, providerErr(err)
	}
	return s.applyAndView(ctx, user.ID, sub)
}

// perMonth is a plan's price per month at the interval, in cents.
func perMonth(plan store.Plan, interval string) int32 {
	if interval == "year" {
		return plan.YearlyPriceCents / 12
	}
	return plan.MonthlyPriceCents
}

// Cancel ends the subscription at the end of the paid period.
func (s *Service) Cancel(ctx context.Context, user store.User) (*Subscription, error) {
	return s.setCancel(ctx, user, true)
}

// Resume undoes Cancel while the period is still running.
func (s *Service) Resume(ctx context.Context, user store.User) (*Subscription, error) {
	return s.setCancel(ctx, user, false)
}

func (s *Service) setCancel(ctx context.Context, user store.User, cancel bool) (*Subscription, error) {
	live, err := s.managed(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if live.CancelAtPeriodEnd == cancel {
		return s.view(ctx, live)
	}
	sub, err := s.polar.UpdateSubscription(ctx, *live.ProviderSubscriptionID, map[string]any{"cancel_at_period_end": cancel})
	if err != nil {
		s.log.ErrorContext(ctx, "cancel change failed", "user", user.ID, "cancel", cancel, "err", err)
		return nil, providerErr(err)
	}
	return s.applyAndView(ctx, user.ID, sub)
}

// managed returns the user's live provider-run subscription.
func (s *Service) managed(ctx context.Context, userID uuid.UUID) (*store.Subscription, error) {
	if s.polar == nil {
		return nil, ErrClosed
	}
	live, err := s.liveSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}
	if live == nil || live.Provider == nil || live.ProviderSubscriptionID == nil || !store.SubscriptionLive(live.Status) {
		return nil, ErrNotSubscribed
	}
	return live, nil
}

func (s *Service) applyAndView(ctx context.Context, userID uuid.UUID, sub PolarSubscription) (*Subscription, error) {
	if err := s.applySubscription(ctx, sub); err != nil {
		return nil, err
	}
	live, err := s.liveSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, live)
}

// PortalURL opens the provider's customer portal for the account: past
// invoices, the payment method, and the subscription itself.
func (s *Service) PortalURL(ctx context.Context, user store.User) (string, error) {
	if s.polar == nil {
		return "", ErrClosed
	}
	has, err := s.st.HasProviderCustomer(ctx, user.ID, ProviderPolar)
	if err != nil {
		return "", err
	}
	if !has {
		return "", ErrNoCustomer
	}
	u, err := s.polar.CustomerPortalURL(ctx, user.ID.String(), s.cfg.WebURL+"/billing")
	if err != nil {
		if errors.Is(providerErr(err), store.ErrNotFound) {
			return "", ErrNoCustomer
		}
		return "", providerErr(err)
	}
	return u, nil
}

// ---------------------------------------------------------------------------
// Webhooks and applying the provider's view
// ---------------------------------------------------------------------------

// HandleWebhook verifies and applies one delivery. Only subscription
// events change anything; every other type is acknowledged and dropped. A
// delivery seen before (by its id) is acknowledged without being applied
// again, and one older than what the table holds is ignored, so retries
// and reordering cannot roll a subscription back.
func (s *Service) HandleWebhook(ctx context.Context, id, timestamp, signature string, body []byte) error {
	secret := ""
	if s.cfg != nil {
		secret = s.cfg.PolarWebhookSecret
	}
	if err := VerifyWebhook(secret, id, timestamp, signature, body, time.Now()); err != nil {
		return err
	}
	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookSignature, err)
	}
	if !strings.HasPrefix(env.Type, "subscription.") {
		return nil
	}
	var sub PolarSubscription
	if err := json.Unmarshal(env.Data, &sub); err != nil {
		return fmt.Errorf("polar webhook %s: %w", env.Type, err)
	}
	return s.st.Tx(ctx, func(_ pgx.Tx, st *store.Store) error {
		fresh, err := st.RecordBillingEvent(ctx, ProviderPolar, id, env.Type)
		if err != nil || !fresh {
			return err
		}
		return s.apply(ctx, st, env.Type, sub)
	})
}

// applySubscription writes what Polar returned from a call, in its own
// transaction.
func (s *Service) applySubscription(ctx context.Context, sub PolarSubscription) error {
	return s.st.Tx(ctx, func(_ pgx.Tx, st *store.Store) error {
		return s.apply(ctx, st, "api", sub)
	})
}

// apply maps a Polar subscription onto the account it belongs to. A
// subscription that names no account, or a product this environment does
// not know, is logged and skipped rather than failed: nothing we do would
// make a retry succeed.
func (s *Service) apply(ctx context.Context, st *store.Store, source string, sub PolarSubscription) error {
	userID, ok := subscriptionUser(sub)
	if !ok {
		s.log.WarnContext(ctx, "subscription without an account", "subscription", sub.ID, "source", source)
		return nil
	}
	cat, err := s.catalogue(ctx, st)
	if err != nil {
		return err
	}
	product, ok := cat.byProduct[sub.ProductID]
	if !ok {
		s.log.WarnContext(ctx, "subscription for an unknown product", "subscription", sub.ID, "product", sub.ProductID, "source", source)
		return nil
	}
	ended := sub.EndedAt
	if ended == nil {
		switch sub.Status {
		case "canceled", "incomplete_expired", "unpaid":
			now := time.Now()
			ended = &now
		}
	}
	ps := store.ProviderSubscription{
		Provider: ProviderPolar, SubscriptionID: sub.ID, CustomerID: sub.CustomerID, UserID: userID,
		PlanID: product.PlanID, Interval: product.Interval, Status: sub.Status,
		CurrentPeriodStart: sub.CurrentPeriodStart, CurrentPeriodEnd: sub.CurrentPeriodEnd,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd, EndedAt: ended, UpdatedAt: sub.LastChanged(),
	}
	if sub.PendingUpdate != nil && sub.PendingUpdate.ProductID != nil {
		if next, ok := cat.byProduct[*sub.PendingUpdate.ProductID]; ok {
			at := sub.PendingUpdate.AppliesAt
			ps.PendingPlanID, ps.PendingInterval, ps.PendingAt = &next.PlanID, &next.Interval, &at
		}
	}
	applied, err := st.ApplyProviderSubscription(ctx, ps)
	if err != nil {
		return err
	}
	if applied {
		s.log.InfoContext(ctx, "subscription applied", "user", userID, "plan", ps.PlanID, "interval", ps.Interval, "status", ps.Status, "source", source)
	}
	return nil
}

// subscriptionUser finds the account: the customer's external id, which
// checkout set to the account id, or the metadata the checkout carried.
func subscriptionUser(sub PolarSubscription) (uuid.UUID, bool) {
	if sub.Customer.ExternalID != nil {
		if id, err := uuid.Parse(*sub.Customer.ExternalID); err == nil {
			return id, true
		}
	}
	if raw, ok := sub.Metadata["simhook_user_id"].(string); ok {
		if id, err := uuid.Parse(raw); err == nil {
			return id, true
		}
	}
	return uuid.Nil, false
}
