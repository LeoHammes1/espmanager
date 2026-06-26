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
		if terminal(r.targets[i].Status) || r.targets[i].Status == status {
			return 0, nil
		}
		r.targets[i].Status = status
		r.targets[i].UpdatedAt = at
		return 1, nil
	}
	return 0, nil
}

func (r *fakeRepo) AdvanceTargetStatusBySequence(_ context.Context, deviceID string, sequence int64, status Status, at time.Time) (int64, error) {
	for i := range r.targets {
		if r.targets[i].DeviceID != deviceID || r.targets[i].Sequence != sequence {
			continue
		}
		if terminal(r.targets[i].Status) || r.targets[i].Status == status {
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

func (r *fakeRepo) ListDeploys(_ context.Context) ([]Deploy, error) {
	out := make([]Deploy, 0, len(r.deploys))
	for _, d := range r.deploys {
		out = append(out, *d)
	}
	return out, nil
}

func (r *fakeRepo) GetDeploy(_ context.Context, deployID string) (Deploy, bool, error) {
	if d, ok := r.deploys[deployID]; ok {
		return *d, true, nil
	}
	return Deploy{}, false, nil
}

func (r *fakeRepo) ResetFailedTargets(_ context.Context, deployID string, at time.Time) (int64, error) {
	var n int64
	for i := range r.targets {
		if r.targets[i].DeployID == deployID && (r.targets[i].Status == StatusFailed || r.targets[i].Status == StatusLost) {
			r.targets[i].Status = StatusPending
			r.targets[i].UpdatedAt = at
			n++
		}
	}
	return n, nil
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

type fakeNotifier struct{ count int }

func (f *fakeNotifier) Changed() { f.count++ }

func newService(repo Repository, pub Publisher, devices DeviceSource, baseURL string, opts Options) *Service {
	art := fakeArtifacts{a: artifact.Artifact{SHA256: "abc", Signature: "sig", Sequence: 1}}
	return NewService(repo, devices, art, pub, &fakeNotifier{}, baseURL, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestBatchTripped(t *testing.T) {
	cases := []struct {
		name      string
		targets   []Target
		threshold int
		want      bool
	}{
		{"all terminal over threshold", []Target{{Status: StatusFailed}, {Status: StatusSucceeded}, {Status: StatusSucceeded}}, 20, true},
		{"all terminal under threshold", []Target{{Status: StatusSucceeded}, {Status: StatusSucceeded}, {Status: StatusSucceeded}}, 20, false},
		{"not all terminal", []Target{{Status: StatusFailed}, {Status: StatusTriggered}}, 20, false},
		{"lost counts as failure", []Target{{Status: StatusLost}}, 20, true},
		{"empty", nil, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BatchTripped(tc.targets, tc.threshold); got != tc.want {
				t.Fatalf("BatchTripped = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCancelStopsActiveDeploy(t *testing.T) {
	repo := newFakeRepo()
	repo.deploys["d1"] = &Deploy{ID: "d1", State: StateInProgress}
	s := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, "http://x", Options{})

	if err := s.Cancel(context.Background(), "d1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if repo.deploys["d1"].State != StateCancelled {
		t.Fatalf("want cancelled, got %s", repo.deploys["d1"].State)
	}
}

func TestCancelRejectsTerminalDeploy(t *testing.T) {
	repo := newFakeRepo()
	repo.deploys["d1"] = &Deploy{ID: "d1", State: StateCompleted}
	s := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, "http://x", Options{})

	if err := s.Cancel(context.Background(), "d1"); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("want ErrNotCancellable, got %v", err)
	}
}

func TestResumeRequiresPaused(t *testing.T) {
	repo := newFakeRepo()
	repo.deploys["d1"] = &Deploy{ID: "d1", State: StateInProgress}
	s := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, "http://x", Options{})

	if err := s.Resume(context.Background(), "d1"); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("want ErrNotPaused, got %v", err)
	}
}

func TestResumeRetriesFailedTargets(t *testing.T) {
	repo := newFakeRepo()
	repo.deploys["d1"] = &Deploy{ID: "d1", State: StatePaused}
	repo.targets = []Target{
		{DeployID: "d1", DeviceID: "a", Status: StatusFailed},
		{DeployID: "d1", DeviceID: "b", Status: StatusLost},
		{DeployID: "d1", DeviceID: "c", Status: StatusSucceeded},
	}
	s := newService(repo, &fakePublisher{sent: map[string][]byte{}}, fakeDevices{}, "http://x", Options{})

	if err := s.Resume(context.Background(), "d1"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if repo.deploys["d1"].State != StateInProgress {
		t.Fatalf("want in_progress, got %s", repo.deploys["d1"].State)
	}
	if repo.statusOf("a") != StatusPending || repo.statusOf("b") != StatusPending {
		t.Fatal("failed and lost targets should be reset to pending")
	}
	if repo.statusOf("c") != StatusSucceeded {
		t.Fatal("succeeded target must not be touched")
	}
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

func TestOnStatusMatchesBySequence(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1"}}, "https://espm.test", Options{CanaryPercent: 100, FailureThreshold: 20})
	_ = svc.Rollout(context.Background(), "drv", "v1")

	// A report for a different sequence must not touch this target.
	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"failed","sequence":"999"}`))
	if repo.statusOf("d1") != StatusTriggered {
		t.Fatalf("stale-sequence report must not change the target, got %s", repo.statusOf("d1"))
	}
	// The matching sequence (artifact sequence is 1) advances it.
	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"ok","sequence":"1"}`))
	if repo.statusOf("d1") != StatusSucceeded {
		t.Fatalf("matching-sequence report should succeed the target, got %s", repo.statusOf("d1"))
	}
}

func TestReconcileResendsTriggeredCommand(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1"}}, "https://espm.test", Options{CanaryPercent: 100, FailureThreshold: 20, TargetTimeout: time.Hour})
	_ = svc.Rollout(context.Background(), "drv", "v1")

	delete(pub.sent, topics.CmdOTA("d1"))
	svc.Reconcile(context.Background())
	if _, ok := pub.sent[topics.CmdOTA("d1")]; !ok {
		t.Fatal("reconcile should re-deliver the command to a still-triggered device")
	}

	// Once the device acknowledges (downloading), the command is no longer resent.
	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"updating","sequence":"1"}`))
	if repo.statusOf("d1") != StatusDownloading {
		t.Fatalf("updating report should move target to downloading, got %s", repo.statusOf("d1"))
	}
	delete(pub.sent, topics.CmdOTA("d1"))
	svc.Reconcile(context.Background())
	if _, ok := pub.sent[topics.CmdOTA("d1")]; ok {
		t.Fatal("a downloading device must not be resent the command")
	}
}

func TestRepeatedDownloadingStillTimesOut(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{sent: map[string][]byte{}}
	svc := newService(repo, pub, fakeDevices{ids: []string{"d1"}}, "https://espm.test", Options{CanaryPercent: 100, FailureThreshold: 20, TargetTimeout: time.Minute})

	base := time.Unix(2000, 0).UTC()
	svc.now = func() time.Time { return base }
	_ = svc.Rollout(context.Background(), "drv", "v1")

	svc.now = func() time.Time { return base.Add(10 * time.Second) }
	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"updating","sequence":"1"}`))
	// Repeated downloading reports must not keep pushing the timeout out.
	svc.now = func() time.Time { return base.Add(40 * time.Second) }
	svc.OnStatus(context.Background(), "d1", []byte(`{"status":"updating","sequence":"1"}`))

	svc.now = func() time.Time { return base.Add(80 * time.Second) }
	svc.Reconcile(context.Background())
	if repo.statusOf("d1") != StatusLost {
		t.Fatalf("a stuck downloading target must eventually be lost, got %s", repo.statusOf("d1"))
	}
}

func TestOnStatusToleratesSequenceShapes(t *testing.T) {
	for _, payload := range []string{
		`{"status":"ok","sequence":"1"}`,
		`{"status":"ok","sequence":1}`,
		`{"status":"ok","sequence":""}`,
		`{"status":"ok","sequence":null}`,
		`{"status":"ok"}`,
	} {
		repo := newFakeRepo()
		pub := &fakePublisher{sent: map[string][]byte{}}
		svc := newService(repo, pub, fakeDevices{ids: []string{"d1"}}, "https://espm.test", Options{CanaryPercent: 100, FailureThreshold: 20})
		_ = svc.Rollout(context.Background(), "drv", "v1")

		svc.OnStatus(context.Background(), "d1", []byte(payload))
		if repo.statusOf("d1") != StatusSucceeded {
			t.Fatalf("payload %s should still advance the target, got %s", payload, repo.statusOf("d1"))
		}
	}
}

func TestRolloutWithoutPublicURLFails(t *testing.T) {
	svc := newService(newFakeRepo(), &fakePublisher{sent: map[string][]byte{}}, fakeDevices{ids: []string{"d1"}}, "", Options{})
	if err := svc.Rollout(context.Background(), "drv", "v1"); !errors.Is(err, ErrNoPublicURL) {
		t.Fatalf("expected ErrNoPublicURL, got %v", err)
	}
}
