package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Polar is the merchant of record: it sells the plans, collects the money,
// handles tax, and tells us about subscriptions. This client covers the
// calls the service makes; the API is plain JSON over HTTPS, so no SDK is
// needed and nothing else pulls in a dependency.
type Polar struct {
	base  string
	token string
	http  *http.Client
}

const (
	polarProduction = "https://api.polar.sh"
	polarSandbox    = "https://sandbox-api.polar.sh"
)

// NewPolar returns a client for the environment named in configuration:
// "production" or "sandbox", which is a separate Polar with its own
// accounts, tokens, products, and test cards.
func NewPolar(environment, token string) *Polar {
	base := polarSandbox
	if environment == "production" {
		base = polarProduction
	}
	return &Polar{base: base, token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

// PolarError is a non-2xx answer.
type PolarError struct {
	Status int
	Body   string
}

func (e *PolarError) Error() string { return fmt.Sprintf("polar: HTTP %d: %s", e.Status, e.Body) }

func (p *Polar) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	u := p.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "simhook (+https://simhook.dev)")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("polar: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("polar: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := string(data)
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return &PolarError{Status: res.StatusCode, Body: msg}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("polar: decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

// PolarPrice is one price of a product. Only fixed prices are used.
type PolarPrice struct {
	ID            string `json:"id"`
	AmountType    string `json:"amount_type"`
	PriceAmount   int64  `json:"price_amount"`
	PriceCurrency string `json:"price_currency"`
	IsArchived    bool   `json:"is_archived"`
}

// PolarProduct is a product as Polar returns it.
type PolarProduct struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Description       *string        `json:"description"`
	RecurringInterval *string        `json:"recurring_interval"`
	IsArchived        bool           `json:"is_archived"`
	Metadata          map[string]any `json:"metadata"`
	Prices            []PolarPrice   `json:"prices"`
}

// ActivePrice returns the live fixed price, if any.
func (p PolarProduct) ActivePrice() (PolarPrice, bool) {
	for _, pr := range p.Prices {
		if pr.AmountType == "fixed" && !pr.IsArchived {
			return pr, true
		}
	}
	return PolarPrice{}, false
}

// PriceCreate is a fixed price for a new or updated product.
type PriceCreate struct {
	AmountType    string `json:"amount_type"`
	PriceAmount   int64  `json:"price_amount"`
	PriceCurrency string `json:"price_currency"`
}

// ProductCreate is a recurring product.
type ProductCreate struct {
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	RecurringInterval string            `json:"recurring_interval"`
	Visibility        string            `json:"visibility,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Prices            []PriceCreate     `json:"prices"`
}

type listPage[T any] struct {
	Items      []T `json:"items"`
	Pagination struct {
		MaxPage int `json:"max_page"`
	} `json:"pagination"`
}

// ListProducts returns the live products, optionally only those whose
// metadata matches every given pair.
func (p *Polar) ListProducts(ctx context.Context, metadata map[string]string) ([]PolarProduct, error) {
	var all []PolarProduct
	for page := 1; ; page++ {
		q := url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}, "is_archived": {"false"}}
		for k, v := range metadata {
			q.Set("metadata["+k+"]", v)
		}
		var out listPage[PolarProduct]
		if err := p.do(ctx, http.MethodGet, "/v1/products/", q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if page >= out.Pagination.MaxPage {
			return all, nil
		}
	}
}

// CreateProduct adds a product.
func (p *Polar) CreateProduct(ctx context.Context, in ProductCreate) (PolarProduct, error) {
	var out PolarProduct
	err := p.do(ctx, http.MethodPost, "/v1/products/", nil, in, &out)
	return out, err
}

// UpdateProduct changes the given fields. A "prices" list replaces the
// prices: existing ones not listed are archived.
func (p *Polar) UpdateProduct(ctx context.Context, id string, in map[string]any) (PolarProduct, error) {
	var out PolarProduct
	err := p.do(ctx, http.MethodPatch, "/v1/products/"+id, nil, in, &out)
	return out, err
}

// ---------------------------------------------------------------------------
// Checkouts, subscriptions, the customer portal
// ---------------------------------------------------------------------------

// CheckoutCreate opens a hosted checkout for one product. The external
// customer id is our account id, so the customer Polar creates is bound to
// the account, and the metadata rides along to the subscription.
type CheckoutCreate struct {
	Products           []string          `json:"products"`
	ExternalCustomerID string            `json:"external_customer_id,omitempty"`
	CustomerEmail      string            `json:"customer_email,omitempty"`
	CustomerName       string            `json:"customer_name,omitempty"`
	CustomerIPAddress  string            `json:"customer_ip_address,omitempty"`
	SuccessURL         string            `json:"success_url,omitempty"`
	ReturnURL          string            `json:"return_url,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// PolarCheckout is a checkout session.
type PolarCheckout struct {
	ID                 string  `json:"id"`
	Status             string  `json:"status"` // open, expired, confirmed, succeeded, failed
	URL                string  `json:"url"`
	CustomerID         *string `json:"customer_id"`
	ExternalCustomerID *string `json:"external_customer_id"`
	SubscriptionID     *string `json:"subscription_id"`
	ProductID          *string `json:"product_id"`
}

// CreateCheckout opens a checkout and returns it with the page to send the
// customer to.
func (p *Polar) CreateCheckout(ctx context.Context, in CheckoutCreate) (PolarCheckout, error) {
	var out PolarCheckout
	err := p.do(ctx, http.MethodPost, "/v1/checkouts/", nil, in, &out)
	return out, err
}

// GetCheckout reads a checkout by id.
func (p *Polar) GetCheckout(ctx context.Context, id string) (PolarCheckout, error) {
	var out PolarCheckout
	err := p.do(ctx, http.MethodGet, "/v1/checkouts/"+id, nil, nil, &out)
	return out, err
}

// PolarSubscription is a subscription as Polar returns it and as its
// webhooks carry it.
type PolarSubscription struct {
	ID                 string         `json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	ModifiedAt         *time.Time     `json:"modified_at"`
	Status             string         `json:"status"`
	RecurringInterval  string         `json:"recurring_interval"`
	CurrentPeriodStart *time.Time     `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time     `json:"current_period_end"`
	CancelAtPeriodEnd  bool           `json:"cancel_at_period_end"`
	CanceledAt         *time.Time     `json:"canceled_at"`
	EndsAt             *time.Time     `json:"ends_at"`
	EndedAt            *time.Time     `json:"ended_at"`
	CustomerID         string         `json:"customer_id"`
	ProductID          string         `json:"product_id"`
	Amount             int64          `json:"amount"`
	Currency           string         `json:"currency"`
	Metadata           map[string]any `json:"metadata"`
	Customer           struct {
		ID         string  `json:"id"`
		ExternalID *string `json:"external_id"`
		Email      *string `json:"email"`
	} `json:"customer"`
	PendingUpdate *struct {
		AppliesAt time.Time `json:"applies_at"`
		ProductID *string   `json:"product_id"`
	} `json:"pending_update"`
}

// LastChanged is when Polar last touched the subscription.
func (s PolarSubscription) LastChanged() time.Time {
	if s.ModifiedAt != nil {
		return *s.ModifiedAt
	}
	return s.CreatedAt
}

// GetSubscription reads a subscription by id.
func (p *Polar) GetSubscription(ctx context.Context, id string) (PolarSubscription, error) {
	var out PolarSubscription
	err := p.do(ctx, http.MethodGet, "/v1/subscriptions/"+id, nil, nil, &out)
	return out, err
}

// UpdateSubscription applies a change: a new product with a proration
// behavior, or cancel_at_period_end on or off.
func (p *Polar) UpdateSubscription(ctx context.Context, id string, in map[string]any) (PolarSubscription, error) {
	var out PolarSubscription
	err := p.do(ctx, http.MethodPatch, "/v1/subscriptions/"+id, nil, in, &out)
	return out, err
}

// CustomerPortalURL opens a session on Polar's customer portal for the
// customer bound to our account id and returns its address. The portal
// shows invoices and lets the customer change the payment method.
func (p *Polar) CustomerPortalURL(ctx context.Context, externalID, returnURL string) (string, error) {
	var out struct {
		URL string `json:"customer_portal_url"`
	}
	in := map[string]any{"external_customer_id": externalID}
	if returnURL != "" {
		in["return_url"] = returnURL
	}
	err := p.do(ctx, http.MethodPost, "/v1/customer-sessions/", nil, in, &out)
	return out.URL, err
}

// ---------------------------------------------------------------------------
// Webhook endpoints
// ---------------------------------------------------------------------------

// PolarWebhookEndpoint is a registered delivery address. The secret is the
// one Polar signs deliveries with; it is shown here, so `simhook billing
// sync` can hand it to the operator.
type PolarWebhookEndpoint struct {
	ID      string   `json:"id"`
	URL     string   `json:"url"`
	Name    *string  `json:"name"`
	Format  string   `json:"format"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// ListWebhookEndpoints returns every endpoint of the organization.
func (p *Polar) ListWebhookEndpoints(ctx context.Context) ([]PolarWebhookEndpoint, error) {
	var all []PolarWebhookEndpoint
	for page := 1; ; page++ {
		q := url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}}
		var out listPage[PolarWebhookEndpoint]
		if err := p.do(ctx, http.MethodGet, "/v1/webhooks/endpoints", q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if page >= out.Pagination.MaxPage {
			return all, nil
		}
	}
}

// CreateWebhookEndpoint registers an address for raw JSON deliveries of
// the given events.
func (p *Polar) CreateWebhookEndpoint(ctx context.Context, address, name string, events []string) (PolarWebhookEndpoint, error) {
	var out PolarWebhookEndpoint
	in := map[string]any{"url": address, "name": name, "format": "raw", "events": events}
	err := p.do(ctx, http.MethodPost, "/v1/webhooks/endpoints", nil, in, &out)
	return out, err
}

// UpdateWebhookEndpoint changes an endpoint's events or enabled flag.
func (p *Polar) UpdateWebhookEndpoint(ctx context.Context, id string, in map[string]any) (PolarWebhookEndpoint, error) {
	var out PolarWebhookEndpoint
	err := p.do(ctx, http.MethodPatch, "/v1/webhooks/endpoints/"+id, nil, in, &out)
	return out, err
}
