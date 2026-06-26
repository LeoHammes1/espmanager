package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LeoHammes1/espmanager/internal/deploy"
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

type DeployService interface {
	ListDeploys(ctx context.Context) ([]deploy.Deploy, error)
	Targets(ctx context.Context, id string) ([]deploy.Target, error)
	DeployDetail(ctx context.Context, id string) (deploy.Deploy, []deploy.Target, error)
	Resume(ctx context.Context, id string) error
	Cancel(ctx context.Context, id string) error
}

// ProvisionInfo is the device-reachable manager address the onboarding wizard
// writes into a device, derived from the server's authoritative configuration so
// onboarding is correct regardless of how the browser reached the UI.
type ProvisionInfo struct {
	Host     string `json:"host"`
	HTTPPort int    `json:"httpPort"`
	MQTTPort int    `json:"mqttPort"`
}

type Options struct {
	Devices          DeviceService
	Drivers          DriverService
	Artifacts        ArtifactStore
	Deployer         Deployer
	Deploys          DeployService
	Enroller         Enroller
	Bus              DeviceBus
	Hub              *SSEHub
	Queue            *queue.Queue
	Webhook          http.Handler
	Sessions         SessionStore
	Log              *slog.Logger
	WorkerToken      string
	AdminUser        string
	AdminPassword    string
	SecureCookies    bool
	FailureThreshold int
	PublicURL        string
	Provision        ProvisionInfo
}

func NewRouter(opts Options) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	agentFW, err := agentFirmware()
	if err != nil {
		return nil, err
	}
	r.Handle("/firmware/agent/*", agentFW)

	if opts.Webhook != nil {
		r.Post("/webhook/git/{driverID}", opts.Webhook.ServeHTTP)
	}
	r.Get("/firmware/{driver}/{file}", serveFirmware(opts.Artifacts))
	r.With(middleware.Throttle(20)).Post("/v1/claim", claimDevice(opts.Enroller, opts.Log))

	r.Group(func(wr chi.Router) {
		wr.Use(httpx.BearerAuth(opts.WorkerToken))
		wr.Get("/v1/jobs/next", nextJob(opts.Queue))
		wr.Post("/v1/jobs/{id}/complete", completeJob(opts.Queue))
		wr.Post("/v1/artifacts", uploadArtifact(opts.Artifacts, opts.Deployer, opts.Log))
	})

	guard := &authGuard{
		sessions:      opts.Sessions,
		user:          opts.AdminUser,
		password:      opts.AdminPassword,
		secureCookies: opts.SecureCookies,
		log:           opts.Log,
	}

	r.Mount("/api", apiRouter(opts, guard))
	r.With(guard.requireAPI).Get("/events", opts.Hub.Handler())

	sub, err := web.SPA()
	if err != nil {
		return nil, err
	}
	r.NotFound(spaHandler(sub))

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
