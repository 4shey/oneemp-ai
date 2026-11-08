package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"one_more_percent/internal/ai"
	"one_more_percent/internal/config"
	"one_more_percent/internal/domain"
	"one_more_percent/internal/prompt"
)

type fakeAI struct {
	responses []string
	requests  []ai.Request
	idx       int
}

func (f *fakeAI) Complete(_ context.Context, req ai.Request) (ai.Response, error) {
	f.requests = append(f.requests, req)
	if f.idx >= len(f.responses) {
		return ai.Response{Content: ""}, nil
	}
	resp := f.responses[f.idx]
	f.idx++
	return ai.Response{Content: resp}, nil
}

type fakeSchedules struct {
	items  []domain.Schedule
	nextID int
}

func (f *fakeSchedules) List(context.Context) ([]domain.Schedule, error) {
	out := make([]domain.Schedule, len(f.items))
	copy(out, f.items)
	return out, nil
}

func (f *fakeSchedules) Get(_ context.Context, id int) (domain.Schedule, error) {
	for _, item := range f.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Schedule{}, domain.ErrNotFound
}

func (f *fakeSchedules) Create(_ context.Context, s domain.Schedule) (domain.Schedule, error) {
	if err := s.Validate(); err != nil {
		return domain.Schedule{}, err
	}
	f.nextID++
	s.ID = f.nextID
	f.items = append(f.items, s)
	return s, nil
}

func (f *fakeSchedules) Update(_ context.Context, s domain.Schedule) (domain.Schedule, error) {
	for i := range f.items {
		if f.items[i].ID == s.ID {
			f.items[i] = s
			return s, nil
		}
	}
	return domain.Schedule{}, domain.ErrNotFound
}

func (f *fakeSchedules) Delete(_ context.Context, id int) error {
	for i := range f.items {
		if f.items[i].ID == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeSchedules) DeleteMany(_ context.Context, ids []int) (int, []int, error) {
	wanted := make(map[int]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	kept := make([]domain.Schedule, 0, len(f.items))
	found := make(map[int]bool, len(ids))
	for _, item := range f.items {
		if wanted[item.ID] {
			found[item.ID] = true
			continue
		}
		kept = append(kept, item)
	}

	missing := make([]int, 0)
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}

	deleted := len(f.items) - len(kept)
	f.items = kept
	return deleted, missing, nil
}

type fakeProgress struct {
	items  []domain.Progress
	nextID int
}

func (f *fakeProgress) ListByDate(_ context.Context, date string) ([]domain.Progress, error) {
	var out []domain.Progress
	for _, item := range f.items {
		if item.Date == date {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeProgress) Upsert(_ context.Context, p domain.Progress) (domain.Progress, error) {
	for i := range f.items {
		if f.items[i].Date == p.Date && f.items[i].ScheduleID == p.ScheduleID {
			p.ID = f.items[i].ID
			f.items[i] = p
			return p, nil
		}
	}
	f.nextID++
	p.ID = f.nextID
	f.items = append(f.items, p)
	return p, nil
}

func (f *fakeProgress) Delete(_ context.Context, filter domain.ProgressFilter) (int, error) {
	if filter.Empty() {
		return 0, errors.New("filter hapus progres kosong")
	}

	needle := strings.ToLower(strings.TrimSpace(filter.Activity))
	kept := make([]domain.Progress, 0, len(f.items))
	deleted := 0
	for _, item := range f.items {
		match := filter.All
		if !filter.All {
			match = true
			if filter.Date != "" && item.Date != filter.Date {
				match = false
			}
			if filter.ScheduleID != 0 && item.ScheduleID != filter.ScheduleID {
				match = false
			}
			if needle != "" && !strings.Contains(strings.ToLower(item.Activity), needle) {
				match = false
			}
		}
		if match {
			deleted++
			continue
		}
		kept = append(kept, item)
	}
	f.items = kept
	return deleted, nil
}

func (f *fakeProgress) MarkPendingAsMissed(context.Context, string) error { return nil }

func (f *fakeProgress) ArchiveBefore(context.Context, string) (int, error) { return 0, nil }

func (f *fakeProgress) ListAll(context.Context) ([]domain.Progress, error) {
	return append([]domain.Progress(nil), f.items...), nil
}

func (f *fakeProgress) ListArchive(context.Context) ([]domain.Progress, error) {
	return nil, nil
}

func (f *fakeProgress) UpsertArchive(_ context.Context, p domain.Progress) (domain.Progress, error) {
	return p, nil
}

func (f *fakeProgress) DeleteArchive(context.Context, domain.ProgressFilter) (int, error) {
	return 0, nil
}

type fakeReminders struct {
	sent map[string]struct{}
}

func newFakeReminders() *fakeReminders {
	return &fakeReminders{sent: map[string]struct{}{}}
}

func (f *fakeReminders) WasSent(_ context.Context, date string, scheduleID int, kind string) (bool, error) {
	_, ok := f.sent[date+"|"+kind+"|"+strconv.Itoa(scheduleID)]
	return ok, nil
}

func (f *fakeReminders) MarkSent(_ context.Context, date string, scheduleID int, kind, _ string) error {
	f.sent[date+"|"+kind+"|"+strconv.Itoa(scheduleID)] = struct{}{}
	return nil
}

func (f *fakeReminders) PurgeBefore(context.Context, string) error { return nil }
func (f *fakeReminders) ListAll(context.Context) ([]domain.ReminderState, error) {
	return nil, nil
}
func (f *fakeReminders) Upsert(_ context.Context, state domain.ReminderState) (domain.ReminderState, error) {
	return state, nil
}
func (f *fakeReminders) Delete(context.Context, domain.ReminderFilter) (int, error) {
	return 0, nil
}

type fakeMessenger struct {
	messages []string
}

func (f *fakeMessenger) SendMessage(_ context.Context, _ int64, text string) error {
	f.messages = append(f.messages, text)
	return nil
}

func (f *fakeMessenger) SendTyping(context.Context, int64) error { return nil }

func testConfig() *config.Config {
	return &config.Config{
		Location:              time.UTC,
		TelegramUserID:        123,
		AIJSONRetries:         1,
		AITimeout:             5 * time.Second,
		ReminderRetentionDays: 7,
		ProgressRetentionDays: 90,
		SheetArchiveEnabled:   false,
	}
}

func testPrompts(t *testing.T) *prompt.Loader {
	t.Helper()
	loader, err := prompt.New(t.TempDir(), false)
	if err != nil {
		t.Fatalf("prompt loader: %v", err)
	}
	return loader
}

func TestHandleCreatesScheduleFromAIJSON(t *testing.T) {
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"create_schedule","schedule":{"type":"weekly","day_of_week":"Friday","start_time":"09:00","end_time":"10:00","activity":"Belajar Go"},"progress":{},"reply":null}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo ingetin belajar go jumat jam 9")

	if len(schedules.items) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(schedules.items))
	}
	got := schedules.items[0]
	if got.Activity != "Belajar Go" || got.StartTime != "09:00" || got.EndTime != "10:00" {
		t.Fatalf("unexpected schedule: %+v", got)
	}
	if reply == "" {
		t.Fatal("expected reply")
	}
}

func TestHandleCompletesProgress(t *testing.T) {
	schedules := &fakeSchedules{nextID: 1, items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Membaca Buku", Active: true,
	}}}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"complete_progress","schedule":{"id":1},"progress":{"schedule_id":1,"activity":"Membaca Buku","status":"completed","note":"30 menit"},"reply":null}`,
		`Mantap, lanjutkan.`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	svc.Handle(context.Background(), 123, "/sudo sudah baca buku 30 menit")

	if len(progress.items) != 1 {
		t.Fatalf("expected 1 progress row, got %d", len(progress.items))
	}
	if progress.items[0].Status != domain.ProgressCompleted {
		t.Fatalf("expected completed, got %s", progress.items[0].Status)
	}
	if progress.items[0].ScheduleID != 1 {
		t.Fatalf("expected schedule id 1, got %d", progress.items[0].ScheduleID)
	}
}

func TestIntentAcceptsQuotedIDsAndBooleans(t *testing.T) {
	var parsed intentResult
	raw := `{"action":"update_schedule","schedule":{"id":"7","active":"false"},"progress":{"schedule_id":"9"}}`
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if parsed.Schedule.ID == nil || *parsed.Schedule.ID != 7 {
		t.Fatalf("expected schedule id 7, got %v", parsed.Schedule.ID)
	}
	if parsed.Schedule.Active == nil || *parsed.Schedule.Active {
		t.Fatalf("expected active false, got %v", parsed.Schedule.Active)
	}
	if parsed.Progress.ScheduleID == nil || *parsed.Progress.ScheduleID != 9 {
		t.Fatalf("expected progress schedule id 9, got %v", parsed.Progress.ScheduleID)
	}
}

func TestIntentAcceptsIDArray(t *testing.T) {
	var parsed intentResult
	raw := `{"action":"delete_schedule","schedule":{"id":[2,"3",4]}}`
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	want := []int{2, 3, 4}
	if len(parsed.Schedule.IDs) != len(want) {
		t.Fatalf("expected ids %v, got %v", want, parsed.Schedule.IDs)
	}
	for i := range want {
		if parsed.Schedule.IDs[i] != want[i] {
			t.Fatalf("expected ids %v, got %v", want, parsed.Schedule.IDs)
		}
	}
}

func TestHandleDeletesMultipleSchedules(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{
		{ID: 1, Activity: "Satu"},
		{ID: 2, Activity: "Dua"},
		{ID: 3, Activity: "Tiga"},
	}}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"delete_schedule","schedule":{"id":[1,2,3]}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo hapus jadwal id 1 sampai 3")

	if len(schedules.items) != 0 {
		t.Fatalf("expected all schedules deleted, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "udah aku hapus") {
		t.Fatalf("expected delete confirmation in reply, got %q", reply)
	}
	if strings.Contains(reply, "#") {
		t.Fatalf("reply must not expose ids, got %q", reply)
	}
}

func TestHandleCompletesProgressWithQuotedIDs(t *testing.T) {
	schedules := &fakeSchedules{nextID: 1, items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Membaca Buku", Active: true,
	}}}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"complete_progress","schedule":{"id":"1"},"progress":{"schedule_id":"1","activity":"Membaca Buku","status":"completed","note":"30 menit"}}`,
		`Baca 30 menit kelar.`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	svc.Handle(context.Background(), 123, "/sudo sudah baca buku 30 menit")

	if len(progress.items) != 1 {
		t.Fatalf("expected 1 progress row, got %d", len(progress.items))
	}
	if progress.items[0].ScheduleID != 1 {
		t.Fatalf("expected schedule id 1, got %d", progress.items[0].ScheduleID)
	}
}

func TestIntentParsesProgressDeleteFilters(t *testing.T) {
	var parsed intentResult
	raw := `{"action":"delete_progress","progress":{"all":"true","date":"2026-11-06","schedule_id":"3"}}`
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if parsed.Progress.All == nil || !*parsed.Progress.All {
		t.Fatalf("expected all=true, got %+v", parsed.Progress.All)
	}
	if parsed.Progress.Date == nil || *parsed.Progress.Date != "2026-11-06" {
		t.Fatalf("expected date, got %+v", parsed.Progress.Date)
	}
	if parsed.Progress.ScheduleID == nil || *parsed.Progress.ScheduleID != 3 {
		t.Fatalf("expected schedule id 3, got %+v", parsed.Progress.ScheduleID)
	}
}

func TestHandleDeletesAllProgressWithoutTouchingSchedules(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Baca Buku", Active: true,
	}}}
	progress := &fakeProgress{items: []domain.Progress{
		{ID: 1, Date: "2026-11-06", ScheduleID: 1, Activity: "Baca Buku"},
		{ID: 2, Date: "2026-11-06", ScheduleID: 1, Activity: "Lari"},
	}}
	aiClient := &fakeAI{responses: []string{
		`{"action":"delete_progress","progress":{"all":true}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo hapus semua progres log")

	if len(progress.items) != 0 {
		t.Fatalf("expected all progress deleted, got %d", len(progress.items))
	}
	if len(schedules.items) != 1 {
		t.Fatalf("schedules must not be touched, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "2 baris progres") {
		t.Fatalf("expected deleted count in reply, got %q", reply)
	}
}

func TestHandleDeletesProgressByActivity(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Baca Buku", Active: true,
	}}}
	progress := &fakeProgress{items: []domain.Progress{
		{ID: 1, Date: "2026-11-06", ScheduleID: 1, Activity: "Baca Buku"},
		{ID: 2, Date: "2026-11-06", ScheduleID: 1, Activity: "Lari"},
	}}
	aiClient := &fakeAI{responses: []string{
		`{"action":"delete_progress","progress":{"activity":"baca"}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo hapus progres baca buku")

	if len(progress.items) != 1 || progress.items[0].Activity != "Lari" {
		t.Fatalf("expected only Lari to remain, got %+v", progress.items)
	}
	if len(schedules.items) != 1 {
		t.Fatalf("schedules must not be touched, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "1 baris progres") {
		t.Fatalf("expected deleted count in reply, got %q", reply)
	}
}

func TestHandleCreatesMultipleSchedules(t *testing.T) {
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"create_schedule","schedules":[` +
			`{"type":"weekly","day_of_week":"Saturday","start_time":"07:00","end_time":"08:00","activity":"olahraga","active":true},` +
			`{"type":"weekly","day_of_week":"Sunday","start_time":"07:00","end_time":"08:00","activity":"olahraga","active":true}]}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo olahraga sabtu minggu jam 7-8 pagi")

	if len(schedules.items) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "2 jadwal") {
		t.Fatalf("expected multi-create confirmation, got %q", reply)
	}
	if strings.Contains(reply, "#") {
		t.Fatalf("reply must not expose ids, got %q", reply)
	}
}

func TestHandleListsTomorrowWithoutIDs(t *testing.T) {
	tomorrow := time.Now().In(time.UTC).AddDate(0, 0, 1).Format("2006-01-02")
	schedules := &fakeSchedules{items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "20:00", EndTime: "21:00", Activity: "Baca Buku", Active: true,
	}}}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"list_schedules","schedule":{"date":"` + tomorrow + `"}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "apa jadwal saya besok")

	if !strings.Contains(reply, "Besok") || !strings.Contains(reply, "Baca Buku") {
		t.Fatalf("expected tomorrow schedule, got %q", reply)
	}
	if strings.Contains(reply, "#") {
		t.Fatalf("reply must not expose ids, got %q", reply)
	}
}

func TestPendingCreateWaitsForConfirmation(t *testing.T) {
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"clarify","reply":"Oke jam 7-8 Sabtu dan Minggu, setuju ga?","proposal":{"action":"create_schedule","schedules":[` +
			`{"type":"weekly","day_of_week":"Saturday","start_time":"07:00","end_time":"08:00","activity":"olahraga","active":true},` +
			`{"type":"weekly","day_of_week":"Sunday","start_time":"07:00","end_time":"08:00","activity":"olahraga","active":true}]}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo sabtu minggu olahraga 1 jam pagi")

	if len(schedules.items) != 0 {
		t.Fatalf("must not create before confirmation, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "setuju") {
		t.Fatalf("expected confirmation question, got %q", reply)
	}

	svc.Handle(context.Background(), 123, "/sudo setuju")

	if len(schedules.items) != 2 {
		t.Fatalf("expected 2 schedules after confirmation, got %d", len(schedules.items))
	}
	if len(aiClient.requests) != 1 {
		t.Fatalf("confirmation should not call AI again, got %d requests", len(aiClient.requests))
	}
}

func TestPendingConfirmWithoutSudoIsDiscarded(t *testing.T) {
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"clarify","reply":"Oke jam 7-8 Sabtu dan Minggu, setuju ga?","proposal":{"action":"create_schedule","schedules":[` +
			`{"type":"weekly","day_of_week":"Saturday","start_time":"07:00","end_time":"08:00","activity":"olahraga","active":true}]}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	svc.Handle(context.Background(), 123, "/sudo sabtu olahraga jam 7 pagi")

	reply := svc.Handle(context.Background(), 123, "setuju")

	if len(schedules.items) != 0 {
		t.Fatalf("confirm without sudo must not create, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "pakai /sudo dulu") {
		t.Fatalf("expected sudo prompt, got %q", reply)
	}
}

func TestPendingDeleteWaitsForConfirmation(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{
		{ID: 1, Type: domain.RecurrenceWeekly, DayOfWeek: "Saturday", StartTime: "07:00", EndTime: "08:00", Activity: "olahraga", Active: true},
		{ID: 2, Type: domain.RecurrenceWeekly, DayOfWeek: "Sunday", StartTime: "07:00", EndTime: "08:00", Activity: "olahraga", Active: true},
	}}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"clarify","reply":"Yang tadi itu olahraga Sabtu dan Minggu, beneran mau dibatalin?","proposal":{"action":"delete_schedule","schedule":{"id":[1,2]}}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo eh batalkan saja yg tadi")

	if len(schedules.items) != 2 {
		t.Fatalf("must not delete before confirmation, got %d", len(schedules.items))
	}
	if !strings.Contains(reply, "beneran mau dibatalin") {
		t.Fatalf("expected cancellation question, got %q", reply)
	}

	reply = svc.Handle(context.Background(), 123, "/sudo ya")

	if len(schedules.items) != 0 {
		t.Fatalf("expected all schedules deleted after confirmation, got %d", len(schedules.items))
	}
	if strings.Contains(reply, "#") {
		t.Fatalf("reply must not expose ids, got %q", reply)
	}
	if len(aiClient.requests) != 1 {
		t.Fatalf("confirmation should not call AI again, got %d requests", len(aiClient.requests))
	}
}

func TestHandleClarifiesAmbiguousDelete(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Baca Buku", Active: true,
	}}}
	progress := &fakeProgress{items: []domain.Progress{
		{ID: 1, Date: "2026-11-06", ScheduleID: 1, Activity: "Baca Buku"},
	}}
	question := "Mau hapus semua progres log, atau tanggal tertentu?"
	aiClient := &fakeAI{responses: []string{
		`{"action":"clarify","reply":"` + question + `"}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "hapus data progres log")

	if reply != question {
		t.Fatalf("expected clarification question, got %q", reply)
	}
	if len(progress.items) != 1 || len(schedules.items) != 1 {
		t.Fatal("clarify must not delete anything")
	}
}

func TestHandleBlocksWrongProgressDelete(t *testing.T) {
	schedules := &fakeSchedules{items: []domain.Schedule{{
		ID: 1, Type: domain.RecurrenceDaily, StartTime: "09:00", EndTime: "10:00", Activity: "Baca Buku", Active: true,
	}}}
	progress := &fakeProgress{items: []domain.Progress{
		{ID: 1, Date: "2026-11-06", ScheduleID: 1, Activity: "Baca Buku"},
	}}
	aiClient := &fakeAI{responses: []string{
		`{"action":"delete_schedule","schedule":{"activity":"Baca Buku"}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	reply := svc.Handle(context.Background(), 123, "/sudo hapus semua progres log")

	if !strings.Contains(reply, "progres") {
		t.Fatalf("expected progress clarification, got %q", reply)
	}
	if len(schedules.items) != 1 || len(progress.items) != 1 {
		t.Fatal("wrongly classified request must not delete anything")
	}
}

func TestIntentExtractionIncludesHistory(t *testing.T) {
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	aiClient := &fakeAI{responses: []string{
		`{"action":"clarify","reply":"Mau hapus semua progres log, atau tanggal tertentu?"}`,
		`{"action":"delete_progress","progress":{"all":true}}`,
	}}

	svc := NewChatService(testConfig(), aiClient, testPrompts(t), schedules, progress, slog.Default())
	svc.Handle(context.Background(), 123, "hapus data progres log")
	svc.Handle(context.Background(), 123, "/sudo semua")

	if len(aiClient.requests) < 2 {
		t.Fatalf("expected at least 2 AI requests, got %d", len(aiClient.requests))
	}
	secondPrompt := aiClient.requests[1].Messages[0].Content
	for _, want := range []string{"RIWAYAT PERCAKAPAN", "hapus data progres log", "Mau hapus semua progres log"} {
		if !strings.Contains(secondPrompt, want) {
			t.Fatalf("expected history prompt to contain %q", want)
		}
	}
}

func TestReminderIsIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	schedules := &fakeSchedules{}
	progress := &fakeProgress{}
	reminders := newFakeReminders()
	messenger := &fakeMessenger{}
	aiClient := &fakeAI{responses: []string{"Waktunya belajar!"}}

	svc := NewReminderService(testConfig(), aiClient, testPrompts(t), schedules, progress, reminders, messenger, slog.Default())
	schedule := domain.Schedule{ID: 7, Type: domain.RecurrenceOnce, Date: "2026-11-06", StartTime: "09:00", EndTime: "10:00", Activity: "Belajar", Active: true}

	if err := svc.sendReminder(context.Background(), schedule, now); err != nil {
		t.Fatalf("send reminder: %v", err)
	}
	if err := svc.sendReminder(context.Background(), schedule, now); err != nil {
		t.Fatalf("send reminder again: %v", err)
	}

	if len(messenger.messages) != 1 {
		t.Fatalf("expected exactly 1 message, got %d", len(messenger.messages))
	}
}
