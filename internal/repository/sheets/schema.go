package sheets

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"one_more_percent/internal/domain"
)

// Column headers. Keep these in sync with cmd/setup and docs.
var (
	ScheduleHeaders = []string{"id", "type", "date", "day_of_week", "start_time", "end_time", "activity", "active"}
	ProgressHeaders = []string{"id", "date", "schedule_id", "activity", "status", "note", "created_at"}
	ReminderHeaders = []string{"date", "schedule_id", "type", "sent_at"}
)

func scheduleToRow(s domain.Schedule) []any {
	return []any{
		s.ID, string(s.Type), s.Date, s.DayOfWeek,
		s.StartTime, s.EndTime, s.Activity, boolString(s.Active),
	}
}

func rowToSchedule(row []string) (domain.Schedule, error) {
	row = pad(row, len(ScheduleHeaders))
	id, err := strconv.Atoi(strings.TrimSpace(row[0]))
	if err != nil {
		return domain.Schedule{}, fmt.Errorf("id tidak valid: %w", err)
	}

	recurrence := domain.Recurrence(strings.ToLower(strings.TrimSpace(row[1])))
	if recurrence == "" {
		recurrence = domain.RecurrenceWeekly
	}

	return domain.Schedule{
		ID:        id,
		Type:      recurrence,
		Date:      normalizeSheetDate(row[2]),
		DayOfWeek: strings.TrimSpace(row[3]),
		StartTime: normalizeSheetTime(row[4]),
		EndTime:   normalizeSheetTime(row[5]),
		Activity:  strings.TrimSpace(row[6]),
		Active:    parseBool(row[7]),
	}, nil
}

func progressToRow(p domain.Progress) []any {
	created := p.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return []any{p.ID, p.Date, p.ScheduleID, p.Activity, p.Status, p.Note, created.Format(time.RFC3339)}
}

func rowToProgress(row []string) (domain.Progress, error) {
	row = pad(row, len(ProgressHeaders))
	id, err := strconv.Atoi(strings.TrimSpace(row[0]))
	if err != nil {
		return domain.Progress{}, fmt.Errorf("id tidak valid: %w", err)
	}
	scheduleID, _ := strconv.Atoi(strings.TrimSpace(row[2]))
	created, _ := time.Parse(time.RFC3339, strings.TrimSpace(row[6]))
	return domain.Progress{
		ID:         id,
		Date:       normalizeSheetDate(row[1]),
		ScheduleID: scheduleID,
		Activity:   strings.TrimSpace(row[3]),
		Status:     strings.TrimSpace(row[4]),
		Note:       strings.TrimSpace(row[5]),
		CreatedAt:  created,
	}, nil
}

// normalizeSheetDate turns Google Sheets serial dates (produced when a user
// types a date manually and the cell is auto-formatted) into YYYY-MM-DD.
func normalizeSheetDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return value
	}
	serial, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	days := int(serial)
	if serial < 0 || days > 100000 {
		return value
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, days).Format("2006-01-02")
}

// normalizeSheetTime turns a Sheets time serial (fraction of a day) into HH:MM.
func normalizeSheetTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := time.Parse("15:04", value); err == nil {
		return value
	}
	if parsed, err := time.Parse("3:04", value); err == nil {
		return parsed.Format("15:04")
	}
	serial, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	if serial < 0 {
		return value
	}
	fraction := serial - float64(int(serial))
	minutes := int(fraction*24*60 + 0.5)
	if minutes >= 24*60 {
		minutes = 0
	}
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func pad(row []string, n int) []string {
	if len(row) >= n {
		return row
	}
	out := make([]string, n)
	copy(out, row)
	return out
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "y", "aktif", "active":
		return true
	default:
		return false
	}
}

func boolString(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}
