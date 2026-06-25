package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/LeoHammes1/espmanager/internal/driver"
)

type DriverRepository struct {
	db *sql.DB
}

func NewDriverRepository(db *sql.DB) *DriverRepository {
	return &DriverRepository{db: db}
}

const driverColumns = "id, name, repo_url, branch, pio_env, partition_scheme, webhook_secret, created_at"

func (r *DriverRepository) Create(ctx context.Context, d driver.Driver) error {
	_, err := r.db.ExecContext(ctx, `
		insert into drivers (`+driverColumns+`)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Name, d.RepoURL, d.Branch, d.PioEnv, d.PartitionScheme, d.WebhookSecret,
		d.CreatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DriverRepository) List(ctx context.Context) ([]driver.Driver, error) {
	return r.query(ctx, `select `+driverColumns+` from drivers order by name`)
}

func (r *DriverRepository) ListByRepo(ctx context.Context, repoURL string) ([]driver.Driver, error) {
	return r.query(ctx, `select `+driverColumns+` from drivers where repo_url = ?`, repoURL)
}

func (r *DriverRepository) Get(ctx context.Context, id string) (driver.Driver, error) {
	row := r.db.QueryRowContext(ctx, `select `+driverColumns+` from drivers where id = ?`, id)
	return scanDriver(row)
}

func (r *DriverRepository) query(ctx context.Context, q string, args ...any) ([]driver.Driver, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []driver.Driver
	for rows.Next() {
		d, err := scanDriver(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanDriver(s rowScanner) (driver.Driver, error) {
	var d driver.Driver
	var createdAt string
	if err := s.Scan(&d.ID, &d.Name, &d.RepoURL, &d.Branch, &d.PioEnv, &d.PartitionScheme, &d.WebhookSecret, &createdAt); err != nil {
		return driver.Driver{}, err
	}
	d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return d, nil
}
