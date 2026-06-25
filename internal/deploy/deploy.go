package deploy

import (
	"context"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusTriggered Status = "triggered"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Deploy struct {
	ID        string
	DriverID  string
	Version   string
	CreatedAt time.Time
}

type Target struct {
	DeployID  string
	DeviceID  string
	Version   string
	Status    Status
	UpdatedAt time.Time
}

type Repository interface {
	CreateDeploy(ctx context.Context, d Deploy) error
	AddTarget(ctx context.Context, t Target) error
	SetTargetStatus(ctx context.Context, deployID, deviceID string, status Status, at time.Time) error
	LatestTargetForDevice(ctx context.Context, deviceID string) (Target, bool, error)
}

type Publisher interface {
	Publish(topic string, payload []byte) error
}

type DeviceSource interface {
	IDsForDriver(ctx context.Context, driverID string) ([]string, error)
}

type ArtifactSource interface {
	Get(ctx context.Context, driverID, version string) (artifact.Artifact, error)
}
