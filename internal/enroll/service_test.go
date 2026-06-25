package enroll

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	tokens      map[string]time.Time
	used        map[string]bool
	credentials map[string]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{tokens: map[string]time.Time{}, used: map[string]bool{}, credentials: map[string]string{}}
}

func (r *fakeRepo) CreateToken(_ context.Context, t Token) error {
	r.tokens[t.Value] = t.ExpiresAt
	return nil
}

func (r *fakeRepo) ConsumeToken(_ context.Context, value string, now time.Time) (bool, error) {
	exp, ok := r.tokens[value]
	if !ok || r.used[value] || !now.Before(exp) {
		return false, nil
	}
	r.used[value] = true
	return true, nil
}

func (r *fakeRepo) SaveCredential(_ context.Context, deviceID, passwordHash string, _ time.Time) error {
	r.credentials[deviceID] = passwordHash
	return nil
}

func (r *fakeRepo) CredentialHash(_ context.Context, deviceID string) (string, bool, error) {
	h, ok := r.credentials[deviceID]
	return h, ok, nil
}

type fakeEnroller struct{ enrolled []string }

func (e *fakeEnroller) Enroll(_ context.Context, deviceID string) error {
	e.enrolled = append(e.enrolled, deviceID)
	return nil
}

func TestClaimIssuesUsableCredentials(t *testing.T) {
	repo := newFakeRepo()
	en := &fakeEnroller{}
	svc := NewService(repo, en, 15*time.Minute)

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
	if len(en.enrolled) != 1 || en.enrolled[0] != "001122aabbcc" {
		t.Fatalf("device should be enrolled, got %v", en.enrolled)
	}
}

func TestClaimTokenIsSingleUse(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEnroller{}, 15*time.Minute)
	tok, _ := svc.Mint(context.Background())

	if _, err := svc.Claim(context.Background(), "dev1", tok.Value); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := svc.Claim(context.Background(), "dev2", tok.Value); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on reuse, got %v", err)
	}
}

func TestClaimRejectsExpiredToken(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeEnroller{}, -time.Minute)
	tok, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "dev1", tok.Value); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on expired token, got %v", err)
	}
}

func TestClaimRejectsInvalidDeviceID(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEnroller{}, 15*time.Minute)
	tok, _ := svc.Mint(context.Background())
	if _, err := svc.Claim(context.Background(), "bad id/../x", tok.Value); err != ErrInvalidDevice {
		t.Fatalf("expected ErrInvalidDevice, got %v", err)
	}
}
