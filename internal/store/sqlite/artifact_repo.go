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

const artifactColumns = "driver_id, version, commit_sha, env, sha256, signature, size, created_at"

func (r *ArtifactRepository) Create(ctx context.Context, a artifact.Artifact) error {
	_, err := r.db.ExecContext(ctx, `
		insert into firmware_artifacts (`+artifactColumns+`)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(driver_id, version) do update set
			commit_sha = excluded.commit_sha,
			env = excluded.env,
			sha256 = excluded.sha256,
			signature = excluded.signature,
			size = excluded.size,
			created_at = excluded.created_at`,
		a.DriverID, a.Version, a.Commit, a.Env, a.SHA256, a.Signature, a.Size,
		a.CreatedAt.UTC().Format(timeFormat))
	return err
}

func (r *ArtifactRepository) Get(ctx context.Context, driverID, version string) (artifact.Artifact, error) {
	row := r.db.QueryRowContext(ctx,
		`select `+artifactColumns+` from firmware_artifacts where driver_id = ? and version = ?`,
		driverID, version)

	var a artifact.Artifact
	var createdAt string
	err := row.Scan(&a.DriverID, &a.Version, &a.Commit, &a.Env, &a.SHA256, &a.Signature, &a.Size, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return artifact.Artifact{}, artifact.ErrNotFound
	}
	if err != nil {
		return artifact.Artifact{}, err
	}
	a.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return a, nil
}
