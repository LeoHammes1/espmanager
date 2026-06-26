package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/id"
	"github.com/LeoHammes1/espmanager/internal/topics"
)

var (
	ErrNoPublicURL    = errors.New("deploy: ESPM_PUBLIC_URL is not set; OTA rollout disabled")
	ErrStaleArtifact  = errors.New("deploy: artifact predates the signed-sequence scheme; re-publish to deploy")
	ErrDeployNotFound = errors.New("deploy: not found")
	ErrNotPaused      = errors.New("deploy: only a paused deploy can be resumed")
	ErrNotCancellable = errors.New("deploy: only an active or paused deploy can be cancelled")
)

const resendGracePeriod = 90 * time.Second

type Options struct {
	CanaryPercent    int
	FailureThreshold int
	TargetTimeout    time.Duration
}

type Service struct {
	repo      Repository
	devices   DeviceSource
	artifacts ArtifactSource
	publisher Publisher
	notifier  Notifier
	baseURL   string
	opts      Options
	log       *slog.Logger
	now       func() time.Time
}

func NewService(repo Repository, devices DeviceSource, artifacts ArtifactSource, publisher Publisher, notifier Notifier, baseURL string, opts Options, log *slog.Logger) *Service {
	if opts.CanaryPercent <= 0 || opts.CanaryPercent > 100 {
		opts.CanaryPercent = 100
	}
	if opts.FailureThreshold < 0 {
		opts.FailureThreshold = 0
	} else if opts.FailureThreshold > 100 {
		opts.FailureThreshold = 100
	}
	if opts.TargetTimeout <= 0 {
		opts.TargetTimeout = 5 * time.Minute
	}
	return &Service{
		repo:      repo,
		devices:   devices,
		artifacts: artifacts,
		publisher: publisher,
		notifier:  notifier,
		baseURL:   baseURL,
		opts:      opts,
		log:       log,
		now:       time.Now,
	}
}

func (s *Service) notify() {
	if s.notifier != nil {
		s.notifier.Changed()
	}
}

type otaCommand struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	Sequence  int64  `json:"sequence,string"`
}

func (s *Service) Rollout(ctx context.Context, driverID, version string) error {
	if s.baseURL == "" {
		return ErrNoPublicURL
	}

	a, err := s.artifacts.Get(ctx, driverID, version)
	if err != nil {
		return err
	}
	deviceIDs, err := s.devices.IDsForDriver(ctx, driverID)
	if err != nil {
		return err
	}
	if len(deviceIDs) == 0 {
		return nil
	}

	deployID, err := id.New(8)
	if err != nil {
		return err
	}
	d := Deploy{ID: deployID, DriverID: driverID, Version: version, State: StateInProgress, CreatedAt: s.now().UTC()}
	if err := s.repo.CreateDeploy(ctx, d); err != nil {
		return err
	}

	canaryCount := (len(deviceIDs)*s.opts.CanaryPercent + 99) / 100
	if canaryCount < 1 {
		canaryCount = 1
	}
	if canaryCount > len(deviceIDs) {
		canaryCount = len(deviceIDs)
	}

	var errs []error
	var canary []string
	for i, deviceID := range deviceIDs {
		batch := 0
		if i >= canaryCount {
			batch = 1
		}
		t := Target{DeployID: d.ID, DeviceID: deviceID, Version: version, Sequence: a.Sequence, Batch: batch, Status: StatusPending, UpdatedAt: s.now().UTC()}
		if err := s.repo.AddTarget(ctx, t); err != nil {
			errs = append(errs, fmt.Errorf("add target %s: %w", deviceID, err))
			continue
		}
		if batch == 0 {
			canary = append(canary, deviceID)
		}
	}

	cmd, err := s.command(driverID, version, a)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, deviceID := range canary {
		errs = append(errs, s.trigger(ctx, d.ID, deviceID, cmd))
	}
	s.notify()
	return errors.Join(errs...)
}

func (s *Service) ListDeploys(ctx context.Context) ([]Deploy, error) {
	return s.repo.ListDeploys(ctx)
}

func (s *Service) Targets(ctx context.Context, deployID string) ([]Target, error) {
	return s.repo.TargetsForDeploy(ctx, deployID)
}

func (s *Service) DeployDetail(ctx context.Context, deployID string) (Deploy, []Target, error) {
	d, found, err := s.repo.GetDeploy(ctx, deployID)
	if err != nil {
		return Deploy{}, nil, err
	}
	if !found {
		return Deploy{}, nil, ErrDeployNotFound
	}
	targets, err := s.repo.TargetsForDeploy(ctx, deployID)
	if err != nil {
		return Deploy{}, nil, err
	}
	return d, targets, nil
}

// Resume retries the failed and lost targets of a paused deploy; the reconcile
// loop then re-triggers them and continues the rollout from where it stalled.
func (s *Service) Resume(ctx context.Context, deployID string) error {
	d, found, err := s.repo.GetDeploy(ctx, deployID)
	if err != nil {
		return err
	}
	if !found {
		return ErrDeployNotFound
	}
	if d.State != StatePaused {
		return ErrNotPaused
	}
	if _, err := s.repo.ResetFailedTargets(ctx, deployID, s.now().UTC()); err != nil {
		return err
	}
	if err := s.repo.SetDeployState(ctx, deployID, StateInProgress); err != nil {
		return err
	}
	s.notify()
	return nil
}

// Cancel abandons an active or paused deploy; untriggered targets stay pending
// and are never sent.
func (s *Service) Cancel(ctx context.Context, deployID string) error {
	d, found, err := s.repo.GetDeploy(ctx, deployID)
	if err != nil {
		return err
	}
	if !found {
		return ErrDeployNotFound
	}
	if d.State != StateInProgress && d.State != StatePaused {
		return ErrNotCancellable
	}
	if err := s.repo.SetDeployState(ctx, deployID, StateCancelled); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Service) Reconcile(ctx context.Context) {
	deploys, err := s.repo.ListActiveDeploys(ctx)
	if err != nil {
		s.log.Error("list active deploys failed", "err", err)
		return
	}
	for _, d := range deploys {
		s.reconcile(ctx, d)
	}
}

func (s *Service) reconcile(ctx context.Context, d Deploy) {
	targets, err := s.repo.TargetsForDeploy(ctx, d.ID)
	if err != nil {
		s.log.Error("load deploy targets failed", "deploy", d.ID, "err", err)
		return
	}

	now := s.now().UTC()
	var resend []byte
	for i := range targets {
		t := &targets[i]
		if (t.Status == StatusTriggered || t.Status == StatusDownloading) && now.Sub(t.UpdatedAt) > s.opts.TargetTimeout {
			n, err := s.repo.AdvanceTargetStatus(ctx, d.ID, t.DeviceID, StatusLost, now)
			if err != nil {
				s.log.Error("mark lost failed", "deploy", d.ID, "device", t.DeviceID, "err", err)
				continue
			}
			if n > 0 {
				t.Status = StatusLost
				s.notify()
			}
			continue
		}
		// Re-deliver to devices that were commanded but have not acknowledged: the
		// non-retained command can be lost in a reconnect window. The device's
		// sequence/in-flight guards make a duplicate command a no-op. Bounded to a
		// short grace window after the trigger so it covers a reconnect without
		// becoming a publish storm against a device that is simply offline.
		if t.Status == StatusTriggered && now.Sub(t.UpdatedAt) < resendGracePeriod {
			if resend == nil {
				resend = s.reconcileCommand(ctx, d)
			}
			if resend != nil {
				_ = s.publisher.Publish(topics.CmdOTA(t.DeviceID), resend)
			}
		}
	}

	for _, batch := range Batches(targets) {
		group := TargetsInBatch(targets, batch)
		if allTerminal(group) {
			if failureExceeded(group, s.opts.FailureThreshold) {
				s.setState(ctx, d.ID, StatePaused)
				return
			}
			continue
		}
		if pending := filterPending(group); len(pending) > 0 {
			s.promote(ctx, d, pending)
			return
		}
		return
	}
	s.setState(ctx, d.ID, StateCompleted)
}

func (s *Service) promote(ctx context.Context, d Deploy, pending []Target) {
	cmd := s.reconcileCommand(ctx, d)
	if cmd == nil {
		return
	}
	for _, t := range pending {
		_ = s.trigger(ctx, d.ID, t.DeviceID, cmd)
	}
	s.notify()
}

func (s *Service) reconcileCommand(ctx context.Context, d Deploy) []byte {
	a, err := s.artifacts.Get(ctx, d.DriverID, d.Version)
	if err != nil {
		s.log.Error("artifact lookup failed", "deploy", d.ID, "err", err)
		return nil
	}
	cmd, err := s.command(d.DriverID, d.Version, a)
	if err != nil {
		s.log.Error("build command failed", "deploy", d.ID, "err", err)
		return nil
	}
	return cmd
}

func (s *Service) trigger(ctx context.Context, deployID, deviceID string, cmd []byte) error {
	if err := s.publisher.Publish(topics.CmdOTA(deviceID), cmd); err != nil {
		_, _ = s.repo.AdvanceTargetStatus(ctx, deployID, deviceID, StatusFailed, s.now().UTC())
		return fmt.Errorf("publish %s: %w", deviceID, err)
	}
	if _, err := s.repo.AdvanceTargetStatus(ctx, deployID, deviceID, StatusTriggered, s.now().UTC()); err != nil {
		return fmt.Errorf("trigger %s: %w", deviceID, err)
	}
	return nil
}

func (s *Service) setState(ctx context.Context, deployID string, state State) {
	if err := s.repo.SetDeployState(ctx, deployID, state); err != nil {
		s.log.Error("set deploy state failed", "deploy", deployID, "state", state, "err", err)
		return
	}
	s.notify()
}

func (s *Service) command(driverID, version string, a artifact.Artifact) ([]byte, error) {
	if a.Sequence == 0 {
		return nil, ErrStaleArtifact
	}
	return json.Marshal(otaCommand{
		Version:   version,
		URL:       s.baseURL + artifact.FirmwarePath(driverID, version),
		SHA256:    a.SHA256,
		Signature: a.Signature,
		Sequence:  a.Sequence,
	})
}

func (s *Service) OnStatus(ctx context.Context, deviceID string, payload []byte) {
	var msg struct {
		Status   string          `json:"status"`
		Sequence json.RawMessage `json:"sequence"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		s.log.Warn("ignoring malformed ota status", "device", deviceID, "err", err)
		return
	}

	status := mapStatus(msg.Status)
	if status == "" {
		s.log.Warn("ignoring unmapped ota status", "device", deviceID, "status", msg.Status)
		return
	}

	now := s.now().UTC()
	sequence := parseSequence(msg.Sequence)
	// A sequence-tagged report is matched to the exact target it concerns, so a
	// late or duplicated report can never mutate an unrelated deploy. Clients that
	// do not report a sequence fall back to the most recent target.
	if sequence > 0 {
		n, err := s.repo.AdvanceTargetStatusBySequence(ctx, deviceID, sequence, status, now)
		if err != nil {
			s.log.Error("advance deploy target failed", "device", deviceID, "err", err)
			return
		}
		if n > 0 {
			s.notify()
		}
		return
	}

	t, found, err := s.repo.LatestTargetForDevice(ctx, deviceID)
	if err != nil {
		s.log.Error("lookup deploy target failed", "device", deviceID, "err", err)
		return
	}
	if !found {
		return
	}
	n, err := s.repo.AdvanceTargetStatus(ctx, t.DeployID, deviceID, status, now)
	if err != nil {
		s.log.Error("advance deploy target failed", "device", deviceID, "err", err)
		return
	}
	if n > 0 {
		s.notify()
	}
}

// parseSequence reads the report sequence leniently: an absent, null, empty, or
// malformed value yields 0 (handled as a legacy/unsequenced report) rather than
// discarding an otherwise-valid status report.
func parseSequence(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func mapStatus(reported string) Status {
	switch strings.ToLower(strings.TrimSpace(reported)) {
	case "ok", "success", "valid":
		return StatusSucceeded
	case "fail", "failed", "error":
		return StatusFailed
	case "downloading", "applying", "updating":
		return StatusDownloading
	default:
		return ""
	}
}

// Batches returns the distinct batch numbers present, in ascending order
// (batch 0 is the canary).
func Batches(targets []Target) []int {
	seen := map[int]bool{}
	var out []int
	for _, t := range targets {
		if !seen[t.Batch] {
			seen[t.Batch] = true
			out = append(out, t.Batch)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TargetsInBatch returns the targets belonging to a batch.
func TargetsInBatch(targets []Target, batch int) []Target {
	var out []Target
	for _, t := range targets {
		if t.Batch == batch {
			out = append(out, t)
		}
	}
	return out
}

// BatchTripped reports whether a batch has fully settled with its failure rate
// over the threshold — the exact condition the reconcile loop pauses on, exposed
// so the UI can explain a pause without re-deriving the rule.
func BatchTripped(targets []Target, thresholdPercent int) bool {
	return allTerminal(targets) && failureExceeded(targets, thresholdPercent)
}

func filterPending(targets []Target) []Target {
	var out []Target
	for _, t := range targets {
		if t.Status == StatusPending {
			out = append(out, t)
		}
	}
	return out
}

func allTerminal(targets []Target) bool {
	for _, t := range targets {
		if !terminal(t.Status) {
			return false
		}
	}
	return true
}

func failureExceeded(targets []Target, thresholdPercent int) bool {
	if len(targets) == 0 {
		return false
	}
	failures := 0
	for _, t := range targets {
		if t.Status == StatusFailed || t.Status == StatusLost {
			failures++
		}
	}
	return failures*100 > thresholdPercent*len(targets)
}
