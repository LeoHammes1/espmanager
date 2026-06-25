package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

const (
	maxArtifactSize = 16 << 20
	maxFormSlack    = 1 << 20
)

type ArtifactStore interface {
	Store(ctx context.Context, in artifact.NewArtifact) (artifact.Artifact, error)
	Get(ctx context.Context, driverID, version string) (artifact.Artifact, error)
	Path(driverID, version string) string
}

func uploadArtifact(store ArtifactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxArtifactSize+maxFormSlack)
		if err := r.ParseMultipartForm(maxFormSlack); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "firmware too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid upload", http.StatusBadRequest)
			return
		}
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()

		file, _, err := r.FormFile("firmware")
		if err != nil {
			http.Error(w, "missing firmware file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(io.LimitReader(file, maxArtifactSize+1))
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		if len(content) > maxArtifactSize {
			http.Error(w, "firmware too large", http.StatusRequestEntityTooLarge)
			return
		}

		a, err := store.Store(r.Context(), artifact.NewArtifact{
			DriverID: r.FormValue("driver_id"),
			Version:  r.FormValue("version"),
			Commit:   r.FormValue("commit"),
			Env:      r.FormValue("env"),
			Content:  content,
		})
		switch {
		case errors.Is(err, artifact.ErrInvalid), errors.Is(err, artifact.ErrBadVersion):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, artifact.ErrUnknownDriver):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, artifact.ErrAlreadyExists):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			httpx.WriteJSON(w, http.StatusCreated, map[string]any{
				"driver_id": a.DriverID,
				"version":   a.Version,
				"sha256":    a.SHA256,
				"signature": a.Signature,
				"url":       firmwareURL(a.DriverID, a.Version),
			})
		}
	}
}

func serveFirmware(store ArtifactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		driverID := chi.URLParam(r, "driver")
		version, ok := strings.CutSuffix(chi.URLParam(r, "file"), ".bin")
		if !ok {
			http.NotFound(w, r)
			return
		}

		a, err := store.Get(r.Context(), driverID, version)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, store.Path(a.DriverID, a.Version))
	}
}

func firmwareURL(driverID, version string) string {
	return "/firmware/" + driverID + "/" + version + ".bin"
}
