package sheets

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"one_more_percent/internal/domain"
)

// ReminderStateRepository implements domain.ReminderStateRepository. The
// sheet is the durable source of truth; an in-memory set avoids a read on
// every scheduler tick.
type ReminderStateRepository struct {
	client *Client
	mu     sync.Mutex
	sent   map[string]struct{}
	loaded bool
}

// NewReminderStateRepository builds the repository.
func NewReminderStateRepository(client *Client) *ReminderStateRepository {
	return &ReminderStateRepository{client: client, sent: map[string]struct{}{}}
}

// WasSent reports whether a reminder/recap was already delivered.
func (r *ReminderStateRepository) WasSent(ctx context.Context, date string, scheduleID int, reminderType string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureLoaded(ctx); err != nil {
		return false, err
	}
	_, ok := r.sent[reminderKey(date, scheduleID, reminderType)]
	return ok, nil
}

// MarkSent appends a durable row and updates the in-memory set.
func (r *ReminderStateRepository) MarkSent(ctx context.Context, date string, scheduleID int, reminderType string, sentAt string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureLoaded(ctx); err != nil {
		return err
	}
	row := []any{date, scheduleID, reminderType, sentAt}
	if err := r.client.AppendValues(ctx, r.client.cfg.SheetReminderState, [][]any{row}); err != nil {
		return err
	}
	r.sent[reminderKey(date, scheduleID, reminderType)] = struct{}{}
	return nil
}

// ListAll returns every reminder_state row.
func (r *ReminderStateRepository) ListAll(ctx context.Context) ([]domain.ReminderState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.client.GetValues(ctx, r.client.cfg.SheetReminderState+"!A1:"+columnLetter(len(ReminderHeaders)))
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}

	out := make([]domain.ReminderState, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		row := pad(rows[i], len(ReminderHeaders))
		date := normalizeSheetDate(row[0])
		if date == "" {
			continue
		}
		scheduleID, _ := strconv.Atoi(strings.TrimSpace(row[1]))
		out = append(out, domain.ReminderState{
			Date:       date,
			ScheduleID: scheduleID,
			Type:       strings.TrimSpace(row[2]),
			SentAt:     strings.TrimSpace(row[3]),
		})
	}
	return out, nil
}

// Upsert inserts or updates one reminder_state row.
func (r *ReminderStateRepository) Upsert(ctx context.Context, state domain.ReminderState) (domain.ReminderState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.client.GetValues(ctx, r.client.cfg.SheetReminderState+"!A1:"+columnLetter(len(ReminderHeaders)))
	if err != nil {
		return domain.ReminderState{}, err
	}
	for i := 1; i < len(rows); i++ {
		row := pad(rows[i], len(ReminderHeaders))
		date := normalizeSheetDate(row[0])
		scheduleID, _ := strconv.Atoi(strings.TrimSpace(row[1]))
		kind := strings.TrimSpace(row[2])
		if date == state.Date && scheduleID == state.ScheduleID && kind == state.Type {
			a1 := fmt.Sprintf("%s!A%d:%s%d", r.client.cfg.SheetReminderState, i+1, columnLetter(len(ReminderHeaders)), i+1)
			if err := r.client.UpdateValues(ctx, a1, [][]any{{state.Date, state.ScheduleID, state.Type, state.SentAt}}); err != nil {
				return domain.ReminderState{}, err
			}
			r.loaded = false
			r.sent = map[string]struct{}{}
			return state, nil
		}
	}
	if err := r.client.AppendValues(ctx, r.client.cfg.SheetReminderState, [][]any{{state.Date, state.ScheduleID, state.Type, state.SentAt}}); err != nil {
		return domain.ReminderState{}, err
	}
	r.loaded = false
	r.sent = map[string]struct{}{}
	return state, nil
}

// Delete removes reminder_state rows matching the filter.
func (r *ReminderStateRepository) Delete(ctx context.Context, filter domain.ReminderFilter) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.client.GetValues(ctx, r.client.cfg.SheetReminderState+"!A1:"+columnLetter(len(ReminderHeaders)))
	if err != nil {
		return 0, err
	}
	if len(rows) <= 1 {
		return 0, nil
	}

	var deleteIndexes []int
	for i := 1; i < len(rows); i++ {
		row := pad(rows[i], len(ReminderHeaders))
		date := normalizeSheetDate(row[0])
		scheduleID, _ := strconv.Atoi(strings.TrimSpace(row[1]))
		kind := strings.TrimSpace(row[2])
		if !matchesReminderFilter(date, scheduleID, kind, filter) {
			continue
		}
		deleteIndexes = append(deleteIndexes, i)
	}
	if len(deleteIndexes) == 0 {
		return 0, nil
	}

	sort.Sort(sort.Reverse(sort.IntSlice(deleteIndexes)))
	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetReminderState, deleteIndexes); err != nil {
		return 0, err
	}
	r.loaded = false
	r.sent = map[string]struct{}{}
	return len(deleteIndexes), nil
}

func matchesReminderFilter(date string, scheduleID int, kind string, filter domain.ReminderFilter) bool {
	if filter.All {
		return true
	}
	if filter.Before != "" && date >= filter.Before {
		return false
	}
	if filter.Date != "" && date != filter.Date {
		return false
	}
	if filter.ScheduleID != 0 && scheduleID != filter.ScheduleID {
		return false
	}
	if filter.Type != "" && kind != filter.Type {
		return false
	}
	return filter.Before != "" || filter.Date != "" || filter.ScheduleID != 0 || filter.Type != ""
}

// PurgeBefore deletes state rows older than cutoffDate.
func (r *ReminderStateRepository) PurgeBefore(ctx context.Context, cutoffDate string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.client.GetValues(ctx, r.client.cfg.SheetReminderState+"!A1:"+columnLetter(len(ReminderHeaders)))
	if err != nil {
		return err
	}
	if len(rows) <= 1 {
		return nil
	}

	var deleteIndexes []int
	for i := 1; i < len(rows); i++ {
		row := pad(rows[i], len(ReminderHeaders))
		date := normalizeSheetDate(row[0])
		if date != "" && date < cutoffDate {
			deleteIndexes = append(deleteIndexes, i)
		}
	}
	if len(deleteIndexes) == 0 {
		return nil
	}

	sort.Sort(sort.Reverse(sort.IntSlice(deleteIndexes)))
	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetReminderState, deleteIndexes); err != nil {
		return err
	}
	r.loaded = false
	r.sent = map[string]struct{}{}
	return nil
}

func (r *ReminderStateRepository) ensureLoaded(ctx context.Context) error {
	if r.loaded {
		return nil
	}

	rows, err := r.client.GetValues(ctx, r.client.cfg.SheetReminderState+"!A1:"+columnLetter(len(ReminderHeaders)))
	if err != nil {
		return err
	}
	r.sent = map[string]struct{}{}
	for i := 1; i < len(rows); i++ {
		row := pad(rows[i], len(ReminderHeaders))
		date := normalizeSheetDate(row[0])
		if date == "" {
			continue
		}
		scheduleID, _ := strconv.Atoi(strings.TrimSpace(row[1]))
		r.sent[reminderKey(date, scheduleID, strings.TrimSpace(row[2]))] = struct{}{}
	}
	r.loaded = true
	return nil
}

func reminderKey(date string, scheduleID int, reminderType string) string {
	return date + "|" + strconv.Itoa(scheduleID) + "|" + reminderType
}

var _ domain.ReminderStateRepository = (*ReminderStateRepository)(nil)
