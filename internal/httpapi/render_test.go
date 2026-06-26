package httpapi

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
