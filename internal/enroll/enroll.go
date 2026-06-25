package enroll

import (
	"context"
	"time"
)

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type Repository interface {
	CreateToken(ctx context.Context, t Token) error
	TokenValid(ctx context.Context, value string, now time.Time) (bool, error)
	Claim(ctx context.Context, deviceID, token, passwordHash string, now time.Time) error
	CredentialHash(ctx context.Context, deviceID string) (string, bool, error)
}
