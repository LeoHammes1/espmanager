package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"

	"github.com/LeoHammes1/espmanager/internal/enroll"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

type Enroller interface {
	Mint(ctx context.Context) (enroll.Token, error)
	Claim(ctx context.Context, deviceID, token string) (string, error)
}

func enrollDevice(enroller Enroller, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := enroller.Mint(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		render(w, tmpl, "device_enrolled.html", map[string]string{
			"Token":     t.Value,
			"ExpiresAt": t.ExpiresAt.Format("2006-01-02 15:04:05 MST"),
		})
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
