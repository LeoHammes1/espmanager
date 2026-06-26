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
	ID         string
	Name       string
	Version    string
	LastSeenAt time.Time
	DriverID   string
	DriverName string
	Online     bool
}

type driverOption struct {
	ID   string
	Name string
}

type devicesData struct {
	Devices []deviceView
	Drivers []driverOption
}

func devicesPage(devices DeviceService, drivers DriverService, tmpl *template.Template, user string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := devicesDataFor(r.Context(), devices, drivers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderShell(w, tmpl, pageView{
			Title:   "Devices",
			Nav:     "devices",
			User:    user,
			Content: "page-devices",
			Data:    data,
		})
	}
}

func devicesRows(devices DeviceService, drivers DriverService, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := devicesDataFor(r.Context(), devices, drivers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, tmpl, "devices-rows", data)
	}
}

func devicesDataFor(ctx context.Context, devices DeviceService, drivers DriverService) (devicesData, error) {
	ds, err := devices.List(ctx)
	if err != nil {
		return devicesData{}, err
	}
	drs, err := drivers.List(ctx)
	if err != nil {
		return devicesData{}, err
	}

	names := make(map[string]string, len(drs))
	data := devicesData{
		Devices: make([]deviceView, 0, len(ds)),
		Drivers: make([]driverOption, 0, len(drs)),
	}
	for _, d := range drs {
		names[d.ID] = d.Name
		data.Drivers = append(data.Drivers, driverOption{ID: d.ID, Name: d.Name})
	}
	for _, d := range ds {
		data.Devices = append(data.Devices, deviceView{
			ID:         d.ID,
			Name:       d.Name,
			Version:    d.ReportedVersion,
			LastSeenAt: d.LastSeenAt,
			DriverID:   d.DriverID,
			DriverName: names[d.DriverID],
			Online:     d.Online,
		})
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
			http.Redirect(w, r, "/devices", http.StatusSeeOther)
		}
	}
}
