package enroll

import (
	"context"
	"errors"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/LeoHammes1/espmanager/internal/id"
)

var (
	ErrInvalidToken  = errors.New("enroll: invalid or expired claim token")
	ErrInvalidDevice = errors.New("enroll: invalid device id")
)

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

type Service struct {
	repo    Repository
	devices DeviceEnroller
	ttl     time.Duration
	now     func() time.Time
}

func NewService(repo Repository, devices DeviceEnroller, ttl time.Duration) *Service {
	return &Service{repo: repo, devices: devices, ttl: ttl, now: time.Now}
}

func (s *Service) Mint(ctx context.Context) (Token, error) {
	t := Token{Value: id.New(24), ExpiresAt: s.now().UTC().Add(s.ttl)}
	if err := s.repo.CreateToken(ctx, t); err != nil {
		return Token{}, err
	}
	return t, nil
}

func (s *Service) Claim(ctx context.Context, deviceID, token string) (string, error) {
	if !deviceIDPattern.MatchString(deviceID) {
		return "", ErrInvalidDevice
	}

	consumed, err := s.repo.ConsumeToken(ctx, token, s.now().UTC())
	if err != nil {
		return "", err
	}
	if !consumed {
		return "", ErrInvalidToken
	}

	password := id.New(24)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.repo.SaveCredential(ctx, deviceID, string(hash), s.now().UTC()); err != nil {
		return "", err
	}
	if err := s.devices.Enroll(ctx, deviceID); err != nil {
		return "", err
	}
	return password, nil
}

func (s *Service) Authenticate(ctx context.Context, deviceID, password string) bool {
	hash, found, err := s.repo.CredentialHash(ctx, deviceID)
	if err != nil || !found {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
