package httpapi

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LeoHammes1/espmanager/internal/deploy"
	"github.com/LeoHammes1/espmanager/internal/device"
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

func (c deployCounts) SucceededPct() int { return c.pct(c.Succeeded) }
func (c deployCounts) InflightPct() int  { return c.pct(c.Inflight) }
func (c deployCounts) FailedPct() int    { return c.pct(c.Failed) }
func (c deployCounts) LostPct() int      { return c.pct(c.Lost) }

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
	ID        string
	Driver    string
	Version   string
	State     deploy.State
	Counts    deployCounts
	CreatedAt time.Time
}

func (r deployRow) StateText() string { return stateText(r.State) }

type targetRow struct {
	DeviceID   string
	DeviceName string
	Sequence   int64
	Batch      int
	Status     deploy.Status
	UpdatedAt  time.Time
}

type batchView struct {
	Batch   int
	Label   string
	Counts  deployCounts
	Targets []targetRow
}

type pauseInfo struct {
	BatchLabel string
	Failed     int
	Lost       int
	Total      int
	Threshold  int
}

type deployDetailView struct {
	ID        string
	Driver    string
	Version   string
	State     deploy.State
	CreatedAt time.Time
	Counts    deployCounts
	Batches   []batchView
	Pause     *pauseInfo
}

func (v deployDetailView) StateText() string { return stateText(v.State) }

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

// deployBuilder turns deploy + target rows into view models, resolving driver and
// device names from the current fleet.
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
		CreatedAt: d.CreatedAt,
		Counts:    countTargets(targets),
	}
	for _, batch := range deploy.Batches(targets) {
		group := deploy.TargetsInBatch(targets, batch)
		bv := batchView{Batch: batch, Label: batchLabel(batch), Counts: countTargets(group)}
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

func deploysPage(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, user string, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := deploysData(r.Context(), deploys, drivers, devices, threshold)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderShell(w, tmpl, pageView{Title: "Deploys", Nav: "deploys", User: user, Content: "page-deploys", Data: data})
	}
}

func deploysRows(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := deploysData(r.Context(), deploys, drivers, devices, threshold)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, tmpl, "deploys-rows", data)
	}
}

type deploysListData struct {
	Deploys []deployRow
}

func deploysData(ctx context.Context, deploys DeployService, drivers DriverService, devices DeviceService, threshold int) (deploysListData, error) {
	b, _, err := newDeployBuilder(ctx, drivers, devices, threshold)
	if err != nil {
		return deploysListData{}, err
	}
	list, err := deploys.ListDeploys(ctx)
	if err != nil {
		return deploysListData{}, err
	}
	data := deploysListData{Deploys: make([]deployRow, 0, len(list))}
	for _, d := range list {
		targets, err := deploys.Targets(ctx, d.ID)
		if err != nil {
			return deploysListData{}, err
		}
		data.Deploys = append(data.Deploys, b.row(d, targets))
	}
	return data, nil
}

func deployDetailPage(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, user string, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := deployDetailData(r.Context(), chi.URLParam(r, "id"), deploys, drivers, devices, threshold)
		if errors.Is(err, deploy.ErrDeployNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderShell(w, tmpl, pageView{Title: "Deploy " + v.Version, Nav: "deploys", User: user, Content: "page-deploy", Data: v})
	}
}

func deployTargets(deploys DeployService, drivers DriverService, devices DeviceService, tmpl *template.Template, threshold int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := deployDetailData(r.Context(), chi.URLParam(r, "id"), deploys, drivers, devices, threshold)
		if errors.Is(err, deploy.ErrDeployNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, tmpl, "deploy-body", v)
	}
}

func deployDetailData(ctx context.Context, id string, deploys DeployService, drivers DriverService, devices DeviceService, threshold int) (deployDetailView, error) {
	b, _, err := newDeployBuilder(ctx, drivers, devices, threshold)
	if err != nil {
		return deployDetailView{}, err
	}
	d, targets, err := deploys.DeployDetail(ctx, id)
	if err != nil {
		return deployDetailView{}, err
	}
	return b.detail(d, targets), nil
}

func resumeDeploy(deploys DeployService) http.HandlerFunc {
	return deployAction(deploys.Resume)
}

func cancelDeploy(deploys DeployService) http.HandlerFunc {
	return deployAction(deploys.Cancel)
}

func deployAction(action func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		err := action(r.Context(), id)
		switch {
		case errors.Is(err, deploy.ErrDeployNotFound):
			http.NotFound(w, r)
		case errors.Is(err, deploy.ErrNotPaused), errors.Is(err, deploy.ErrNotCancellable):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			http.Redirect(w, r, actionDest(r, id), http.StatusSeeOther)
		}
	}
}

// actionDest returns the operator to the Overview when the action originated
// there, otherwise to the deploy detail page.
func actionDest(r *http.Request, id string) string {
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Path == "/" {
			return "/"
		}
	}
	return "/deploys/" + id
}
