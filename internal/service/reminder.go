package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"one_more_percent/internal/ai"
	"one_more_percent/internal/config"
	"one_more_percent/internal/domain"
	"one_more_percent/internal/prompt"
)

const endReminderGrace = 30 * time.Minute

// ReminderService owns reminders, daily recaps, and retention maintenance.
type ReminderService struct {
	cfg       *config.Config
	ai        ai.Client
	prompts   *prompt.Loader
	schedules domain.ScheduleRepository
	progress  domain.ProgressRepository
	reminders domain.ReminderStateRepository
	messenger domain.Messenger
	logger    *slog.Logger

	mu              sync.Mutex
	lastMaintenance string
	recapCooldown   time.Time
}

// NewReminderService wires reminder behaviour.
func NewReminderService(
	cfg *config.Config,
	aiClient ai.Client,
	prompts *prompt.Loader,
	schedules domain.ScheduleRepository,
	progress domain.ProgressRepository,
	reminders domain.ReminderStateRepository,
	messenger domain.Messenger,
	logger *slog.Logger,
) *ReminderService {
	return &ReminderService{
		cfg:       cfg,
		ai:        aiClient,
		prompts:   prompts,
		schedules: schedules,
		progress:  progress,
		reminders: reminders,
		messenger: messenger,
		logger:    logger,
	}
}

// CatchUp runs once at startup: it sends reminders for windows that are
// already open and a missing recap for yesterday.
func (s *ReminderService) CatchUp(ctx context.Context) {
	now := time.Now().In(s.cfg.Location)
	s.runMaintenance(ctx, now)
	s.runRecap(ctx, now)
	s.runReminders(ctx, now)
}

// Tick runs on every scheduler interval.
func (s *ReminderService) Tick(ctx context.Context) {
	now := time.Now().In(s.cfg.Location)
	s.runMaintenance(ctx, now)
	s.runRecap(ctx, now)
	s.runReminders(ctx, now)
}

func (s *ReminderService) runReminders(ctx context.Context, now time.Time) {
	schedules, err := s.schedules.List(ctx)
	if err != nil {
		s.logger.Error("scheduler baca jadwal", "error", err)
		return
	}

	currentHHMM := now.Format("15:04")

	for _, schedule := range schedules {
		if !schedule.Active {
			continue
		}

		// Use the whole window, not only the exact start minute: this makes
		// reminders resilient to cache TTL, delayed ticks, and restarts.
		if schedule.InWindow(now, currentHHMM) {
			if err := s.sendReminder(ctx, schedule, now); err != nil {
				s.logger.Error("kirim reminder", "schedule_id", schedule.ID, "error", err)
			}
		}
		if _, ok := endOccurrenceDate(schedule, now, endReminderGrace); ok {
			if err := s.sendEndReminder(ctx, schedule, now); err != nil {
				s.logger.Error("kirim reminder selesai", "schedule_id", schedule.ID, "error", err)
			}
		}
	}
}

func (s *ReminderService) sendReminder(ctx context.Context, schedule domain.Schedule, now time.Time) error {
	startDate, ok := startOccurrenceDate(schedule, now)
	if !ok {
		return nil
	}
	reminderDate := startDate.Format("2006-01-02")
	progressDate := progressDateFor(schedule, startDate)

	sent, err := s.reminders.WasSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeSchedule)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	existing, err := s.progress.ListByDate(ctx, progressDate)
	if err == nil {
		for _, item := range existing {
			if item.ScheduleID != schedule.ID {
				continue
			}
			if item.Status != domain.ProgressPending {
				// Already completed/missed; never nag again.
				if err := s.reminders.MarkSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeSchedule, now.Format(time.RFC3339)); err != nil {
					s.logger.Warn("tandai reminder lama", "error", err)
				}
				return nil
			}
		}
	}
	if _, err := s.progress.Upsert(ctx, domain.Progress{
		Date:       progressDate,
		ScheduleID: schedule.ID,
		Activity:   schedule.Activity,
		Status:     domain.ProgressPending,
	}); err != nil {
		return fmt.Errorf("buat progres pending: %w", err)
	}

	message := s.reminderMessage(ctx, schedule, now)
	if err := s.messenger.SendMessage(ctx, s.cfg.TelegramUserID, message); err != nil {
		return fmt.Errorf("kirim telegram: %w", err)
	}

	if err := s.reminders.MarkSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeSchedule, now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("tandai reminder: %w", err)
	}
	s.logger.Info("reminder terkirim", "schedule_id", schedule.ID, "activity", schedule.Activity)
	return nil
}

func (s *ReminderService) sendEndReminder(ctx context.Context, schedule domain.Schedule, now time.Time) error {
	startDate, ok := endOccurrenceDate(schedule, now, endReminderGrace)
	if !ok {
		return nil
	}
	reminderDate := startDate.Format("2006-01-02")
	progressDate := progressDateFor(schedule, startDate)

	sent, err := s.reminders.WasSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeEnd)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	exists := false
	existing, err := s.progress.ListByDate(ctx, progressDate)
	if err == nil {
		for _, item := range existing {
			if item.ScheduleID != schedule.ID {
				continue
			}
			exists = true
			if item.Status == domain.ProgressMissed {
				if err := s.reminders.MarkSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeEnd, now.Format(time.RFC3339)); err != nil {
					s.logger.Warn("tandai reminder selesai lama", "error", err)
				}
				return nil
			}
			if item.Status == domain.ProgressCompleted && !schedule.IsOvernight() {
				if err := s.reminders.MarkSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeEnd, now.Format(time.RFC3339)); err != nil {
					s.logger.Warn("tandai reminder selesai lama", "error", err)
				}
				return nil
			}
		}
	}
	if !exists {
		if _, err := s.progress.Upsert(ctx, domain.Progress{
			Date:       progressDate,
			ScheduleID: schedule.ID,
			Activity:   schedule.Activity,
			Status:     domain.ProgressPending,
		}); err != nil {
			return fmt.Errorf("buat progres pending selesai: %w", err)
		}
	}

	message := s.endReminderMessage(ctx, schedule, now)
	if err := s.messenger.SendMessage(ctx, s.cfg.TelegramUserID, message); err != nil {
		return fmt.Errorf("kirim telegram selesai: %w", err)
	}

	if err := s.reminders.MarkSent(ctx, reminderDate, schedule.ID, domain.ReminderTypeEnd, now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("tandai reminder selesai: %w", err)
	}
	s.logger.Info("reminder selesai terkirim", "schedule_id", schedule.ID, "activity", schedule.Activity)
	return nil
}

func (s *ReminderService) reminderMessage(ctx context.Context, schedule domain.Schedule, now time.Time) string {
	rendered, err := s.prompts.Render("reminder_message", map[string]any{
		"Now":       now.Format("Monday, 02 January 2006 15:04 MST"),
		"Activity":  schedule.Activity,
		"StartTime": schedule.StartTime,
		"EndTime":   schedule.EndTime,
	})
	if err == nil {
		resp, aiErr := s.ai.Complete(ctx, ai.Request{
			Messages: []ai.Message{{Role: ai.RoleUser, Content: rendered}},
		})
		if aiErr == nil && strings.TrimSpace(resp.Content) != "" {
			return resp.Content
		}
		s.logger.Warn("AI reminder gagal, pakai fallback", "error", aiErr)
	} else {
		s.logger.Warn("render prompt reminder", "error", err)
	}

	return fmt.Sprintf("Waktunya %s %s. Gerak, jangan males.", schedule.Activity, scheduleTimeLabel(schedule))
}

func (s *ReminderService) endReminderMessage(ctx context.Context, schedule domain.Schedule, now time.Time) string {
	rendered, err := s.prompts.Render("end_reminder_message", map[string]any{
		"Now":         now.Format("Monday, 02 January 2006 15:04 MST"),
		"Activity":    schedule.Activity,
		"StartTime":   schedule.StartTime,
		"EndTime":     schedule.EndTime,
		"UserContext": s.cfg.UserContext,
	})
	if err == nil {
		resp, aiErr := s.ai.Complete(ctx, ai.Request{
			Messages: []ai.Message{{Role: ai.RoleUser, Content: rendered}},
		})
		if aiErr == nil && strings.TrimSpace(resp.Content) != "" {
			return resp.Content
		}
		s.logger.Warn("AI reminder selesai gagal, pakai fallback", "error", aiErr)
	} else {
		s.logger.Warn("render prompt reminder selesai", "error", err)
	}

	return fmt.Sprintf("Jadwal %s harusnya selesai jam %s. Udah kelar?", schedule.Activity, schedule.EndTime)
}

func (s *ReminderService) runRecap(ctx context.Context, now time.Time) {
	if now.Before(s.recapCooldown) {
		return
	}
	// Yesterday's recap only becomes due once DAILY_RECAP_TIME is reached today.
	if now.Format("15:04") < s.cfg.DailyRecapTime {
		return
	}

	recapDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	sent, err := s.reminders.WasSent(ctx, recapDate, 0, domain.ReminderTypeRecap)
	if err != nil {
		s.logger.Error("cek state recap", "error", err)
		return
	}
	if sent {
		return
	}

	if err := s.progress.MarkPendingAsMissed(ctx, recapDate); err != nil {
		s.logger.Error("tandai progres missed", "error", err)
	}

	items, err := s.progress.ListByDate(ctx, recapDate)
	if err != nil {
		s.logger.Error("baca progres recap", "error", err)
		return
	}
	if len(items) == 0 {
		s.markRecapSent(ctx, recapDate)
		return
	}

	message, err := s.recapMessage(ctx, recapDate, items)
	if err != nil {
		s.logger.Error("buat recap", "error", err)
		s.recapCooldown = now.Add(5 * time.Minute)
		return
	}
	if err := s.messenger.SendMessage(ctx, s.cfg.TelegramUserID, message); err != nil {
		s.logger.Error("kirim recap", "error", err)
		s.recapCooldown = now.Add(5 * time.Minute)
		return
	}
	s.markRecapSent(ctx, recapDate)
	s.logger.Info("recap terkirim", "date", recapDate)
}

func (s *ReminderService) recapMessage(ctx context.Context, date string, items []domain.Progress) (string, error) {
	var completed, missed []string
	for _, item := range items {
		if item.Status == domain.ProgressCompleted {
			completed = append(completed, "- "+item.Activity)
		} else {
			missed = append(missed, "- "+item.Activity)
		}
	}

	completedText := "tidak ada"
	if len(completed) > 0 {
		completedText = strings.Join(completed, "\n")
	}
	missedText := "tidak ada"
	if len(missed) > 0 {
		missedText = strings.Join(missed, "\n")
	}

	userContext := ""
	if len(missed) > 0 {
		userContext = s.cfg.UserContext
	}
	rendered, err := s.prompts.Render("daily_recap", map[string]any{
		"Date":           date,
		"Completed":      completedText,
		"Missed":         missedText,
		"CompletedCount": len(completed),
		"Total":          len(items),
		"UserContext":    userContext,
	})
	if err != nil {
		return "", err
	}

	resp, err := s.ai.Complete(ctx, ai.Request{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: rendered}},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("recap AI kosong")
	}
	return resp.Content, nil
}

func (s *ReminderService) markRecapSent(ctx context.Context, recapDate string) {
	if err := s.reminders.MarkSent(ctx, recapDate, 0, domain.ReminderTypeRecap, time.Now().In(s.cfg.Location).Format(time.RFC3339)); err != nil {
		s.logger.Warn("tandai recap", "error", err)
	}
}

func (s *ReminderService) runMaintenance(ctx context.Context, now time.Time) {
	today := now.Format("2006-01-02")

	s.mu.Lock()
	if s.lastMaintenance == today {
		s.mu.Unlock()
		return
	}
	s.lastMaintenance = today
	s.mu.Unlock()

	reminderCutoff := now.AddDate(0, 0, -s.cfg.ReminderRetentionDays).Format("2006-01-02")
	if err := s.reminders.PurgeBefore(ctx, reminderCutoff); err != nil {
		s.logger.Warn("bersihkan reminder_state", "error", err)
	}

	if s.cfg.SheetArchiveEnabled {
		progressCutoff := now.AddDate(0, 0, -s.cfg.ProgressRetentionDays).Format("2006-01-02")
		if count, err := s.progress.ArchiveBefore(ctx, progressCutoff); err != nil {
			s.logger.Warn("arsipkan progress", "error", err)
		} else if count > 0 {
			s.logger.Info("progress lama diarsipkan", "rows", count)
		}
	}

	s.deactivateExpiredOnceSchedules(ctx, now)
}

func (s *ReminderService) deactivateExpiredOnceSchedules(ctx context.Context, now time.Time) {
	schedules, err := s.schedules.List(ctx)
	if err != nil {
		s.logger.Warn("baca jadwal maintenance", "error", err)
		return
	}
	for _, schedule := range schedules {
		if schedule.Type != domain.RecurrenceOnce || !schedule.Active || schedule.Date == "" {
			continue
		}
		startDate, err := time.ParseInLocation("2006-01-02", schedule.Date, now.Location())
		if err != nil {
			continue
		}
		endDate := startDate
		if schedule.IsOvernight() {
			endDate = endDate.AddDate(0, 0, 1)
		}
		endAt := combineDateAndTime(endDate, schedule.EffectiveEnd())
		if endAt.IsZero() || now.Before(endAt.Add(endReminderGrace)) {
			continue
		}
		schedule.Active = false
		if _, err := s.schedules.Update(ctx, schedule); err != nil {
			s.logger.Warn("nonaktifkan jadwal sekali", "schedule_id", schedule.ID, "error", err)
		}
	}
}
