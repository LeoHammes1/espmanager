package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("driver: name and repo_url are required")

type NewDriver struct {
	Name            string
	RepoURL         string
	Branch          string
	PioEnv          string
	PartitionScheme string
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Create(ctx context.Context, in NewDriver) (Driver, error) {
	name := strings.TrimSpace(in.Name)
	repoURL := strings.TrimSpace(in.RepoURL)
	if name == "" || repoURL == "" {
		return Driver{}, ErrInvalid
	}

	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "main"
	}

	d := Driver{
		ID:              token(8),
		Name:            name,
		RepoURL:         repoURL,
		Branch:          branch,
		PioEnv:          strings.TrimSpace(in.PioEnv),
		PartitionScheme: strings.TrimSpace(in.PartitionScheme),
		WebhookSecret:   token(32),
		CreatedAt:       s.now().UTC(),
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return Driver{}, err
	}
	return d, nil
}

func (s *Service) List(ctx context.Context) ([]Driver, error) {
	return s.repo.List(ctx)
}

func (s *Service) ByRepo(ctx context.Context, repoURL string) ([]Driver, error) {
	return s.repo.ListByRepo(ctx, strings.TrimSpace(repoURL))
}

func token(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
