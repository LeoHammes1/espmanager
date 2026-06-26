package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/enroll"
	"github.com/LeoHammes1/espmanager/internal/httpx"
	"github.com/LeoHammes1/espmanager/internal/topics"
)

type Enroller interface {
	Mint(ctx context.Context) (enroll.Token, error)
	MintFor(ctx context.Context, mac string) (enroll.Token, error)
	Claim(ctx context.Context, deviceID, token string) (string, error)
	Rotate(ctx context.Context, deviceID string) (string, error)
	Revoke(ctx context.Context, deviceID string) error
}

type DeviceBus interface {
	Publish(topic string, payload []byte) error
	Disconnect(deviceID string) error
	Online(deviceID string) bool
}

func apiEnroll(enroller Enroller, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Body is optional: {"mac": "..."} binds the token to one device (wizard),
		// no body mints an unbound token (manual fallback).
		var req struct {
			MAC string `json:"mac"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
		}

		var (
			t   enroll.Token
			err error
		)
		if req.MAC != "" {
			t, err = enroller.MintFor(r.Context(), req.MAC)
			if errors.Is(err, enroll.ErrInvalidMAC) {
				apiErr(w, http.StatusBadRequest, "invalid_mac", "Invalid device MAC.")
				return
			}
		} else {
			t, err = enroller.Mint(r.Context())
		}
		if err != nil {
			apiInternal(w, log, "mint claim token failed", err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]string{
			"token":     t.Value,
			"expiresAt": t.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

func apiRotate(enroller Enroller, bus DeviceBus, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "id")
		password, err := enroller.Rotate(r.Context(), deviceID)
		switch {
		case errors.Is(err, enroll.ErrNotEnrolled):
			apiErr(w, http.StatusNotFound, "not_enrolled", "Device is not enrolled.")
			return
		case errors.Is(err, enroll.ErrRotationPending):
			apiErr(w, http.StatusConflict, "rotation_pending", "A credential rotation is already pending; let the device adopt it or revoke first.")
			return
		case err != nil:
			log.Error("credential rotation failed", "device", deviceID, "err", err)
			apiErr(w, http.StatusInternalServerError, "internal", "Internal error.")
			return
		}

		// The previous credential stays valid until the device adopts the new one,
		// so an undelivered rotation never locks the device out — but the operator
		// must know it was not applied. Only report success when it was delivered.
		if !bus.Online(deviceID) {
			httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"deviceId": deviceID, "password": password, "delivered": false})
			return
		}
		payload, _ := json.Marshal(map[string]string{"password": password})
		if err := bus.Publish(topics.CmdCred(deviceID), payload); err != nil {
			log.Error("publish rotated credential failed", "device", deviceID, "err", err)
			apiErr(w, http.StatusBadGateway, "delivery_failed", "Credential staged but delivery failed; retry once the device is online.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"deviceId": deviceID, "password": password, "delivered": true})
	}
}

func apiRevoke(enroller Enroller, bus DeviceBus, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "id")
		err := enroller.Revoke(r.Context(), deviceID)
		switch {
		case errors.Is(err, enroll.ErrNotEnrolled):
			apiErr(w, http.StatusNotFound, "not_enrolled", "Device is not enrolled.")
			return
		case err != nil:
			log.Error("credential revoke failed", "device", deviceID, "err", err)
			apiErr(w, http.StatusInternalServerError, "internal", "Internal error.")
			return
		}
		if err := bus.Disconnect(deviceID); err != nil {
			log.Error("disconnect revoked device failed", "device", deviceID, "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func claimDevice(enroller Enroller, log *slog.Logger) http.HandlerFunc {
	type request struct {
		DeviceID string `json:"device_id"`
		Token    string `json:"token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		password, err := enroller.Claim(r.Context(), req.DeviceID, req.Token)
		switch {
		case errors.Is(err, enroll.ErrInvalidDevice):
			http.Error(w, "invalid device id", http.StatusBadRequest)
		case errors.Is(err, enroll.ErrInvalidToken):
			http.Error(w, "invalid or expired claim token", http.StatusUnauthorized)
		case errors.Is(err, enroll.ErrAlreadyEnrolled):
			http.Error(w, "device already enrolled", http.StatusConflict)
		case err != nil:
			log.Error("device claim failed", "device", req.DeviceID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{
				"username": req.DeviceID,
				"password": password,
			})
		}
	}
}
