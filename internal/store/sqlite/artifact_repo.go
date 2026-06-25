package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
)

type ArtifactRepository struct {
	db *sql.DB
}

func NewArtifactRepository(db *sql.DB) *ArtifactRepository {
	return &ArtifactRepository{db: db}
}

const artifactColumns = "driver_id, version, commit_sha, env, sha256, signature, sequence, size, created_at"

func (r *ArtifactRepository) Create(ctx context.Context, a artifact.Artifact) error {
	_, err := r.db.ExecContext(ctx, `
		insert into firmware_artifacts (`+artifactColumns+`)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.DriverID, a.Version, a.Commit, a.Env, a.SHA256, a.Signature, a.Sequence, a.Size,
		a.CreatedAt.UTC().Format(timeFormat))
	if isUniqueViolation(err) {
		return artifact.ErrAlreadyExists
	}
	return err
}

func (r *ArtifactRepository) NextSequence(ctx context.Context) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `update artifact_sequence set value = value + 1 where id = 1`); err != nil {
		return 0, err
	}
	var next int64
	if err := tx.QueryRowContext(ctx, `select value from artifact_sequence where id = 1`).Scan(&next); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return next, nil
}

func (r *ArtifactRepository) Delete(ctx context.Context, driverID, version string) error {
	_, err := r.db.ExecContext(ctx,
		`delete from firmware_artifacts where driver_id = ? and version = ?`, driverID, version)
	return err
}

func (r *ArtifactRepository) Get(ctx context.Context, driverID, version string) (artifact.Artifact, error) {
	row := r.db.QueryRowContext(ctx,
		`select `+artifactColumns+` from firmware_artifacts where driver_id = ? and version = ?`,
		driverID, version)

	var a artifact.Artifact
	var createdAt string
	err := row.Scan(&a.DriverID, &a.Version, &a.Commit, &a.Env, &a.SHA256, &a.Signature, &a.Sequence, &a.Size, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return artifact.Artifact{}, artifact.ErrNotFound
	}
	if err != nil {
		return artifact.Artifact{}, err
	}
	a.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return a, nil
}
