package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/LeoHammes1/espmanager/internal/driver"
)

type DriverRepository struct {
	db *sql.DB
}

func NewDriverRepository(db *sql.DB) *DriverRepository {
	return &DriverRepository{db: db}
}

const driverColumns = "id, name, repo_url, branch, pio_env, webhook_secret, created_at"

func (r *DriverRepository) Create(ctx context.Context, d driver.Driver) error {
	_, err := r.db.ExecContext(ctx, `
		insert into drivers (`+driverColumns+`)
		values (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Name, d.RepoURL, d.Branch, d.PioEnv, d.WebhookSecret,
		d.CreatedAt.UTC().Format(timeFormat))
	return err
}

func (r *DriverRepository) List(ctx context.Context) ([]driver.Driver, error) {
	rows, err := r.db.QueryContext(ctx, `select `+driverColumns+` from drivers order by name`)
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

func (r *DriverRepository) Get(ctx context.Context, id string) (driver.Driver, error) {
	row := r.db.QueryRowContext(ctx, `select `+driverColumns+` from drivers where id = ?`, id)
	d, err := scanDriver(row)
	if errors.Is(err, sql.ErrNoRows) {
		return driver.Driver{}, driver.ErrNotFound
	}
	return d, err
}

func scanDriver(s rowScanner) (driver.Driver, error) {
	var d driver.Driver
	var createdAt string
	if err := s.Scan(&d.ID, &d.Name, &d.RepoURL, &d.Branch, &d.PioEnv, &d.WebhookSecret, &createdAt); err != nil {
		return driver.Driver{}, err
	}
	d.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return d, nil
}
