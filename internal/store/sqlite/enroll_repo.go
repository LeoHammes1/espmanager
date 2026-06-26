package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/LeoHammes1/espmanager/internal/enroll"
)

type EnrollRepository struct {
	db *sql.DB
}

func NewEnrollRepository(db *sql.DB) *EnrollRepository {
	return &EnrollRepository{db: db}
}

// nullable maps an empty string to a SQL NULL so optional columns stay NULL
// rather than an empty string (the `device_id is null` checks rely on it).
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *EnrollRepository) CreateToken(ctx context.Context, t enroll.Token) error {
	if _, err := r.db.ExecContext(ctx,
		`delete from claim_tokens where expires_at <= strftime('%Y-%m-%dT%H:%M:%fZ')`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`insert into claim_tokens (token, expires_at, device_id) values (?, ?, ?)`,
		t.Value, t.ExpiresAt.UTC().Format(timeFormat), nullable(t.DeviceID))
	return err
}

func (r *EnrollRepository) TokenValid(ctx context.Context, value, deviceID string, now time.Time) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`select 1 from claim_tokens where token = ? and expires_at > ? and (device_id is null or device_id = ?)`,
		value, now.UTC().Format(timeFormat), deviceID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *EnrollRepository) Claim(ctx context.Context, deviceID, token, passwordHash string, now time.Time) error {
	ts := now.UTC().Format(timeFormat)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`delete from claim_tokens where token = ? and expires_at > ? and (device_id is null or device_id = ?)`,
		token, ts, deviceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return enroll.ErrInvalidToken
	}

	if _, err := tx.ExecContext(ctx,
		`insert into device_credentials (device_id, password_hash, created_at) values (?, ?, ?)`,
		deviceID, passwordHash, ts); err != nil {
		if isUniqueViolation(err) {
			return enroll.ErrAlreadyEnrolled
		}
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`insert into devices (id, enrolled_at) values (?, ?) on conflict(id) do nothing`,
		deviceID, ts); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *EnrollRepository) Credentials(ctx context.Context, deviceID string) (enroll.Credentials, bool, error) {
	var creds enroll.Credentials
	err := r.db.QueryRowContext(ctx,
		`select password_hash, coalesce(pending_hash, '') from device_credentials where device_id = ?`,
		deviceID).Scan(&creds.Hash, &creds.Pending)
	if errors.Is(err, sql.ErrNoRows) {
		return enroll.Credentials{}, false, nil
	}
	if err != nil {
		return enroll.Credentials{}, false, err
	}
	return creds, true, nil
}

func (r *EnrollRepository) Revoke(ctx context.Context, deviceID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`delete from device_credentials where device_id = ?`, deviceID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *EnrollRepository) SetPendingHash(ctx context.Context, deviceID, pendingHash string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`update device_credentials set pending_hash = ?
		 where device_id = ? and pending_hash is null`, pendingHash, deviceID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *EnrollRepository) PromotePending(ctx context.Context, deviceID string) error {
	_, err := r.db.ExecContext(ctx,
		`update device_credentials set password_hash = pending_hash, pending_hash = null
		 where device_id = ? and pending_hash is not null`, deviceID)
	return err
}
