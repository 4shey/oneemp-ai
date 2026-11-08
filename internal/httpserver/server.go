// Package httpserver exposes the Telegram webhook and health check.
package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"one_more_percent/internal/config"
	"one_more_percent/internal/domain"
	"one_more_percent/internal/service"
	"one_more_percent/internal/telegram"
)

const maxWebhookBody = 1 << 20

// Server is the HTTP entrypoint.
type Server struct {
	cfg       *config.Config
	chat      *service.ChatService
	messenger domain.Messenger
	logger    *slog.Logger
	sem       chan struct{}
}

// New builds the HTTP handler set.
func New(cfg *config.Config, chat *service.ChatService, messenger domain.Messenger, logger *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		chat:      chat,
		messenger: messenger,
		logger:    logger,
		sem:       make(chan struct{}, 1),
	}
}

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc(s.cfg.WebhookPath, s.handleWebhook)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().In(s.cfg.Location).Format(time.RFC3339),
	})
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(secret), []byte(s.cfg.TelegramWebhookSecret)) != 1 {
		s.logger.Warn("webhook ditolak: secret tidak cocok", "remote", r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var update telegram.Update
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if update.Message == nil || update.Message.Chat.ID != s.cfg.TelegramUserID {
		s.logger.Warn("webhook ditolak: chat_id tidak diizinkan")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ignored"))
		return
	}
	if update.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	// Telegram expects a fast 200; AI work happens asynchronously.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))

	message := *update.Message
	go s.process(message)
}

func (s *Server) process(message telegram.Message) {
	timeout := s.cfg.AITimeout*3 + 30*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	typingCtx, stopTyping := context.WithCancel(ctx)
	defer stopTyping()
	go s.keepTyping(typingCtx, message.Chat.ID)

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	reply := s.chat.Handle(ctx, message.Chat.ID, message.Text)
	stopTyping()
	if reply == "" {
		return
	}
	if err := s.messenger.SendMessage(ctx, message.Chat.ID, reply); err != nil {
		s.logger.Error("kirim balasan telegram", "error", err)
	}
}

func (s *Server) keepTyping(ctx context.Context, chatID int64) {
	s.sendTyping(ctx, chatID)
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendTyping(ctx, chatID)
		}
	}
}

func (s *Server) sendTyping(ctx context.Context, chatID int64) {
	if err := s.messenger.SendTyping(ctx, chatID); err != nil {
		s.logger.Debug("kirim status mengetik telegram", "error", err)
	}
}
