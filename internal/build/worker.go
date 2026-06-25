package build

import (
	"context"
	"log/slog"
	"time"
)

type Worker struct {
	source   JobSource
	compiler Compiler
	sink     ArtifactSink
	log      *slog.Logger
	interval time.Duration
}

func NewWorker(source JobSource, compiler Compiler, sink ArtifactSink, log *slog.Logger, interval time.Duration) *Worker {
	return &Worker{source: source, compiler: compiler, sink: sink, log: log, interval: interval}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	job, err := w.source.Next(ctx)
	if err != nil {
		w.log.Error("fetch job failed", "err", err)
		return
	}
	if job == nil {
		return
	}

	w.log.Info("building", "id", job.ID, "repo", job.Repo, "commit", job.Commit)
	result, err := w.compiler.Compile(ctx, *job)
	if err != nil {
		w.log.Error("build failed", "id", job.ID, "err", err)
		return
	}

	if err := w.sink.Upload(ctx, *job, result); err != nil {
		w.log.Error("artifact upload failed", "id", job.ID, "err", err)
		return
	}

	w.log.Info("build published", "id", job.ID, "version", result.Version)
	if err := w.source.Complete(ctx, job.ID); err != nil {
		w.log.Error("complete job failed", "id", job.ID, "err", err)
	}
}
