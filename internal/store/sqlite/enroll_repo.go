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
	_, err := r.db.ExecContext(ctx,
		`insert into claim_tokens (token, expires_at) values (?, ?)`,
		t.Value, t.ExpiresAt.UTC().Format(timeFormat))
	return err
}

func (r *EnrollRepository) ConsumeToken(ctx context.Context, value string, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		update claim_tokens set used_at = ?
		where token = ? and used_at = '' and expires_at > ?`,
		now.UTC().Format(timeFormat), value, now.UTC().Format(timeFormat))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *EnrollRepository) SaveCredential(ctx context.Context, deviceID, passwordHash string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		insert into device_credentials (device_id, password_hash, created_at) values (?, ?, ?)
		on conflict(device_id) do update set password_hash = excluded.password_hash, created_at = excluded.created_at`,
		deviceID, passwordHash, at.UTC().Format(timeFormat))
	return err
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
