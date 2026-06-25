package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/queue"
)

const maxBodySize = 1 << 20

type Enqueuer interface {
	Enqueue(ctx context.Context, job queue.BuildJob) error
}

type DriverResolver interface {
	ByRepo(ctx context.Context, repoURL string) ([]driver.Driver, error)
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
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	candidates, err := h.drivers.ByRepo(r.Context(), p.Repository.CloneURL)
	if err != nil {
		h.log.Error("resolve drivers failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	enqueued, rejected := 0, false

	for _, d := range candidates {
		if p.Ref != "refs/heads/"+d.Branch {
			continue
		}
		if !verify(signature, body, d.WebhookSecret) {
			rejected = true
			continue
		}
		job := queue.BuildJob{DriverID: d.ID, Repo: d.RepoURL, Commit: p.After, Env: d.PioEnv}
		if err := h.enqueuer.Enqueue(r.Context(), job); err != nil {
			h.log.Error("enqueue failed", "driver", d.ID, "err", err)
			http.Error(w, "enqueue failed", http.StatusInternalServerError)
			return
		}
		enqueued++
	}

	if enqueued == 0 && rejected {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusAccepted)
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
