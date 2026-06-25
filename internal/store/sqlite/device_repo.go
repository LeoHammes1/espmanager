package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/leohammes/espmanager/internal/device"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

type DeviceRepository struct {
	db *sql.DB
}

func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) List(ctx context.Context) ([]device.Device, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, name, chip_type, flash_size, driver_id, online, last_seen_at, reported_version, enrolled_at
		from devices order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []device.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DeviceRepository) Get(ctx context.Context, id string) (device.Device, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, name, chip_type, flash_size, driver_id, online, last_seen_at, reported_version, enrolled_at
		from devices where id = ?`, id)
	return scanDevice(row)
}

func (r *DeviceRepository) SetPresence(ctx context.Context, id string, online bool, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		insert into devices (id, online, last_seen_at) values (?, ?, ?)
		on conflict(id) do update set online = excluded.online, last_seen_at = excluded.last_seen_at`,
		id, boolToInt(online), at.UTC().Format(timeFormat))
	return err
}

func (r *DeviceRepository) Touch(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		insert into devices (id, online, last_seen_at) values (?, 1, ?)
		on conflict(id) do update set last_seen_at = excluded.last_seen_at`,
		id, at.UTC().Format(timeFormat))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDevice(s rowScanner) (device.Device, error) {
	var d device.Device
	var online int
	var lastSeen, enrolledAt string
	if err := s.Scan(&d.ID, &d.Name, &d.ChipType, &d.FlashSize, &d.DriverID, &online, &lastSeen, &d.ReportedVersion, &enrolledAt); err != nil {
		return device.Device{}, err
	}
	d.Online = online == 1
	d.LastSeenAt, _ = time.Parse(timeFormat, lastSeen)
	d.EnrolledAt, _ = time.Parse(timeFormat, enrolledAt)
	return d, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
