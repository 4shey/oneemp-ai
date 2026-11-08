package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"one_more_percent/internal/ai"
	"one_more_percent/internal/config"
	"one_more_percent/internal/domain"
	"one_more_percent/internal/prompt"
)

const (
	actionNone             = "none"
	actionCreateSchedule   = "create_schedule"
	actionUpdateSchedule   = "update_schedule"
	actionDeleteSchedule   = "delete_schedule"
	actionDeleteProgress   = "delete_progress"
	actionListSchedules    = "list_schedules"
	actionListProgress     = "list_progress"
	actionListArchive      = "list_archive"
	actionUpsertArchive    = "upsert_archive"
	actionDeleteArchive    = "delete_archive"
	actionListReminders    = "list_reminders"
	actionUpsertReminder   = "upsert_reminder"
	actionDeleteReminders  = "delete_reminders"
	actionCompleteProgress = "complete_progress"
	actionClarify          = "clarify"
	actionConfirmPending   = "confirm_pending"
	actionCancelPending    = "cancel_pending"
)

type extractedSchedule struct {
	ID        *int    `json:"id"`
	IDs       []int   `json:"ids"`
	Type      *string `json:"type"`
	Date      *string `json:"date"`
	DayOfWeek *string `json:"day_of_week"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
	Activity  *string `json:"activity"`
	Active    *bool   `json:"active"`
	All       *bool   `json:"all"`
}

type extractedProgress struct {
	ScheduleID *int    `json:"schedule_id"`
	Activity   *string `json:"activity"`
	Status     *string `json:"status"`
	Note       *string `json:"note"`
	Date       *string `json:"date"`
	All        *bool   `json:"all"`
}

type extractedReminder struct {
	Date       *string `json:"date"`
	ScheduleID *int    `json:"schedule_id"`
	Type       *string `json:"type"`
	SentAt     *string `json:"sent_at"`
	Before     *string `json:"before"`
	All        *bool   `json:"all"`
}

type intentProposal struct {
	Action    string              `json:"action"`
	Schedule  extractedSchedule   `json:"schedule"`
	Schedules []extractedSchedule `json:"schedules"`
}

type intentResult struct {
	Action    string              `json:"action"`
	Schedule  extractedSchedule   `json:"schedule"`
	Schedules []extractedSchedule `json:"schedules"`
	Progress  extractedProgress   `json:"progress"`
	Reminder  extractedReminder   `json:"reminder"`
	Proposal  intentProposal      `json:"proposal"`
	Reply     *string             `json:"reply"`
}

// ChatService turns free-form Telegram messages into schedule CRUD,
// progress logging, or normal AI conversation.
type ChatService struct {
	cfg       *config.Config
	ai        ai.Client
	prompts   *prompt.Loader
	schedules domain.ScheduleRepository
	progress  domain.ProgressRepository
	reminders domain.ReminderStateRepository
	history   *historyStore
	pending   *pendingStore
	logger    *slog.Logger
}

// NewChatService wires chat behaviour.
func NewChatService(
	cfg *config.Config,
	aiClient ai.Client,
	prompts *prompt.Loader,
	schedules domain.ScheduleRepository,
	progress domain.ProgressRepository,
	logger *slog.Logger,
	reminders ...domain.ReminderStateRepository,
) *ChatService {
	svc := &ChatService{
		cfg:       cfg,
		ai:        aiClient,
		prompts:   prompts,
		schedules: schedules,
		progress:  progress,
		history:   newHistoryStore(20),
		pending:   newPendingStore(15 * time.Minute),
		logger:    logger,
	}
	if len(reminders) > 0 {
		svc.reminders = reminders[0]
	}
	return svc
}

// Handle processes one inbound message and returns the reply text.
func (s *ChatService) Handle(ctx context.Context, chatID int64, text string) string {
	now := time.Now().In(s.cfg.Location)
	today := now.Format("2006-01-02")

	schedules, err := s.schedules.List(ctx)
	if err != nil {
		s.logger.Error("baca jadwal", "error", err)
		return "Lagi ga bisa baca database jadwal. Coba lagi nanti."
	}
	todayProgress, err := s.progress.ListByDate(ctx, today)
	if err != nil {
		s.logger.Warn("baca progres hari ini", "error", err)
	}

	if pending, ok := s.pending.Get(chatID); ok {
		if hasSudo(text) {
			rest := stripSudoText(text)
			if isSimpleConfirm(rest) {
				return s.finish(chatID, text, s.executePending(ctx, chatID, pending, schedules, now))
			}
			if isSimpleCancel(rest) {
				s.pending.Clear(chatID)
				return s.finish(chatID, text, "Oke, ga jadi")
			}
			s.pending.Clear(chatID)
		} else {
			s.pending.Clear(chatID)
			if isSimpleConfirm(text) {
				return s.finish(chatID, text, "kalau mau konfirmasi, pakai /sudo dulu ya")
			}
			if isSimpleCancel(text) {
				return s.finish(chatID, text, "Oke, ga jadi")
			}
		}
	}

	if strings.TrimRight(text, " \t\r\n") == "/sudo" {
		return s.finish(chatID, text, "mau ubah apa? tulis perintahnya setelah /sudo")
	}

	history := s.history.Get(chatID)
	intent, err := s.extractIntent(ctx, text, history, schedules, todayProgress, now)
	if err != nil {
		s.logger.Error("ekstraksi intent", "error", err)
		return friendlyAIError(err)
	}
	intent = guardDestructiveIntent(intent, text)
	intent = guardProgressIntent(intent, text)

	scheduleExt := intent.Schedule
	if scheduleExt.ID == nil && len(scheduleExt.IDs) == 0 {
		scheduleExt.ID = intent.Progress.ScheduleID
	}
	if scheduleExt.Activity == nil {
		scheduleExt.Activity = intent.Progress.Activity
	}
	progressExt := intent.Progress
	if progressExt.ScheduleID == nil {
		progressExt.ScheduleID = intent.Schedule.ID
	}
	if progressExt.Activity == nil {
		progressExt.Activity = intent.Schedule.Activity
	}

	action := strings.ToLower(strings.TrimSpace(intent.Action))
	sudo := hasSudo(text)
	if !sudo && requiresSudo(action) {
		s.pending.Clear(chatID)
		s.logger.Warn("aksi database ditolak tanpa sudo", "chat_id", chatID, "action", action)
		return s.finish(chatID, text, sudoRequiredReply(action))
	}
	if !sudo && intent.Proposal.Action != "" {
		s.pending.Clear(chatID)
		s.logger.Warn("proposal database ditolak tanpa sudo", "chat_id", chatID, "action", intent.Proposal.Action)
		return s.finish(chatID, text, sudoRequiredReply(intent.Proposal.Action))
	}
	if sudo && action == actionNone {
		return s.finish(chatID, text, "mau ubah apa? tulis perintahnya setelah /sudo")
	}
	if sudo {
		s.logger.Info("aksi sudo", "chat_id", chatID, "action", action)
	}
	scheduleExt.Activity = stripSudoPtr(scheduleExt.Activity)
	progressExt.Activity = stripSudoPtr(progressExt.Activity)
	progressExt.Note = stripSudoPtr(progressExt.Note)

	if action != actionClarify && action != actionNone && action != actionConfirmPending && action != actionCancelPending {
		s.pending.Clear(chatID)
	}

	var reply string
	switch action {
	case actionCreateSchedule:
		reply = s.createSchedules(ctx, scheduleExt, intent.Schedules, now)
	case actionUpdateSchedule:
		reply = s.updateSchedule(ctx, scheduleExt, schedules, now)
	case actionDeleteSchedule:
		reply = s.deleteSchedule(ctx, scheduleExt, schedules)
	case actionDeleteProgress:
		reply = s.deleteProgress(ctx, progressExt, today, now)
	case actionListSchedules:
		reply = s.listSchedules(scheduleExt, schedules, now)
	case actionListProgress:
		reply = s.listAllProgress(ctx)
	case actionListArchive:
		reply = s.listArchive(ctx)
	case actionUpsertArchive:
		reply = s.upsertArchive(ctx, progressExt, now)
	case actionDeleteArchive:
		reply = s.deleteArchive(ctx, progressExt)
	case actionListReminders:
		reply = s.listReminders(ctx)
	case actionUpsertReminder:
		reply = s.upsertReminder(ctx, intent.Reminder)
	case actionDeleteReminders:
		reply = s.deleteReminders(ctx, intent.Reminder)
	case actionCompleteProgress:
		reply = s.saveProgress(ctx, progressExt, schedules, todayProgress, now)
	case actionClarify:
		if intent.Proposal.Action != "" {
			s.pending.Set(chatID, pendingProposal{
				action:    intent.Proposal.Action,
				schedule:  intent.Proposal.Schedule,
				schedules: intent.Proposal.Schedules,
			})
		}
		reply = clarifyReply(intent.Reply)
	case actionConfirmPending:
		if pending, ok := s.pending.Get(chatID); ok {
			reply = s.executePending(ctx, chatID, pending, schedules, now)
		} else {
			reply = "Ga ada yang perlu dikonfirmasi"
		}
	case actionCancelPending:
		s.pending.Clear(chatID)
		reply = "Oke, ga jadi"
	default:
		reply = s.normalChat(ctx, chatID, text, schedules, todayProgress, now)
	}

	return s.finish(chatID, text, reply)
}

func (s *ChatService) finish(chatID int64, text, reply string) string {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		reply = msgSafeFallback
	}
	s.history.Add(chatID, ai.RoleUser, text)
	s.history.Add(chatID, ai.RoleAssistant, reply)
	return truncateRunes(reply, 4000)
}

func (s *ChatService) extractIntent(
	ctx context.Context,
	message string,
	history []ai.Message,
	schedules []domain.Schedule,
	progress []domain.Progress,
	now time.Time,
) (intentResult, error) {
	rendered, err := s.prompts.Render("intent_extraction", map[string]any{
		"Now":           now.Format("Monday, 02 January 2006 15:04 MST"),
		"Today":         now.Format("2006-01-02"),
		"Weekday":       now.Weekday().String(),
		"Schedules":     formatScheduleLines(schedules),
		"TodayProgress": formatProgressLines(progress),
		"History":       formatHistory(history),
		"UserMessage":   message,
	})
	if err != nil {
		return intentResult{}, err
	}

	attempts := s.cfg.AIJSONRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	prompt := rendered
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			prompt = rendered + fmt.Sprintf(
				"\n\nPERBAIKAN: balasan JSON sebelumnya tidak valid (%v). "+
					"Balas ulang HANYA JSON valid. "+
					"schedule.id dan progress.schedule_id wajib angka integer tanpa tanda kutip, atau null; "+
					"khusus delete_schedule schedule.id boleh array integer. "+
					"schedule.active dan progress.all wajib boolean true/false, bukan string.",
				lastErr,
			)
		}

		resp, err := s.ai.Complete(ctx, ai.Request{
			Messages: []ai.Message{{Role: ai.RoleUser, Content: prompt}},
			JSON:     true,
		})
		if err != nil {
			return intentResult{}, err
		}

		var parsed intentResult
		if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &parsed); err != nil {
			lastErr = fmt.Errorf("%w: %v", errInvalidIntentJSON, err)
			s.logger.Warn("intent JSON tidak valid", "attempt", attempt+1, "error", err,
				"raw", truncateRunes(extractJSON(resp.Content), 400))
			continue
		}
		if parsed.Action == "" {
			parsed.Action = actionNone
		}
		return parsed, nil
	}
	return intentResult{}, lastErr
}

func (s *ChatService) createSchedules(ctx context.Context, single extractedSchedule, many []extractedSchedule, now time.Time) string {
	items := many
	if len(items) == 0 {
		items = []extractedSchedule{single}
	}
	if len(items) == 0 {
		return "Jadwalnya mana? Sebutin aktivitas dan waktunya"
	}

	created := make([]domain.Schedule, 0, len(items))
	for _, ext := range items {
		schedule, problem := buildCreateSchedule(ext, now)
		if problem != "" {
			return problem
		}
		saved, err := s.schedules.Create(ctx, schedule)
		if err != nil {
			s.logger.Error("buat jadwal", "error", err)
			return "Gagal nyimpen jadwal: " + err.Error()
		}
		created = append(created, saved)
	}
	return formatCreatedSchedules(created)
}

func (s *ChatService) updateSchedule(ctx context.Context, ext extractedSchedule, all []domain.Schedule, now time.Time) string {
	if len(ext.IDs) > 1 {
		return "Ubah jadwal satu per satu dulu ya, biar ga salah."
	}
	if ext.primaryID() == nil && isEmptyStringPtr(ext.Activity) {
		return "Jadwal yang mana yang mau diubah? Sebutin nama atau id-nya."
	}
	current, ok := resolveSchedule(all, ext.primaryID(), ext.Activity)
	if !ok {
		return "Jadwal yang mau diubah ga ketemu. Cek lagi nama atau id-nya."
	}

	updated := current
	applySchedulePatch(&updated, ext, now)
	if err := updated.Validate(); err != nil {
		return "Perubahan jadwal ga valid: " + err.Error()
	}

	saved, err := s.schedules.Update(ctx, updated)
	if err != nil {
		s.logger.Error("ubah jadwal", "error", err)
		return "Gagal ngubah jadwal: " + err.Error()
	}
	return fmt.Sprintf("Beres, jadwal %s sekarang %s",
		scheduleLabel(saved), scheduleTimeLabel(saved))
}

func (s *ChatService) deleteSchedule(ctx context.Context, ext extractedSchedule, all []domain.Schedule) string {
	if len(ext.IDs) > 0 {
		return s.deleteSchedules(ctx, ext.IDs, all)
	}
	if ext.primaryID() == nil && isEmptyStringPtr(ext.Activity) {
		if len(all) == 0 {
			return "Belum ada jadwal yang bisa dihapus."
		}
		return "Hapus jadwal yang mana? Sebutin nama atau waktunya, atau bilang 'semua jadwal'"
	}
	current, ok := resolveSchedule(all, ext.primaryID(), ext.Activity)
	if !ok {
		return "Jadwal yang mau dibatalin ga ketemu. Cek lagi nama atau waktunya"
	}
	if err := s.schedules.Delete(ctx, current.ID); err != nil {
		s.logger.Error("hapus jadwal", "error", err)
		return "Gagal ngehapus jadwal: " + err.Error()
	}
	return fmt.Sprintf("Jadwal %s %s udah aku hapus",
		scheduleLabel(current), scheduleTimeLabel(current))
}

func (s *ChatService) deleteSchedules(ctx context.Context, ids []int, all []domain.Schedule) string {
	ids = uniqueInts(ids)
	names := scheduleNames(all, ids)
	deleted, missing, err := s.schedules.DeleteMany(ctx, ids)
	if err != nil {
		s.logger.Error("hapus banyak jadwal", "error", err)
		return "Gagal ngehapus jadwal: " + err.Error()
	}
	if deleted == 0 {
		if len(missing) > 0 {
			return fmt.Sprintf("Ga ada jadwal yang cocok, %d jadwal ga ketemu", len(missing))
		}
		return "Ga ada jadwal yang bisa dihapus"
	}

	if deleted == 1 && len(names) == 1 {
		return fmt.Sprintf("Jadwal %s udah aku hapus", names[0])
	}
	if deleted <= 3 && len(names) == deleted {
		return fmt.Sprintf("Beres, %s udah aku hapus", strings.Join(names, ", "))
	}

	reply := fmt.Sprintf("Beres, %d jadwal udah aku hapus", deleted)
	if len(missing) > 0 {
		reply += fmt.Sprintf(", %d jadwal ga ketemu", len(missing))
	}
	return reply
}

func (s *ChatService) listSchedules(ext extractedSchedule, all []domain.Schedule, now time.Time) string {
	if ext.Date != nil && strings.TrimSpace(*ext.Date) != "" {
		date, ok := resolveListDate(*ext.Date, now)
		if !ok {
			return "Tanggalnya ga kebaca. Sebutin hari atau tanggalnya"
		}
		return formatSchedulesOnDate(all, date, now)
	}
	return formatScheduleList(all)
}

func (s *ChatService) listAllProgress(ctx context.Context) string {
	items, err := s.progress.ListAll(ctx)
	if err != nil {
		s.logger.Error("baca semua progres", "error", err)
		return "Gagal baca progres: " + err.Error()
	}
	return formatProgressAdmin(items, "Semua progres")
}

func (s *ChatService) listArchive(ctx context.Context) string {
	items, err := s.progress.ListArchive(ctx)
	if err != nil {
		s.logger.Error("baca arsip progres", "error", err)
		return "Gagal baca arsip: " + err.Error()
	}
	return formatProgressAdmin(items, "Arsip progres")
}

func (s *ChatService) upsertArchive(ctx context.Context, ext extractedProgress, now time.Time) string {
	activity := stringValue(ext.Activity)
	if activity == "" {
		return "Aktivitas arsipnya apa? Sebutin nama aktivitasnya."
	}
	date := stringValue(ext.Date)
	if date == "" {
		date = now.Format("2006-01-02")
	}
	status := domain.ProgressCompleted
	if ext.Status != nil && strings.EqualFold(strings.TrimSpace(*ext.Status), domain.ProgressMissed) {
		status = domain.ProgressMissed
	}
	progress := domain.Progress{
		Date:       date,
		ScheduleID: intValue(ext.ScheduleID),
		Activity:   activity,
		Status:     status,
		Note:       stringValue(ext.Note),
	}
	saved, err := s.progress.UpsertArchive(ctx, progress)
	if err != nil {
		s.logger.Error("simpan arsip", "error", err)
		return "Gagal nyimpen arsip: " + err.Error()
	}
	return fmt.Sprintf("oke, arsip %s tanggal %s udah aku simpan", saved.Activity, saved.Date)
}

func (s *ChatService) deleteArchive(ctx context.Context, ext extractedProgress) string {
	filter := domain.ProgressFilter{
		All:        boolValue(ext.All),
		Date:       stringValue(ext.Date),
		ScheduleID: intValue(ext.ScheduleID),
		Activity:   stringValue(ext.Activity),
	}
	if filter.Empty() {
		return "Mau hapus arsip yang mana? Sebutin tanggal, aktivitas, atau bilang semua."
	}
	deleted, err := s.progress.DeleteArchive(ctx, filter)
	if err != nil {
		s.logger.Error("hapus arsip", "error", err)
		return "Gagal hapus arsip: " + err.Error()
	}
	if deleted == 0 {
		return "Ga ada arsip yang cocok buat dihapus."
	}
	return fmt.Sprintf("oke, %d baris arsip udah aku hapus", deleted)
}

func (s *ChatService) listReminders(ctx context.Context) string {
	if s.reminders == nil {
		return "Reminder state belum tersedia."
	}
	items, err := s.reminders.ListAll(ctx)
	if err != nil {
		s.logger.Error("baca reminder state", "error", err)
		return "Gagal baca reminder state: " + err.Error()
	}
	return formatReminderAdmin(items)
}

func (s *ChatService) upsertReminder(ctx context.Context, ext extractedReminder) string {
	if s.reminders == nil {
		return "Reminder state belum tersedia."
	}
	date := stringValue(ext.Date)
	kind := stringValue(ext.Type)
	if date == "" || kind == "" {
		return "Isi tanggal dan type reminder dulu ya."
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "Tanggal reminder harus format YYYY-MM-DD ya."
	}
	if !validReminderType(kind) {
		return "Type reminder harus reminder, end, atau recap."
	}
	sentAt := stringValue(ext.SentAt)
	if sentAt == "" {
		sentAt = time.Now().Format(time.RFC3339)
	}
	state, err := s.reminders.Upsert(ctx, domain.ReminderState{
		Date:       date,
		ScheduleID: intValue(ext.ScheduleID),
		Type:       kind,
		SentAt:     sentAt,
	})
	if err != nil {
		s.logger.Error("simpan reminder state", "error", err)
		return "Gagal nyimpen reminder state: " + err.Error()
	}
	return fmt.Sprintf("oke, reminder state %s %s udah aku simpan", state.Date, state.Type)
}

func (s *ChatService) deleteReminders(ctx context.Context, ext extractedReminder) string {
	if s.reminders == nil {
		return "Reminder state belum tersedia."
	}
	filter := domain.ReminderFilter{
		All:        boolValue(ext.All),
		Date:       stringValue(ext.Date),
		ScheduleID: intValue(ext.ScheduleID),
		Type:       stringValue(ext.Type),
		Before:     stringValue(ext.Before),
	}
	if filter.Type != "" && !validReminderType(filter.Type) {
		return "Type reminder harus reminder, end, atau recap."
	}
	if !filter.All && filter.Date == "" && filter.ScheduleID == 0 && filter.Type == "" && filter.Before == "" {
		return "Mau hapus reminder state yang mana? Sebutin tanggal, type, atau bilang semua."
	}
	deleted, err := s.reminders.Delete(ctx, filter)
	if err != nil {
		s.logger.Error("hapus reminder state", "error", err)
		return "Gagal hapus reminder state: " + err.Error()
	}
	if deleted == 0 {
		return "Ga ada reminder state yang cocok buat dihapus."
	}
	return fmt.Sprintf("oke, %d baris reminder state udah aku hapus", deleted)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func validReminderType(value string) bool {
	switch value {
	case domain.ReminderTypeSchedule, domain.ReminderTypeEnd, domain.ReminderTypeRecap:
		return true
	default:
		return false
	}
}

func (s *ChatService) executePending(
	ctx context.Context,
	chatID int64,
	pending pendingProposal,
	all []domain.Schedule,
	now time.Time,
) string {
	s.pending.Clear(chatID)

	switch pending.action {
	case actionCreateSchedule:
		items := pending.schedules
		if len(items) == 0 && hasScheduleData(pending.schedule) {
			items = []extractedSchedule{pending.schedule}
		}
		return s.createSchedules(ctx, extractedSchedule{}, items, now)
	case actionDeleteSchedule:
		return s.deleteSchedule(ctx, pending.schedule, all)
	default:
		return "Oke"
	}
}

func hasScheduleData(ext extractedSchedule) bool {
	return ext.Activity != nil || ext.Type != nil || ext.Date != nil ||
		ext.DayOfWeek != nil || ext.StartTime != nil || ext.EndTime != nil
}

func (s *ChatService) deleteProgress(ctx context.Context, ext extractedProgress, today string, now time.Time) string {
	filter := domain.ProgressFilter{}
	if ext.All != nil {
		filter.All = *ext.All
	}
	if !filter.All {
		if ext.Date != nil {
			date, ok := resolveProgressDate(*ext.Date, today, now)
			if !ok {
				return "Tanggalnya ga valid. Pakai format YYYY-MM-DD ya."
			}
			filter.Date = date
		}
		if ext.Activity != nil {
			filter.Activity = strings.TrimSpace(*ext.Activity)
		}
		if ext.ScheduleID != nil {
			filter.ScheduleID = *ext.ScheduleID
		}
	}
	if filter.Empty() {
		return "Mau hapus progres yang mana? Semua, tanggal tertentu, atau aktivitas tertentu?"
	}

	deleted, err := s.progress.Delete(ctx, filter)
	if err != nil {
		s.logger.Error("hapus progres", "error", err)
		return "Gagal ngehapus progres: " + err.Error()
	}
	if deleted == 0 {
		return "Ga ada progres yang cocok buat dihapus."
	}
	return fmt.Sprintf("Beres. %d baris progres udah aku hapus.", deleted)
}

func guardDestructiveIntent(intent intentResult, text string) intentResult {
	action := strings.ToLower(strings.TrimSpace(intent.Action))
	progressWords := containsAnyWord(text, "progres", "progress", "log", "riwayat", "catatan")
	scheduleWords := containsAnyWord(text, "jadwal", "pengingat", "reminder", "agenda", "schedule")

	if action == actionDeleteSchedule && progressWords {
		intent.Action = actionClarify
		question := "Itu soal progres, bukan jadwal. Mau hapus semua progres, tanggal tertentu, atau aktivitas tertentu?"
		intent.Reply = &question
	}
	if action == actionDeleteProgress && scheduleWords && !progressWords {
		intent.Action = actionClarify
		question := "Itu soal jadwal, bukan progres. Mau hapus jadwal yang mana?"
		intent.Reply = &question
	}
	return intent
}

func hasSudo(text string) bool {
	trimmed := strings.TrimRight(text, " \t\r\n")
	return trimmed == "/sudo" || strings.HasPrefix(trimmed, "/sudo ")
}

func stripSudoText(value string) string {
	trimmed := strings.TrimRight(value, " \t\r\n")
	if trimmed == "/sudo" {
		return ""
	}
	if strings.HasPrefix(trimmed, "/sudo ") {
		return strings.TrimSpace(trimmed[len("/sudo"):])
	}
	return strings.TrimSpace(value)
}

func stripSudoPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := stripSudoText(*value)
	return &cleaned
}

func requiresSudo(action string) bool {
	switch action {
	case actionCreateSchedule, actionUpdateSchedule, actionDeleteSchedule,
		actionDeleteProgress, actionCompleteProgress, actionListProgress, actionListArchive,
		actionUpsertArchive, actionDeleteArchive, actionListReminders,
		actionUpsertReminder, actionDeleteReminders, actionConfirmPending:
		return true
	default:
		return false
	}
}

func sudoRequiredReply(action string) string {
	switch action {
	case actionDeleteProgress, actionCompleteProgress:
		return "kalau mau nyatet atau hapus progres, pakai /sudo dulu ya"
	case actionListProgress, actionListArchive, actionListReminders:
		return "kalau mau lihat semua data, pakai /sudo dulu ya"
	case actionUpsertArchive, actionDeleteArchive, actionUpsertReminder, actionDeleteReminders:
		return "kalau mau ubah data, pakai /sudo dulu ya"
	default:
		return "kalau mau ubah data, pakai /sudo dulu ya"
	}
}

func guardProgressIntent(intent intentResult, text string) intentResult {
	if strings.ToLower(strings.TrimSpace(intent.Action)) != actionCompleteProgress {
		return intent
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "belum selesai") || strings.Contains(lower, "belum kelar") {
		intent.Action = actionNone
		return intent
	}
	notDone := containsAnyWord(lower, "siap", "mau mulai", "baru mulai", "berangkat", "lagi ngerjain", "nanti", "belum")
	done := containsAnyWord(lower, "selesai", "kelar", "beres", "sudah")
	if notDone && !done {
		intent.Action = actionNone
	}
	return intent
}

func containsAnyWord(text string, words ...string) bool {
	lower := strings.ToLower(text)
	for _, word := range words {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func clarifyReply(reply *string) string {
	if reply != nil {
		if text := strings.TrimSpace(*reply); text != "" {
			return text
		}
	}
	return "maksud kamu yang mana? Sebutin lebih spesifik ya, biar aku ga salah hapus."
}

func resolveProgressDate(value, today string, now time.Time) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	switch strings.ToLower(value) {
	case "hari ini", "today":
		return today, true
	case "kemarin", "yesterday":
		return now.AddDate(0, 0, -1).Format("2006-01-02"), true
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", false
	}
	return value, true
}

func isEmptyStringPtr(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}

func (s *ChatService) saveProgress(
	ctx context.Context,
	ext extractedProgress,
	all []domain.Schedule,
	todayProgress []domain.Progress,
	now time.Time,
) string {
	today := now.Format("2006-01-02")
	activity := ""
	if ext.Activity != nil {
		activity = strings.TrimSpace(*ext.Activity)
	}

	scheduleID := 0
	if ext.ScheduleID != nil {
		scheduleID = *ext.ScheduleID
	}
	if scheduleID == 0 {
		if match := matchTodaySchedule(all, activity, todayProgress); match != nil {
			scheduleID = match.ID
			if activity == "" {
				activity = match.Activity
			}
		}
	}
	if activity == "" {
		for _, schedule := range all {
			if schedule.ID == scheduleID {
				activity = schedule.Activity
				break
			}
		}
	}
	if activity == "" {
		return "Progres apa yang mau dicatat? Sebutin nama aktivitasnya."
	}

	status := domain.ProgressCompleted
	if ext.Status != nil && strings.EqualFold(strings.TrimSpace(*ext.Status), domain.ProgressMissed) {
		status = domain.ProgressMissed
	}
	note := ""
	if ext.Note != nil {
		note = strings.TrimSpace(*ext.Note)
	}
	progressDate := today
	for _, schedule := range all {
		if schedule.ID != scheduleID {
			continue
		}
		if startDate, ok := startOccurrenceDate(schedule, now); ok {
			progressDate = progressDateFor(schedule, startDate)
		}
		break
	}
	saved, err := s.progress.Upsert(ctx, domain.Progress{
		Date:       progressDate,
		ScheduleID: scheduleID,
		Activity:   activity,
		Status:     status,
		Note:       note,
	})
	if err != nil {
		s.logger.Error("simpan progres", "error", err)
		return "Gagal nyimpen progres: " + err.Error()
	}
	todayProgress = mergeProgress(todayProgress, saved)

	if status == domain.ProgressMissed {
		return fmt.Sprintf("oke, %s aku catat belum kelar. besok jangan diulang ya", activity)
	}
	return s.completionReply(ctx, activity, all, todayProgress, now, scheduleID)
}

func mergeProgress(items []domain.Progress, saved domain.Progress) []domain.Progress {
	for i := range items {
		if items[i].Date == saved.Date && items[i].ScheduleID == saved.ScheduleID {
			items[i] = saved
			return items
		}
	}
	return append(items, saved)
}

func (s *ChatService) completionReply(
	ctx context.Context,
	activity string,
	all []domain.Schedule,
	todayProgress []domain.Progress,
	now time.Time,
	scheduleID int,
) string {
	rendered, err := s.prompts.Render("completion_reply", map[string]any{
		"Activity":      activity,
		"Now":           now.Format("Monday, 02 January 2006 15:04 MST"),
		"TodayStatus":   formatTodayStatus(all, todayProgress, now),
		"NextSchedules": formatNextSchedules(all, now, scheduleID),
	})
	if err != nil {
		return "oke, " + activity + " udah dicatat"
	}
	resp, err := s.ai.Complete(ctx, ai.Request{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: rendered}},
	})
	if err != nil {
		s.logger.Warn("buat balasan completion", "error", err)
		return "oke, " + activity + " udah dicatat"
	}
	return resp.Content
}

func shouldUseUserContext(text string, missed bool) bool {
	if missed {
		return true
	}
	lower := strings.ToLower(text)
	for _, word := range []string{
		"alasan", "impian", "tujuan", "motivasi", "buat apa",
		"nyerah", "gagal", "berubah", "demi ",
	} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func (s *ChatService) normalChat(
	ctx context.Context,
	chatID int64,
	text string,
	all []domain.Schedule,
	todayProgress []domain.Progress,
	now time.Time,
) string {
	mode := currentScheduleMode(all, now)
	userContext := ""
	if shouldUseUserContext(text, hasMissedToday(all, todayProgress, now)) {
		userContext = s.cfg.UserContext
	}
	data := map[string]any{
		"Now":              now.Format("Monday, 02 January 2006 15:04 MST"),
		"Today":            now.Format("2006-01-02"),
		"UserName":         "Ambatukam",
		"UserContext":      userContext,
		"ScheduleMode":     mode,
		"ScheduleOverview": formatScheduleOverview(all),
		"TodayStatus":      formatTodayStatus(all, todayProgress, now),
		"MissedToday":      formatMissedToday(all, todayProgress, now),
		"NextSchedules":    formatNextSchedules(all, now, 0),
		"TodayProgress":    formatProgressNotes(todayProgress),
	}
	system, err := s.prompts.Render("system_chat", data)
	if err != nil {
		s.logger.Error("render system chat", "error", err)
		return msgSafeFallback
	}

	messages := s.history.Get(chatID)
	messages = append(messages, ai.Message{Role: ai.RoleUser, Content: text})

	resp, err := s.ai.Complete(ctx, ai.Request{System: system, Messages: messages})
	if err != nil {
		s.logger.Error("chat AI", "error", err)
		return friendlyAIError(err)
	}
	return resp.Content
}
