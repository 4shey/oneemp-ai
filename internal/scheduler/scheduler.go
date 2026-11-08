// Package scheduler drives the reminder/recap ticker.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"one_more_percent/internal/config"
	"one_more_percent/internal/service"
)

// Scheduler runs periodic reminder checks.
type Scheduler struct {
	cfg     *config.Config
	service *service.ReminderService
	logger  *slog.Logger
}

// New builds a scheduler.
func New(cfg *config.Config, svc *service.ReminderService, logger *slog.Logger) *Scheduler {
	return &Scheduler{cfg: cfg, service: svc, logger: logger}
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.service.CatchUp(ctx)

	ticker := time.NewTicker(s.cfg.CronTick)
	defer ticker.Stop()

	s.logger.Info("scheduler berjalan", "tick", s.cfg.CronTick.String(), "recap_time", s.cfg.DailyRecapTime)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler berhenti")
			return
		case <-ticker.C:
			s.service.Tick(ctx)
		}
	}
}
