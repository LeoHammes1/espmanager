package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/LeoHammes1/espmanager/internal/deploy"
)

type DeployRepository struct {
	db *sql.DB
}

func NewDeployRepository(db *sql.DB) *DeployRepository {
	return &DeployRepository{db: db}
}

func (r *DeployRepository) CreateDeploy(ctx context.Context, d deploy.Deploy) error {
	_, err := r.db.ExecContext(ctx,
		`insert into deploys (id, driver_id, version, created_at) values (?, ?, ?, ?)`,
		d.ID, d.DriverID, d.Version, d.CreatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DeployRepository) AddTarget(ctx context.Context, t deploy.Target) error {
	_, err := r.db.ExecContext(ctx, `
		insert into deploy_targets (deploy_id, device_id, version, status, updated_at)
		values (?, ?, ?, ?, ?)`,
		t.DeployID, t.DeviceID, t.Version, string(t.Status), t.UpdatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DeployRepository) SetTargetStatus(ctx context.Context, deployID, deviceID string, status deploy.Status, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		update deploy_targets set status = ?, updated_at = ?
		where deploy_id = ? and device_id = ?`,
		string(status), at.UTC().Format(timeFormat), deployID, deviceID)
	return err
}

func (r *DeployRepository) AdvanceTargetStatus(ctx context.Context, deployID, deviceID string, status deploy.Status, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		update deploy_targets set status = ?, updated_at = ?
		where deploy_id = ? and device_id = ? and status not in ('succeeded', 'failed')`,
		string(status), at.UTC().Format(timeFormat), deployID, deviceID)
	return err
}

func (r *DeployRepository) LatestTargetForDevice(ctx context.Context, deviceID string) (deploy.Target, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		select t.deploy_id, t.device_id, t.version, t.status, t.updated_at
		from deploy_targets t join deploys d on d.id = t.deploy_id
		where t.device_id = ? order by d.created_at desc, t.updated_at desc limit 1`, deviceID)

	var t deploy.Target
	var status, updatedAt string
	err := row.Scan(&t.DeployID, &t.DeviceID, &t.Version, &status, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return deploy.Target{}, false, nil
	}
	if err != nil {
		return deploy.Target{}, false, err
	}
	t.Status = deploy.Status(status)
	t.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
	return t, true, nil
}
