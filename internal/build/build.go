package build

import "context"

type Job struct {
	ID       string
	DriverID string
	Repo     string
	Commit   string
	Env      string
}

type Result struct {
	FirmwarePath string
	Version      string
}

type JobSource interface {
	Next(ctx context.Context) (*Job, error)
	Complete(ctx context.Context, id string) error
}

type Compiler interface {
	Compile(ctx context.Context, job Job) (Result, error)
}

type ArtifactSink interface {
	Upload(ctx context.Context, job Job, result Result) error
}
