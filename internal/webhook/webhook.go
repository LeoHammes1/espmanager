package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/queue"
)

const maxBodySize = 1 << 20

var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

type Enqueuer interface {
	Enqueue(ctx context.Context, job queue.BuildJob) error
}

type DriverResolver interface {
	Get(ctx context.Context, id string) (driver.Driver, error)
}

type Handler struct {
	drivers  DriverResolver
	enqueuer Enqueuer
	log      *slog.Logger
}

func NewHandler(drivers DriverResolver, enqueuer Enqueuer, log *slog.Logger) *Handler {
	return &Handler{drivers: drivers, enqueuer: enqueuer, log: log}
}

type pushPayload struct {
	Ref   string `json:"ref"`
	After string `json:"after"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	driverID := chi.URLParam(r, "driverID")

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	d, err := h.drivers.Get(r.Context(), driverID)
	if errors.Is(err, driver.ErrNotFound) {
		http.Error(w, "unknown driver", http.StatusNotFound)
		return
	}
	if err != nil {
		h.log.Error("resolve driver failed", "driver", driverID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !verify(r.Header.Get("X-Hub-Signature-256"), body, d.WebhookSecret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if p.Ref != "refs/heads/"+d.Branch {
		h.log.Info("webhook ignored: untracked ref", "driver", d.ID, "ref", p.Ref)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if !commitPattern.MatchString(p.After) || isZeroSHA(p.After) {
		h.log.Info("webhook ignored: no buildable commit", "driver", d.ID)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	job := queue.BuildJob{DriverID: d.ID, Repo: d.RepoURL, Commit: p.After, Env: d.PioEnv}
	if err := h.enqueuer.Enqueue(r.Context(), job); err != nil {
		h.log.Error("enqueue failed", "driver", d.ID, "err", err)
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func isZeroSHA(commit string) bool {
	return strings.Trim(commit, "0") == ""
}

func verify(signature string, body []byte, secret string) bool {
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
