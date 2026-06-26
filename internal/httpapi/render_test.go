package httpapi

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/LeoHammes1/espmanager/internal/deploy"
)

func TestParseTemplatesRendersShellAndContent(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	devices := devicesData{
		Devices: []deviceView{
			{ID: "esp32-abc", Name: "lab-sensor", Version: "1.2.0", Online: true, DriverID: "drv1", DriverName: "sensor-fw"},
			{ID: "esp32-def", LastSeenAt: time.Now().Add(-2 * time.Hour)},
		},
		Drivers: []driverOption{{ID: "drv1", Name: "sensor-fw"}},
	}

	cases := []struct {
		name    string
		view    any
		wantSub string
	}{
		{"page-devices", pageView{Title: "Devices", Nav: "devices", User: "admin", Content: "page-devices", Data: devices}, "lab-sensor"},
		{"page-drivers", pageView{Title: "Drivers", Nav: "drivers", User: "admin", Content: "page-drivers", Data: driversData{Drivers: []driverView{{ID: "drv1", Name: "sensor-fw", RepoURL: "https://example.com/fw.git", Branch: "main", WebhookURL: "/webhook/git/drv1"}}}}, "sensor-fw"},
		{"page-driver-created", pageView{Title: "Driver created", Nav: "drivers", User: "admin", Content: "page-driver-created", Data: createdDriverData{Name: "fw", WebhookURL: "/webhook/git/x", WebhookSecret: "whsec_123"}}, "whsec_123"},
		{"page-device-enrolled", pageView{Title: "Device enrolled", Nav: "devices", User: "admin", Content: "page-device-enrolled", Data: map[string]string{"Token": "clm_abc", "ExpiresAt": "2026-01-01"}}, "clm_abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := tmpl.ExecuteTemplate(&b, "layout", tc.view); err != nil {
				t.Fatalf("execute layout(%s): %v", tc.name, err)
			}
			out := b.String()
			if !strings.Contains(out, "<!doctype html>") {
				t.Error("shell did not render the document")
			}
			if !strings.Contains(out, tc.wantSub) {
				t.Errorf("rendered shell missing %q", tc.wantSub)
			}
		})
	}
}

func TestDevicesRowsRendersOnlineState(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var b bytes.Buffer
	data := devicesData{Devices: []deviceView{{ID: "esp32-abc", Online: true}}}
	if err := tmpl.ExecuteTemplate(&b, "devices-rows", data); err != nil {
		t.Fatalf("execute devices-rows: %v", err)
	}
	if !strings.Contains(b.String(), `data-online="true"`) {
		t.Error("online device should render an online dot")
	}
}

func TestDevicesRowsEmptyState(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "devices-rows", devicesData{}); err != nil {
		t.Fatalf("execute devices-rows: %v", err)
	}
	if !strings.Contains(b.String(), "No devices yet") {
		t.Error("empty device list should render the empty state")
	}
}

func TestOverviewBodyRenders(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	v := overviewView{
		DevicesOnline: 2, DevicesTotal: 3, AttentionCount: 1,
		Rollouts: []deployRow{{
			ID: "d1", Driver: "sensor-fw", Version: "1.0.0", State: deploy.StatePaused,
			Counts: deployCounts{Total: 5, Succeeded: 2, Failed: 1, Pending: 2},
		}},
		Offline:       []deviceRef{{ID: "esp32-x"}},
		FailedUpdates: []failedRef{{DeployID: "d1", DeviceID: "esp32-x", Driver: "sensor-fw", Version: "1.0.0", Status: deploy.StatusFailed}},
	}
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "overview-body", v); err != nil {
		t.Fatalf("execute overview-body: %v", err)
	}
	out := b.String()
	for _, want := range []string{"Active rollouts", "Paused", "Resume", "width:40%", "Needs attention"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview-body missing %q", want)
		}
	}
	if strings.Contains(out, "ZgotmplZ") {
		t.Error("progress bar width was rejected by the template CSS sanitizer")
	}
}

func TestDeploysAndDetailRender(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	list := deploysListData{Deploys: []deployRow{{
		ID: "d1", Driver: "fw", Version: "2.0.0", State: deploy.StateInProgress,
		Counts: deployCounts{Total: 4, Succeeded: 1, Inflight: 1, Failed: 1, Pending: 1},
	}}}
	var lb bytes.Buffer
	if err := tmpl.ExecuteTemplate(&lb, "deploys-rows", list); err != nil {
		t.Fatalf("execute deploys-rows: %v", err)
	}
	if !strings.Contains(lb.String(), "at-risk-row") {
		t.Error("a deploy with a failed target should carry the at-risk gutter")
	}
	if !strings.Contains(lb.String(), "1 failed") {
		t.Error("deploys list should surface the failed counter")
	}

	detail := deployDetailView{
		ID: "d1", Driver: "fw", Version: "2.0.0", State: deploy.StatePaused,
		Counts:  deployCounts{Total: 2, Succeeded: 0, Failed: 1, Pending: 1},
		Pause:   &pauseInfo{BatchLabel: "Canary", Failed: 1, Total: 1, Threshold: 20},
		Batches: []batchView{{Batch: 0, Label: "Canary", Counts: deployCounts{Total: 1, Failed: 1}, Targets: []targetRow{{DeviceID: "esp32-x", Sequence: 7, Status: deploy.StatusFailed}}}},
	}
	var db bytes.Buffer
	if err := tmpl.ExecuteTemplate(&db, "deploy-body", detail); err != nil {
		t.Fatalf("execute deploy-body: %v", err)
	}
	out := db.String()
	for _, want := range []string{"Auto-paused", "Canary", "Resume", "Cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("deploy-body missing %q", want)
		}
	}
}

func TestLoginTemplateRenders(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "login", loginView{Error: "Wrong password. Try again."}); err != nil {
		t.Fatalf("execute login: %v", err)
	}
	if !strings.Contains(b.String(), "Wrong password") {
		t.Error("login should render the error banner")
	}
}
