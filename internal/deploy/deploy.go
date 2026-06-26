package deploy

import (
	"context"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
)

type Status string

const (
	StatusPending     Status = "pending"
	StatusTriggered   Status = "triggered"
	StatusDownloading Status = "downloading"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusLost        Status = "lost"
)

type State string

const (
	StateInProgress State = "in_progress"
	StatePaused     State = "paused"
	StateCompleted  State = "completed"
)

type Deploy struct {
	ID        string
	DriverID  string
	Version   string
	State     State
	CreatedAt time.Time
}

type Target struct {
	DeployID  string
	DeviceID  string
	Version   string
	Sequence  int64
	Batch     int
	Status    Status
	UpdatedAt time.Time
}

type Repository interface {
	CreateDeploy(ctx context.Context, d Deploy) error
	AddTarget(ctx context.Context, t Target) error
	AdvanceTargetStatus(ctx context.Context, deployID, deviceID string, status Status, at time.Time) (int64, error)
	AdvanceTargetStatusBySequence(ctx context.Context, deviceID string, sequence int64, status Status, at time.Time) (int64, error)
	LatestTargetForDevice(ctx context.Context, deviceID string) (Target, bool, error)
	ListActiveDeploys(ctx context.Context) ([]Deploy, error)
	TargetsForDeploy(ctx context.Context, deployID string) ([]Target, error)
	SetDeployState(ctx context.Context, deployID string, state State) error
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

func terminal(s Status) bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusLost
}
