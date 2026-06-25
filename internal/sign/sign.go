package sign

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
)

const pemType = "ED25519 PRIVATE KEY"

type Signer struct {
	private ed25519.PrivateKey
}

func LoadOrCreate(path string) (*Signer, error) {
	key, err := load(path)
	switch {
	case err == nil:
		return &Signer{private: key}, nil
	case errors.Is(err, os.ErrNotExist):
		return create(path)
	default:
		return nil, err
	}
}

func (s *Signer) Sign(digest []byte) []byte {
	return ed25519.Sign(s.private, digest)
}

func (s *Signer) PublicKey() ed25519.PublicKey {
	return s.private.Public().(ed25519.PublicKey)
}

func create(path string) (*Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block := pem.EncodeToMemory(&pem.Block{Type: pemType, Bytes: priv})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		return nil, err
	}
	return &Signer{private: priv}, nil
}

func load(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != pemType {
		return nil, errors.New("invalid signing key file")
	}
	return ed25519.PrivateKey(block.Bytes), nil
}
