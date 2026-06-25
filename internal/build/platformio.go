package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type PlatformIOCompiler struct {
	workspace string
}

func NewPlatformIOCompiler(workspace string) *PlatformIOCompiler {
	return &PlatformIOCompiler{workspace: workspace}
}

func (c *PlatformIOCompiler) Compile(ctx context.Context, job Job) (string, error) {
	dir, err := os.MkdirTemp(c.workspace, "build-*")
	if err != nil {
		return "", err
	}

	if err := run(ctx, dir, "git", "clone", "--depth", "1", "--", job.Repo, dir); err != nil {
		return "", fmt.Errorf("clone: %w", err)
	}
	if job.Commit != "" {
		if err := run(ctx, dir, "git", "fetch", "--depth", "1", "origin", "--end-of-options", job.Commit); err == nil {
			_ = run(ctx, dir, "git", "checkout", "--detach", "--end-of-options", job.Commit)
		}
	}

	args := []string{"run"}
	if job.Env != "" {
		args = append(args, "-e", job.Env)
	}
	if err := run(ctx, dir, "pio", args...); err != nil {
		return "", fmt.Errorf("pio run: %w", err)
	}

	return filepath.Join(dir, ".pio", "build", job.Env, "firmware.bin"), nil
}

func run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GIT_ALLOW_PROTOCOL=http:https")
	return cmd.Run()
}
