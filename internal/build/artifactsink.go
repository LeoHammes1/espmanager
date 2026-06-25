package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

type HTTPArtifactSink struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPArtifactSink(baseURL, token string, client *http.Client) *HTTPArtifactSink {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPArtifactSink{baseURL: baseURL, token: token, client: client}
}

func (s *HTTPArtifactSink) Upload(ctx context.Context, job Job, result Result) error {
	file, err := os.Open(result.FirmwarePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fields := map[string]string{
		"driver_id": job.DriverID,
		"version":   result.Version,
		"commit":    job.Commit,
		"env":       job.Env,
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	part, err := mw.CreateFormFile("firmware", "firmware.bin")
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/artifacts", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("artifact upload responded %d", resp.StatusCode)
	}
	return nil
}
