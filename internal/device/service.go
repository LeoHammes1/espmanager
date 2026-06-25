package device

import (
	"context"
	"log/slog"
	"time"
)

type Service struct {
	repo     Repository
	drivers  DriverChecker
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

func NewService(repo Repository, drivers DriverChecker, notifier Notifier, log *slog.Logger) *Service {
	return &Service{repo: repo, drivers: drivers, notifier: notifier, log: log, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]Device, error) {
	return s.repo.List(ctx)
}

func (s *Service) Assign(ctx context.Context, id, driverID string) error {
	if driverID != "" {
		exists, err := s.drivers.Exists(ctx, driverID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrDriverNotFound
		}
	}
	if err := s.repo.Assign(ctx, id, driverID); err != nil {
		return err
	}
	s.notifier.DeviceChanged()
	return nil
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
