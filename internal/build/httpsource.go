package build

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type HTTPJobSource struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPJobSource(baseURL, token string, client *http.Client) *HTTPJobSource {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPJobSource{baseURL: baseURL, token: token, client: client}
}

type leasedJob struct {
	ID  string `json:"id"`
	Job struct {
		DriverID string `json:"driver_id"`
		Repo     string `json:"repo"`
		Commit   string `json:"commit"`
		Env      string `json:"env"`
	} `json:"job"`
}

func (s *HTTPJobSource) Next(ctx context.Context) (*Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/v1/jobs/next", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var leased leasedJob
	if err := json.NewDecoder(resp.Body).Decode(&leased); err != nil {
		return nil, err
	}

	return &Job{
		ID:       leased.ID,
		DriverID: leased.Job.DriverID,
		Repo:     leased.Job.Repo,
		Commit:   leased.Job.Commit,
		Env:      leased.Job.Env,
	}, nil
}

func (s *HTTPJobSource) Complete(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/v1/jobs/%s/complete", s.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
