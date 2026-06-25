package main

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/LeoHammes1/espmanager/internal/httpx"
	"github.com/LeoHammes1/espmanager/internal/sign"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	addr := env("ESPM_SIGNER_ADDR", ":8090")
	keyPath := env("ESPM_SIGNER_KEY", "data/signing.key")
	token := os.Getenv("ESPM_SIGNER_TOKEN")

	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		log.Error("key dir", "err", err)
		os.Exit(1)
	}

	signer, err := sign.LoadOrCreate(keyPath)
	if err != nil {
		log.Error("load key", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /v1/sign", httpx.BearerAuth(token)(signHandler(signer)))
	mux.HandleFunc("GET /v1/public-key", publicKeyHandler(signer))

	log.Info("signer listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func signHandler(signer *sign.Signer) http.HandlerFunc {
	type request struct {
		Digest string `json:"digest"`
	}
	type response struct {
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		digest, err := base64.StdEncoding.DecodeString(req.Digest)
		if err != nil {
			http.Error(w, "invalid digest", http.StatusBadRequest)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, response{
			Signature: base64.StdEncoding.EncodeToString(signer.Sign(digest)),
			PublicKey: base64.StdEncoding.EncodeToString(signer.PublicKey()),
		})
	}
}

func publicKeyHandler(signer *sign.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"public_key": base64.StdEncoding.EncodeToString(signer.PublicKey()),
		})
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
