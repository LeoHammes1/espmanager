package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"maragu.dev/goqite"
)

type BuildJob struct {
	DriverID string `json:"driver_id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Env      string `json:"env"`
}

type LeasedJob struct {
	ID  string   `json:"id"`
	Job BuildJob `json:"job"`
}

type Queue struct {
	q *goqite.Queue
}

func New(db *sql.DB, name string, timeout time.Duration) *Queue {
	return &Queue{q: goqite.New(goqite.NewOpts{DB: db, Name: name, Timeout: timeout})}
}

func (q *Queue) Enqueue(ctx context.Context, job BuildJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.q.Send(ctx, goqite.Message{Body: body})
}

func (q *Queue) Lease(ctx context.Context) (*LeasedJob, error) {
	msg, err := q.q.Receive(ctx)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	var job BuildJob
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		return nil, err
	}
	return &LeasedJob{ID: string(msg.ID), Job: job}, nil
}

func (q *Queue) Complete(ctx context.Context, id string) error {
	return q.q.Delete(ctx, goqite.ID(id))
}
