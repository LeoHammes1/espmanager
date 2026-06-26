package httpapi

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/LeoHammes1/espmanager/internal/driver"
)

type driverView struct {
	ID         string
	Name       string
	RepoURL    string
	Branch     string
	PioEnv     string
	WebhookURL string
	CreatedAt  string
}

type driversData struct {
	Drivers []driverView
}

type createdDriverData struct {
	Name          string
	WebhookURL    string
	WebhookSecret string
}

func webhookPath(driverID string) string {
	return "/webhook/git/" + driverID
}

func driversPage(drivers DriverService, tmpl *template.Template, user string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := drivers.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := driversData{Drivers: make([]driverView, 0, len(list))}
		for _, d := range list {
			data.Drivers = append(data.Drivers, driverView{
				ID:         d.ID,
				Name:       d.Name,
				RepoURL:    d.RepoURL,
				Branch:     d.Branch,
				PioEnv:     d.PioEnv,
				WebhookURL: webhookPath(d.ID),
				CreatedAt:  d.CreatedAt.Format("2006-01-02 15:04"),
			})
		}

		renderShell(w, tmpl, pageView{
			Title:   "Drivers",
			Nav:     "drivers",
			User:    user,
			Content: "page-drivers",
			Data:    data,
		})
	}
}

func createDriver(drivers DriverService, tmpl *template.Template, user string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, err := drivers.Create(r.Context(), driver.NewDriver{
			Name:    r.FormValue("name"),
			RepoURL: r.FormValue("repo_url"),
			Branch:  r.FormValue("branch"),
			PioEnv:  r.FormValue("pio_env"),
		})
		switch {
		case errors.Is(err, driver.ErrInvalid), errors.Is(err, driver.ErrInvalidRepo):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			renderShell(w, tmpl, pageView{
				Title:   "Driver created",
				Nav:     "drivers",
				User:    user,
				Content: "page-driver-created",
				Data: createdDriverData{
					Name:          d.Name,
					WebhookURL:    webhookPath(d.ID),
					WebhookSecret: d.WebhookSecret,
				},
			})
		}
	}
}
