package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

type deviceView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Version    string     `json:"reportedVersion"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	DriverID   string     `json:"driverId"`
	DriverName string     `json:"driverName"`
	Online     bool       `json:"online"`
}

type driverOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type devicesData struct {
	Devices []deviceView   `json:"devices"`
	Drivers []driverOption `json:"drivers"`
}

func apiDevices(devices DeviceService, drivers DriverService, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := devicesDataFor(r.Context(), devices, drivers)
		if err != nil {
			apiInternal(w, log, "list devices failed", err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, data)
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
		var lastSeen *time.Time
		if !d.LastSeenAt.IsZero() {
			t := d.LastSeenAt
			lastSeen = &t
		}
		data.Devices = append(data.Devices, deviceView{
			ID:         d.ID,
			Name:       d.Name,
			Version:    d.ReportedVersion,
			LastSeenAt: lastSeen,
			DriverID:   d.DriverID,
			DriverName: names[d.DriverID],
			Online:     d.Online,
		})
	}
	return data, nil
}

func apiAssignDriver(devices DeviceService, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DriverID string `json:"driverId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid_request", "Malformed request.")
			return
		}
		err := devices.Assign(r.Context(), chi.URLParam(r, "id"), req.DriverID)
		switch {
		case errors.Is(err, device.ErrDeviceNotFound):
			apiErr(w, http.StatusNotFound, "not_found", "Device not found.")
		case errors.Is(err, device.ErrDriverNotFound):
			apiErr(w, http.StatusBadRequest, "invalid_driver", "Driver not found.")
		case err != nil:
			apiInternal(w, log, "assign driver failed", err)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
