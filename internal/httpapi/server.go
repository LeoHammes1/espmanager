package httpapi

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/httpx"
	"github.com/LeoHammes1/espmanager/internal/queue"
	"github.com/LeoHammes1/espmanager/internal/web"
)

type DeviceService interface {
	List(ctx context.Context) ([]device.Device, error)
	Assign(ctx context.Context, id, driverID string) error
}

type DriverService interface {
	List(ctx context.Context) ([]driver.Driver, error)
	Create(ctx context.Context, in driver.NewDriver) (driver.Driver, error)
}

type Deployer interface {
	Rollout(ctx context.Context, driverID, version string) error
}

type Options struct {
	Devices       DeviceService
	Drivers       DriverService
	Artifacts     ArtifactStore
	Deployer      Deployer
	Hub           *SSEHub
	Templates     *template.Template
	Queue         *queue.Queue
	Webhook       http.Handler
	Log           *slog.Logger
	WorkerToken   string
	AdminUser     string
	AdminPassword string
}

func NewRouter(opts Options) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, err
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	if opts.Webhook != nil {
		r.Post("/webhook/git/{driverID}", opts.Webhook.ServeHTTP)
	}

	r.Get("/firmware/{driver}/{file}", serveFirmware(opts.Artifacts))

	r.Group(func(wr chi.Router) {
		wr.Use(httpx.BearerAuth(opts.WorkerToken))
		wr.Get("/v1/jobs/next", nextJob(opts.Queue))
		wr.Post("/v1/jobs/{id}/complete", completeJob(opts.Queue))
		wr.Post("/v1/artifacts", uploadArtifact(opts.Artifacts, opts.Deployer, opts.Log))
	})

	r.Group(func(ur chi.Router) {
		ur.Use(httpx.BasicAuth(opts.AdminUser, opts.AdminPassword))
		ur.Get("/", renderPage(opts.Devices, opts.Drivers, opts.Templates, "index.html"))
		ur.Get("/partials/devices", renderPage(opts.Devices, opts.Drivers, opts.Templates, "devices"))
		ur.Post("/devices/{id}/driver", assignDriver(opts.Devices))
		ur.Get("/drivers", driversPage(opts.Drivers, opts.Templates))
		ur.Post("/drivers", createDriver(opts.Drivers, opts.Templates))
		ur.Get("/events", opts.Hub.Handler())
	})

	return r, nil
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
