package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/billing"
)

// The CF-IPCountry header is the visitor's country as Cloudflare reports
// it on proxied requests. Trusted only behind our own proxy, like the
// address; see country().

type billingInput struct {
	Country string `header:"CF-IPCountry" hidden:"true"`
}

type billingOutput struct {
	Body billing.BillingStatus
}

type checkoutInput struct {
	Country string `header:"CF-IPCountry" hidden:"true"`
	Body    struct {
		PlanID   string `json:"plan_id" doc:"A paid plan from GET /v1/plans."`
		Interval string `json:"interval" enum:"month,year" doc:"Billing interval."`
	}
}

type planChangeInput struct {
	Body struct {
		PlanID   string `json:"plan_id" doc:"A paid plan from GET /v1/plans."`
		Interval string `json:"interval" enum:"month,year" doc:"Billing interval."`
	}
}

type urlOutput struct {
	Body struct {
		URL string `json:"url" doc:"Page to send the browser to."`
	}
}

type subscriptionOutput struct {
	Body struct {
		Subscription *billing.Subscription `json:"subscription"`
	}
}

type checkoutIDInput struct {
	ID string `path:"id" doc:"The checkout id the provider appended to the return address."`
}

type checkoutStateOutput struct {
	Body billing.CheckoutResult
}

type polarWebhookInput struct {
	ID        string `header:"webhook-id" hidden:"true"`
	Timestamp string `header:"webhook-timestamp" hidden:"true"`
	Signature string `header:"webhook-signature" hidden:"true"`
	RawBody   []byte
}

func (s *Server) country(header string) string {
	if !s.deps.Config.TrustProxy {
		return ""
	}
	return header
}

func (s *Server) registerBilling() {
	tags := []string{"billing"}

	huma.Register(s.api, huma.Operation{
		OperationID: "billing-status", Method: http.MethodGet, Path: "/v1/billing",
		Extensions: scoped(scopeSession),
		Summary:    "Billing status", Tags: tags, Security: securityUser,
		Description: "The account's subscription, whether a paid plan can be bought right now, and whether the provider's portal is available.",
	}, func(ctx context.Context, in *billingInput) (*billingOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		st, err := s.deps.Billing.Status(ctx, *p.User, s.country(in.Country))
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &billingOutput{Body: st}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-checkout", Method: http.MethodPost, Path: "/v1/billing/checkout",
		Extensions: scoped(scopeSession),
		Summary:    "Start a checkout", Tags: tags, Security: securityUser, DefaultStatus: http.StatusCreated,
		Description: "Opens a hosted checkout for a paid plan and returns its address. The provider sends the browser back to the dashboard with the checkout id when it is paid.",
	}, func(ctx context.Context, in *checkoutInput) (*urlOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		addr, _ := ctx.Value(remoteAddrKey{}).(string)
		u, err := s.deps.Billing.Checkout(ctx, *p.User, in.Body.PlanID, in.Body.Interval, s.country(in.Country), hostOf(addr))
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &urlOutput{}
		out.Body.URL = u
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "checkout-state", Method: http.MethodGet, Path: "/v1/billing/checkouts/{id}",
		Extensions: scoped(scopeSession),
		Summary:    "Read a checkout", Tags: tags, Security: securityUser,
		Description: "The state of a checkout the browser came back from. A paid one puts its subscription on the account at once, without waiting for the provider's webhook.",
	}, func(ctx context.Context, in *checkoutIDInput) (*checkoutStateOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		res, err := s.deps.Billing.CheckoutState(ctx, *p.User, in.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &checkoutStateOutput{Body: res}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "change-plan", Method: http.MethodPost, Path: "/v1/billing/change",
		Extensions: scoped(scopeSession),
		Summary:    "Change the plan", Tags: tags, Security: securityUser,
		Description: "Moves the live subscription to another plan or interval. A change that costs more per month is charged and applied now; one that costs less applies at the next period.",
	}, func(ctx context.Context, in *planChangeInput) (*subscriptionOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := s.deps.Billing.ChangePlan(ctx, *p.User, in.Body.PlanID, in.Body.Interval)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &subscriptionOutput{}
		out.Body.Subscription = sub
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "cancel-subscription", Method: http.MethodPost, Path: "/v1/billing/cancel",
		Extensions: scoped(scopeSession),
		Summary:    "Cancel at period end", Tags: tags, Security: securityUser,
		Description: "The subscription runs to the end of the paid period, then the account is on Free.",
	}, func(ctx context.Context, _ *struct{}) (*subscriptionOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := s.deps.Billing.Cancel(ctx, *p.User)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &subscriptionOutput{}
		out.Body.Subscription = sub
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "resume-subscription", Method: http.MethodPost, Path: "/v1/billing/resume",
		Extensions: scoped(scopeSession),
		Summary:    "Undo a cancellation", Tags: tags, Security: securityUser,
		Description: "Keeps a subscription that was set to end at the period end.",
	}, func(ctx context.Context, _ *struct{}) (*subscriptionOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := s.deps.Billing.Resume(ctx, *p.User)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &subscriptionOutput{}
		out.Body.Subscription = sub
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "billing-portal", Method: http.MethodPost, Path: "/v1/billing/portal",
		Extensions: scoped(scopeSession),
		Summary:    "Open the billing portal", Tags: tags, Security: securityUser, DefaultStatus: http.StatusCreated,
		Description: "A short-lived address for the provider's portal: invoices, the payment method, and the subscription.",
	}, func(ctx context.Context, _ *struct{}) (*urlOutput, error) {
		p, err := requireSession(ctx)
		if err != nil {
			return nil, err
		}
		u, err := s.deps.Billing.PortalURL(ctx, *p.User)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &urlOutput{}
		out.Body.URL = u
		return out, nil
	})

	// The provider's deliveries. Not in the reference: nothing but Polar
	// calls it, and the signature is the only credential.
	huma.Register(s.api, huma.Operation{
		OperationID: "polar-webhook", Method: http.MethodPost, Path: "/v1/billing/webhooks/polar",
		Summary: "Polar webhook", Tags: tags, Hidden: true, DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *polarWebhookInput) (*emptyOutput, error) {
		if err := s.deps.Billing.HandleWebhook(ctx, in.ID, in.Timestamp, in.Signature, in.RawBody); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})
}
