package domain

import "time"

// Progress status values stored in the progress_logs sheet.
const (
	ProgressPending   = "pending"
	ProgressCompleted = "completed"
	ProgressMissed    = "missed"
)

// Progress is one row in the progress_logs sheet.
type Progress struct {
	ID         int
	Date       string // YYYY-MM-DD
	ScheduleID int
	Activity   string
	Status     string // pending | completed | missed
	Note       string
	CreatedAt  time.Time
}

// ProgressFilter selects progress rows for deletion. All ignores the other
// fields. Date, ScheduleID, and Activity are combined with AND.
type ProgressFilter struct {
	All        bool
	Date       string
	ScheduleID int
	Activity   string
}

// Empty reports whether the filter would match nothing on purpose.
func (f ProgressFilter) Empty() bool {
	return !f.All && f.Date == "" && f.ScheduleID == 0 && f.Activity == ""
}
