package sheets

import (
	"context"
	"embed"
	"encoding/csv"
	"fmt"
	"strings"

	"one_more_percent/internal/domain"
)

//go:embed seed_schedules.csv
var seedFS embed.FS

// SeedDefaultSchedules fills the schedules sheet when it is empty. Data is
// loaded from a CSV so it can be edited without touching Go code.
func SeedDefaultSchedules(ctx context.Context, client *Client, repo *ScheduleRepository) (int, error) {
	existing, err := repo.List(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, nil
	}

	f, err := seedFS.Open("seed_schedules.csv")
	if err != nil {
		return 0, fmt.Errorf("buka seed: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("parse seed CSV: %w", err)
	}
	if len(records) <= 1 {
		return 0, nil
	}

	var rows [][]any
	id := 0
	for _, record := range records[1:] {
		if len(record) < 6 {
			continue
		}
		id++
		schedule := domain.Schedule{
			ID:        id,
			Type:      domain.Recurrence(strings.TrimSpace(record[0])),
			Date:      strings.TrimSpace(record[1]),
			DayOfWeek: strings.TrimSpace(record[2]),
			StartTime: strings.TrimSpace(record[3]),
			EndTime:   strings.TrimSpace(record[4]),
			Activity:  strings.TrimSpace(record[5]),
			Active:    true,
		}
		if err := schedule.Validate(); err != nil {
			return 0, fmt.Errorf("seed baris %d tidak valid: %w", id+1, err)
		}
		rows = append(rows, scheduleToRow(schedule))
	}
	if len(rows) == 0 {
		return 0, nil
	}

	if err := client.AppendValues(ctx, client.cfg.SheetSchedules, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
