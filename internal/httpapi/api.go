package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LeoHammes1/espmanager/internal/httpx"
)

func apiErr(w http.ResponseWriter, status int, code, message string) {
	httpx.WriteJSON(w, status, map[string]string{"error": code, "message": message})
}

// apiInternal logs the real error server-side and returns a uniform 500 so
// internal/DB error text never reaches the client.
func apiInternal(w http.ResponseWriter, log *slog.Logger, msg string, err error) {
	if log != nil {
		log.Error(msg, "err", err)
	}
	apiErr(w, http.StatusInternalServerError, "internal", "Something went wrong.")
}

// apiRouter assembles the JSON API mounted at /api. Unknown paths and methods
// return JSON (never the SPA shell), session endpoints are public, and every
// data/action endpoint is behind the JSON-401 guard.
func apiRouter(opts Options, guard *authGuard) http.Handler {
	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		apiErr(w, http.StatusNotFound, "not_found", "No such endpoint.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		apiErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
	})

	r.Get("/session", guard.getSession)
	r.With(middleware.Throttle(10)).Post("/session", guard.postSession)
	r.Delete("/session", guard.deleteSession)

	r.Group(func(pr chi.Router) {
		pr.Use(guard.requireAPI)

		pr.Get("/overview", apiOverview(opts.Deploys, opts.Drivers, opts.Devices, opts.FailureThreshold, opts.Log))

		pr.Get("/devices", apiDevices(opts.Devices, opts.Drivers, opts.Log))
		pr.Put("/devices/{id}/driver", apiAssignDriver(opts.Devices, opts.Log))
		pr.Post("/devices/enroll", apiEnroll(opts.Enroller, opts.Log))
		pr.Post("/devices/{id}/rotate", apiRotate(opts.Enroller, opts.Bus, opts.Log))
		pr.Post("/devices/{id}/revoke", apiRevoke(opts.Enroller, opts.Bus, opts.Log))

		pr.Get("/drivers", apiDrivers(opts.Drivers, opts.PublicURL, opts.Log))
		pr.Post("/drivers", apiCreateDriver(opts.Drivers, opts.PublicURL, opts.Log))

		pr.Get("/deploys", apiDeploys(opts.Deploys, opts.Drivers, opts.Devices, opts.FailureThreshold, opts.Log))
		pr.Get("/deploys/{id}", apiDeployDetail(opts.Deploys, opts.Drivers, opts.Devices, opts.FailureThreshold, opts.Log))
		pr.Post("/deploys/{id}/resume", deployAction(opts.Deploys.Resume, opts.Log))
		pr.Post("/deploys/{id}/cancel", deployAction(opts.Deploys.Cancel, opts.Log))
	})
	return r
}
