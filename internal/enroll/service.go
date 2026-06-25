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
	ErrInvalidToken    = errors.New("enroll: invalid or expired claim token")
	ErrInvalidDevice   = errors.New("enroll: invalid device id")
	ErrAlreadyEnrolled = errors.New("enroll: device already enrolled")
)

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

type Service struct {
	repo Repository
	ttl  time.Duration
	now  func() time.Time
}

func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl, now: time.Now}
}

func (s *Service) Mint(ctx context.Context) (Token, error) {
	value, err := id.New(24)
	if err != nil {
		return Token{}, err
	}
	t := Token{Value: value, ExpiresAt: s.now().UTC().Add(s.ttl)}
	if err := s.repo.CreateToken(ctx, t); err != nil {
		return Token{}, err
	}
	return t, nil
}

func (s *Service) Claim(ctx context.Context, deviceID, token string) (string, error) {
	if !deviceIDPattern.MatchString(deviceID) {
		return "", ErrInvalidDevice
	}

	now := s.now().UTC()
	valid, err := s.repo.TokenValid(ctx, token, now)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", ErrInvalidToken
	}
	switch _, found, err := s.repo.CredentialHash(ctx, deviceID); {
	case err != nil:
		return "", err
	case found:
		return "", ErrAlreadyEnrolled
	}

	password, err := id.New(24)
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	if err := s.repo.Claim(ctx, deviceID, token, string(hash), now); err != nil {
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
