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
	Sequence  int64
	Size      int64
	CreatedAt time.Time
}

func FirmwarePath(driverID, version string) string {
	return "/firmware/" + driverID + "/" + version + ".bin"
}

type Repository interface {
	Create(ctx context.Context, a Artifact) error
	Get(ctx context.Context, driverID, version string) (Artifact, error)
	Delete(ctx context.Context, driverID, version string) error
	NextSequence(ctx context.Context) (int64, error)
}

type Signer interface {
	Sign(ctx context.Context, digest []byte) ([]byte, error)
}

type DriverChecker interface {
	Exists(ctx context.Context, driverID string) (bool, error)
}
