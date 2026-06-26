package enroll

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	tokens      map[string]Token
	credentials map[string]Credentials
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{tokens: map[string]Token{}, credentials: map[string]Credentials{}}
}

func (r *fakeRepo) CreateToken(_ context.Context, t Token) error {
	r.tokens[t.Value] = t
	return nil
}

func (r *fakeRepo) tokenOK(value, deviceID string, now time.Time) bool {
	t, ok := r.tokens[value]
	return ok && now.Before(t.ExpiresAt) && (t.DeviceID == "" || t.DeviceID == deviceID)
}

func (r *fakeRepo) TokenValid(_ context.Context, value, deviceID string, now time.Time) (bool, error) {
	return r.tokenOK(value, deviceID, now), nil
}

func (r *fakeRepo) Claim(_ context.Context, deviceID, token, passwordHash string, now time.Time) error {
	if !r.tokenOK(token, deviceID, now) {
		return ErrInvalidToken
	}
	if _, exists := r.credentials[deviceID]; exists {
		return ErrAlreadyEnrolled
	}
	delete(r.tokens, token)
	r.credentials[deviceID] = Credentials{Hash: passwordHash}
	return nil
}

func (r *fakeRepo) Credentials(_ context.Context, deviceID string) (Credentials, bool, error) {
	c, ok := r.credentials[deviceID]
	return c, ok, nil
}

func (r *fakeRepo) Revoke(_ context.Context, deviceID string) (bool, error) {
	_, ok := r.credentials[deviceID]
	delete(r.credentials, deviceID)
	return ok, nil
}

func (r *fakeRepo) SetPendingHash(_ context.Context, deviceID, pendingHash string) (bool, error) {
	c, ok := r.credentials[deviceID]
	if !ok || c.Pending != "" {
		return false, nil
	}
	c.Pending = pendingHash
	r.credentials[deviceID] = c
	return true, nil
}

func (r *fakeRepo) PromotePending(_ context.Context, deviceID string) error {
	c := r.credentials[deviceID]
	if c.Pending != "" {
		c.Hash = c.Pending
		c.Pending = ""
		r.credentials[deviceID] = c
	}
	return nil
}

func TestMintForBindsTokenToMAC(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	tok, err := svc.MintFor(context.Background(), "AA:BB:CC:11:22:33")
	if err != nil {
		t.Fatalf("mintFor: %v", err)
	}
	if tok.DeviceID != "aabbcc112233" {
		t.Fatalf("token should be bound to the normalized MAC, got %q", tok.DeviceID)
	}

	// A different device cannot use the bound token.
	if _, err := svc.Claim(context.Background(), "ffeeddccbbaa", tok.Value); err != ErrInvalidToken {
		t.Fatalf("bound token must reject a different device, got %v", err)
	}
	// The bound device can.
	if _, err := svc.Claim(context.Background(), "aabbcc112233", tok.Value); err != nil {
		t.Fatalf("bound device should claim, got %v", err)
	}
}

func TestMintForRejectsInvalidMAC(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	for _, bad := range []string{"", "xyz", "aabbcc", "gggggggggggg", "aabbcc11223344"} {
		if _, err := svc.MintFor(context.Background(), bad); err != ErrInvalidMAC {
			t.Fatalf("MintFor(%q) want ErrInvalidMAC, got %v", bad, err)
		}
	}
}

func TestMintUnboundClaimableByAnyDevice(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	tok, err := svc.Mint(context.Background())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if tok.DeviceID != "" {
		t.Fatalf("unbound token should have no device, got %q", tok.DeviceID)
	}
	if _, err := svc.Claim(context.Background(), "any-device-id", tok.Value); err != nil {
		t.Fatalf("unbound token should be claimable by any device, got %v", err)
	}
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

func enroll1(t *testing.T, svc *Service, deviceID string) string {
	t.Helper()
	tok, _ := svc.Mint(context.Background())
	password, err := svc.Claim(context.Background(), deviceID, tok.Value)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return password
}

func TestRotateKeepsOldCredentialUntilNewIsUsed(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	old := enroll1(t, svc, "dev1")

	next, err := svc.Rotate(context.Background(), "dev1")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	// During the grace window both credentials authenticate.
	if !svc.Authenticate(context.Background(), "dev1", old) {
		t.Fatal("old credential must stay valid until the new one is used")
	}
	// Using the new credential promotes it and retires the old.
	if !svc.Authenticate(context.Background(), "dev1", next) {
		t.Fatal("new credential must authenticate")
	}
	if svc.Authenticate(context.Background(), "dev1", old) {
		t.Fatal("old credential must stop working once the new one is used")
	}
}

func TestRevokeBlocksAuthAndAllowsReclaim(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	password := enroll1(t, svc, "dev1")

	if err := svc.Revoke(context.Background(), "dev1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if svc.Authenticate(context.Background(), "dev1", password) {
		t.Fatal("revoked credential must not authenticate")
	}

	fresh := enroll1(t, svc, "dev1")
	if !svc.Authenticate(context.Background(), "dev1", fresh) {
		t.Fatal("device must be able to re-claim after revocation")
	}
}

func TestRotateRejectsOverlappingRotation(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	enroll1(t, svc, "dev1")

	if _, err := svc.Rotate(context.Background(), "dev1"); err != nil {
		t.Fatalf("first rotate: %v", err)
	}
	if _, err := svc.Rotate(context.Background(), "dev1"); err != ErrRotationPending {
		t.Fatalf("second rotate before adoption: expected ErrRotationPending, got %v", err)
	}
}

func TestRotateAndRevokeRequireEnrollment(t *testing.T) {
	svc := NewService(newFakeRepo(), 15*time.Minute)
	if _, err := svc.Rotate(context.Background(), "ghost"); err != ErrNotEnrolled {
		t.Fatalf("rotate unknown: expected ErrNotEnrolled, got %v", err)
	}
	if err := svc.Revoke(context.Background(), "ghost"); err != ErrNotEnrolled {
		t.Fatalf("revoke unknown: expected ErrNotEnrolled, got %v", err)
	}
}
