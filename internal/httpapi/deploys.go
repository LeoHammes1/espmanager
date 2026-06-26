package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/deploy"
	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/httpx"
)

type deployCounts struct {
	Total     int
	Succeeded int
	Inflight  int
	Failed    int
	Lost      int
	Pending   int
}

func countTargets(ts []deploy.Target) deployCounts {
	c := deployCounts{Total: len(ts)}
	for _, t := range ts {
		switch t.Status {
		case deploy.StatusSucceeded:
			c.Succeeded++
		case deploy.StatusTriggered, deploy.StatusDownloading:
			c.Inflight++
		case deploy.StatusFailed:
			c.Failed++
		case deploy.StatusLost:
			c.Lost++
		default:
			c.Pending++
		}
	}
	return c
}

func (c deployCounts) AtRisk() bool { return c.Failed+c.Lost > 0 }

func (c deployCounts) pct(n int) int {
	if c.Total == 0 {
		return 0
	}
	return n * 100 / c.Total
}

func (c deployCounts) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Total        int  `json:"total"`
		Succeeded    int  `json:"succeeded"`
		Inflight     int  `json:"inflight"`
		Failed       int  `json:"failed"`
		Lost         int  `json:"lost"`
		Pending      int  `json:"pending"`
		AtRisk       bool `json:"atRisk"`
		SucceededPct int  `json:"succeededPct"`
		InflightPct  int  `json:"inflightPct"`
		FailedPct    int  `json:"failedPct"`
		LostPct      int  `json:"lostPct"`
	}{
		c.Total, c.Succeeded, c.Inflight, c.Failed, c.Lost, c.Pending,
		c.AtRisk(), c.pct(c.Succeeded), c.pct(c.Inflight), c.pct(c.Failed), c.pct(c.Lost),
	})
}

func activeDeploy(s deploy.State) bool {
	return s == deploy.StateInProgress || s == deploy.StatePaused
}

func stateText(s deploy.State) string {
	switch s {
	case deploy.StateInProgress:
		return "Deploying"
	case deploy.StatePaused:
		return "Paused"
	case deploy.StateCompleted:
		return "Completed"
	case deploy.StateCancelled:
		return "Cancelled"
	default:
		return string(s)
	}
}

type deployRow struct {
	ID        string       `json:"id"`
	Driver    string       `json:"driver"`
	Version   string       `json:"version"`
	State     deploy.State `json:"state"`
	StateText string       `json:"stateText"`
	Counts    deployCounts `json:"counts"`
	CreatedAt time.Time    `json:"createdAt"`
}

type targetRow struct {
	DeviceID   string        `json:"deviceId"`
	DeviceName string        `json:"deviceName"`
	Sequence   int64         `json:"sequence,string"`
	Batch      int           `json:"batch"`
	Status     deploy.Status `json:"status"`
	UpdatedAt  time.Time     `json:"updatedAt"`
}

type batchView struct {
	Batch   int          `json:"batch"`
	Label   string       `json:"label"`
	Counts  deployCounts `json:"counts"`
	Targets []targetRow  `json:"targets"`
}

type pauseInfo struct {
	BatchLabel string `json:"batchLabel"`
	Failed     int    `json:"failed"`
	Lost       int    `json:"lost"`
	Total      int    `json:"total"`
	Threshold  int    `json:"threshold"`
}

type deployDetailView struct {
	ID        string       `json:"id"`
	Driver    string       `json:"driver"`
	Version   string       `json:"version"`
	State     deploy.State `json:"state"`
	StateText string       `json:"stateText"`
	CreatedAt time.Time    `json:"createdAt"`
	Counts    deployCounts `json:"counts"`
	Batches   []batchView  `json:"batches"`
	Pause     *pauseInfo   `json:"pause"`
}

func batchLabel(batch int) string {
	if batch == 0 {
		return "Canary"
	}
	return "Rest"
}

func labelOrID(id, name string) string {
	if name != "" {
		return name
	}
	return id
}

// deployBuilder turns deploy + target rows into view models, resolving driver
// and device names from the current fleet.
type deployBuilder struct {
	drivers   map[string]string
	deviceNm  map[string]string
	threshold int
}

func newDeployBuilder(ctx context.Context, drivers DriverService, devices DeviceService, threshold int) (deployBuilder, []device.Device, error) {
	drs, err := drivers.List(ctx)
	if err != nil {
		return deployBuilder{}, nil, err
	}
	ds, err := devices.List(ctx)
	if err != nil {
		return deployBuilder{}, nil, err
	}
	b := deployBuilder{
		drivers:   make(map[string]string, len(drs)),
		deviceNm:  make(map[string]string, len(ds)),
		threshold: threshold,
	}
	for _, d := range drs {
		b.drivers[d.ID] = d.Name
	}
	for _, d := range ds {
		b.deviceNm[d.ID] = d.Name
	}
	return b, ds, nil
}

func (b deployBuilder) row(d deploy.Deploy, targets []deploy.Target) deployRow {
	return deployRow{
		ID:        d.ID,
		Driver:    labelOrID(d.DriverID, b.drivers[d.DriverID]),
		Version:   d.Version,
		State:     d.State,
		StateText: stateText(d.State),
		Counts:    countTargets(targets),
		CreatedAt: d.CreatedAt,
	}
}

func (b deployBuilder) detail(d deploy.Deploy, targets []deploy.Target) deployDetailView {
	v := deployDetailView{
		ID:        d.ID,
		Driver:    labelOrID(d.DriverID, b.drivers[d.DriverID]),
		Version:   d.Version,
		State:     d.State,
		StateText: stateText(d.State),
		CreatedAt: d.CreatedAt,
		Counts:    countTargets(targets),
		Batches:   []batchView{},
	}
	for _, batch := range deploy.Batches(targets) {
		group := deploy.TargetsInBatch(targets, batch)
		bv := batchView{Batch: batch, Label: batchLabel(batch), Counts: countTargets(group), Targets: []targetRow{}}
		for _, t := range group {
			bv.Targets = append(bv.Targets, targetRow{
				DeviceID:   t.DeviceID,
				DeviceName: b.deviceNm[t.DeviceID],
				Sequence:   t.Sequence,
				Batch:      t.Batch,
				Status:     t.Status,
				UpdatedAt:  t.UpdatedAt,
			})
		}
		v.Batches = append(v.Batches, bv)
		if v.Pause == nil && d.State == deploy.StatePaused && deploy.BatchTripped(group, b.threshold) {
			v.Pause = &pauseInfo{
				BatchLabel: bv.Label,
				Failed:     bv.Counts.Failed,
				Lost:       bv.Counts.Lost,
				Total:      bv.Counts.Total,
				Threshold:  b.threshold,
			}
		}
	}
	return v
}

func apiDeploys(deploys DeployService, drivers DriverService, devices DeviceService, threshold int, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _, err := newDeployBuilder(r.Context(), drivers, devices, threshold)
		if err != nil {
			apiInternal(w, log, "load fleet failed", err)
			return
		}
		list, err := deploys.ListDeploys(r.Context())
		if err != nil {
			apiInternal(w, log, "list deploys failed", err)
			return
		}
		rows := make([]deployRow, 0, len(list))
		for _, d := range list {
			targets, err := deploys.Targets(r.Context(), d.ID)
			if err != nil {
				apiInternal(w, log, "load deploy targets failed", err)
				return
			}
			rows = append(rows, b.row(d, targets))
		}
		httpx.WriteJSON(w, http.StatusOK, rows)
	}
}

func apiDeployDetail(deploys DeployService, drivers DriverService, devices DeviceService, threshold int, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _, err := newDeployBuilder(r.Context(), drivers, devices, threshold)
		if err != nil {
			apiInternal(w, log, "load fleet failed", err)
			return
		}
		d, targets, err := deploys.DeployDetail(r.Context(), chi.URLParam(r, "id"))
		if errors.Is(err, deploy.ErrDeployNotFound) {
			apiErr(w, http.StatusNotFound, "not_found", "Deploy not found.")
			return
		}
		if err != nil {
			apiInternal(w, log, "load deploy failed", err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, b.detail(d, targets))
	}
}

func deployAction(action func(context.Context, string) error, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := action(r.Context(), chi.URLParam(r, "id"))
		switch {
		case errors.Is(err, deploy.ErrDeployNotFound):
			apiErr(w, http.StatusNotFound, "not_found", "Deploy not found.")
		case errors.Is(err, deploy.ErrNotPaused), errors.Is(err, deploy.ErrNotCancellable):
			apiErr(w, http.StatusConflict, "conflict", err.Error())
		case err != nil:
			apiInternal(w, log, "deploy action failed", err)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
