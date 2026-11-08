// Package telegram is a tiny Telegram Bot API client built on net/http.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the Telegram Bot API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// New builds a client. baseURL comes from configuration.
func New(token, baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		token:      token,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Update is the subset of a Telegram update this bot needs.
type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is a Telegram chat message.
type Message struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
}

// Chat identifies a Telegram chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// User identifies a Telegram user.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// SendMessage sends a plain-text message.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	var resp apiResponse
	if err := c.call(ctx, "sendMessage", payload, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram sendMessage: %s", resp.Description)
	}
	return nil
}

// SendTyping shows the typing indicator in a chat.
func (c *Client) SendTyping(ctx context.Context, chatID int64) error {
	payload := map[string]any{
		"chat_id": chatID,
		"action":  "typing",
	}
	var resp apiResponse
	if err := c.call(ctx, "sendChatAction", payload, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram sendChatAction: %s", resp.Description)
	}
	return nil
}

// SetWebhook registers the HTTPS endpoint used by Telegram.
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	payload := map[string]any{
		"url":             url,
		"secret_token":    secret,
		"allowed_updates": []string{"message"},
	}
	var resp apiResponse
	if err := c.call(ctx, "setWebhook", payload, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram setWebhook: %s", resp.Description)
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode telegram payload: %w", err)
	}

	url := c.baseURL + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("buat request telegram: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request telegram: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("baca response telegram: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse response telegram (HTTP %d): %w", resp.StatusCode, err)
	}
	return nil
}
