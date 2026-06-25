package enroll

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	tokens      map[string]time.Time
	credentials map[string]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{tokens: map[string]time.Time{}, credentials: map[string]string{}}
}

func (r *fakeRepo) CreateToken(_ context.Context, t Token) error {
	r.tokens[t.Value] = t.ExpiresAt
	return nil
}

func (r *fakeRepo) TokenValid(_ context.Context, value string, now time.Time) (bool, error) {
	exp, ok := r.tokens[value]
	return ok && now.Before(exp), nil
}

func (r *fakeRepo) Claim(_ context.Context, deviceID, token, passwordHash string, now time.Time) error {
	exp, ok := r.tokens[token]
	if !ok || !now.Before(exp) {
		return ErrInvalidToken
	}
	if _, exists := r.credentials[deviceID]; exists {
		return ErrAlreadyEnrolled
	}
	delete(r.tokens, token)
	r.credentials[deviceID] = passwordHash
	return nil
}

func (r *fakeRepo) CredentialHash(_ context.Context, deviceID string) (string, bool, error) {
	h, ok := r.credentials[deviceID]
	return h, ok, nil
}

func TestClaimIssuesUsableCredentials(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	tok, err := svc.Mint(context.Background())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	password, err := svc.Claim(context.Background(), "001122aabbcc", tok.Value)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !svc.Authenticate(context.Background(), "001122aabbcc", password) {
		t.Fatal("issued credentials should authenticate")
	}
	if svc.Authenticate(context.Background(), "001122aabbcc", "wrong") {
		t.Fatal("wrong password must not authenticate")
	}
}

func TestClaimTokenIsSingleUse(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	tok, _ := svc.Mint(context.Background())

	if _, err := svc.Claim(context.Background(), "dev1", tok.Value); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := svc.Claim(context.Background(), "dev2", tok.Value); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on reuse, got %v", err)
	}
}

func TestClaimRejectsExpiredToken(t *testing.T) {
	svc := NewService(newFakeRepo(), -time.Minute)
	tok, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "dev1", tok.Value); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on expired token, got %v", err)
	}
}

func TestClaimRejectsInvalidDeviceID(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	tok, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "bad id/../x", tok.Value); err != ErrInvalidDevice {
		t.Fatalf("expected ErrInvalidDevice, got %v", err)
	}
}

func TestClaimRejectsAlreadyEnrolledDevice(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	first, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "dev1", first.Value); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "dev1", second.Value); err != ErrAlreadyEnrolled {
		t.Fatalf("expected ErrAlreadyEnrolled, got %v", err)
	}
}
