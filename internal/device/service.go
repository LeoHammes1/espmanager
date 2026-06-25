package device

import (
	"context"
	"log/slog"
	"time"
)

type Service struct {
	repo     Repository
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

func NewService(repo Repository, notifier Notifier, log *slog.Logger) *Service {
	return &Service{repo: repo, notifier: notifier, log: log, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]Device, error) {
	return s.repo.List(ctx)
}

func (s *Service) Connected(id string) {
	s.apply(id, s.repo.SetPresence(context.Background(), id, true, s.now()))
}

func (s *Service) Disconnected(id string) {
	s.apply(id, s.repo.SetPresence(context.Background(), id, false, s.now()))
}

func (s *Service) Seen(id, topic string, payload []byte) {
	s.apply(id, s.repo.Touch(context.Background(), id, s.now()))
}

func (s *Service) apply(id string, err error) {
	if err != nil {
		s.log.Error("device update failed", "device", id, "err", err)
		return
	}
	s.notifier.DeviceChanged()
}
