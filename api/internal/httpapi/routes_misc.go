package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/store"
)

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

type plansOutput struct {
	Body struct {
		Data []store.Plan `json:"data"`
	}
}

func (s *Server) registerMisc() {
	huma.Register(s.api, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/healthz",
		Summary: "Health check", Tags: []string{"meta"}, Hidden: true,
	}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-plans", Method: http.MethodGet, Path: "/v1/plans",
		Summary:     "List plans",
		Description: "The public plan catalogue with limits and prices. No credentials needed.",
		Tags:        []string{"billing"},
	}, func(ctx context.Context, _ *struct{}) (*plansOutput, error) {
		plans, err := s.deps.Billing.Plans(ctx)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &plansOutput{}
		out.Body.Data = plans
		return out, nil
	})
}
