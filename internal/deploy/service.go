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

type Service struct {
	repo      Repository
	devices   DeviceSource
	artifacts ArtifactSource
	publisher Publisher
	baseURL   string
	log       *slog.Logger
	now       func() time.Time
}

func NewService(repo Repository, devices DeviceSource, artifacts ArtifactSource, publisher Publisher, baseURL string, log *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		devices:   devices,
		artifacts: artifacts,
		publisher: publisher,
		baseURL:   baseURL,
		log:       log,
		now:       time.Now,
	}
}

type otaCommand struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
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
	d := Deploy{ID: deployID, DriverID: driverID, Version: version, CreatedAt: s.now().UTC()}
	if err := s.repo.CreateDeploy(ctx, d); err != nil {
		return err
	}

	cmd, err := json.Marshal(otaCommand{
		Version:   version,
		URL:       s.baseURL + artifact.FirmwarePath(driverID, version),
		SHA256:    a.SHA256,
		Signature: a.Signature,
	})
	if err != nil {
		return err
	}

	var errs []error
	for _, deviceID := range deviceIDs {
		if err := s.repo.AddTarget(ctx, Target{DeployID: d.ID, DeviceID: deviceID, Version: version, Status: StatusPending, UpdatedAt: s.now().UTC()}); err != nil {
			errs = append(errs, fmt.Errorf("add target %s: %w", deviceID, err))
			continue
		}
		if err := s.publisher.Publish(topics.CmdOTA(deviceID), cmd); err != nil {
			errs = append(errs, fmt.Errorf("publish %s: %w", deviceID, err))
			_ = s.repo.SetTargetStatus(ctx, d.ID, deviceID, StatusFailed, s.now().UTC())
			continue
		}
		if err := s.repo.SetTargetStatus(ctx, d.ID, deviceID, StatusTriggered, s.now().UTC()); err != nil {
			errs = append(errs, fmt.Errorf("trigger %s: %w", deviceID, err))
		}
	}
	return errors.Join(errs...)
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

	if err := s.repo.AdvanceTargetStatus(ctx, t.DeployID, deviceID, status, s.now().UTC()); err != nil {
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
