// Package gemini implements ai.Client for Google AI Studio (Gemini).
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"one_more_percent/internal/ai"
)

// Client talks to the Google AI Studio Generative Language API.
type Client struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	httpClient  *http.Client
	retry       ai.Options
}

// New builds a Gemini client.
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

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type generationConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type generateRequest struct {
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	Contents          []content         `json:"contents"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// Complete implements ai.Client.
func (c *Client) Complete(ctx context.Context, req ai.Request) (ai.Response, error) {
	payload := generateRequest{
		GenerationConfig: &generationConfig{Temperature: c.temperature},
	}
	if req.JSON {
		payload.GenerationConfig.ResponseMimeType = "application/json"
	}
	if req.System != "" {
		payload.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	for _, m := range req.Messages {
		role := "user"
		if m.Role == ai.RoleAssistant {
			role = "model"
		}
		payload.Contents = append(payload.Contents, content{Role: role, Parts: []part{{Text: m.Content}}})
	}
	if len(payload.Contents) == 0 {
		return ai.Response{}, fmt.Errorf("gemini: minimal satu pesan user diperlukan")
	}

	body, err := ai.Encode(payload)
	if err != nil {
		return ai.Response{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)
	headers := map[string]string{"x-goog-api-key": c.apiKey}
	raw, err := ai.DoJSON(ctx, c.httpClient, http.MethodPost, url, headers, body, c.retry)
	if err != nil {
		return ai.Response{}, fmt.Errorf("gemini: %w", err)
	}

	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ai.Response{}, fmt.Errorf("gemini: parse response: %w", err)
	}
	if parsed.PromptFeedback.BlockReason != "" {
		return ai.Response{}, fmt.Errorf("gemini: prompt diblokir: %s", parsed.PromptFeedback.BlockReason)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return ai.Response{}, fmt.Errorf("gemini: respons kosong")
	}

	var text string
	for _, p := range parsed.Candidates[0].Content.Parts {
		text += p.Text
	}
	if text == "" {
		return ai.Response{}, fmt.Errorf("gemini: respons kosong")
	}
	return ai.Response{Content: text}, nil
}
