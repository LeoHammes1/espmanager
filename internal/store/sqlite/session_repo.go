package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, id string, expiresAt time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`delete from sessions where expires_at <= strftime('%Y-%m-%dT%H:%M:%fZ')`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`insert into sessions (id, expires_at) values (?, ?)`,
		id, expiresAt.UTC().Format(timeFormat))
	return err
}

func (r *SessionRepository) Valid(ctx context.Context, id string, now time.Time) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`select 1 from sessions where id = ? and expires_at > ?`,
		id, now.UTC().Format(timeFormat)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `delete from sessions where id = ?`, id)
	return err
}
