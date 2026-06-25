package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("driver: name and repo_url are required")
	ErrInvalidRepo = errors.New("driver: repo_url must be an http(s) URL")
	ErrNotFound    = errors.New("driver: not found")
)

type NewDriver struct {
	Name    string
	RepoURL string
	Branch  string
	PioEnv  string
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
	if !isHTTPURL(repoURL) {
		return Driver{}, ErrInvalidRepo
	}

	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "main"
	}

	d := Driver{
		ID:            token(8),
		Name:          name,
		RepoURL:       repoURL,
		Branch:        branch,
		PioEnv:        strings.TrimSpace(in.PioEnv),
		WebhookSecret: token(32),
		CreatedAt:     s.now().UTC(),
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return Driver{}, err
	}
	return d, nil
}

func (s *Service) List(ctx context.Context) ([]Driver, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Driver, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Exists(ctx context.Context, id string) (bool, error) {
	_, err := s.repo.Get(ctx, id)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func token(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
