package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/id"
	"github.com/LeoHammes1/espmanager/internal/topics"
)

var ErrNoPublicURL = errors.New("deploy: ESPM_PUBLIC_URL is not set; OTA rollout disabled")

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
	baseURL   string
	opts      Options
	log       *slog.Logger
	now       func() time.Time
}

func NewService(repo Repository, devices DeviceSource, artifacts ArtifactSource, publisher Publisher, baseURL string, opts Options, log *slog.Logger) *Service {
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
		baseURL:   baseURL,
		opts:      opts,
		log:       log,
		now:       time.Now,
	}
}

type otaCommand struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	Sequence  int64  `json:"sequence"`
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
		t := Target{DeployID: d.ID, DeviceID: deviceID, Version: version, Batch: batch, Status: StatusPending, UpdatedAt: s.now().UTC()}
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
	return errors.Join(errs...)
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
	for i := range targets {
		t := &targets[i]
		if t.Status == StatusTriggered && now.Sub(t.UpdatedAt) > s.opts.TargetTimeout {
			n, err := s.repo.AdvanceTargetStatus(ctx, d.ID, t.DeviceID, StatusLost, now)
			if err != nil {
				s.log.Error("mark lost failed", "deploy", d.ID, "device", t.DeviceID, "err", err)
				continue
			}
			if n > 0 {
				t.Status = StatusLost
			}
		}
	}

	for _, batch := range sortedBatches(targets) {
		group := filterBatch(targets, batch)
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
	a, err := s.artifacts.Get(ctx, d.DriverID, d.Version)
	if err != nil {
		s.log.Error("promote: artifact lookup failed", "deploy", d.ID, "err", err)
		return
	}
	cmd, err := s.command(d.DriverID, d.Version, a)
	if err != nil {
		s.log.Error("promote: build command failed", "deploy", d.ID, "err", err)
		return
	}
	for _, t := range pending {
		_ = s.trigger(ctx, d.ID, t.DeviceID, cmd)
	}
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
	}
}

func (s *Service) command(driverID, version string, a artifact.Artifact) ([]byte, error) {
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
		Status string `json:"status"`
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

	t, found, err := s.repo.LatestTargetForDevice(ctx, deviceID)
	if err != nil {
		s.log.Error("lookup deploy target failed", "device", deviceID, "err", err)
		return
	}
	if !found {
		return
	}

	if _, err := s.repo.AdvanceTargetStatus(ctx, t.DeployID, deviceID, status, s.now().UTC()); err != nil {
		s.log.Error("advance deploy target failed", "device", deviceID, "err", err)
	}
}

func mapStatus(reported string) Status {
	switch strings.ToLower(strings.TrimSpace(reported)) {
	case "ok", "success", "valid":
		return StatusSucceeded
	case "fail", "failed", "error":
		return StatusFailed
	case "downloading", "applying", "updating":
		return StatusTriggered
	default:
		return ""
	}
}

func sortedBatches(targets []Target) []int {
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

func filterBatch(targets []Target, batch int) []Target {
	var out []Target
	for _, t := range targets {
		if t.Batch == batch {
			out = append(out, t)
		}
	}
	return out
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
