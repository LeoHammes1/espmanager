package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	ErrAlreadyExists = errors.New("artifact: version already published")
	ErrNotFound      = errors.New("artifact: not found")
)

var (
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	driverIDPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)
)

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
	if !driverIDPattern.MatchString(in.DriverID) {
		return Artifact{}, ErrInvalid
	}
	if !versionPattern.MatchString(in.Version) {
		return Artifact{}, ErrBadVersion
	}

	switch _, err := s.repo.Get(ctx, in.DriverID, in.Version); {
	case err == nil:
		return Artifact{}, ErrAlreadyExists
	case !errors.Is(err, ErrNotFound):
		return Artifact{}, err
	}

	exists, err := s.drivers.Exists(ctx, in.DriverID)
	if err != nil {
		return Artifact{}, err
	}
	if !exists {
		return Artifact{}, ErrUnknownDriver
	}

	sum := sha256.Sum256(in.Content)
	sequence, err := s.repo.NextSequence(ctx)
	if err != nil {
		return Artifact{}, err
	}
	signature, err := s.signer.Sign(ctx, signedMessage(sequence, sum[:]))
	if err != nil {
		return Artifact{}, err
	}

	a := Artifact{
		DriverID:  in.DriverID,
		Version:   in.Version,
		Commit:    in.Commit,
		Env:       in.Env,
		SHA256:    hex.EncodeToString(sum[:]),
		Signature: hex.EncodeToString(signature),
		Sequence:  sequence,
		Size:      int64(len(in.Content)),
		CreatedAt: s.now().UTC(),
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return Artifact{}, err
	}
	if err := s.writeFile(in.DriverID, in.Version, in.Content); err != nil {
		_ = s.repo.Delete(ctx, in.DriverID, in.Version)
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

func signedMessage(sequence int64, sha256 []byte) []byte {
	msg := make([]byte, 8+len(sha256))
	binary.BigEndian.PutUint64(msg[:8], uint64(sequence))
	copy(msg[8:], sha256)
	return msg
}

func (s *Service) writeFile(driverID, version string, content []byte) error {
	path := s.Path(driverID, version)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}
