package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
)

type sendInput struct {
	Body struct {
		To                []string   `json:"to" minItems:"1" maxItems:"5000" doc:"Recipient phone numbers, ideally E.164 such as +14155550123."`
		Body              string     `json:"body" minLength:"1" maxLength:"1600" doc:"Message text. Long texts are sent as concatenated SMS."`
		DeviceID          *string    `json:"device_id,omitempty" doc:"Phone to send from. Defaults to the account's default device, else the most recently online one."`
		SimSubscriptionID *int32     `json:"sim_subscription_id,omitempty" doc:"SIM to use on a multi-SIM phone. Unknown ids fall back to the phone's preferred SIM."`
		ScheduledAt       *time.Time `json:"scheduled_at,omitempty" doc:"Send later, up to 7 days ahead."`
	}
}

type sendOutput struct {
	Body struct {
		Batch      store.Batch `json:"batch"`
		MessageIDs []uuid.UUID `json:"message_ids" doc:"One id per recipient, in the order given after de-duplication."`
	}
}

type listMessagesInput struct {
	DeviceIDs string `query:"device_ids" doc:"Comma-separated device ids. Default: all paired devices."`
	Direction string `query:"direction" enum:"outbound,inbound" doc:"Filter by direction."`
	Status    string `query:"status" enum:"queued,dispatched,sent,delivered,failed,unknown,received"`
	BatchID   string `query:"batch_id" doc:"Only messages from one send."`
	Search    string `query:"q" maxLength:"200" doc:"Match against text, recipient, and sender."`
	From      string `query:"from" doc:"Inclusive lower bound on created_at, RFC 3339."`
	To        string `query:"to" doc:"Exclusive upper bound on created_at, RFC 3339."`
	Order     string `query:"order" enum:"desc,asc" doc:"desc (default) for newest first; asc to walk forward when polling."`
	Cursor    string `query:"cursor" doc:"next_cursor from a previous page."`
	Limit     int    `query:"limit" minimum:"1" maximum:"100" doc:"Page size, default 50."`
}

type messagesOutput struct {
	Body struct {
		Data       []store.Message `json:"data"`
		NextCursor string          `json:"next_cursor,omitempty" doc:"Pass as cursor to get the next page. Absent on the last page."`
	}
}

type messageIDInput struct {
	ID string `path:"id"`
}

type messageOutput struct {
	Body struct {
		Message store.Message `json:"message"`
	}
}

type listBatchesInput struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100"`
}

type batchesOutput struct {
	Body struct {
		Data       []store.Batch `json:"data"`
		NextCursor string        `json:"next_cursor,omitempty"`
	}
}

type batchOutput struct {
	Body struct {
		Batch    store.Batch     `json:"batch"`
		Messages []store.Message `json:"messages"`
	}
}

type statsOutput struct {
	Body gateway.Stats
}

func parseTime(raw, field string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		d, err2 := time.Parse("2006-01-02", raw)
		if err2 != nil {
			return nil, &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
				Message: "The request has invalid fields.", Errors: []FieldError{{Field: "query." + field, Message: "must be RFC 3339 or YYYY-MM-DD"}}}
		}
		t = d
	}
	return &t, nil
}

func (s *Server) messageFilter(in *listMessagesInput) (store.MessageFilter, error) {
	f := store.MessageFilter{Direction: in.Direction, Status: in.Status, Search: strings.TrimSpace(in.Search), Ascending: in.Order == "asc", Limit: in.Limit}
	if in.DeviceIDs != "" {
		for _, raw := range strings.Split(in.DeviceIDs, ",") {
			id, ok := ids.Parse(strings.TrimSpace(raw))
			if !ok {
				return f, &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
					Message: "The request has invalid fields.", Errors: []FieldError{{Field: "query.device_ids", Message: "contains an invalid id"}}}
			}
			f.DeviceIDs = append(f.DeviceIDs, id)
		}
	}
	if in.BatchID != "" {
		id, ok := ids.Parse(in.BatchID)
		if !ok {
			return f, &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
				Message: "The request has invalid fields.", Errors: []FieldError{{Field: "query.batch_id", Message: "invalid id"}}}
		}
		f.BatchID = &id
	}
	var err error
	if f.From, err = parseTime(in.From, "from"); err != nil {
		return f, err
	}
	if f.To, err = parseTime(in.To, "to"); err != nil {
		return f, err
	}
	if f.Cursor, err = gateway.DecodeCursor(in.Cursor); err != nil {
		return f, mapErr(context.Background(), s.deps.Log, err)
	}
	return f, nil
}

func (s *Server) registerMessages() {
	tags := []string{"messages"}

	huma.Register(s.api, huma.Operation{
		OperationID: "send-message", Method: http.MethodPost, Path: "/v1/messages",
		Extensions: scoped(auth.ScopeSend),
		Summary:    "Send an SMS", Tags: tags, DefaultStatus: http.StatusAccepted, Security: securityUser,
		Description: "Queues one text to one or more recipients from a phone on the account. Acceptance is not delivery: follow the batch or subscribe to webhooks for the outcome. Every recipient counts as one message against the plan.",
	}, func(ctx context.Context, in *sendInput) (*sendOutput, error) {
		p, err := requireUser(ctx, auth.ScopeSend)
		if err != nil {
			return nil, err
		}
		gin := gateway.SendInput{To: in.Body.To, Body: in.Body.Body, SimSubscriptionID: in.Body.SimSubscriptionID, ScheduledAt: in.Body.ScheduledAt}
		if in.Body.DeviceID != nil {
			id, ok := ids.Parse(*in.Body.DeviceID)
			if !ok {
				return nil, apiErr(http.StatusNotFound, "not_found", "No such device.")
			}
			gin.DeviceID = &id
		}
		var keyID *uuid.UUID
		if p.APIKey != nil {
			keyID = &p.APIKey.ID
		}
		res, err := s.deps.Gateway.Send(ctx, *p.User, keyID, gin)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &sendOutput{}
		out.Body.Batch = res.Batch
		out.Body.MessageIDs = res.MessageIDs
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-messages", Method: http.MethodGet, Path: "/v1/messages",
		Extensions: scoped(auth.ScopeRead),
		Summary:    "List messages", Tags: tags, Security: securityUser,
		Description: "Sent and received messages across the account, newest first, with delivery state on each. To poll for new messages, request order=asc with a from bound and follow next_cursor.",
	}, func(ctx context.Context, in *listMessagesInput) (*messagesOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		f, err := s.messageFilter(in)
		if err != nil {
			return nil, err
		}
		page, err := s.deps.Gateway.ListMessages(ctx, p.User.ID, f)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &messagesOutput{}
		out.Body.Data = page.Items
		out.Body.NextCursor = page.NextCursor
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-message", Method: http.MethodGet, Path: "/v1/messages/{id}",
		Extensions: scoped(auth.ScopeRead),
		Summary:    "Get a message", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *messageIDInput) (*messageOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such message.")
		}
		m, err := s.deps.Gateway.GetMessage(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &messageOutput{}
		out.Body.Message = m
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-batches", Method: http.MethodGet, Path: "/v1/batches",
		Extensions: scoped(auth.ScopeRead),
		Summary:    "List sends", Tags: tags, Security: securityUser,
		Description: "Every send on the account, newest first, with per-status counts.",
	}, func(ctx context.Context, in *listBatchesInput) (*batchesOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		cursor, err := gateway.DecodeCursor(in.Cursor)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		page, err := s.deps.Gateway.ListBatches(ctx, p.User.ID, cursor, in.Limit)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &batchesOutput{}
		out.Body.Data = page.Items
		out.Body.NextCursor = page.NextCursor
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-batch", Method: http.MethodGet, Path: "/v1/batches/{id}",
		Extensions: scoped(auth.ScopeRead),
		Summary:    "Get a send", Tags: tags, Security: securityUser,
		Description: "The batch plus one message per recipient. Poll this to follow a send.",
	}, func(ctx context.Context, in *messageIDInput) (*batchOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such batch.")
		}
		b, msgs, err := s.deps.Gateway.GetBatch(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &batchOutput{}
		out.Body.Batch = b
		out.Body.Messages = msgs
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-stats", Method: http.MethodGet, Path: "/v1/stats",
		Extensions: scoped(auth.ScopeRead),
		Summary:    "Account totals", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, _ *struct{}) (*statsOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		st, err := s.deps.Gateway.GetStats(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &statsOutput{Body: st}, nil
	})
}
