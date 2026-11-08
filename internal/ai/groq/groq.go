// Package groq implements ai.Client for the Groq OpenAI-compatible API.
package groq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"one_more_percent/internal/ai"
)

// Client talks to Groq.
type Client struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	httpClient  *http.Client
	retry       ai.Options
}

// New builds a Groq client. baseURL must not include a trailing slash.
func New(apiKey, baseURL, model string, temperature float64, retry ai.Options) *Client {
	return &Client{
		apiKey:      apiKey,
		baseURL:     baseURL,
		model:       model,
		temperature: temperature,
		httpClient:  ai.NewHTTPClient(retry.Timeout),
		retry:       retry,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete implements ai.Client.
func (c *Client) Complete(ctx context.Context, req ai.Request) (ai.Response, error) {
	messages := ai.BuildMessages(req)
	payload := chatRequest{
		Model:       c.model,
		Temperature: c.temperature,
		Messages:    make([]chatMessage, 0, len(messages)),
	}
	for _, m := range messages {
		payload.Messages = append(payload.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	if req.JSON {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := ai.Encode(payload)
	if err != nil {
		return ai.Response{}, err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + c.apiKey,
	}
	raw, err := ai.DoJSON(ctx, c.httpClient, http.MethodPost, c.baseURL+"/chat/completions", headers, body, c.retry)
	if err != nil {
		return ai.Response{}, fmt.Errorf("groq: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ai.Response{}, fmt.Errorf("groq: parse response: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return ai.Response{}, fmt.Errorf("groq: respons kosong")
	}
	return ai.Response{Content: parsed.Choices[0].Message.Content}, nil
}
