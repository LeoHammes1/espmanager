package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

type fakeRepo struct {
	items map[string]Artifact
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]Artifact{}} }

func key(driverID, version string) string { return driverID + "/" + version }

func (r *fakeRepo) Create(_ context.Context, a Artifact) error {
	if _, ok := r.items[key(a.DriverID, a.Version)]; ok {
		return ErrAlreadyExists
	}
	r.items[key(a.DriverID, a.Version)] = a
	return nil
}

func (r *fakeRepo) Get(_ context.Context, driverID, version string) (Artifact, error) {
	a, ok := r.items[key(driverID, version)]
	if !ok {
		return Artifact{}, ErrNotFound
	}
	return a, nil
}

func (r *fakeRepo) Delete(_ context.Context, driverID, version string) error {
	delete(r.items, key(driverID, version))
	return nil
}

func (r *fakeRepo) NextSequence(_ context.Context) (int64, error) {
	var max int64
	for _, a := range r.items {
		if a.Sequence > max {
			max = a.Sequence
		}
	}
	return max + 1, nil
}

type fakeSigner struct{ lastMessage []byte }

func (s *fakeSigner) Sign(_ context.Context, message []byte) ([]byte, error) {
	s.lastMessage = append([]byte(nil), message...)
	return []byte("SIGNATURE"), nil
}

type fakeDrivers struct{}

func (fakeDrivers) Exists(_ context.Context, _ string) (bool, error) { return true, nil }

func TestStoreSignsSequenceWithHashAndIsMonotonic(t *testing.T) {
	repo := newFakeRepo()
	signer := &fakeSigner{}
	svc := NewService(repo, signer, fakeDrivers{}, t.TempDir())

	content := []byte("firmware-bytes")
	a, err := svc.Store(context.Background(), NewArtifact{DriverID: "0011223344556677", Version: "v1", Content: content})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if a.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", a.Sequence)
	}

	sum := sha256.Sum256(content)
	if a.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected sha256")
	}
	if !bytes.Equal(signer.lastMessage, signedMessage(1, sum[:])) {
		t.Fatalf("signer must sign sequence||sha256, got %x", signer.lastMessage)
	}
	if len(signer.lastMessage) != 40 {
		t.Fatalf("signed message must be 8-byte sequence + 32-byte hash, got %d bytes", len(signer.lastMessage))
	}

	a2, err := svc.Store(context.Background(), NewArtifact{DriverID: "0011223344556677", Version: "v2", Content: content})
	if err != nil {
		t.Fatalf("store v2: %v", err)
	}
	if a2.Sequence != 2 {
		t.Fatalf("expected sequence 2, got %d", a2.Sequence)
	}
}
