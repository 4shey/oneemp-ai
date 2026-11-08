package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Options are the shared, provider-agnostic knobs.
type Options struct {
	Timeout     time.Duration
	Temperature float64
	MaxRetries  int
	RetryBase   time.Duration
}

// HTTPError captures a non-2xx provider response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("provider membalas HTTP %d: %s", e.StatusCode, truncate(e.Body, 500))
}

// DoJSON performs an HTTP request with retry + exponential backoff for 429
// and 5xx responses. It is shared by all providers.
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, payload []byte, opts Options) ([]byte, error) {
	attempts := opts.MaxRetries
	if attempts < 1 {
		attempts = 1
	}
	base := opts.RetryBase
	if base <= 0 {
		base = 500 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := base * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("buat request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("request gagal: %w", err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("baca response: %w", readErr)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, nil
		}

		httpErr := &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = httpErr
			continue
		}
		return nil, httpErr
	}

	if lastErr == nil {
		lastErr = errors.New("provider gagal tanpa detail")
	}
	return nil, lastErr
}

// BuildMessages turns a Request into a provider message list, omitting an
// empty system prompt.
func BuildMessages(req Request) []Message {
	out := make([]Message, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		out = append(out, Message{Role: "system", Content: req.System})
	}
	out = append(out, req.Messages...)
	return out
}

// NewHTTPClient builds a timeout-bounded HTTP client.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// Encode marshals a provider payload.
func Encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return b, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
