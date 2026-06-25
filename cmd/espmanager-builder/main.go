package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LeoHammes1/espmanager/internal/build"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	coreURL := env("ESPM_CORE_URL", "http://localhost:8080")
	token := os.Getenv("ESPM_WORKER_TOKEN")
	workspace := env("ESPM_BUILD_WORKSPACE", "data/builds")

	if err := os.MkdirAll(workspace, 0o755); err != nil {
		log.Error("workspace", "err", err)
		os.Exit(1)
	}

	worker := build.NewWorker(
		build.NewHTTPJobSource(coreURL, token, nil),
		build.NewPlatformIOCompiler(workspace),
		log,
		5*time.Second,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("builder started", "core", coreURL)
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
