package artifact

import (
	"context"
	"time"
)

type Artifact struct {
	DriverID  string
	Version   string
	Commit    string
	Env       string
	SHA256    string
	Signature string
	Size      int64
	CreatedAt time.Time
}

type Repository interface {
	Create(ctx context.Context, a Artifact) error
	Get(ctx context.Context, driverID, version string) (Artifact, error)
}

type Signer interface {
	Sign(ctx context.Context, digest []byte) ([]byte, error)
}

type DriverChecker interface {
	Exists(ctx context.Context, driverID string) (bool, error)
}
