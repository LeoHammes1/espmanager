package device

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDeviceNotFound = errors.New("device: not found")
	ErrDriverNotFound = errors.New("device: driver not found")
)

type Device struct {
	ID              string
	Name            string
	ChipType        string
	FlashSize       int
	DriverID        string
	Online          bool
	LastSeenAt      time.Time
	ReportedVersion string
	EnrolledAt      time.Time
}

type Repository interface {
	List(ctx context.Context) ([]Device, error)
	Get(ctx context.Context, id string) (Device, error)
	ListByDriver(ctx context.Context, driverID string) ([]Device, error)
	SetPresence(ctx context.Context, id string, online bool, at time.Time) error
	ClearPresence(ctx context.Context) error
	RecordHeartbeat(ctx context.Context, id, version string, at time.Time) error
	Assign(ctx context.Context, id, driverID string) error
}

type Notifier interface {
	DeviceChanged()
}

type DriverChecker interface {
	Exists(ctx context.Context, driverID string) (bool, error)
}
