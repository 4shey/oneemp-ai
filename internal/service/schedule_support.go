package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"one_more_percent/internal/domain"
)

const (
	scheduleModeActive = "aktif"
	scheduleModeSoon   = "sebentar lagi"
	scheduleModeFree   = "santai"
)

func buildCreateSchedule(ext extractedSchedule, now time.Time) (domain.Schedule, string) {
	schedule := domain.Schedule{
		Type:   domain.RecurrenceWeekly,
		Active: true,
	}
	if ext.Type != nil && strings.TrimSpace(*ext.Type) != "" {
		schedule.Type = domain.Recurrence(strings.ToLower(strings.TrimSpace(*ext.Type)))
	}
	switch schedule.Type {
	case domain.RecurrenceOnce, domain.RecurrenceDaily, domain.RecurrenceWeekly:
	default:
		schedule.Type = domain.RecurrenceWeekly
	}

	if ext.Activity == nil || strings.TrimSpace(*ext.Activity) == "" {
		return schedule, "Aktivitasnya apa? Sebutin dong yang mau dijadwalin."
	}
	schedule.Activity = strings.TrimSpace(*ext.Activity)

	if ext.StartTime == nil || strings.TrimSpace(*ext.StartTime) == "" {
		return schedule, "Jam mulainya kapan? Format HH:MM ya."
	}
	schedule.StartTime = normalizeTime(*ext.StartTime)

	if ext.EndTime != nil && strings.TrimSpace(*ext.EndTime) != "" {
		schedule.EndTime = normalizeTime(*ext.EndTime)
	} else {
		schedule.EndTime = addOneHour(schedule.StartTime)
	}
	if ext.Date != nil {
		schedule.Date = strings.TrimSpace(*ext.Date)
	}
	if ext.DayOfWeek != nil {
		schedule.DayOfWeek = canonicalDay(*ext.DayOfWeek)
	}
	if ext.Active != nil {
		schedule.Active = *ext.Active
	}

	switch schedule.Type {
	case domain.RecurrenceOnce:
		schedule.DayOfWeek = ""
		if schedule.Date == "" {
			return schedule, "Tanggalnya kapan? Sebutin tanggal lengkapnya (YYYY-MM-DD)."
		}
	case domain.RecurrenceWeekly:
		schedule.Date = ""
		if schedule.DayOfWeek == "" {
			schedule.DayOfWeek = now.Weekday().String()
		}
	case domain.RecurrenceDaily:
		schedule.Date = ""
		schedule.DayOfWeek = ""
	}

	if err := schedule.Validate(); err != nil {
		return schedule, "Jadwal ga valid: " + err.Error()
	}
	return schedule, ""
}

func applySchedulePatch(schedule *domain.Schedule, ext extractedSchedule, now time.Time) {
	if ext.Type != nil && strings.TrimSpace(*ext.Type) != "" {
		schedule.Type = domain.Recurrence(strings.ToLower(strings.TrimSpace(*ext.Type)))
	}
	if ext.Date != nil {
		schedule.Date = strings.TrimSpace(*ext.Date)
	}
	if ext.DayOfWeek != nil {
		schedule.DayOfWeek = canonicalDay(*ext.DayOfWeek)
	}
	if ext.StartTime != nil && strings.TrimSpace(*ext.StartTime) != "" {
		schedule.StartTime = normalizeTime(*ext.StartTime)
	}
	if ext.EndTime != nil && strings.TrimSpace(*ext.EndTime) != "" {
		schedule.EndTime = normalizeTime(*ext.EndTime)
	}
	if ext.Activity != nil && strings.TrimSpace(*ext.Activity) != "" {
		schedule.Activity = strings.TrimSpace(*ext.Activity)
	}
	if ext.Active != nil {
		schedule.Active = *ext.Active
	}

	switch schedule.Type {
	case domain.RecurrenceOnce:
		schedule.DayOfWeek = ""
	case domain.RecurrenceWeekly:
		schedule.Date = ""
		if schedule.DayOfWeek == "" {
			schedule.DayOfWeek = now.Weekday().String()
		}
	case domain.RecurrenceDaily:
		schedule.Date = ""
		schedule.DayOfWeek = ""
	}
}

func resolveSchedule(all []domain.Schedule, id *int, activity *string) (domain.Schedule, bool) {
	if id != nil {
		for _, schedule := range all {
			if schedule.ID == *id {
				return schedule, true
			}
		}
	}
	if activity == nil {
		return domain.Schedule{}, false
	}
	needle := strings.ToLower(strings.TrimSpace(*activity))
	if needle == "" {
		return domain.Schedule{}, false
	}

	var match *domain.Schedule
	for i := range all {
		schedule := all[i]
		if !strings.Contains(strings.ToLower(schedule.Activity), needle) {
			continue
		}
		if schedule.Active {
			return schedule, true
		}
		if match == nil {
			match = &all[i]
		}
	}
	if match != nil {
		return *match, true
	}
	return domain.Schedule{}, false
}

func matchTodaySchedule(all []domain.Schedule, activity string, todayProgress []domain.Progress) *domain.Schedule {
	needle := strings.ToLower(strings.TrimSpace(activity))
	if needle != "" {
		for i := range all {
			schedule := all[i]
			if strings.Contains(strings.ToLower(schedule.Activity), needle) {
				return &all[i]
			}
		}
		for _, progress := range todayProgress {
			if strings.Contains(strings.ToLower(progress.Activity), needle) {
				for i := range all {
					if all[i].ID == progress.ScheduleID {
						return &all[i]
					}
				}
			}
		}
	}

	for _, progress := range todayProgress {
		if progress.Status == domain.ProgressPending {
			for i := range all {
				if all[i].ID == progress.ScheduleID {
					return &all[i]
				}
			}
		}
	}
	return nil
}

func formatScheduleLines(schedules []domain.Schedule) string {
	if len(schedules) == 0 {
		return "tidak ada"
	}
	var b strings.Builder
	for _, s := range schedules {
		fmt.Fprintf(&b, "- %d | %s | %s | %s | %s | %s | %s | %t\n",
			s.ID, s.Type, s.Date, s.DayOfWeek, s.StartTime, s.EndTime, s.Activity, s.Active)
	}
	return strings.TrimSpace(b.String())
}

func formatProgressLines(progress []domain.Progress) string {
	if len(progress) == 0 {
		return "belum ada progres"
	}
	var lines []string
	for _, p := range progress {
		note := ""
		if p.Note != "" {
			note = " (" + p.Note + ")"
		}
		lines = append(lines, fmt.Sprintf("- #%d | schedule %d | %s | %s%s", p.ID, p.ScheduleID, p.Activity, p.Status, note))
	}
	return strings.Join(lines, "\n")
}

func formatProgressAdmin(progress []domain.Progress, title string) string {
	if len(progress) == 0 {
		return title + ": kosong"
	}
	items := append([]domain.Progress(nil), progress...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Date != items[j].Date {
			return items[i].Date > items[j].Date
		}
		return items[i].ID > items[j].ID
	})
	lines := make([]string, 0, len(items))
	for _, p := range items {
		note := ""
		if p.Note != "" {
			note = " | " + p.Note
		}
		lines = append(lines, fmt.Sprintf("- %s | %s | %s%s", p.Date, p.Activity, progressStatusLabel(p.Status), note))
	}
	return title + ":\n" + strings.Join(lines, "\n")
}

func progressStatusLabel(status string) string {
	switch status {
	case domain.ProgressCompleted:
		return "selesai"
	case domain.ProgressMissed, domain.ProgressPending:
		return "belum selesai"
	default:
		return status
	}
}

func formatReminderAdmin(items []domain.ReminderState) string {
	if len(items) == 0 {
		return "Reminder state: kosong"
	}
	sorted := append([]domain.ReminderState(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Date != sorted[j].Date {
			return sorted[i].Date > sorted[j].Date
		}
		return sorted[i].ScheduleID > sorted[j].ScheduleID
	})
	lines := make([]string, 0, len(sorted))
	for _, item := range sorted {
		lines = append(lines, fmt.Sprintf("- %s | schedule %d | %s | %s", item.Date, item.ScheduleID, item.Type, item.SentAt))
	}
	return "Reminder state:\n" + strings.Join(lines, "\n")
}

func formatProgressNotes(progress []domain.Progress) string {
	if len(progress) == 0 {
		return "belum ada progres"
	}
	lines := make([]string, 0, len(progress))
	for _, p := range progress {
		note := ""
		if p.Note != "" {
			note = " (" + p.Note + ")"
		}
		status := p.Status
		switch p.Status {
		case domain.ProgressCompleted:
			status = "selesai"
		case domain.ProgressMissed, domain.ProgressPending:
			status = "belum selesai"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s%s", p.Activity, status, note))
	}
	return strings.Join(lines, "\n")
}

func formatScheduleList(schedules []domain.Schedule) string {
	if len(schedules) == 0 {
		return "Belum ada jadwal"
	}
	items := append([]domain.Schedule(nil), schedules...)
	sortSchedules(items)
	lines := make([]string, 0, len(items))
	for _, s := range items {
		status := ""
		if !s.Active {
			status = " (nonaktif)"
		}
		lines = append(lines, "- "+scheduleWithTime(s)+status)
	}
	return "Ini jadwal kamu:\n" + strings.Join(lines, "\n")
}

func formatTodayUpcoming(all []domain.Schedule, now time.Time) string {
	items := schedulesForDate(all, now, now)
	if len(items) == 0 {
		return "Hari ini udah ga ada jadwal lagi"
	}
	lines := make([]string, 0, len(items))
	for _, schedule := range items {
		lines = append(lines, "- "+scheduleActivityWithTime(schedule))
	}
	return "Hari ini sisa jadwal kamu:\n" + strings.Join(lines, "\n")
}

func formatScheduleOverview(schedules []domain.Schedule) string {
	active := make([]domain.Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		if schedule.Active {
			active = append(active, schedule)
		}
	}
	if len(active) == 0 {
		return "tidak ada jadwal aktif"
	}
	sortSchedules(active)
	lines := make([]string, 0, len(active))
	for _, schedule := range active {
		lines = append(lines, "- "+scheduleWithTime(schedule))
	}
	return strings.Join(lines, "\n")
}

func formatTodayStatus(all []domain.Schedule, progress []domain.Progress, now time.Time) string {
	today := dateOnly(now)
	yesterday := today.AddDate(0, 0, -1)
	current := now.Format("15:04")
	todayKey := today.Format("2006-01-02")

	progressBySchedule := make(map[int]domain.Progress, len(progress))
	for _, p := range progress {
		if p.Date == todayKey {
			progressBySchedule[p.ScheduleID] = p
		}
	}

	items := make([]domain.Schedule, 0, len(all))
	for _, schedule := range all {
		if !schedule.Active {
			continue
		}
		if schedule.OccursOn(today) || (schedule.IsOvernight() && schedule.OccursOn(yesterday) && schedule.InWindow(now, current)) {
			items = append(items, schedule)
		}
	}
	sortSchedules(items)
	if len(items) == 0 {
		return "tidak ada jadwal hari ini"
	}

	lines := make([]string, 0, len(items))
	for _, schedule := range items {
		status := "belum ada catatan"
		if p, ok := progressBySchedule[schedule.ID]; ok {
			switch p.Status {
			case domain.ProgressCompleted:
				status = "selesai"
			case domain.ProgressMissed:
				status = "belum selesai"
			case domain.ProgressPending:
				if schedule.InWindow(now, current) {
					status = "sedang berjalan"
				} else {
					status = "belum selesai"
				}
			default:
				status = p.Status
			}
		} else if schedule.InWindow(now, current) {
			status = "sedang berjalan"
		} else if schedule.StartTime > current {
			status = "belum waktunya"
		} else if missedAfterGrace(schedule, now) {
			status = "terlewat"
		} else {
			status = "belum selesai"
		}
		lines = append(lines, "- "+scheduleActivityWithTime(schedule)+": "+status)
	}
	return strings.Join(lines, "\n")
}

func formatNextSchedules(all []domain.Schedule, now time.Time, excludeID int) string {
	type item struct {
		at       time.Time
		schedule domain.Schedule
	}
	current := now.Format("15:04")
	items := make([]item, 0, len(all))
	for _, schedule := range all {
		if schedule.ID == excludeID || schedule.InWindow(now, current) {
			continue
		}
		at, ok := nextStartAt(schedule, now)
		if !ok {
			continue
		}
		items = append(items, item{at: at, schedule: schedule})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	if len(items) == 0 {
		return "tidak ada jadwal berikutnya"
	}

	lines := make([]string, 0, 3)
	for i, item := range items {
		if i >= 3 {
			break
		}
		lines = append(lines, fmt.Sprintf("- %s %s (dalam %s)", strings.ToLower(dateLabel(item.at, now)), scheduleActivityWithTime(item.schedule), formatDuration(item.at.Sub(now))))
	}
	return strings.Join(lines, "\n")
}

func currentScheduleMode(all []domain.Schedule, now time.Time) string {
	current := now.Format("15:04")
	for _, schedule := range all {
		if schedule.Active && schedule.InWindow(now, current) {
			return scheduleModeActive
		}
	}
	for _, schedule := range all {
		at, ok := nextStartAt(schedule, now)
		if ok && at.Sub(now) <= 15*time.Minute {
			return scheduleModeSoon
		}
	}
	return scheduleModeFree
}

func hasMissedToday(all []domain.Schedule, progress []domain.Progress, now time.Time) bool {
	todayKey := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1)
	progressBySchedule := make(map[int]domain.Progress, len(progress))
	for _, p := range progress {
		if p.Date == todayKey {
			progressBySchedule[p.ScheduleID] = p
		}
	}
	for _, schedule := range all {
		if !schedule.Active {
			continue
		}
		occurs := schedule.OccursOn(now)
		if schedule.IsOvernight() {
			occurs = occurs || schedule.OccursOn(yesterday)
		}
		if !occurs {
			continue
		}
		if p, ok := progressBySchedule[schedule.ID]; ok && p.Status == domain.ProgressCompleted {
			continue
		}
		if missedAfterGrace(schedule, now) {
			return true
		}
	}
	return false
}

func missedAfterGrace(schedule domain.Schedule, now time.Time) bool {
	end := schedule.EffectiveEnd()
	if end == "" {
		return false
	}
	current := now.Format("15:04")
	if schedule.IsOvernight() && current >= schedule.StartTime {
		return false
	}
	endAt := combineDateAndTime(dateOnly(now), end)
	if endAt.IsZero() {
		return false
	}
	return now.After(endAt.Add(endReminderGrace))
}

func formatMissedToday(all []domain.Schedule, progress []domain.Progress, now time.Time) string {
	todayKey := now.Format("2006-01-02")
	previous := now.AddDate(0, 0, -1)
	progressBySchedule := make(map[int]domain.Progress, len(progress))
	for _, p := range progress {
		if p.Date == todayKey {
			progressBySchedule[p.ScheduleID] = p
		}
	}

	lines := make([]string, 0)
	for _, schedule := range all {
		if !schedule.Active {
			continue
		}
		occurs := schedule.OccursOn(now)
		if schedule.IsOvernight() {
			occurs = occurs || schedule.OccursOn(previous)
		}
		if !occurs {
			continue
		}
		if p, ok := progressBySchedule[schedule.ID]; ok && p.Status == domain.ProgressCompleted {
			continue
		}
		if missedAfterGrace(schedule, now) {
			lines = append(lines, "- "+scheduleActivityWithTime(schedule))
		}
	}
	if len(lines) == 0 {
		return "tidak ada"
	}
	return strings.Join(lines, "\n")
}

func nextStartAt(schedule domain.Schedule, now time.Time) (time.Time, bool) {
	if !schedule.Active || strings.TrimSpace(schedule.StartTime) == "" {
		return time.Time{}, false
	}
	today := dateOnly(now)
	switch schedule.Type {
	case domain.RecurrenceOnce:
		date, err := time.ParseInLocation("2006-01-02", schedule.Date, now.Location())
		if err != nil {
			return time.Time{}, false
		}
		at := combineDateAndTime(date, schedule.StartTime)
		if at.After(now) {
			return at, true
		}
		return time.Time{}, false
	case domain.RecurrenceDaily:
		at := combineDateAndTime(today, schedule.StartTime)
		if at.IsZero() {
			return time.Time{}, false
		}
		if at.After(now) {
			return at, true
		}
		next := combineDateAndTime(today.AddDate(0, 0, 1), schedule.StartTime)
		if next.IsZero() {
			return time.Time{}, false
		}
		return next, true
	case domain.RecurrenceWeekly:
		for i := 0; i < 8; i++ {
			date := today.AddDate(0, 0, i)
			if !schedule.OccursOn(date) {
				continue
			}
			at := combineDateAndTime(date, schedule.StartTime)
			if at.After(now) {
				return at, true
			}
		}
	}
	return time.Time{}, false
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "sebentar lagi"
	}
	total := int(d.Minutes())
	days := total / (24 * 60)
	if days > 0 {
		hours := (total % (24 * 60)) / 60
		if hours > 0 {
			return fmt.Sprintf("%d hari %d jam", days, hours)
		}
		return fmt.Sprintf("%d hari", days)
	}
	hours := total / 60
	minutes := total % 60
	if hours == 0 {
		return fmt.Sprintf("%d menit", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%d jam", hours)
	}
	return fmt.Sprintf("%d jam %d menit", hours, minutes)
}

func formatSchedulesOnDate(all []domain.Schedule, date, now time.Time) string {
	items := schedulesOnDate(all, date)
	label := dateLabel(date, now)
	if len(items) == 0 {
		return label + " ga ada jadwal"
	}
	lines := make([]string, 0, len(items))
	for _, schedule := range items {
		lines = append(lines, "- "+scheduleActivityWithTime(schedule))
	}
	if len(items) == 1 {
		return fmt.Sprintf("%s kamu ada 1 jadwal:\n%s", label, lines[0])
	}
	return fmt.Sprintf("%s kamu ada %d jadwal:\n%s", label, len(items), strings.Join(lines, "\n"))
}

func schedulesOnDate(all []domain.Schedule, date time.Time) []domain.Schedule {
	day := dateOnly(date)
	previous := day.AddDate(0, 0, -1)
	items := make([]domain.Schedule, 0, len(all))
	for _, schedule := range all {
		if !schedule.Active {
			continue
		}
		occurs := schedule.OccursOn(day)
		if schedule.IsOvernight() && schedule.OccursOn(previous) {
			occurs = true
		}
		if occurs {
			items = append(items, schedule)
		}
	}
	sortSchedules(items)
	return items
}

func schedulesForDate(all []domain.Schedule, date, now time.Time) []domain.Schedule {
	items := make([]domain.Schedule, 0, len(all))
	if !sameDate(date, now) {
		for _, schedule := range all {
			if schedule.Active && schedule.OccursOn(date) {
				items = append(items, schedule)
			}
		}
		sortSchedules(items)
		return items
	}

	today := dateOnly(now)
	yesterday := today.AddDate(0, 0, -1)
	current := now.Format("15:04")
	for _, schedule := range all {
		if !schedule.Active {
			continue
		}
		if schedule.IsOvernight() && schedule.OccursOn(yesterday) && current < schedule.EffectiveEnd() {
			items = append(items, schedule)
			continue
		}
		if !schedule.OccursOn(today) {
			continue
		}
		if schedule.StartTime >= current {
			items = append(items, schedule)
			continue
		}
		if schedule.IsOvernight() || schedule.EffectiveEnd() > current {
			items = append(items, schedule)
		}
	}
	sortSchedules(items)
	return items
}

func sortSchedules(items []domain.Schedule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartTime != items[j].StartTime {
			return items[i].StartTime < items[j].StartTime
		}
		return items[i].Activity < items[j].Activity
	})
}

func dateLabel(date, now time.Time) string {
	date = date.In(now.Location())
	now = now.In(now.Location())
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())
	switch int(startDate.Sub(startToday).Hours() / 24) {
	case 0:
		return "Hari ini"
	case 1:
		return "Besok"
	case -1:
		return "Kemarin"
	default:
		return "Tanggal " + date.Format("02 Jan")
	}
}

func resolveListDate(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "hari ini", "today":
		return now, true
	case "besok", "tomorrow":
		return now.AddDate(0, 0, 1), true
	case "kemarin", "yesterday":
		return now.AddDate(0, 0, -1), true
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, now.Location())
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func formatCreatedSchedules(created []domain.Schedule) string {
	if len(created) == 0 {
		return "Ga ada jadwal yang kesimpen"
	}
	if len(created) == 1 {
		return fmt.Sprintf("oke, %s udah aku catat", scheduleWithTime(created[0]))
	}
	lines := make([]string, 0, len(created))
	for _, schedule := range created {
		lines = append(lines, "- "+scheduleWithTime(schedule))
	}
	return fmt.Sprintf("oke, %d jadwal udah aku catat:\n%s", len(created), strings.Join(lines, "\n"))
}

func scheduleWithTime(schedule domain.Schedule) string {
	label := scheduleLabel(schedule)
	timeLabel := scheduleTimeLabel(schedule)
	if timeLabel == "" {
		return label
	}
	return label + " " + timeLabel
}

func scheduleActivityWithTime(schedule domain.Schedule) string {
	timeLabel := scheduleTimeLabel(schedule)
	if timeLabel == "" {
		return schedule.Activity
	}
	return schedule.Activity + " " + timeLabel
}

func scheduleTimeLabel(schedule domain.Schedule) string {
	start := strings.TrimSpace(schedule.StartTime)
	if start == "" {
		return ""
	}
	end := strings.TrimSpace(schedule.EndTime)
	if end == "" {
		return "jam " + start
	}
	return "jam " + start + "-" + end
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sameDate(a, b time.Time) bool {
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}

func combineDateAndTime(date time.Time, hhmm string) time.Time {
	parsed, err := time.ParseInLocation("15:04", strings.TrimSpace(hhmm), date.Location())
	if err != nil {
		return time.Time{}
	}
	return time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), 0, 0, date.Location())
}

func startOccurrenceDate(schedule domain.Schedule, now time.Time) (time.Time, bool) {
	current := now.Format("15:04")
	today := dateOnly(now)
	if schedule.IsOvernight() {
		if schedule.OccursOn(today.AddDate(0, 0, -1)) && current < schedule.EffectiveEnd() {
			return today.AddDate(0, 0, -1), true
		}
		if schedule.OccursOn(today) && current >= schedule.StartTime {
			return today, true
		}
		return time.Time{}, false
	}
	if schedule.OccursOn(today) && current >= schedule.StartTime {
		return today, true
	}
	return time.Time{}, false
}

func endOccurrenceDate(schedule domain.Schedule, now time.Time, grace time.Duration) (time.Time, bool) {
	if !schedule.Active || strings.TrimSpace(schedule.EndTime) == "" {
		return time.Time{}, false
	}
	today := dateOnly(now)
	for _, startDate := range []time.Time{today, today.AddDate(0, 0, -1)} {
		if !schedule.OccursOn(startDate) {
			continue
		}
		endDate := startDate
		if schedule.IsOvernight() {
			endDate = startDate.AddDate(0, 0, 1)
		}
		endAt := combineDateAndTime(endDate, schedule.EndTime)
		if endAt.IsZero() {
			continue
		}
		if !now.Before(endAt) && now.Before(endAt.Add(grace)) {
			return startDate, true
		}
	}
	return time.Time{}, false
}

func progressDateFor(schedule domain.Schedule, startDate time.Time) string {
	if schedule.IsOvernight() {
		return startDate.AddDate(0, 0, 1).Format("2006-01-02")
	}
	return startDate.Format("2006-01-02")
}

func scheduleNames(all []domain.Schedule, ids []int) []string {
	byID := make(map[int]domain.Schedule, len(all))
	for _, schedule := range all {
		byID[schedule.ID] = schedule
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if schedule, ok := byID[id]; ok {
			out = append(out, scheduleShortLabel(schedule))
		}
	}
	return out
}

func scheduleShortLabel(schedule domain.Schedule) string {
	return scheduleLabel(schedule)
}

func scheduleLabel(schedule domain.Schedule) string {
	label := recurrenceLabel(schedule)
	if label == "" {
		return schedule.Activity
	}
	return schedule.Activity + " " + label
}

func recurrenceLabel(s domain.Schedule) string {
	switch s.Type {
	case domain.RecurrenceOnce:
		if strings.TrimSpace(s.Date) == "" {
			return ""
		}
		return "sekali " + s.Date
	case domain.RecurrenceDaily:
		return "tiap hari"
	case domain.RecurrenceWeekly:
		if strings.TrimSpace(s.DayOfWeek) == "" {
			return ""
		}
		return "tiap " + dayIndonesian(s.DayOfWeek)
	default:
		return ""
	}
}

func dayIndonesian(day string) string {
	switch day {
	case "Monday":
		return "Senin"
	case "Tuesday":
		return "Selasa"
	case "Wednesday":
		return "Rabu"
	case "Thursday":
		return "Kamis"
	case "Friday":
		return "Jumat"
	case "Saturday":
		return "Sabtu"
	case "Sunday":
		return "Minggu"
	default:
		return day
	}
}

func canonicalDay(day string) string {
	switch strings.ToLower(strings.TrimSpace(day)) {
	case "monday", "senin", "mon":
		return "Monday"
	case "tuesday", "selasa", "tue":
		return "Tuesday"
	case "wednesday", "rabu", "wed":
		return "Wednesday"
	case "thursday", "kamis", "thu":
		return "Thursday"
	case "friday", "jumat", "jum'at", "fri":
		return "Friday"
	case "saturday", "sabtu", "sat":
		return "Saturday"
	case "sunday", "minggu", "sun":
		return "Sunday"
	default:
		return strings.TrimSpace(day)
	}
}

func normalizeTime(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ".", ":")
	parts := strings.Split(value, ":")
	if len(parts) == 2 {
		hour, err1 := strconv.Atoi(parts[0])
		minute, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			return fmt.Sprintf("%02d:%02d", hour, minute)
		}
	}
	return value
}

func addOneHour(value string) string {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return value
	}
	return parsed.Add(time.Hour).Format("15:04")
}
