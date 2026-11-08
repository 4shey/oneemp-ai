// Package domain holds the core data structures and the repository
// interfaces used by the rest of the application. It must not import any
// other internal package.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// Recurrence describes how often a schedule fires.
type Recurrence string

const (
	// RecurrenceOnce fires on a single explicit date.
	RecurrenceOnce Recurrence = "once"
	// RecurrenceDaily fires every day.
	RecurrenceDaily Recurrence = "daily"
	// RecurrenceWeekly fires every week on DayOfWeek.
	RecurrenceWeekly Recurrence = "weekly"
)

// Schedule is a single reminder definition stored in the schedules sheet.
type Schedule struct {
	ID        int
	Type      Recurrence
	Date      string // YYYY-MM-DD, only for RecurrenceOnce
	DayOfWeek string // Monday..Sunday, only for RecurrenceWeekly
	StartTime string // HH:MM
	EndTime   string // HH:MM
	Activity  string
	Active    bool
}

// OccursOn reports whether the schedule is planned for the given date.
func (s Schedule) OccursOn(date time.Time) bool {
	switch s.Type {
	case RecurrenceOnce:
		return s.Date == date.Format("2006-01-02")
	case RecurrenceDaily:
		return true
	case RecurrenceWeekly:
		return s.DayOfWeek == date.Weekday().String()
	default:
		return false
	}
}

// DueAt reports whether the schedule should trigger a reminder at hhmm on date.
func (s Schedule) DueAt(date time.Time, hhmm string) bool {
	return s.Active && s.OccursOn(date) && s.StartTime == hhmm
}

// EffectiveEnd returns EndTime, or one hour after StartTime when EndTime is
// empty. An empty EndTime means the user only wants a start reminder.
func (s Schedule) EffectiveEnd() string {
	if strings.TrimSpace(s.EndTime) != "" {
		return s.EndTime
	}
	parsed, err := time.Parse("15:04", s.StartTime)
	if err != nil {
		return ""
	}
	return parsed.Add(time.Hour).Format("15:04")
}

// IsOvernight reports whether the window crosses midnight.
func (s Schedule) IsOvernight() bool {
	end := s.EffectiveEnd()
	return end != "" && s.StartTime > end
}

// InWindow reports whether hhmm is inside the schedule window on date.
// Overnight schedules started yesterday remain active until EndTime today.
func (s Schedule) InWindow(date time.Time, hhmm string) bool {
	if !s.Active {
		return false
	}
	end := s.EffectiveEnd()
	if end == "" {
		return false
	}
	if s.IsOvernight() {
		if s.OccursOn(date) && hhmm >= s.StartTime {
			return true
		}
		return s.OccursOn(date.AddDate(0, 0, -1)) && hhmm < end
	}
	return s.OccursOn(date) && s.StartTime <= hhmm && hhmm < end
}

// Validate checks the invariants required before storing a schedule.
func (s Schedule) Validate() error {
	if s.Activity == "" {
		return fmt.Errorf("activity wajib diisi")
	}
	if _, err := time.Parse("15:04", s.StartTime); err != nil {
		return fmt.Errorf("start_time harus HH:MM: %w", err)
	}
	if s.EndTime != "" {
		if _, err := time.Parse("15:04", s.EndTime); err != nil {
			return fmt.Errorf("end_time harus HH:MM: %w", err)
		}
		if s.StartTime == s.EndTime {
			return fmt.Errorf("start_time dan end_time tidak boleh sama")
		}
	}

	switch s.Type {
	case RecurrenceOnce:
		if _, err := time.Parse("2006-01-02", s.Date); err != nil {
			return fmt.Errorf("date harus YYYY-MM-DD untuk jadwal sekali: %w", err)
		}
		if s.DayOfWeek != "" {
			return fmt.Errorf("day_of_week harus kosong untuk jadwal sekali")
		}
	case RecurrenceWeekly:
		if !validDay(s.DayOfWeek) {
			return fmt.Errorf("day_of_week harus Monday..Sunday")
		}
		if s.Date != "" {
			return fmt.Errorf("date harus kosong untuk jadwal mingguan")
		}
	case RecurrenceDaily:
		if s.Date != "" || s.DayOfWeek != "" {
			return fmt.Errorf("date dan day_of_week harus kosong untuk jadwal harian")
		}
	default:
		return fmt.Errorf("type harus once, daily, atau weekly")
	}
	return nil
}

func validDay(day string) bool {
	switch day {
	case "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday":
		return true
	default:
		return false
	}
}
