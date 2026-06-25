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

func (r *EnrollRepository) CreateToken(ctx context.Context, t enroll.Token) error {
	if _, err := r.db.ExecContext(ctx,
		`delete from claim_tokens where expires_at <= strftime('%Y-%m-%dT%H:%M:%fZ')`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`insert into claim_tokens (token, expires_at) values (?, ?)`,
		t.Value, t.ExpiresAt.UTC().Format(timeFormat))
	return err
}

func (r *EnrollRepository) TokenValid(ctx context.Context, value string, now time.Time) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`select 1 from claim_tokens where token = ? and expires_at > ?`,
		value, now.UTC().Format(timeFormat)).Scan(&one)
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
		`delete from claim_tokens where token = ? and expires_at > ?`, token, ts)
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

func (r *EnrollRepository) CredentialHash(ctx context.Context, deviceID string) (string, bool, error) {
	var hash string
	err := r.db.QueryRowContext(ctx,
		`select password_hash from device_credentials where device_id = ?`, deviceID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}
