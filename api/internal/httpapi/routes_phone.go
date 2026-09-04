package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
)

// Endpoints under /v1/device are called by the phone app with its device
// token. They are documented but not part of the developer surface.

type pairInput struct {
	Body struct {
		Code           string  `json:"code" minLength:"8" maxLength:"9" doc:"Pairing code from the dashboard."`
		HardwareKey    string  `json:"hardware_key" minLength:"8" maxLength:"128" doc:"Stable hash the app derives from the handset, so re-pairing finds the same device."`
		Name           *string `json:"name,omitempty" maxLength:"64"`
		Manufacturer   *string `json:"manufacturer,omitempty" maxLength:"64"`
		Brand          *string `json:"brand,omitempty" maxLength:"64"`
		Model          *string `json:"model,omitempty" maxLength:"64"`
		BuildID        *string `json:"build_id,omitempty" maxLength:"64"`
		OSVersion      *string `json:"os_version,omitempty" maxLength:"32"`
		OSAPILevel     *int32  `json:"os_api_level,omitempty"`
		AppVersionName *string `json:"app_version_name,omitempty" maxLength:"32"`
		AppVersionCode *int32  `json:"app_version_code,omitempty"`
		PushToken      *string `json:"push_token,omitempty" maxLength:"4096"`
	}
}

type pairOutput struct {
	Body struct {
		Device      store.Device `json:"device"`
		DeviceToken string       `json:"device_token" doc:"Bearer token for /v1/device endpoints. Shown once."`
	}
}

type heartbeatInput struct {
	Body struct {
		PushToken      *string         `json:"push_token,omitempty" maxLength:"4096"`
		AppVersionName *string         `json:"app_version_name,omitempty" maxLength:"32"`
		AppVersionCode *int32          `json:"app_version_code,omitempty"`
		OSVersion      *string         `json:"os_version,omitempty" maxLength:"32"`
		OSAPILevel     *int32          `json:"os_api_level,omitempty"`
		Telemetry      json.RawMessage `json:"telemetry,omitempty" doc:"Battery, network, storage, uptime, timezone, locale. Free-form object."`
		Sims           json.RawMessage `json:"sims,omitempty" doc:"Array of SIM descriptors with subscription_id, carrier, slot, country."`
	}
}

type pushTokenInput struct {
	Body struct {
		Token string `json:"token" minLength:"1" maxLength:"4096"`
	}
}

type statusReportInput struct {
	ID   string `path:"id" doc:"Message id from the push payload."`
	Body struct {
		Status       string     `json:"status" enum:"sent,delivered,failed"`
		At           *time.Time `json:"at,omitempty" doc:"When it happened on the phone. Defaults to now."`
		ErrorCode    *string    `json:"error_code,omitempty" maxLength:"64"`
		ErrorMessage *string    `json:"error_message,omitempty" maxLength:"500"`
	}
}

type inboundInput struct {
	Body struct {
		Sender            string    `json:"sender" minLength:"1" maxLength:"64"`
		Body              string    `json:"body" minLength:"1" maxLength:"4000"`
		ReceivedAt        time.Time `json:"received_at"`
		Fingerprint       *string   `json:"fingerprint,omitempty" maxLength:"128" doc:"Hash of sender, body, and timestamp computed on the phone. Repeats are ignored."`
		SimSubscriptionID *int32    `json:"sim_subscription_id,omitempty"`
	}
}

type inboundOutput struct {
	Status int
	Body   struct {
		Message  store.Message `json:"message"`
		Inserted bool          `json:"inserted" doc:"False when this message was already stored."`
	}
}

type deviceMessagesInput struct {
	Direction string `query:"direction" enum:"outbound,inbound"`
	Cursor    string `query:"cursor"`
	Limit     int    `query:"limit" minimum:"1" maximum:"100"`
}

func (s *Server) registerPhone() {
	tags := []string{"device"}

	huma.Register(s.api, huma.Operation{
		OperationID: "pair-device", Method: http.MethodPost, Path: "/v1/device/pair",
		Summary: "Pair a phone", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "Exchanges a pairing code for a device record and a device token. Called by the phone app.",
	}, func(ctx context.Context, in *pairInput) (*pairOutput, error) {
		b := in.Body
		d, token, err := s.deps.Gateway.Pair(ctx, gateway.PairInput{
			Code: b.Code, HardwareKey: b.HardwareKey, Name: b.Name, Manufacturer: b.Manufacturer, Brand: b.Brand,
			Model: b.Model, BuildID: b.BuildID, OSVersion: b.OSVersion, OSAPILevel: b.OSAPILevel,
			AppVersionName: b.AppVersionName, AppVersionCode: b.AppVersionCode, PushToken: b.PushToken,
		})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &pairOutput{}
		out.Body.Device = d
		out.Body.DeviceToken = token
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-self", Method: http.MethodGet, Path: "/v1/device",
		Summary: "This phone's record", Tags: tags, Security: securityDevice,
	}, func(ctx context.Context, _ *struct{}) (*deviceOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		out := &deviceOutput{}
		out.Body.Device = *p.Device
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-update-self", Method: http.MethodPatch, Path: "/v1/device",
		Summary: "Change this phone's settings", Tags: tags, Security: securityDevice,
	}, func(ctx context.Context, in *struct{ Body devicePatchBody }) (*deviceOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		d, err := s.deps.Gateway.UpdateOwnDevice(ctx, *p.Device, in.Body.patch())
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deviceOutput{}
		out.Body.Device = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-heartbeat", Method: http.MethodPost, Path: "/v1/device/heartbeat",
		Summary: "Check in", Tags: tags, Security: securityDevice,
		Description: "Reports the phone is alive and uploads its state. The response carries the settings the phone should apply.",
	}, func(ctx context.Context, in *heartbeatInput) (*deviceOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		d, err := s.deps.Gateway.Heartbeat(ctx, *p.Device, gateway.HeartbeatInput{
			PushToken: in.Body.PushToken, AppVersionName: in.Body.AppVersionName, AppVersionCode: in.Body.AppVersionCode,
			OSVersion: in.Body.OSVersion, OSAPILevel: in.Body.OSAPILevel, Telemetry: in.Body.Telemetry, Sims: in.Body.Sims,
		})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deviceOutput{}
		out.Body.Device = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-push-token", Method: http.MethodPost, Path: "/v1/device/push-token",
		Summary: "Register a new push token", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityDevice,
	}, func(ctx context.Context, in *pushTokenInput) (*emptyOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.deps.Gateway.RefreshPushToken(ctx, *p.Device, in.Body.Token); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-report-status", Method: http.MethodPost, Path: "/v1/device/messages/{id}/status",
		Summary: "Report a send outcome", Tags: tags, Security: securityDevice,
		Description: "Idempotent. Out-of-order reports never move a message backwards.",
	}, func(ctx context.Context, in *statusReportInput) (*messageOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such message on this device.")
		}
		var at time.Time
		if in.Body.At != nil {
			at = *in.Body.At
		}
		m, err := s.deps.Gateway.ReportStatus(ctx, *p.Device, id, in.Body.Status, at, in.Body.ErrorCode, in.Body.ErrorMessage)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &messageOutput{}
		out.Body.Message = m
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-report-inbound", Method: http.MethodPost, Path: "/v1/device/inbound",
		Summary: "Report a received SMS", Tags: tags, Security: securityDevice,
		Description: "Stores an incoming message and fans out message.received. Returns 201 when stored, 200 when it was already known.",
	}, func(ctx context.Context, in *inboundInput) (*inboundOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		m, inserted, err := s.deps.Gateway.ReportInbound(ctx, *p.Device, gateway.InboundInput{
			Sender: in.Body.Sender, Body: in.Body.Body, ReceivedAt: in.Body.ReceivedAt,
			Fingerprint: in.Body.Fingerprint, SimSubscriptionID: in.Body.SimSubscriptionID,
		})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &inboundOutput{Status: http.StatusOK}
		if inserted {
			out.Status = http.StatusCreated
		}
		out.Body.Message = m
		out.Body.Inserted = inserted
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "device-messages", Method: http.MethodGet, Path: "/v1/device/messages",
		Summary: "This phone's messages", Tags: tags, Security: securityDevice,
	}, func(ctx context.Context, in *deviceMessagesInput) (*messagesOutput, error) {
		p, err := requireDevice(ctx)
		if err != nil {
			return nil, err
		}
		cursor, err := gateway.DecodeCursor(in.Cursor)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		page, err := s.deps.Gateway.ListDeviceMessages(ctx, *p.Device, store.MessageFilter{Direction: in.Direction, Cursor: cursor, Limit: in.Limit})
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &messagesOutput{}
		out.Body.Data = page.Items
		out.Body.NextCursor = page.NextCursor
		return out, nil
	})
}
