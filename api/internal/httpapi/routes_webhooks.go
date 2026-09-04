package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/webhooks"
)

type createWebhookInput struct {
	Body struct {
		Name   *string  `json:"name,omitempty" maxLength:"64"`
		URL    string   `json:"url" format:"uri" maxLength:"2048" doc:"HTTPS endpoint that receives POSTs."`
		Events []string `json:"events" minItems:"1" doc:"Any of message.received, message.sent, message.delivered, message.failed, message.unknown, device.online, device.offline."`
	}
}

type webhookCreatedOutput struct {
	Body struct {
		Webhook store.Webhook `json:"webhook"`
		Secret  string        `json:"secret" doc:"Signing secret, shown once. Verify X-Simhook-Signature with it."`
	}
}

type webhooksOutput struct {
	Body struct {
		Data []store.Webhook `json:"data"`
	}
}

type webhookOutput struct {
	Body struct {
		Webhook store.Webhook `json:"webhook"`
	}
}

type webhookIDInput struct {
	ID string `path:"id"`
}

type updateWebhookInput struct {
	ID   string `path:"id"`
	Body struct {
		Name    *string  `json:"name,omitempty" maxLength:"64"`
		URL     *string  `json:"url,omitempty" format:"uri" maxLength:"2048"`
		Events  []string `json:"events,omitempty"`
		Enabled *bool    `json:"enabled,omitempty" doc:"Re-enabling clears any automatic pause."`
	}
}

type secretOutput struct {
	Body struct {
		Secret string `json:"secret"`
	}
}

type deliveryOutput struct {
	Body struct {
		Delivery store.Delivery `json:"delivery"`
	}
}

type listDeliveriesInput struct {
	WebhookID string `query:"webhook_id"`
	Status    string `query:"status" enum:"pending,delivered,retrying,failed"`
	Event     string `query:"event"`
	From      string `query:"from"`
	To        string `query:"to"`
	Cursor    string `query:"cursor"`
	Limit     int    `query:"limit" minimum:"1" maximum:"100"`
}

type deliveriesOutput struct {
	Body struct {
		Data       []store.Delivery `json:"data"`
		NextCursor string           `json:"next_cursor,omitempty"`
	}
}

func (s *Server) registerWebhooks() {
	tags := []string{"webhooks"}

	huma.Register(s.api, huma.Operation{
		OperationID: "create-webhook", Method: http.MethodPost, Path: "/v1/webhooks",
		Summary: "Create a webhook", Tags: tags, DefaultStatus: http.StatusCreated, Security: securityUser,
		Description: "Subscribes a URL to events. Each delivery is a JSON POST signed with the returned secret: X-Simhook-Signature is t=<unix>,v1=<hex hmac-sha256 of \"<t>.<body>\">. Respond 2xx within 30 seconds; other responses are retried over a day.",
	}, func(ctx context.Context, in *createWebhookInput) (*webhookCreatedOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		w, secret, err := s.deps.Webhooks.Create(ctx, p.User.ID, webhooks.CreateInput{Name: in.Body.Name, URL: in.Body.URL, Events: in.Body.Events})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &webhookCreatedOutput{}
		out.Body.Webhook = w
		out.Body.Secret = secret
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-webhooks", Method: http.MethodGet, Path: "/v1/webhooks",
		Summary: "List webhooks", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, _ *struct{}) (*webhooksOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		list, err := s.deps.Webhooks.List(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &webhooksOutput{}
		out.Body.Data = list
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-deliveries", Method: http.MethodGet, Path: "/v1/webhooks/deliveries",
		Summary: "List deliveries", Tags: tags, Security: securityUser,
		Description: "Delivery attempts across every webhook on the account, newest first, including webhooks since deleted.",
	}, func(ctx context.Context, in *listDeliveriesInput) (*deliveriesOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		f := store.DeliveryFilter{Status: in.Status, Event: in.Event, Limit: in.Limit}
		if f.Limit <= 0 {
			f.Limit = 50
		}
		if in.WebhookID != "" {
			id, ok := ids.Parse(in.WebhookID)
			if !ok {
				return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
			}
			f.WebhookID = &id
		}
		if f.From, err = parseTime(in.From, "from"); err != nil {
			return nil, err
		}
		if f.To, err = parseTime(in.To, "to"); err != nil {
			return nil, err
		}
		if f.Cursor, err = gateway.DecodeCursor(in.Cursor); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		rows, err := s.deps.Webhooks.ListDeliveries(ctx, p.User.ID, f)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deliveriesOutput{}
		out.Body.Data = rows
		if len(rows) > f.Limit {
			out.Body.Data = rows[:f.Limit]
			last := out.Body.Data[len(out.Body.Data)-1]
			out.Body.NextCursor = gateway.EncodeCursor(store.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-delivery", Method: http.MethodGet, Path: "/v1/webhooks/deliveries/{id}",
		Summary: "Get a delivery", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *webhookIDInput) (*deliveryOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such delivery.")
		}
		d, err := s.deps.Webhooks.GetDelivery(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deliveryOutput{}
		out.Body.Delivery = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-webhook", Method: http.MethodGet, Path: "/v1/webhooks/{id}",
		Summary: "Get a webhook", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *webhookIDInput) (*webhookOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
		}
		w, err := s.deps.Webhooks.Get(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &webhookOutput{}
		out.Body.Webhook = w
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-webhook", Method: http.MethodPatch, Path: "/v1/webhooks/{id}",
		Summary: "Update a webhook", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *updateWebhookInput) (*webhookOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
		}
		w, err := s.deps.Webhooks.Update(ctx, p.User.ID, id, webhooks.UpdateInput{Name: in.Body.Name, URL: in.Body.URL, Events: in.Body.Events, Enabled: in.Body.Enabled})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &webhookOutput{}
		out.Body.Webhook = w
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "rotate-webhook-secret", Method: http.MethodPost, Path: "/v1/webhooks/{id}/rotate-secret",
		Summary: "Rotate the signing secret", Tags: tags, Security: securityUser,
		Description: "Deliveries queued from now on are signed with the new secret.",
	}, func(ctx context.Context, in *webhookIDInput) (*secretOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
		}
		secret, err := s.deps.Webhooks.RotateSecret(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &secretOutput{}
		out.Body.Secret = secret
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "test-webhook", Method: http.MethodPost, Path: "/v1/webhooks/{id}/test",
		Summary: "Send a test event", Tags: tags, DefaultStatus: http.StatusAccepted, Security: securityUser,
		Description: "Queues a ping delivery so you can check your endpoint and signature verification.",
	}, func(ctx context.Context, in *webhookIDInput) (*deliveryOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
		}
		d, err := s.deps.Webhooks.SendTest(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deliveryOutput{}
		out.Body.Delivery = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-webhook", Method: http.MethodDelete, Path: "/v1/webhooks/{id}",
		Summary: "Delete a webhook", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityUser,
	}, func(ctx context.Context, in *webhookIDInput) (*emptyOutput, error) {
		p, err := requireUser(ctx, auth.ScopeWebhooks)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such webhook.")
		}
		if err := s.deps.Webhooks.Delete(ctx, p.User.ID, id); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})
}
