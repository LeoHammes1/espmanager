package enroll

import (
	"context"
	"time"
)

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type Credentials struct {
	Hash    string
	Pending string
}

type Repository interface {
	CreateToken(ctx context.Context, t Token) error
	TokenValid(ctx context.Context, value string, now time.Time) (bool, error)
	Claim(ctx context.Context, deviceID, token, passwordHash string, now time.Time) error
	Credentials(ctx context.Context, deviceID string) (Credentials, bool, error)
	Revoke(ctx context.Context, deviceID string) (bool, error)
	SetPendingHash(ctx context.Context, deviceID, pendingHash string) (bool, error)
	PromotePending(ctx context.Context, deviceID string) error
}
