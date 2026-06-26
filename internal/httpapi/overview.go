package httpapi

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"github.com/LeoHammes1/espmanager/internal/deploy"
)

type deviceRef struct {
	ID         string
	Name       string
	LastSeenAt time.Time
}

type failedRef struct {
	DeployID   string
	DeviceID   string
	DeviceName string
	Driver     string
	Version    string
	Status     deploy.Status
}

type overviewView struct {
	DevicesOnline  int
	DevicesTotal   int
	AttentionCount int
	Rollouts       []deployRow
	Offline        []deviceRef
	FailedUpdates  []failedRef
}

func overviewPage(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, user string, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := overviewData(r.Context(), deploys, drivers, devices, threshold)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderShell(w, tmpl, pageView{Title: "Overview", Nav: "overview", User: user, Content: "page-overview", Data: data})
	}
}

func overviewBody(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := overviewData(r.Context(), deploys, drivers, devices, threshold)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, tmpl, "overview-body", data)
	}
}

func overviewData(ctx context.Context, deploys DeployService, drivers DriverService, devices DeviceService, threshold int) (overviewView, error) {
	b, ds, err := newDeployBuilder(ctx, drivers, devices, threshold)
	if err != nil {
		return overviewView{}, err
	}

	v := overviewView{DevicesTotal: len(ds)}
	attention := make(map[string]struct{})
	for _, d := range ds {
		if d.Online {
			v.DevicesOnline++
			continue
		}
		v.Offline = append(v.Offline, deviceRef{ID: d.ID, Name: d.Name, LastSeenAt: d.LastSeenAt})
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
