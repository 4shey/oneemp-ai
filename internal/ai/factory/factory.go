// Package factory wires the configured AI provider into an ai.Client.
package factory

import (
	"fmt"

	"one_more_percent/internal/ai"
	"one_more_percent/internal/ai/gemini"
	"one_more_percent/internal/ai/groq"
	"one_more_percent/internal/config"
)

// New selects the configured AI provider.
func New(cfg *config.Config) (ai.Client, error) {
	opts := ai.Options{
		Timeout:     cfg.AITimeout,
		Temperature: cfg.AITemperature,
		MaxRetries:  cfg.SheetsMaxRetries,
		RetryBase:   cfg.SheetsRetryBaseDelay,
	}

	switch cfg.AIProvider {
	case "groq":
		return groq.New(cfg.GroqAPIKey, cfg.GroqBaseURL, cfg.GroqModel, cfg.AITemperature, opts), nil
	case "gemini":
		return gemini.New(cfg.GeminiAPIKey, cfg.GeminiBaseURL, cfg.GeminiModel, cfg.AITemperature, opts), nil
	default:
		return nil, fmt.Errorf("AI provider tidak dikenal: %s", cfg.AIProvider)
	}
}
