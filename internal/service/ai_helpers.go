package service

import (
	"errors"
	"strings"

	"one_more_percent/internal/ai"
)

const (
	msgAIUnavailable   = "Model AI lagi ga bisa dihubungi. Coba lagi bentar lagi."
	msgAIRateLimited   = "Kuota AI lagi penuh. Coba lagi beberapa menit lagi."
	msgAIInvalidFormat = "AI-nya balas format yang ga bisa aku baca. Coba kirim ulang pesannya."
	msgSafeFallback    = "aku ga paham maksud kamu. Coba ulang pakai kalimat yang lebih jelas."
)

var errInvalidIntentJSON = errors.New("intent JSON tidak valid")

func friendlyAIError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errInvalidIntentJSON) {
		return msgAIInvalidFormat
	}
	var httpErr *ai.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 429 {
		return msgAIRateLimited
	}
	return msgAIUnavailable
}

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}
