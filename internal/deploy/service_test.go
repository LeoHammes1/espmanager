package deploy

import (
	"context"
	"encoding/json"
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

func (r *fakeRepo) LatestTargetForDevice(_ context.Context, deviceID string) (Target, bool, error) {
	t, ok := r.targets[deviceID]
	return t, ok, nil
}

type fakePublisher struct{ sent map[string][]byte }

func (p *fakePublisher) Publish(topic string, payload []byte) error {
	p.sent[topic] = payload
	return nil
}

type fakeDevices struct{ ids []string }

func (d fakeDevices) IDsForDriver(_ context.Context, _ string) ([]string, error) {
	return d.ids, nil
}

type fakeArtifacts struct{ a artifact.Artifact }

func (f fakeArtifacts) Get(_ context.Context, _, _ string) (artifact.Artifact, error) {
	return f.a, nil
}

func newService(repo Repository, pub Publisher, devices DeviceSource, artifacts ArtifactSource) *Service {
	return NewService(repo, devices, artifacts, pub, "https://espm.test", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRolloutPublishesSignedCommandPerDevice(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	art := fakeArtifacts{a: artifact.Artifact{SHA256: "abc", Signature: "sig"}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"dev1", "dev2"}}, art)

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

func TestRolloutNoDevicesSkips(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: nil}, fakeArtifacts{})

	if err := svc.Rollout(context.Background(), "drv1", "v1.0.0"); err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if len(repo.deploys) != 0 || len(pub.sent) != 0 {
		t.Fatalf("expected no deploy/publish for empty device set")
	}
}

func TestOnHeartbeatMarksSucceededOnMatchingVersion(t *testing.T) {
	repo := newFakeRepo()
	repo.targets["dev1"] = Target{DeployID: "d1", DeviceID: "dev1", Version: "v2", Status: StatusTriggered}
	svc := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, fakeArtifacts{})

	svc.OnHeartbeat(context.Background(), "dev1", "v1")
	if repo.targets["dev1"].Status != StatusTriggered {
		t.Fatalf("wrong version should not succeed")
	}

	svc.OnHeartbeat(context.Background(), "dev1", "v2")
	if repo.targets["dev1"].Status != StatusSucceeded {
		t.Fatalf("matching version should succeed, got %s", repo.targets["dev1"].Status)
	}
}

func TestOnStatusMapsReportedState(t *testing.T) {
	repo := newFakeRepo()
	repo.targets["dev1"] = Target{DeployID: "d1", DeviceID: "dev1", Version: "v2", Status: StatusTriggered}
	svc := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, fakeArtifacts{})

	svc.OnStatus(context.Background(), "dev1", []byte(`{"status":"failed"}`))
	if repo.targets["dev1"].Status != StatusFailed {
		t.Fatalf("expected failed, got %s", repo.targets["dev1"].Status)
	}

	svc.OnStatus(context.Background(), "dev1", []byte(`{"status":"ok"}`))
	if repo.targets["dev1"].Status != StatusFailed {
		t.Fatalf("terminal status must not change, got %s", repo.targets["dev1"].Status)
	}
}
