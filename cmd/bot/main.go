package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	aifactory "one_more_percent/internal/ai/factory"
	"one_more_percent/internal/config"
	"one_more_percent/internal/httpserver"
	"one_more_percent/internal/prompt"
	sheetrepo "one_more_percent/internal/repository/sheets"
	"one_more_percent/internal/scheduler"
	"one_more_percent/internal/service"
	"one_more_percent/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi gagal", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sheetsClient, err := sheetrepo.NewClient(ctx, cfg)
	if err != nil {
		logger.Error("inisialisasi Google Sheets", "error", err)
		os.Exit(1)
	}

	scheduleRepo := sheetrepo.NewScheduleRepository(sheetsClient, cfg.SheetsCacheTTL)
	progressRepo := sheetrepo.NewProgressRepository(sheetsClient)
	reminderRepo := sheetrepo.NewReminderStateRepository(sheetsClient)

	aiClient, err := aifactory.New(cfg)
	if err != nil {
		logger.Error("inisialisasi AI", "error", err)
		os.Exit(1)
	}

	prompts, err := prompt.New(cfg.PromptsDir, cfg.PromptsHotReload)
	if err != nil {
		logger.Error("inisialisasi prompt", "error", err)
		os.Exit(1)
	}

	telegramClient := telegram.New(cfg.TelegramBotToken, cfg.TelegramAPIBaseURL, cfg.AITimeout)
	registerWebhook(ctx, cfg, telegramClient, logger)

	chatService := service.NewChatService(cfg, aiClient, prompts, scheduleRepo, progressRepo, logger, reminderRepo)
	reminderService := service.NewReminderService(
		cfg, aiClient, prompts, scheduleRepo, progressRepo, reminderRepo, telegramClient, logger,
	)

	sched := scheduler.New(cfg, reminderService, logger)
	go sched.Run(ctx)

	api := httpserver.New(cfg, chatService, telegramClient, logger)
	httpServer := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("graceful shutdown", "error", err)
		}
	}()

	notifyStartup(telegramClient, cfg, logger)

	logger.Info("server berjalan", "port", cfg.AppPort, "webhook", cfg.WebhookURL())
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server berhenti", "error", err)
		os.Exit(1)
	}
	logger.Info("server berhenti dengan bersih")
}

func registerWebhook(ctx context.Context, cfg *config.Config, client *telegram.Client, logger *slog.Logger) {
	webhookCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if err := client.SetWebhook(webhookCtx, cfg.WebhookURL(), cfg.TelegramWebhookSecret); err != nil {
		logger.Warn("setWebhook gagal, cek PUBLIC_BASE_URL dan koneksi", "error", err)
		return
	}
	logger.Info("webhook terdaftar", "url", cfg.WebhookURL())
}

func notifyStartup(client *telegram.Client, cfg *config.Config, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	message := "Woi gw udah nyala nih. Cek jadwal lu, jangan males-malesan!"
	if err := client.SendMessage(ctx, cfg.TelegramUserID, message); err != nil {
		logger.Warn("kirim pesan startup", "error", err)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
