package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/ids"
	"github.com/simhook/simhook/internal/store"
)

type pairingCodeOutput struct {
	Body struct {
		Code      string    `json:"code" doc:"Type this into the app, or scan pair_url as a QR code."`
		ExpiresAt time.Time `json:"expires_at"`
		PairURL   string    `json:"pair_url" doc:"Deep link the app understands. Encode it as a QR code."`
	}
}

type devicesOutput struct {
	Body struct {
		Data []store.Device `json:"data"`
	}
}

type deviceOutput struct {
	Body struct {
		Device store.Device `json:"device"`
	}
}

type deviceIDInput struct {
	ID string `path:"id" doc:"Device id from GET /v1/devices."`
}

type devicePatchBody struct {
	Name                       *string `json:"name,omitempty" maxLength:"64"`
	Enabled                    *bool   `json:"enabled,omitempty" doc:"A disabled phone receives no sends."`
	ReceiveEnabled             *bool   `json:"receive_enabled,omitempty" doc:"Forward incoming SMS from this phone."`
	SendDelaySeconds           *int32  `json:"send_delay_seconds,omitempty" minimum:"0" maximum:"3600" doc:"Pause between consecutive sends on the phone."`
	HeartbeatIntervalMinutes   *int32  `json:"heartbeat_interval_minutes,omitempty" minimum:"15" maximum:"1440"`
	PreferredSimSubscriptionID *int32  `json:"preferred_sim_subscription_id,omitempty" doc:"SIM to send from when a send names none. Send null to clear."`
}

type updateDeviceInput struct {
	ID   string `path:"id"`
	Body devicePatchBody
}

func (b devicePatchBody) patch() gateway.DevicePatch {
	return gateway.DevicePatch{
		Name: b.Name, Enabled: b.Enabled, ReceiveEnabled: b.ReceiveEnabled,
		SendDelaySeconds: b.SendDelaySeconds, HeartbeatIntervalMinutes: b.HeartbeatIntervalMinutes,
		PreferredSimSubscriptionID: b.PreferredSimSubscriptionID,
	}
}

func (s *Server) registerDevices() {
	tags := []string{"devices"}

	huma.Register(s.api, huma.Operation{
		OperationID: "create-pairing-code", Method: http.MethodPost, Path: "/v1/devices/pairing-codes",
		Summary: "Create a pairing code", Tags: tags, DefaultStatus: http.StatusCreated, Security: securityUser,
		Description: "Mints a code valid for 10 minutes. Enter it in the phone app, or show pair_url as a QR code for the app to scan.",
	}, func(ctx context.Context, _ *struct{}) (*pairingCodeOutput, error) {
		p, err := requireUser(ctx, auth.ScopeDevices)
		if err != nil {
			return nil, err
		}
		code, expires, err := s.deps.Gateway.CreatePairingCode(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &pairingCodeOutput{}
		out.Body.Code = code
		out.Body.ExpiresAt = expires
		out.Body.PairURL = "simhook://pair?" + url.Values{"code": {code}, "api": {s.deps.Config.PublicURL}}.Encode()
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-devices", Method: http.MethodGet, Path: "/v1/devices",
		Summary: "List devices", Tags: tags, Security: securityUser,
		Description: "Every phone paired with the account, default first.",
	}, func(ctx context.Context, _ *struct{}) (*devicesOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		devices, err := s.deps.Gateway.ListDevices(ctx, p.User.ID)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &devicesOutput{}
		out.Body.Data = devices
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-device", Method: http.MethodGet, Path: "/v1/devices/{id}",
		Summary: "Get a device", Tags: tags, Security: securityUser,
	}, func(ctx context.Context, in *deviceIDInput) (*deviceOutput, error) {
		p, err := requireUser(ctx, auth.ScopeRead)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such device.")
		}
		d, err := s.deps.Gateway.GetDevice(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deviceOutput{}
		out.Body.Device = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-device", Method: http.MethodPatch, Path: "/v1/devices/{id}",
		Summary: "Update device settings", Tags: tags, Security: securityUser,
		Description: "Only the fields you send change. The phone picks the new settings up on its next check-in.",
	}, func(ctx context.Context, in *updateDeviceInput) (*deviceOutput, error) {
		p, err := requireUser(ctx, auth.ScopeDevices)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such device.")
		}
		d, err := s.deps.Gateway.UpdateDevice(ctx, p.User.ID, id, in.Body.patch())
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deviceOutput{}
		out.Body.Device = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "set-default-device", Method: http.MethodPost, Path: "/v1/devices/{id}/default",
		Summary: "Make this the default device", Tags: tags, Security: securityUser,
		Description: "Sends that omit device_id go out from the default device.",
	}, func(ctx context.Context, in *deviceIDInput) (*deviceOutput, error) {
		p, err := requireUser(ctx, auth.ScopeDevices)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such device.")
		}
		d, err := s.deps.Gateway.SetDefaultDevice(ctx, p.User.ID, id)
		if err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		out := &deviceOutput{}
		out.Body.Device = d
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "unpair-device", Method: http.MethodDelete, Path: "/v1/devices/{id}",
		Summary: "Unpair a device", Tags: tags, DefaultStatus: http.StatusNoContent, Security: securityUser,
		Description: "Revokes the phone's token and stops sends to it. Message history is kept.",
	}, func(ctx context.Context, in *deviceIDInput) (*emptyOutput, error) {
		p, err := requireUser(ctx, auth.ScopeDevices)
		if err != nil {
			return nil, err
		}
		id, ok := ids.Parse(in.ID)
		if !ok {
			return nil, apiErr(http.StatusNotFound, "not_found", "No such device.")
		}
		if err := s.deps.Gateway.UnpairDevice(ctx, p.User.ID, id); err != nil {
			return nil, mapErr(ctx, s.deps.Log, err)
		}
		return &emptyOutput{}, nil
	})
}
