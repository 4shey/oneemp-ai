// Package config loads and validates every runtime setting from the
// environment (optionally hydrated from a local .env file).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the single source of truth for runtime configuration.
type Config struct {
	// App
	AppPort         string
	AppTimezone     string
	PublicBaseURL   string
	WebhookPath     string
	CronTick        time.Duration
	DailyRecapTime  string
	Location        *time.Location
	LogLevel        string
	ShutdownTimeout time.Duration

	// Telegram
	TelegramBotToken      string
	TelegramUserID        int64
	TelegramAPIBaseURL    string
	TelegramWebhookSecret string

	// Google Sheets
	SheetsSpreadsheetID   string
	GoogleCredentialsFile string
	GoogleCredentialsJSON string
	GoogleScope           string
	SheetsAPIBaseURL      string
	SheetsCacheTTL        time.Duration
	SheetSchedules        string
	SheetProgress         string
	SheetReminderState    string
	SheetProgressArchive  string
	ProgressRetentionDays int
	ReminderRetentionDays int
	SheetsMaxRetries      int
	SheetsRetryBaseDelay  time.Duration
	SheetSetupAutoCreate  bool
	SheetArchiveEnabled   bool

	// AI
	AIProvider    string
	AITimeout     time.Duration
	AITemperature float64
	AIJSONRetries int
	GroqAPIKey    string
	GroqBaseURL   string
	GroqModel     string
	GeminiAPIKey  string
	GeminiBaseURL string
	GeminiModel   string

	// Prompts
	PromptsDir       string
	PromptsHotReload bool
	UserContext      string
}

// Load reads .env (when present) and the process environment, then validates.
func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	appPort := get("APP_PORT", "")
	if appPort == "" {
		// Render/Railway inject PORT; keep APP_PORT for local/Docker use.
		appPort = get("PORT", "8080")
	}

	cfg := &Config{
		AppPort:               appPort,
		AppTimezone:           get("APP_TIMEZONE", "Asia/Jakarta"),
		PublicBaseURL:         strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/"),
		WebhookPath:           normalizePath(get("WEBHOOK_PATH", "/webhook")),
		CronTick:              time.Duration(getInt("CRON_TICK_SECONDS", 60)) * time.Second,
		DailyRecapTime:        get("DAILY_RECAP_TIME", "00:00"),
		LogLevel:              get("LOG_LEVEL", "info"),
		ShutdownTimeout:       time.Duration(getInt("SHUTDOWN_TIMEOUT_SECONDS", 15)) * time.Second,
		TelegramBotToken:      get("TELEGRAM_BOT_TOKEN", ""),
		TelegramAPIBaseURL:    strings.TrimRight(get("TELEGRAM_API_BASE_URL", ""), "/"),
		TelegramWebhookSecret: get("TELEGRAM_WEBHOOK_SECRET", ""),
		SheetsSpreadsheetID:   get("SHEETS_SPREADSHEET_ID", ""),
		GoogleCredentialsFile: get("GOOGLE_CREDENTIALS_FILE", ""),
		GoogleCredentialsJSON: get("GOOGLE_CREDENTIALS_JSON", ""),
		GoogleScope:           get("GOOGLE_SCOPE", ""),
		SheetsAPIBaseURL:      strings.TrimRight(get("SHEETS_API_BASE_URL", ""), "/"),
		SheetsCacheTTL:        time.Duration(getInt("SHEETS_CACHE_TTL_SECONDS", 60)) * time.Second,
		SheetSchedules:        get("SHEET_NAME_SCHEDULES", "schedules"),
		SheetProgress:         get("SHEET_NAME_PROGRESS", "progress_logs"),
		SheetReminderState:    get("SHEET_NAME_REMINDER_STATE", "reminder_state"),
		SheetProgressArchive:  get("SHEET_NAME_PROGRESS_ARCHIVE", "progress_archive"),
		ProgressRetentionDays: getInt("PROGRESS_RETENTION_DAYS", 90),
		ReminderRetentionDays: getInt("REMINDER_RETENTION_DAYS", 7),
		SheetsMaxRetries:      getInt("SHEETS_MAX_RETRIES", 4),
		SheetsRetryBaseDelay:  time.Duration(getInt("SHEETS_RETRY_BASE_MS", 500)) * time.Millisecond,
		SheetSetupAutoCreate:  getBool("SHEET_AUTO_CREATE", true),
		SheetArchiveEnabled:   getBool("SHEET_ARCHIVE_ENABLED", true),
		AIProvider:            strings.ToLower(get("AI_PROVIDER", "gemini")),
		AITimeout:             time.Duration(getInt("AI_TIMEOUT_SECONDS", 30)) * time.Second,
		AITemperature:         getFloat("AI_TEMPERATURE", 0.3),
		AIJSONRetries:         getInt("AI_JSON_RETRIES", 2),
		GroqAPIKey:            get("GROQ_API_KEY", ""),
		GroqBaseURL:           strings.TrimRight(get("GROQ_BASE_URL", ""), "/"),
		GroqModel:             get("GROQ_MODEL", "openai/gpt-oss-120b"),
		GeminiAPIKey:          get("GEMINI_API_KEY", ""),
		GeminiBaseURL:         strings.TrimRight(get("GEMINI_BASE_URL", ""), "/"),
		GeminiModel:           get("GEMINI_MODEL", ""),
		PromptsDir:            get("PROMPTS_DIR", "./prompts"),
		PromptsHotReload:      getBool("PROMPTS_HOT_RELOAD", true),
		UserContext:           get("USER_CONTEXT", ""),
	}

	userID, err := strconv.ParseInt(strings.TrimSpace(get("TELEGRAM_USER_ID", "")), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_USER_ID must be a numeric chat id: %w", err)
	}
	cfg.TelegramUserID = userID

	loc, err := time.LoadLocation(cfg.AppTimezone)
	if err != nil {
		return nil, fmt.Errorf("APP_TIMEZONE %q is not a valid IANA timezone: %w", cfg.AppTimezone, err)
	}
	cfg.Location = loc

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// WebhookURL is the fully qualified Telegram webhook endpoint.
func (c *Config) WebhookURL() string {
	return c.PublicBaseURL + c.WebhookPath
}

func validate(c *Config) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.PublicBaseURL == "" {
		add("PUBLIC_BASE_URL wajib diisi (dipakai untuk setWebhook Telegram)")
	}
	if c.TelegramBotToken == "" {
		add("TELEGRAM_BOT_TOKEN wajib diisi")
	}
	if c.TelegramWebhookSecret == "" {
		add("TELEGRAM_WEBHOOK_SECRET wajib diisi")
	}
	if c.TelegramAPIBaseURL == "" {
		add("TELEGRAM_API_BASE_URL wajib diisi")
	}
	if c.SheetsSpreadsheetID == "" {
		add("SHEETS_SPREADSHEET_ID wajib diisi")
	}
	if c.SheetsAPIBaseURL == "" {
		add("SHEETS_API_BASE_URL wajib diisi")
	}
	if c.GoogleCredentialsFile == "" && c.GoogleCredentialsJSON == "" {
		add("salah satu dari GOOGLE_CREDENTIALS_FILE atau GOOGLE_CREDENTIALS_JSON wajib diisi")
	}
	if c.GoogleScope == "" {
		add("GOOGLE_SCOPE wajib diisi (lihat .env.example)")
	}
	if _, err := time.Parse("15:04", c.DailyRecapTime); err != nil {
		add("DAILY_RECAP_TIME harus berformat HH:MM (contoh 00:00)")
	}
	if c.CronTick <= 0 {
		add("CRON_TICK_SECONDS harus lebih dari 0")
	}
	if c.SheetsCacheTTL <= 0 {
		add("SHEETS_CACHE_TTL_SECONDS harus lebih dari 0")
	}

	switch c.AIProvider {
	case "groq":
		if c.GroqAPIKey == "" {
			add("GROQ_API_KEY wajib diisi saat AI_PROVIDER=groq")
		}
		if c.GroqModel == "" {
			add("GROQ_MODEL wajib diisi saat AI_PROVIDER=groq")
		}
		if c.GroqBaseURL == "" {
			add("GROQ_BASE_URL wajib diisi saat AI_PROVIDER=groq")
		}
	case "gemini":
		if c.GeminiAPIKey == "" {
			add("GEMINI_API_KEY wajib diisi saat AI_PROVIDER=gemini")
		}
		if c.GeminiModel == "" {
			add("GEMINI_MODEL wajib diisi saat AI_PROVIDER=gemini")
		}
		if c.GeminiBaseURL == "" {
			add("GEMINI_BASE_URL wajib diisi saat AI_PROVIDER=gemini")
		}
	default:
		add("AI_PROVIDER harus salah satu dari: groq, gemini")
	}

	if len(problems) > 0 {
		return fmt.Errorf("konfigurasi tidak valid:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func getInt(key string, fallback int) int {
	raw := get(key, "")
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getFloat(key string, fallback float64) float64 {
	raw := get(key, "")
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}

func getBool(key string, fallback bool) bool {
	raw := strings.ToLower(get(key, ""))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// loadDotEnv is a tiny dependency-free .env parser. Existing environment
// variables always win so container/hosting overrides keep working.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("buka %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseDotEnvValue(strings.TrimSpace(value))
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return scanner.Err()
}

func parseDotEnvValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}
