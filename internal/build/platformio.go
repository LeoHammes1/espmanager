package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type PlatformIOCompiler struct {
	workspace string
}

func NewPlatformIOCompiler(workspace string) *PlatformIOCompiler {
	return &PlatformIOCompiler{workspace: workspace}
}

func (c *PlatformIOCompiler) Compile(ctx context.Context, job Job) (Result, error) {
	dir, err := os.MkdirTemp(c.workspace, "build-*")
	if err != nil {
		return Result{}, err
	}

	if err := run(ctx, dir, "git", "clone", "--depth", "1", "--", job.Repo, dir); err != nil {
		return Result{}, fmt.Errorf("clone: %w", err)
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
		return Result{}, fmt.Errorf("pio run: %w", err)
	}

	return Result{
		FirmwarePath: filepath.Join(dir, ".pio", "build", job.Env, "firmware.bin"),
		Version:      version(ctx, dir, job.Commit),
	}, nil
}

func version(ctx context.Context, dir, commit string) string {
	out, err := output(ctx, dir, "git", "describe", "--tags", "--always")
	if v := strings.TrimSpace(out); err == nil && v != "" {
		return v
	}
	if len(commit) >= 12 {
		return commit[:12]
	}
	return commit
}

func run(ctx context.Context, dir, name string, args ...string) error {
	cmd := command(ctx, dir, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func output(ctx context.Context, dir, name string, args ...string) (string, error) {
	out, err := command(ctx, dir, name, args...).Output()
	return string(out), err
}

func command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_ALLOW_PROTOCOL=http:https")
	return cmd
}
