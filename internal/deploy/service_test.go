package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/topics"
)

type fakeRepo struct {
	deploys map[string]*Deploy
	targets []Target
}

func newFakeRepo() *fakeRepo { return &fakeRepo{deploys: map[string]*Deploy{}} }

func (r *fakeRepo) CreateDeploy(_ context.Context, d Deploy) error {
	dd := d
	r.deploys[d.ID] = &dd
	return nil
}

func (r *fakeRepo) AddTarget(_ context.Context, t Target) error {
	r.targets = append(r.targets, t)
	return nil
}

func (r *fakeRepo) AdvanceTargetStatus(_ context.Context, _, deviceID string, status Status, at time.Time) (int64, error) {
	for i := range r.targets {
		if r.targets[i].DeviceID != deviceID {
			continue
		}
		if terminal(r.targets[i].Status) {
			return 0, nil
		}
		r.targets[i].Status = status
		r.targets[i].UpdatedAt = at
		return 1, nil
	}
	return 0, nil
}

func (r *fakeRepo) LatestTargetForDevice(_ context.Context, deviceID string) (Target, bool, error) {
	for i := range r.targets {
		if r.targets[i].DeviceID == deviceID {
			return r.targets[i], true, nil
		}
	}
	return Target{}, false, nil
}

func (r *fakeRepo) ListActiveDeploys(_ context.Context) ([]Deploy, error) {
	var out []Deploy
	for _, d := range r.deploys {
		if d.State == StateInProgress {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (r *fakeRepo) TargetsForDeploy(_ context.Context, deployID string) ([]Target, error) {
	var out []Target
	for _, t := range r.targets {
		if t.DeployID == deployID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *fakeRepo) SetDeployState(_ context.Context, deployID string, state State) error {
	if d, ok := r.deploys[deployID]; ok {
		d.State = state
	}
	return nil
}

func (r *fakeRepo) statusOf(deviceID string) Status {
	for _, t := range r.targets {
		if t.DeviceID == deviceID {
			return t.Status
		}
	}
	return ""
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

func newService(repo Repository, pub Publisher, devices DeviceSource, baseURL string, opts Options) *Service {
	art := fakeArtifacts{a: artifact.Artifact{SHA256: "abc", Signature: "sig"}}
	return NewService(repo, devices, art, pub, baseURL, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRolloutTriggersOnlyCanaryBatch(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1", "d2", "d3", "d4"}}, "https://espm.test", Options{CanaryPercent: 25, FailureThreshold: 20})

	if err := svc.Rollout(context.Background(), "drv", "v1"); err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if _, ok := pub.sent[topics.CmdOTA("d1")]; !ok {
		t.Fatal("canary device d1 should be triggered")
	}
	for _, dev := range []string{"d2", "d3", "d4"} {
		if _, ok := pub.sent[topics.CmdOTA(dev)]; ok {
			t.Fatalf("non-canary device %s must not be triggered yet", dev)
		}
		if repo.statusOf(dev) != StatusPending {
			t.Fatalf("device %s should stay pending, got %s", dev, repo.statusOf(dev))
		}
	}
}

func TestReconcilePromotesAfterCanarySuccess(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1", "d2", "d3", "d4"}}, "https://espm.test", Options{CanaryPercent: 25, FailureThreshold: 20})
	_ = svc.Rollout(context.Background(), "drv", "v1")

	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"ok"}`))
	svc.Reconcile(context.Background())

	for _, dev := range []string{"d2", "d3", "d4"} {
		if repo.statusOf(dev) != StatusTriggered {
			t.Fatalf("device %s should be triggered after canary success, got %s", dev, repo.statusOf(dev))
		}
	}

	for _, dev := range []string{"d2", "d3", "d4"} {
		svc.OnStatus(context.Background(), dev, []byte(`{"status":"ok"}`))
	}
	svc.Reconcile(context.Background())
	for _, d := range repo.deploys {
		if d.State != StateCompleted {
			t.Fatalf("deploy should be completed, got %s", d.State)
		}
	}
}

func TestReconcilePausesOnCanaryFailure(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1", "d2", "d3", "d4"}}, "https://espm.test", Options{CanaryPercent: 25, FailureThreshold: 20})
	_ = svc.Rollout(context.Background(), "drv", "v1")

	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"failed"}`))
	svc.Reconcile(context.Background())

	for _, d := range repo.deploys {
		if d.State != StatePaused {
			t.Fatalf("deploy should be paused on canary failure, got %s", d.State)
		}
	}
	if _, ok := pub.sent[topics.CmdOTA("d2")]; ok {
		t.Fatal("must not promote next batch after canary failure")
	}
}

func TestReconcileMarksLostAfterTimeout(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1"}}, "https://espm.test", Options{CanaryPercent: 100, FailureThreshold: 20, TargetTimeout: time.Minute})

	base := time.Unix(1000, 0).UTC()
	svc.now = func() time.Time { return base }
	_ = svc.Rollout(context.Background(), "drv", "v1")

	svc.now = func() time.Time { return base.Add(2 * time.Minute) }
	svc.Reconcile(context.Background())

	if repo.statusOf("d1") != StatusLost {
		t.Fatalf("expected lost after timeout, got %s", repo.statusOf("d1"))
	}
	for _, d := range repo.deploys {
		if d.State != StatePaused {
			t.Fatalf("single lost canary should pause deploy, got %s", d.State)
		}
	}
}

func TestRolloutWithoutPublicURLFails(t *testing.T) {
	svc := newService(newFakeRepo(), &fakePublisher{sent: map[string][]byte{}}, fakeDevices{ids: []string{"d1"}}, "", Options{})
	if err := svc.Rollout(context.Background(), "drv", "v1"); !errors.Is(err, ErrNoPublicURL) {
		t.Fatalf("expected ErrNoPublicURL, got %v", err)
	}
}
