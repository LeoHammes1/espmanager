package driver

import (
	"context"
	"time"
)

type Driver struct {
	ID            string
	Name          string
	RepoURL       string
	Branch        string
	PioEnv        string
	WebhookSecret string
	CreatedAt     time.Time
}

type Repository interface {
	Create(ctx context.Context, d Driver) error
	List(ctx context.Context) ([]Driver, error)
	Get(ctx context.Context, id string) (Driver, error)
}
