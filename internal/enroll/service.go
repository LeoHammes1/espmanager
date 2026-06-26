package enroll

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/LeoHammes1/espmanager/internal/id"
)

var (
	ErrInvalidToken    = errors.New("enroll: invalid or expired claim token")
	ErrInvalidDevice   = errors.New("enroll: invalid device id")
	ErrInvalidMAC      = errors.New("enroll: invalid device MAC")
	ErrAlreadyEnrolled = errors.New("enroll: device already enrolled")
	ErrNotEnrolled     = errors.New("enroll: device is not enrolled")
	ErrRotationPending = errors.New("enroll: a credential rotation is already pending for this device")
)

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

// macPattern matches the canonical device_id the firmware derives from the WiFi
// STA base MAC: 12 lowercase hex chars, no separators.
var macPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

var macStripper = strings.NewReplacer(":", "", "-", "", ".", "", " ", "")

// NormalizeMAC canonicalizes a MAC to the firmware's device_id form.
func NormalizeMAC(raw string) (string, bool) {
	mac := macStripper.Replace(strings.ToLower(strings.TrimSpace(raw)))
	if !macPattern.MatchString(mac) {
		return "", false
	}
	return mac, true
}

type Service struct {
	repo Repository
	ttl  time.Duration
	now  func() time.Time
}

func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl, now: time.Now}
}

// Mint issues an unbound claim token (the manual fallback flow — any device may
// claim it).
func (s *Service) Mint(ctx context.Context) (Token, error) {
	return s.mint(ctx, "")
}

// MintFor issues a claim token bound to a single device MAC, so only that device
// can claim it. Used by the browser onboarding wizard.
func (s *Service) MintFor(ctx context.Context, mac string) (Token, error) {
	deviceID, ok := NormalizeMAC(mac)
	if !ok {
		return Token{}, ErrInvalidMAC
	}
	return s.mint(ctx, deviceID)
}

func (s *Service) mint(ctx context.Context, deviceID string) (Token, error) {
	value, err := id.New(24)
	if err != nil {
		return Token{}, err
	}
	t := Token{Value: value, ExpiresAt: s.now().UTC().Add(s.ttl), DeviceID: deviceID}
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
	// A MAC-bound token is only valid for its device; an unbound token is valid
	// for any. A mismatch reads as simply invalid (no oracle on which tokens exist).
	valid, err := s.repo.TokenValid(ctx, token, deviceID, now)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", ErrInvalidToken
	}
	switch _, found, err := s.repo.Credentials(ctx, deviceID); {
	case err != nil:
		return "", err
	case found:
		return "", ErrAlreadyEnrolled
	}

	password, hash, err := newSecret()
	if err != nil {
		return "", err
	}

	if err := s.repo.Claim(ctx, deviceID, token, hash, now); err != nil {
		return "", err
	}
	return password, nil
}

// Rotate issues a fresh credential for an already-enrolled device and stores it
// as pending. The current credential stays valid until the device authenticates
// with the new one (see Authenticate), so a device that misses the rotation is
// never locked out.
func (s *Service) Rotate(ctx context.Context, deviceID string) (string, error) {
	switch _, found, err := s.repo.Credentials(ctx, deviceID); {
	case err != nil:
		return "", err
	case !found:
		return "", ErrNotEnrolled
	}

	password, hash, err := newSecret()
	if err != nil {
		return "", err
	}
	switch ok, err := s.repo.SetPendingHash(ctx, deviceID, hash); {
	case err != nil:
		return "", err
	case !ok:
		// No row updated: either a rotation is already pending (the device must
		// adopt or be revoked first) or the device was revoked concurrently.
		return "", ErrRotationPending
	}
	return password, nil
}

// Revoke removes a device's credential so it can no longer authenticate, until
// it is claimed again with a fresh token.
func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	existed, err := s.repo.Revoke(ctx, deviceID)
	if err != nil {
		return err
	}
	if !existed {
		return ErrNotEnrolled
	}
	return nil
}

func (s *Service) Authenticate(ctx context.Context, deviceID, password string) bool {
	creds, found, err := s.repo.Credentials(ctx, deviceID)
	if err != nil || !found {
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(creds.Hash), []byte(password)) == nil {
		return true
	}
	if creds.Pending != "" && bcrypt.CompareHashAndPassword([]byte(creds.Pending), []byte(password)) == nil {
		if err := s.repo.PromotePending(ctx, deviceID); err != nil {
			return false
		}
		return true
	}
	return false
}

func newSecret() (password, hash string, err error) {
	password, err = id.New(24)
	if err != nil {
		return "", "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return password, string(h), nil
}
