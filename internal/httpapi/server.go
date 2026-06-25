package httpapi

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/httpx"
	"github.com/LeoHammes1/espmanager/internal/queue"
	"github.com/LeoHammes1/espmanager/internal/web"
)

type DeviceLister interface {
	List(ctx context.Context) ([]device.Device, error)
}

type Options struct {
	Devices     DeviceLister
	Hub         *SSEHub
	Templates   *template.Template
	Queue       *queue.Queue
	Webhook     http.Handler
	WorkerToken string
}

func NewRouter(opts Options) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, err
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/", renderDevices(opts.Devices, opts.Templates, "index.html"))
	r.Get("/partials/devices", renderDevices(opts.Devices, opts.Templates, "devices"))
	r.Get("/events", opts.Hub.Handler())

	if opts.Webhook != nil {
		r.Method(http.MethodPost, "/webhook/git", opts.Webhook)
	}

	r.Group(func(pr chi.Router) {
		pr.Use(httpx.BearerAuth(opts.WorkerToken))
		pr.Get("/v1/jobs/next", nextJob(opts.Queue))
		pr.Post("/v1/jobs/{id}/complete", completeJob(opts.Queue))
	})

	return r, nil
}

type deviceView struct {
	ID       string
	Name     string
	ChipType string
	Version  string
	LastSeen string
	Online   bool
}

type devicesData struct {
	Devices []deviceView
}

func renderDevices(lister DeviceLister, tmpl *template.Template, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devices, err := lister.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := devicesData{Devices: make([]deviceView, 0, len(devices))}
		for _, d := range devices {
			data.Devices = append(data.Devices, deviceView{
				ID:       d.ID,
				Name:     d.Name,
				ChipType: d.ChipType,
				Version:  d.ReportedVersion,
				Online:   d.Online,
				LastSeen: formatLastSeen(d.LastSeenAt),
			})
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func formatLastSeen(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func nextJob(q *queue.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := q.Lease(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if job == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, job)
	}
}

func completeJob(q *queue.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := q.Complete(r.Context(), chi.URLParam(r, "id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
