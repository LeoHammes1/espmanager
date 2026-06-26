package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeoHammes1/espmanager/internal/deploy"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

type deviceRef struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
}

type failedRef struct {
	DeployID   string        `json:"deployId"`
	DeviceID   string        `json:"deviceId"`
	DeviceName string        `json:"deviceName"`
	Driver     string        `json:"driver"`
	Version    string        `json:"version"`
	Status     deploy.Status `json:"status"`
}

type overviewView struct {
	DevicesOnline  int         `json:"devicesOnline"`
	DevicesTotal   int         `json:"devicesTotal"`
	AttentionCount int         `json:"attentionCount"`
	Rollouts       []deployRow `json:"rollouts"`
	Offline        []deviceRef `json:"offline"`
	FailedUpdates  []failedRef `json:"failedUpdates"`
}

func apiOverview(deploys DeployService, drivers DriverService, devices DeviceService, threshold int, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := overviewData(r.Context(), deploys, drivers, devices, threshold)
		if err != nil {
			apiInternal(w, log, "overview failed", err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, data)
	}
}

func overviewData(ctx context.Context, deploys DeployService, drivers DriverService, devices DeviceService, threshold int) (overviewView, error) {
	b, ds, err := newDeployBuilder(ctx, drivers, devices, threshold)
	if err != nil {
		return overviewView{}, err
	}

	v := overviewView{
		DevicesTotal:  len(ds),
		Rollouts:      []deployRow{},
		Offline:       []deviceRef{},
		FailedUpdates: []failedRef{},
	}
	attention := make(map[string]struct{})
	for _, d := range ds {
		if d.Online {
			v.DevicesOnline++
			continue
		}
		ref := deviceRef{ID: d.ID, Name: d.Name}
		if !d.LastSeenAt.IsZero() {
			t := d.LastSeenAt
			ref.LastSeenAt = &t
		}
		v.Offline = append(v.Offline, ref)
		attention[d.ID] = struct{}{}
	}

	list, err := deploys.ListDeploys(ctx)
	if err != nil {
		return overviewView{}, err
	}
	for _, d := range list {
		if !activeDeploy(d.State) {
			continue
		}
		targets, err := deploys.Targets(ctx, d.ID)
		if err != nil {
			return overviewView{}, err
		}
		v.Rollouts = append(v.Rollouts, b.row(d, targets))
		for _, t := range targets {
			if t.Status != deploy.StatusFailed && t.Status != deploy.StatusLost {
				continue
			}
			v.FailedUpdates = append(v.FailedUpdates, failedRef{
				DeployID:   d.ID,
				DeviceID:   t.DeviceID,
				DeviceName: b.deviceNm[t.DeviceID],
				Driver:     labelOrID(d.DriverID, b.drivers[d.DriverID]),
				Version:    d.Version,
				Status:     t.Status,
			})
			attention[t.DeviceID] = struct{}{}
		}
	}
	v.AttentionCount = len(attention)
	return v, nil
}
