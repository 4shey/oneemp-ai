package prompt

import "testing"

func TestDefaultPromptsRender(t *testing.T) {
	loader, err := New(t.TempDir(), false)
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	data := map[string]any{
		"Now":              "Monday, 05 January 2026 09:00 WIB",
		"Today":            "2026-01-05",
		"Weekday":          "Monday",
		"UserName":         "Test",
		"UserContext":      "",
		"ScheduleMode":     "santai",
		"ScheduleOverview": "tidak ada",
		"TodayStatus":      "tidak ada",
		"MissedToday":      "tidak ada",
		"NextSchedules":    "tidak ada",
		"TodayProgress":    "belum ada",
		"History":          "belum ada",
		"Schedules":        "tidak ada",
		"UserMessage":      "halo",
		"Activity":         "Belajar",
		"StartTime":        "09:00",
		"EndTime":          "10:00",
		"Date":             "2026-01-04",
		"Completed":        "- Belajar",
		"Missed":           "tidak ada",
		"CompletedCount":   1,
		"Total":            1,
	}

	for _, name := range Names {
		if _, err := loader.Render(name, data); err != nil {
			t.Errorf("render %s: %v", name, err)
		}
	}
}
