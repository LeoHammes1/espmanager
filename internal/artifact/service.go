package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	ErrInvalid       = errors.New("artifact: driver_id, version and content are required")
	ErrBadVersion    = errors.New("artifact: invalid version")
	ErrUnknownDriver = errors.New("artifact: unknown driver")
	ErrNotFound      = errors.New("artifact: not found")
)

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type NewArtifact struct {
	DriverID string
	Version  string
	Commit   string
	Env      string
	Content  []byte
}

type Service struct {
	repo    Repository
	signer  Signer
	drivers DriverChecker
	dir     string
	now     func() time.Time
}

func NewService(repo Repository, signer Signer, drivers DriverChecker, dir string) *Service {
	return &Service{repo: repo, signer: signer, drivers: drivers, dir: dir, now: time.Now}
}

func (s *Service) Store(ctx context.Context, in NewArtifact) (Artifact, error) {
	if in.DriverID == "" || in.Version == "" || len(in.Content) == 0 {
		return Artifact{}, ErrInvalid
	}
	if !versionPattern.MatchString(in.Version) {
		return Artifact{}, ErrBadVersion
	}

	exists, err := s.drivers.Exists(ctx, in.DriverID)
	if err != nil {
		return Artifact{}, err
	}
	if !exists {
		return Artifact{}, ErrUnknownDriver
	}

	sum := sha256.Sum256(in.Content)
	signature, err := s.signer.Sign(ctx, sum[:])
	if err != nil {
		return Artifact{}, err
	}

	path := s.Path(in.DriverID, in.Version)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Artifact{}, err
	}
	if err := os.WriteFile(path, in.Content, 0o644); err != nil {
		return Artifact{}, err
	}

	a := Artifact{
		DriverID:  in.DriverID,
		Version:   in.Version,
		Commit:    in.Commit,
		Env:       in.Env,
		SHA256:    hex.EncodeToString(sum[:]),
		Signature: hex.EncodeToString(signature),
		Size:      int64(len(in.Content)),
		CreatedAt: s.now().UTC(),
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return Artifact{}, err
	}
	return a, nil
}

func (s *Service) Get(ctx context.Context, driverID, version string) (Artifact, error) {
	return s.repo.Get(ctx, driverID, version)
}

func (s *Service) Path(driverID, version string) string {
	return filepath.Join(s.dir, driverID, version+".bin")
}
