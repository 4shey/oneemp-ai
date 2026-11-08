package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"one_more_percent/internal/domain"
)

type scheduleRow struct {
	Schedule domain.Schedule
	Index    int // 0-based index in the values array (header is 0)
}

// ScheduleRepository implements domain.ScheduleRepository with an in-memory
// cache so the scheduler does not read the sheet every tick.
type ScheduleRepository struct {
	client *Client
	ttl    time.Duration

	mu       sync.Mutex
	cache    []domain.Schedule
	cachedAt time.Time
}

// NewScheduleRepository builds the repository.
func NewScheduleRepository(client *Client, ttl time.Duration) *ScheduleRepository {
	return &ScheduleRepository{client: client, ttl: ttl}
}

// List returns all schedules (cached for ttl).
func (r *ScheduleRepository) List(ctx context.Context) ([]domain.Schedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ttl > 0 && !r.cachedAt.IsZero() && time.Since(r.cachedAt) < r.ttl {
		return cloneSchedules(r.cache), nil
	}

	rows, err := r.readAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Schedule, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Schedule)
	}
	r.cache = out
	r.cachedAt = time.Now()
	return cloneSchedules(out), nil
}

// Get returns one schedule by id.
func (r *ScheduleRepository) Get(ctx context.Context, id int) (domain.Schedule, error) {
	list, err := r.List(ctx)
	if err != nil {
		return domain.Schedule{}, err
	}
	for _, s := range list {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Schedule{}, domain.ErrNotFound
}

// Create appends a validated schedule and assigns the next id.
func (r *ScheduleRepository) Create(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error) {
	if err := schedule.Validate(); err != nil {
		return domain.Schedule{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return domain.Schedule{}, err
	}
	maxID := 0
	for _, row := range rows {
		if row.Schedule.ID > maxID {
			maxID = row.Schedule.ID
		}
	}
	schedule.ID = maxID + 1

	if err := r.client.AppendValues(ctx, r.client.cfg.SheetSchedules, [][]any{scheduleToRow(schedule)}); err != nil {
		return domain.Schedule{}, err
	}
	r.invalidateLocked()
	return schedule, nil
}

// Update overwrites an existing schedule row.
func (r *ScheduleRepository) Update(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error) {
	if err := schedule.Validate(); err != nil {
		return domain.Schedule{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return domain.Schedule{}, err
	}
	for _, row := range rows {
		if row.Schedule.ID != schedule.ID {
			continue
		}
		a1 := fmt.Sprintf("%s!A%d:%s%d", r.client.cfg.SheetSchedules, row.Index+1, columnLetter(len(ScheduleHeaders)), row.Index+1)
		if err := r.client.UpdateValues(ctx, a1, [][]any{scheduleToRow(schedule)}); err != nil {
			return domain.Schedule{}, err
		}
		r.invalidateLocked()
		return schedule, nil
	}
	return domain.Schedule{}, domain.ErrNotFound
}

// Delete removes a schedule row.
func (r *ScheduleRepository) Delete(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Schedule.ID != id {
			continue
		}
		if err := r.client.DeleteRows(ctx, r.client.cfg.SheetSchedules, []int{row.Index}); err != nil {
			return err
		}
		r.invalidateLocked()
		return nil
	}
	return domain.ErrNotFound
}

// DeleteMany removes every schedule whose id is in ids.
func (r *ScheduleRepository) DeleteMany(ctx context.Context, ids []int) (int, []int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return 0, nil, err
	}

	wanted := make(map[int]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	indexes := make([]int, 0, len(ids))
	found := make(map[int]bool, len(ids))
	for _, row := range rows {
		if !wanted[row.Schedule.ID] {
			continue
		}
		indexes = append(indexes, row.Index)
		found[row.Schedule.ID] = true
	}

	missing := make([]int, 0)
	seenMissing := make(map[int]bool, len(ids))
	for _, id := range ids {
		if found[id] || seenMissing[id] {
			continue
		}
		seenMissing[id] = true
		missing = append(missing, id)
	}
	if len(indexes) == 0 {
		return 0, missing, nil
	}

	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetSchedules, indexes); err != nil {
		return 0, missing, err
	}
	r.invalidateLocked()
	return len(indexes), missing, nil
}

func (r *ScheduleRepository) invalidateLocked() {
	r.cachedAt = time.Time{}
	r.cache = nil
}

func (r *ScheduleRepository) readAll(ctx context.Context) ([]scheduleRow, error) {
	values, err := r.client.GetValues(ctx, r.client.cfg.SheetSchedules+"!A1:"+columnLetter(len(ScheduleHeaders)))
	if err != nil {
		return nil, err
	}
	if len(values) <= 1 {
		return nil, nil
	}

	out := make([]scheduleRow, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		row := values[i]
		if isEmptyRow(row) {
			continue
		}
		schedule, err := rowToSchedule(row)
		if err != nil {
			slog.Warn("melewati baris jadwal tidak valid", "row", i+1, "error", err)
			continue
		}
		out = append(out, scheduleRow{Schedule: schedule, Index: i})
	}
	return out, nil
}

func cloneSchedules(in []domain.Schedule) []domain.Schedule {
	out := make([]domain.Schedule, len(in))
	copy(out, in)
	return out
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if cell != "" {
			return false
		}
	}
	return true
}
