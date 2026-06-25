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

	"github.com/leohammes/espmanager/internal/queue"
)

const maxBodySize = 1 << 20

type Enqueuer interface {
	Enqueue(ctx context.Context, job queue.BuildJob) error
}

type Handler struct {
	secret   string
	enqueuer Enqueuer
	log      *slog.Logger
}

func NewHandler(secret string, enqueuer Enqueuer, log *slog.Logger) *Handler {
	return &Handler{secret: secret, enqueuer: enqueuer, log: log}
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

	if !h.verify(r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if p.Ref != "refs/heads/main" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	job := queue.BuildJob{Repo: p.Repository.CloneURL, Commit: p.After}
	if err := h.enqueuer.Enqueue(r.Context(), job); err != nil {
		h.log.Error("enqueue failed", "err", err)
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) verify(signature string, body []byte) bool {
	if h.secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
