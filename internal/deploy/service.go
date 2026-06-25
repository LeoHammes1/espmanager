package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/LeoHammes1/espmanager/internal/topics"
)

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

	d := Deploy{ID: token(), DriverID: driverID, Version: version, CreatedAt: s.now().UTC()}
	if err := s.repo.CreateDeploy(ctx, d); err != nil {
		return err
	}

	cmd, err := json.Marshal(otaCommand{
		Version:   version,
		URL:       s.baseURL + "/firmware/" + driverID + "/" + version + ".bin",
		SHA256:    a.SHA256,
		Signature: a.Signature,
	})
	if err != nil {
		return err
	}

	for _, deviceID := range deviceIDs {
		t := Target{DeployID: d.ID, DeviceID: deviceID, Version: version, Status: StatusPending, UpdatedAt: s.now().UTC()}
		if err := s.repo.AddTarget(ctx, t); err != nil {
			s.log.Error("add deploy target failed", "device", deviceID, "err", err)
			continue
		}
		if err := s.publisher.Publish(topics.CmdOTA(deviceID), cmd); err != nil {
			s.log.Error("publish ota command failed", "device", deviceID, "err", err)
			continue
		}
		if err := s.repo.SetTargetStatus(ctx, d.ID, deviceID, StatusTriggered, s.now().UTC()); err != nil {
			s.log.Error("set target status failed", "device", deviceID, "err", err)
		}
	}
	return nil
}

func (s *Service) OnHeartbeat(ctx context.Context, deviceID, version string) {
	if version == "" {
		return
	}
	t, found, err := s.repo.LatestTargetForDevice(ctx, deviceID)
	if err != nil || !found || isTerminal(t.Status) {
		return
	}
	if version == t.Version {
		s.update(deviceID, s.repo.SetTargetStatus(ctx, t.DeployID, deviceID, StatusSucceeded, s.now().UTC()))
	}
}

func (s *Service) OnStatus(ctx context.Context, deviceID string, payload []byte) {
	var msg struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	status := mapStatus(msg.Status)
	if status == "" {
		return
	}
	t, found, err := s.repo.LatestTargetForDevice(ctx, deviceID)
	if err != nil || !found || isTerminal(t.Status) {
		return
	}
	s.update(deviceID, s.repo.SetTargetStatus(ctx, t.DeployID, deviceID, status, s.now().UTC()))
}

func (s *Service) update(deviceID string, err error) {
	if err != nil {
		s.log.Error("update deploy target failed", "device", deviceID, "err", err)
	}
}

func mapStatus(reported string) Status {
	switch reported {
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

func isTerminal(s Status) bool {
	return s == StatusSucceeded || s == StatusFailed
}

func token() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
