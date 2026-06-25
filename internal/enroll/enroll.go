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
	ConsumeToken(ctx context.Context, value string, now time.Time) (bool, error)
	SaveCredential(ctx context.Context, deviceID, passwordHash string, at time.Time) error
	CredentialHash(ctx context.Context, deviceID string) (string, bool, error)
}

type DeviceEnroller interface {
	Enroll(ctx context.Context, deviceID string) error
}
