package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/store"
	"github.com/simhook/simhook/internal/turnstile"
	"github.com/simhook/simhook/internal/validate"
	"github.com/simhook/simhook/internal/webhooks"
)

// FieldError points at one bad input.
type FieldError struct {
	Field   string `json:"field,omitempty" doc:"Location of the problem, for example body.to[0]."`
	Message string `json:"message" doc:"What is wrong with it."`
}

// APIError is the single error shape every endpoint returns.
type APIError struct {
	Status  int          `json:"status" doc:"HTTP status code."`
	Code    string       `json:"code" doc:"Stable machine-readable code."`
	Message string       `json:"message" doc:"Human-readable explanation."`
	Errors  []FieldError `json:"errors,omitempty" doc:"Per-field problems, when applicable."`
}

func (e *APIError) Error() string { return e.Message }

// GetStatus implements huma.StatusError.
func (e *APIError) GetStatus() int { return e.Status }

// ContentType implements huma.ContentTypeFilter so errors are plain JSON.
func (e *APIError) ContentType(string) string { return "application/json" }

func init() {
	// Lists are always present, possibly empty. Never null.
	huma.DefaultArrayNullable = false
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		e := &APIError{Status: status, Code: codeForStatus(status), Message: msg}
		for _, err := range errs {
			if d, ok := err.(huma.ErrorDetailer); ok {
				det := d.ErrorDetail()
				e.Errors = append(e.Errors, FieldError{Field: det.Location, Message: det.Message})
			} else if err != nil {
				e.Errors = append(e.Errors, FieldError{Message: err.Error()})
			}
		}
		if status == http.StatusUnprocessableEntity && msg == "validation failed" {
			e.Code = "validation_failed"
			e.Message = "The request has invalid fields."
		}
		return e
	}
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "unavailable"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "error"
	}
}

func apiErr(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

// mapErr converts a domain error into the API error model. Unknown errors
// are logged and hidden behind a 500.
func mapErr(ctx context.Context, log *slog.Logger, err error) error {
	if err == nil {
		return nil
	}
	var apiE *APIError
	if errors.As(err, &apiE) {
		return apiE
	}
	var ve *validate.Error
	if errors.As(err, &ve) {
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
			Message: "The request has invalid fields.", Errors: []FieldError{{Field: "body." + ve.Field, Message: ve.Message}}}
	}
	var le *billing.LimitError
	if errors.As(err, &le) {
		return apiErr(http.StatusTooManyRequests, "plan_limit_"+le.Kind, le.Error())
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return apiErr(http.StatusNotFound, "not_found", "No such resource on this account.")
	case errors.Is(err, store.ErrConflict):
		return apiErr(http.StatusConflict, "conflict", "That already exists.")
	case errors.Is(err, auth.ErrUnauthenticated):
		return apiErr(http.StatusUnauthorized, "unauthenticated", "Missing, invalid, or revoked credentials.")
	case errors.Is(err, auth.ErrForbidden):
		return apiErr(http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, auth.ErrBanned):
		return apiErr(http.StatusForbidden, "account_suspended", err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials):
		return apiErr(http.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, auth.ErrTooManyAttempts):
		return apiErr(http.StatusTooManyRequests, "rate_limited", err.Error())
	case errors.Is(err, auth.ErrEmailTaken):
		return apiErr(http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, auth.ErrInvalidCode):
		return apiErr(http.StatusBadRequest, "invalid_code", err.Error())
	case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrInvalidEmail):
		return apiErr(http.StatusUnprocessableEntity, "validation_failed", err.Error())
	case errors.Is(err, auth.ErrNoPassword):
		return apiErr(http.StatusBadRequest, "no_password", err.Error())
	case errors.Is(err, auth.ErrGoogleEmailUnverified):
		return apiErr(http.StatusBadRequest, "google_email_unverified", err.Error())
	case errors.Is(err, turnstile.ErrFailed):
		return apiErr(http.StatusBadRequest, "turnstile_failed", "The bot check did not pass. Reload the page and try again.")
	case errors.Is(err, gateway.ErrInvalidPairingCode):
		return apiErr(http.StatusBadRequest, "invalid_pairing_code", err.Error())
	case errors.Is(err, gateway.ErrEmailUnverified):
		return apiErr(http.StatusForbidden, "email_unverified", err.Error())
	case errors.Is(err, gateway.ErrNoDevice):
		return apiErr(http.StatusBadRequest, "no_device", err.Error())
	case errors.Is(err, gateway.ErrDeviceDisabled):
		return apiErr(http.StatusBadRequest, "device_disabled", err.Error())
	case errors.Is(err, webhooks.ErrInvalidURL), errors.Is(err, webhooks.ErrInvalidEvents):
		return apiErr(http.StatusUnprocessableEntity, "validation_failed", err.Error())
	case errors.Is(err, webhooks.ErrTooMany):
		return apiErr(http.StatusTooManyRequests, "plan_limit_webhooks", err.Error())
	case errors.Is(err, gateway.ErrQueueNotReady), errors.Is(err, webhooks.ErrQueueNotReady):
		return apiErr(http.StatusServiceUnavailable, "unavailable", "The service is starting up. Try again in a moment.")
	case errors.Is(err, context.Canceled):
		return apiErr(499, "client_closed", "The client closed the request.")
	}
	log.ErrorContext(ctx, "unhandled error", "err", err)
	return apiErr(http.StatusInternalServerError, "internal_error", "Something went wrong on our side.")
}
