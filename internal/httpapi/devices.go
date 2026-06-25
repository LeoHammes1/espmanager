package httpapi

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/device"
)

type deviceView struct {
	ID       string
	Name     string
	ChipType string
	Version  string
	LastSeen string
	DriverID string
	Online   bool
}

type driverOption struct {
	ID   string
	Name string
}

type pageData struct {
	Devices []deviceView
	Drivers []driverOption
}

func renderPage(devices DeviceService, drivers DriverService, tmpl *template.Template, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := pageDataFor(r.Context(), devices, drivers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, tmpl, name, data)
	}
}

func pageDataFor(ctx context.Context, devices DeviceService, drivers DriverService) (pageData, error) {
	ds, err := devices.List(ctx)
	if err != nil {
		return pageData{}, err
	}
	drs, err := drivers.List(ctx)
	if err != nil {
		return pageData{}, err
	}

	data := pageData{
		Devices: make([]deviceView, 0, len(ds)),
		Drivers: make([]driverOption, 0, len(drs)),
	}
	for _, d := range ds {
		data.Devices = append(data.Devices, deviceView{
			ID:       d.ID,
			Name:     d.Name,
			ChipType: d.ChipType,
			Version:  d.ReportedVersion,
			DriverID: d.DriverID,
			Online:   d.Online,
			LastSeen: formatLastSeen(d.LastSeenAt),
		})
	}
	for _, d := range drs {
		data.Drivers = append(data.Drivers, driverOption{ID: d.ID, Name: d.Name})
	}
	return data, nil
}

func assignDriver(devices DeviceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := devices.Assign(r.Context(), chi.URLParam(r, "id"), r.FormValue("driver_id"))
		switch {
		case errors.Is(err, device.ErrDeviceNotFound):
			http.Error(w, "device not found", http.StatusNotFound)
		case errors.Is(err, device.ErrDriverNotFound):
			http.Error(w, "driver not found", http.StatusBadRequest)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}
}

func formatLastSeen(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}
