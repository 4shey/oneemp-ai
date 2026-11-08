package sheets

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"one_more_percent/internal/domain"
)

type progressRow struct {
	Progress domain.Progress
	Index    int
}

// ProgressRepository implements domain.ProgressRepository.
type ProgressRepository struct {
	client *Client
	mu     sync.Mutex
}

// NewProgressRepository builds the repository.
func NewProgressRepository(client *Client) *ProgressRepository {
	return &ProgressRepository{client: client}
}

// ListByDate returns progress rows for a single date.
func (r *ProgressRepository) ListByDate(ctx context.Context, date string) ([]domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.Progress
	for _, row := range rows {
		if row.Progress.Date == date {
			out = append(out, row.Progress)
		}
	}
	return out, nil
}

// ListAll returns every progress row in the main worksheet.
func (r *ProgressRepository) ListAll(ctx context.Context) ([]domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Progress, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Progress)
	}
	return out, nil
}

// ListArchive returns every row in the archive worksheet.
func (r *ProgressRepository) ListArchive(ctx context.Context) ([]domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	values, err := r.client.GetValues(ctx, r.client.cfg.SheetProgressArchive+"!A1:"+columnLetter(len(ProgressHeaders)))
	if err != nil {
		return nil, err
	}
	if len(values) <= 1 {
		return nil, nil
	}

	out := make([]domain.Progress, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		if isEmptyRow(values[i]) {
			continue
		}
		progress, err := rowToProgress(values[i])
		if err != nil {
			continue
		}
		out = append(out, progress)
	}
	return out, nil
}

// UpsertArchive inserts or updates a row in the archive worksheet.
func (r *ProgressRepository) UpsertArchive(ctx context.Context, progress domain.Progress) (domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readArchiveAll(ctx)
	if err != nil {
		return domain.Progress{}, err
	}

	maxID := 0
	for _, row := range rows {
		if row.Progress.ID > maxID {
			maxID = row.Progress.ID
		}
		if row.Progress.Date == progress.Date && row.Progress.ScheduleID == progress.ScheduleID {
			progress.ID = row.Progress.ID
			if progress.CreatedAt.IsZero() {
				progress.CreatedAt = row.Progress.CreatedAt
			}
			a1 := fmt.Sprintf("%s!A%d:%s%d", r.client.cfg.SheetProgressArchive, row.Index+1, columnLetter(len(ProgressHeaders)), row.Index+1)
			if err := r.client.UpdateValues(ctx, a1, [][]any{progressToRow(progress)}); err != nil {
				return domain.Progress{}, err
			}
			return progress, nil
		}
	}

	progress.ID = maxID + 1
	if progress.CreatedAt.IsZero() {
		progress.CreatedAt = time.Now()
	}
	if err := r.client.AppendValues(ctx, r.client.cfg.SheetProgressArchive, [][]any{progressToRow(progress)}); err != nil {
		return domain.Progress{}, err
	}
	return progress, nil
}

// DeleteArchive removes archive rows matching the filter.
func (r *ProgressRepository) DeleteArchive(ctx context.Context, filter domain.ProgressFilter) (int, error) {
	if filter.Empty() {
		return 0, fmt.Errorf("filter hapus arsip kosong")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readArchiveAll(ctx)
	if err != nil {
		return 0, err
	}

	needle := strings.ToLower(strings.TrimSpace(filter.Activity))
	indexes := make([]int, 0)
	for _, row := range rows {
		if matchesProgressFilter(row.Progress, filter, needle) {
			indexes = append(indexes, row.Index)
		}
	}
	if len(indexes) == 0 {
		return 0, nil
	}
	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetProgressArchive, indexes); err != nil {
		return 0, fmt.Errorf("hapus arsip: %w", err)
	}
	return len(indexes), nil
}

// Upsert inserts a new progress row or updates the matching date+schedule row.
func (r *ProgressRepository) Upsert(ctx context.Context, progress domain.Progress) (domain.Progress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return domain.Progress{}, err
	}

	maxID := 0
	for _, row := range rows {
		if row.Progress.ID > maxID {
			maxID = row.Progress.ID
		}
		if row.Progress.Date == progress.Date && row.Progress.ScheduleID == progress.ScheduleID {
			progress.ID = row.Progress.ID
			if progress.CreatedAt.IsZero() {
				progress.CreatedAt = row.Progress.CreatedAt
			}
			a1 := fmt.Sprintf("%s!A%d:%s%d", r.client.cfg.SheetProgress, row.Index+1, columnLetter(len(ProgressHeaders)), row.Index+1)
			if err := r.client.UpdateValues(ctx, a1, [][]any{progressToRow(progress)}); err != nil {
				return domain.Progress{}, err
			}
			return progress, nil
		}
	}

	progress.ID = maxID + 1
	if progress.CreatedAt.IsZero() {
		progress.CreatedAt = time.Now()
	}
	if err := r.client.AppendValues(ctx, r.client.cfg.SheetProgress, [][]any{progressToRow(progress)}); err != nil {
		return domain.Progress{}, err
	}
	return progress, nil
}

// Delete removes progress rows matching the filter.
func (r *ProgressRepository) Delete(ctx context.Context, filter domain.ProgressFilter) (int, error) {
	if filter.Empty() {
		return 0, fmt.Errorf("filter hapus progres kosong")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return 0, err
	}

	needle := strings.ToLower(strings.TrimSpace(filter.Activity))
	indexes := make([]int, 0)
	for _, row := range rows {
		if !matchesProgressFilter(row.Progress, filter, needle) {
			continue
		}
		indexes = append(indexes, row.Index)
	}
	if len(indexes) == 0 {
		return 0, nil
	}

	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetProgress, indexes); err != nil {
		return 0, fmt.Errorf("hapus progres: %w", err)
	}
	return len(indexes), nil
}

func matchesProgressFilter(progress domain.Progress, filter domain.ProgressFilter, needle string) bool {
	if filter.All {
		return true
	}
	if filter.Date != "" && progress.Date != filter.Date {
		return false
	}
	if filter.ScheduleID != 0 && progress.ScheduleID != filter.ScheduleID {
		return false
	}
	if needle != "" && !strings.Contains(strings.ToLower(progress.Activity), needle) {
		return false
	}
	return true
}

// MarkPendingAsMissed flips every pending row on a date to missed.
func (r *ProgressRepository) MarkPendingAsMissed(ctx context.Context, date string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return err
	}

	var updates []ValueRange
	for _, row := range rows {
		if row.Progress.Date != date || row.Progress.Status != domain.ProgressPending {
			continue
		}
		row.Progress.Status = domain.ProgressMissed
		a1 := fmt.Sprintf("%s!A%d:%s%d", r.client.cfg.SheetProgress, row.Index+1, columnLetter(len(ProgressHeaders)), row.Index+1)
		updates = append(updates, ValueRange{Range: a1, Values: [][]any{progressToRow(row.Progress)}})
	}
	if len(updates) == 0 {
		return nil
	}
	return r.client.BatchUpdateValues(ctx, updates)
}

// ArchiveBefore moves rows older than cutoffDate to the archive worksheet and
// deletes them from progress_logs, keeping the main sheet bounded forever.
func (r *ProgressRepository) ArchiveBefore(ctx context.Context, cutoffDate string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.readAll(ctx)
	if err != nil {
		return 0, err
	}

	var oldRows [][]any
	var oldIndexes []int
	for _, row := range rows {
		if row.Progress.Date == "" || row.Progress.Date >= cutoffDate {
			continue
		}
		oldRows = append(oldRows, progressToRow(row.Progress))
		oldIndexes = append(oldIndexes, row.Index)
	}
	if len(oldRows) == 0 {
		return 0, nil
	}

	if r.client.cfg.SheetArchiveEnabled {
		if err := r.client.AppendValues(ctx, r.client.cfg.SheetProgressArchive, oldRows); err != nil {
			return 0, fmt.Errorf("arsipkan progress: %w", err)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(oldIndexes)))
	if err := r.client.DeleteRows(ctx, r.client.cfg.SheetProgress, oldIndexes); err != nil {
		return 0, fmt.Errorf("hapus progress lama: %w", err)
	}
	return len(oldRows), nil
}

func (r *ProgressRepository) readAll(ctx context.Context) ([]progressRow, error) {
	return r.readRows(ctx, r.client.cfg.SheetProgress)
}

func (r *ProgressRepository) readArchiveAll(ctx context.Context) ([]progressRow, error) {
	return r.readRows(ctx, r.client.cfg.SheetProgressArchive)
}

func (r *ProgressRepository) readRows(ctx context.Context, sheet string) ([]progressRow, error) {
	values, err := r.client.GetValues(ctx, sheet+"!A1:"+columnLetter(len(ProgressHeaders)))
	if err != nil {
		return nil, err
	}
	if len(values) <= 1 {
		return nil, nil
	}

	out := make([]progressRow, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		if isEmptyRow(values[i]) {
			continue
		}
		progress, err := rowToProgress(values[i])
		if err != nil {
			continue
		}
		out = append(out, progressRow{Progress: progress, Index: i})
	}
	return out, nil
}
