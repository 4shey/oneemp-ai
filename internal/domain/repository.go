package domain

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("data tidak ditemukan")

// Reminder types stored in reminder_state.
const (
	ReminderTypeSchedule = "reminder"
	ReminderTypeEnd      = "end"
	ReminderTypeRecap    = "recap"
)

// ScheduleRepository abstracts the schedules table.
type ScheduleRepository interface {
	List(ctx context.Context) ([]Schedule, error)
	Get(ctx context.Context, id int) (Schedule, error)
	Create(ctx context.Context, schedule Schedule) (Schedule, error)
	Update(ctx context.Context, schedule Schedule) (Schedule, error)
	Delete(ctx context.Context, id int) error
	DeleteMany(ctx context.Context, ids []int) (deleted int, missing []int, err error)
}

// ProgressRepository abstracts the progress_logs table.
type ProgressRepository interface {
	ListByDate(ctx context.Context, date string) ([]Progress, error)
	ListAll(ctx context.Context) ([]Progress, error)
	ListArchive(ctx context.Context) ([]Progress, error)
	Upsert(ctx context.Context, progress Progress) (Progress, error)
	UpsertArchive(ctx context.Context, progress Progress) (Progress, error)
	Delete(ctx context.Context, filter ProgressFilter) (int, error)
	DeleteArchive(ctx context.Context, filter ProgressFilter) (int, error)
	MarkPendingAsMissed(ctx context.Context, date string) error
	ArchiveBefore(ctx context.Context, cutoffDate string) (int, error)
}

// ReminderState is one row in the reminder_state sheet.
type ReminderState struct {
	Date       string
	ScheduleID int
	Type       string
	SentAt     string
}

// ReminderFilter selects reminder_state rows for deletion. All ignores the
// other fields. Before deletes rows with date < Before.
type ReminderFilter struct {
	All        bool
	Date       string
	ScheduleID int
	Type       string
	Before     string
}

// ReminderStateRepository abstracts the reminder_state table. It is used to
// make reminder delivery idempotent across restarts.
type ReminderStateRepository interface {
	WasSent(ctx context.Context, date string, scheduleID int, reminderType string) (bool, error)
	MarkSent(ctx context.Context, date string, scheduleID int, reminderType string, sentAt string) error
	ListAll(ctx context.Context) ([]ReminderState, error)
	Upsert(ctx context.Context, state ReminderState) (ReminderState, error)
	Delete(ctx context.Context, filter ReminderFilter) (int, error)
	PurgeBefore(ctx context.Context, cutoffDate string) error
}
