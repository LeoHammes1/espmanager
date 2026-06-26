package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

type driverView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RepoURL     string `json:"repoUrl"`
	Branch      string `json:"branch"`
	PioEnv      string `json:"pioEnv"`
	WebhookPath string `json:"webhookPath"`
	WebhookURL  string `json:"webhookUrl"`
	CreatedAt   string `json:"createdAt"`
}

func webhookPath(driverID string) string {
	return "/webhook/git/" + driverID
}

func webhookURL(publicURL, driverID string) string {
	if publicURL == "" {
		return webhookPath(driverID)
	}
	return publicURL + webhookPath(driverID)
}

func driverToView(d driver.Driver, publicURL string) driverView {
	return driverView{
		ID:          d.ID,
		Name:        d.Name,
		RepoURL:     d.RepoURL,
		Branch:      d.Branch,
		PioEnv:      d.PioEnv,
		WebhookPath: webhookPath(d.ID),
		WebhookURL:  webhookURL(publicURL, d.ID),
		CreatedAt:   d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func apiDrivers(drivers DriverService, publicURL string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := drivers.List(r.Context())
		if err != nil {
			apiInternal(w, log, "list drivers failed", err)
			return
		}
		views := make([]driverView, 0, len(list))
		for _, d := range list {
			views = append(views, driverToView(d, publicURL))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"drivers": views})
	}
}

func apiCreateDriver(drivers DriverService, publicURL string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name    string `json:"name"`
			RepoURL string `json:"repoUrl"`
			Branch  string `json:"branch"`
			PioEnv  string `json:"pioEnv"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid_request", "Malformed request.")
			return
		}
		d, err := drivers.Create(r.Context(), driver.NewDriver{
			Name:    req.Name,
			RepoURL: req.RepoURL,
			Branch:  req.Branch,
			PioEnv:  req.PioEnv,
		})
		switch {
		case errors.Is(err, driver.ErrInvalidRepo):
			apiErr(w, http.StatusBadRequest, "invalid_repo", err.Error())
		case errors.Is(err, driver.ErrInvalid):
			apiErr(w, http.StatusBadRequest, "invalid", err.Error())
		case err != nil:
			apiInternal(w, log, "create driver failed", err)
		default:
			httpx.WriteJSON(w, http.StatusCreated, map[string]any{
				"driver":        driverToView(d, publicURL),
				"webhookSecret": d.WebhookSecret,
			})
		}
	}
}
