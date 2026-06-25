package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/topics"
)

type fakeRepo struct {
	deploys []Deploy
	targets map[string]Target
}

func newFakeRepo() *fakeRepo { return &fakeRepo{targets: map[string]Target{}} }

func (r *fakeRepo) CreateDeploy(_ context.Context, d Deploy) error {
	r.deploys = append(r.deploys, d)
	return nil
}

func (r *fakeRepo) AddTarget(_ context.Context, t Target) error {
	r.targets[t.DeviceID] = t
	return nil
}

func (r *fakeRepo) SetTargetStatus(_ context.Context, _, deviceID string, status Status, at time.Time) error {
	t := r.targets[deviceID]
	t.Status = status
	t.UpdatedAt = at
	r.targets[deviceID] = t
	return nil
}

func (r *fakeRepo) AdvanceTargetStatus(_ context.Context, _, deviceID string, status Status, at time.Time) error {
	t := r.targets[deviceID]
	if t.Status == StatusSucceeded || t.Status == StatusFailed {
		return nil
	}
	t.Status = status
	t.UpdatedAt = at
	r.targets[deviceID] = t
	return nil
}

func (r *fakeRepo) LatestTargetForDevice(_ context.Context, deviceID string) (Target, bool, error) {
	t, ok := r.targets[deviceID]
	return t, ok, nil
}

type fakePublisher struct {
	sent map[string][]byte
	fail bool
}

func (p *fakePublisher) Publish(topic string, payload []byte) error {
	if p.fail {
		return errors.New("publish failed")
	}
	p.sent[topic] = payload
	return nil
}

type fakeDevices struct{ ids []string }

func (d fakeDevices) IDsForDriver(_ context.Context, _ string) ([]string, error) { return d.ids, nil }

type fakeArtifacts struct{ a artifact.Artifact }

func (f fakeArtifacts) Get(_ context.Context, _, _ string) (artifact.Artifact, error) {
	return f.a, nil
}

func newService(repo Repository, pub Publisher, devices DeviceSource, baseURL string) *Service {
	art := fakeArtifacts{a: artifact.Artifact{SHA256: "abc", Signature: "sig"}}
	return NewService(repo, devices, art, pub, baseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRolloutPublishesSignedCommandPerDevice(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"dev1", "dev2"}}, "https://espm.test")

	if err := svc.Rollout(context.Background(), "drv1", "v1.0.0"); err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if len(repo.deploys) != 1 {
		t.Fatalf("expected 1 deploy, got %d", len(repo.deploys))
	}
	for _, dev := range []string{"dev1", "dev2"} {
		payload, ok := pub.sent[topics.CmdOTA(dev)]
		if !ok {
			t.Fatalf("no ota command for %s", dev)
		}
		var cmd otaCommand
		if err := json.Unmarshal(payload, &cmd); err != nil {
			t.Fatalf("unmarshal cmd: %v", err)
		}
		if cmd.Version != "v1.0.0" || cmd.SHA256 != "abc" || cmd.Signature != "sig" {
			t.Fatalf("unexpected cmd: %+v", cmd)
		}
		if cmd.URL != "https://espm.test/firmware/drv1/v1.0.0.bin" {
			t.Fatalf("unexpected url: %s", cmd.URL)
		}
		if repo.targets[dev].Status != StatusTriggered {
			t.Fatalf("expected triggered, got %s", repo.targets[dev].Status)
		}
	}
}

func TestRolloutWithoutPublicURLFails(t *testing.T) {
	svc := newService(newFakeRepo(), &fakePublisher{sent: map[string][]byte{}}, fakeDevices{ids: []string{"dev1"}}, "")
	if err := svc.Rollout(context.Background(), "drv1", "v1.0.0"); !errors.Is(err, ErrNoPublicURL) {
		t.Fatalf("expected ErrNoPublicURL, got %v", err)
	}
}

func TestRolloutNoDevicesSkips(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	if err := newService(repo, pub, fakeDevices{ids: nil}, "https://espm.test").Rollout(context.Background(), "drv1", "v1.0.0"); err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if len(repo.deploys) != 0 || len(pub.sent) != 0 {
		t.Fatalf("expected no deploy/publish for empty device set")
	}
}

func TestRolloutPublishFailureMarksTargetFailed(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}, fail: true}
	err := newService(repo, pub, fakeDevices{ids: []string{"dev1"}}, "https://espm.test").Rollout(context.Background(), "drv1", "v1.0.0")
	if err == nil {
		t.Fatal("expected aggregated rollout error")
	}
	if repo.targets["dev1"].Status != StatusFailed {
		t.Fatalf("expected failed target, got %s", repo.targets["dev1"].Status)
	}
}

func TestOnStatusMapsReportedStateAndRespectsTerminal(t *testing.T) {
	repo := newFakeRepo()
	repo.targets["dev1"] = Target{DeployID: "d1", DeviceID: "dev1", Version: "v2", Status: StatusTriggered}
	svc := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, "https://espm.test")

	svc.OnStatus(context.Background(), "dev1", []byte(`{"status":"FAILED"}`))
	if repo.targets["dev1"].Status != StatusFailed {
		t.Fatalf("expected failed, got %s", repo.targets["dev1"].Status)
	}

	svc.OnStatus(context.Background(), "dev1", []byte(`{"status":"ok"}`))
	if repo.targets["dev1"].Status != StatusFailed {
		t.Fatalf("terminal status must not change, got %s", repo.targets["dev1"].Status)
	}
}
