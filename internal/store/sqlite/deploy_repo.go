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
		`insert into deploys (id, driver_id, version, state, created_at) values (?, ?, ?, ?, ?)`,
		d.ID, d.DriverID, d.Version, string(d.State), d.CreatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DeployRepository) AddTarget(ctx context.Context, t deploy.Target) error {
	_, err := r.db.ExecContext(ctx, `
		insert into deploy_targets (deploy_id, device_id, version, sequence, batch, status, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)`,
		t.DeployID, t.DeviceID, t.Version, t.Sequence, t.Batch, string(t.Status), t.UpdatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DeployRepository) AdvanceTargetStatus(ctx context.Context, deployID, deviceID string, status deploy.Status, at time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		update deploy_targets set status = ?, updated_at = ?
		where deploy_id = ? and device_id = ? and status not in ('succeeded', 'failed', 'lost') and status <> ?`,
		string(status), at.UTC().Format(timeFormat), deployID, deviceID, string(status))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *DeployRepository) AdvanceTargetStatusBySequence(ctx context.Context, deviceID string, sequence int64, status deploy.Status, at time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		update deploy_targets set status = ?, updated_at = ?
		where rowid = (
			select t.rowid from deploy_targets t join deploys d on d.id = t.deploy_id
			where t.device_id = ? and t.sequence = ? and t.status not in ('succeeded', 'failed', 'lost') and t.status <> ?
			order by d.created_at desc limit 1
		)`,
		string(status), at.UTC().Format(timeFormat), deviceID, sequence, string(status))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *DeployRepository) LatestTargetForDevice(ctx context.Context, deviceID string) (deploy.Target, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		select t.deploy_id, t.device_id, t.version, t.sequence, t.batch, t.status, t.updated_at
		from deploy_targets t join deploys d on d.id = t.deploy_id
		where t.device_id = ? order by d.created_at desc, t.updated_at desc limit 1`, deviceID)

	t, err := scanTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return deploy.Target{}, false, nil
	}
	if err != nil {
		return deploy.Target{}, false, err
	}
	return t, true, nil
}

func (r *DeployRepository) ListActiveDeploys(ctx context.Context) ([]deploy.Deploy, error) {
	rows, err := r.db.QueryContext(ctx,
		`select id, driver_id, version, state, created_at from deploys where state = ? order by created_at`,
		string(deploy.StateInProgress))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []deploy.Deploy
	for rows.Next() {
		var d deploy.Deploy
		var state, createdAt string
		if err := rows.Scan(&d.ID, &d.DriverID, &d.Version, &state, &createdAt); err != nil {
			return nil, err
		}
		d.State = deploy.State(state)
		d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DeployRepository) TargetsForDeploy(ctx context.Context, deployID string) ([]deploy.Target, error) {
	rows, err := r.db.QueryContext(ctx, `
		select deploy_id, device_id, version, sequence, batch, status, updated_at
		from deploy_targets where deploy_id = ?`, deployID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []deploy.Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *DeployRepository) SetDeployState(ctx context.Context, deployID string, state deploy.State) error {
	_, err := r.db.ExecContext(ctx,
		`update deploys set state = ? where id = ?`, string(state), deployID)
	return err
}

func scanTarget(s rowScanner) (deploy.Target, error) {
	var t deploy.Target
	var status, updatedAt string
	if err := s.Scan(&t.DeployID, &t.DeviceID, &t.Version, &t.Sequence, &t.Batch, &status, &updatedAt); err != nil {
		return deploy.Target{}, err
	}
	t.Status = deploy.Status(status)
	t.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
	return t, nil
}
