// Package sheets implements the domain repositories on top of the Google
// Sheets REST API using a service account (no OAuth flow, no SQL driver).
package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"one_more_percent/internal/config"
)

// Client is a small, dependency-free Google Sheets REST client.
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
	tokens     *tokenSource
	sheetIDsMu sync.Mutex
	sheetIDs   map[string]int
	sheetIDsAt time.Time
}

// NewClient authenticates and (optionally) prepares every worksheet.
func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	tokens, err := newTokenSource(cfg)
	if err != nil {
		return nil, err
	}

	c := &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     tokens,
		sheetIDs:   map[string]int{},
	}

	if cfg.SheetSetupAutoCreate {
		if err := c.EnsureSheets(ctx); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// EnsureSheets creates missing worksheets and header rows.
func (c *Client) EnsureSheets(ctx context.Context) error {
	type spec struct {
		name    string
		headers []string
	}
	specs := []spec{
		{c.cfg.SheetSchedules, ScheduleHeaders},
		{c.cfg.SheetProgress, ProgressHeaders},
		{c.cfg.SheetReminderState, ReminderHeaders},
	}
	if c.cfg.SheetArchiveEnabled {
		specs = append(specs, spec{c.cfg.SheetProgressArchive, ProgressHeaders})
	}

	ids, err := c.SheetIDs(ctx)
	if err != nil {
		return err
	}

	for _, s := range specs {
		if _, ok := ids[s.name]; !ok {
			if err := c.addSheet(ctx, s.name); err != nil {
				return err
			}
		}
		if err := c.ensureHeader(ctx, s.name, s.headers); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ensureHeader(ctx context.Context, sheet string, headers []string) error {
	lastCol := columnLetter(len(headers))
	existing, err := c.GetValues(ctx, sheet+"!A1:"+lastCol+"1")
	if err != nil {
		return err
	}
	if len(existing) > 0 && len(existing[0]) > 0 && strings.TrimSpace(existing[0][0]) != "" {
		return nil
	}

	row := make([]any, len(headers))
	for i, h := range headers {
		row[i] = h
	}
	return c.UpdateValues(ctx, sheet+"!A1:"+lastCol+"1", [][]any{row})
}

// SheetIDs returns worksheet name -> numeric sheet id.
func (c *Client) SheetIDs(ctx context.Context) (map[string]int, error) {
	c.sheetIDsMu.Lock()
	defer c.sheetIDsMu.Unlock()

	if c.sheetIDsAt.After(time.Now().Add(-10*time.Minute)) && len(c.sheetIDs) > 0 {
		out := make(map[string]int, len(c.sheetIDs))
		for k, v := range c.sheetIDs {
			out[k] = v
		}
		return out, nil
	}

	path := fmt.Sprintf("/v4/spreadsheets/%s?fields=sheets.properties", url.PathEscape(c.cfg.SheetsSpreadsheetID))
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Sheets []struct {
			Properties struct {
				SheetID int    `json:"sheetId"`
				Title   string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse metadata spreadsheet: %w", err)
	}

	ids := make(map[string]int, len(parsed.Sheets))
	for _, s := range parsed.Sheets {
		ids[s.Properties.Title] = s.Properties.SheetID
	}
	c.sheetIDs = ids
	c.sheetIDsAt = time.Now()

	out := make(map[string]int, len(ids))
	for k, v := range ids {
		out[k] = v
	}
	return out, nil
}

func (c *Client) addSheet(ctx context.Context, name string) error {
	payload := map[string]any{
		"requests": []map[string]any{
			{"addSheet": map[string]any{"properties": map[string]any{"title": name}}},
		},
	}
	if _, err := c.BatchUpdate(ctx, payload); err != nil {
		return fmt.Errorf("buat worksheet %s: %w", name, err)
	}
	c.sheetIDsMu.Lock()
	c.sheetIDsAt = time.Time{}
	c.sheetIDsMu.Unlock()
	return nil
}

// GetValues reads a range and normalizes every cell to a string.
func (c *Client) GetValues(ctx context.Context, a1Range string) ([][]string, error) {
	path := fmt.Sprintf(
		"/v4/spreadsheets/%s/values/%s?majorDimension=ROWS&valueRenderOption=UNFORMATTED_VALUE",
		url.PathEscape(c.cfg.SheetsSpreadsheetID),
		url.PathEscape(a1Range),
	)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Values [][]any `json:"values"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse values: %w", err)
	}

	out := make([][]string, 0, len(parsed.Values))
	for _, row := range parsed.Values {
		converted := make([]string, len(row))
		for i, cell := range row {
			converted[i] = cellString(cell)
		}
		out = append(out, converted)
	}
	return out, nil
}

// UpdateValues overwrites a range.
func (c *Client) UpdateValues(ctx context.Context, a1Range string, values [][]any) error {
	path := fmt.Sprintf(
		"/v4/spreadsheets/%s/values/%s?valueInputOption=RAW",
		url.PathEscape(c.cfg.SheetsSpreadsheetID),
		url.PathEscape(a1Range),
	)
	_, err := c.do(ctx, http.MethodPut, path, map[string]any{"values": values})
	return err
}

// AppendValues appends rows to a worksheet.
func (c *Client) AppendValues(ctx context.Context, sheet string, values [][]any) error {
	path := fmt.Sprintf(
		"/v4/spreadsheets/%s/values/%s:append?valueInputOption=RAW&insertDataOption=INSERT_ROWS",
		url.PathEscape(c.cfg.SheetsSpreadsheetID),
		url.PathEscape(sheet),
	)
	_, err := c.do(ctx, http.MethodPost, path, map[string]any{"values": values})
	return err
}

// BatchUpdateValues writes several ranges in one call.
func (c *Client) BatchUpdateValues(ctx context.Context, data []ValueRange) error {
	path := fmt.Sprintf("/v4/spreadsheets/%s/values:batchUpdate", url.PathEscape(c.cfg.SheetsSpreadsheetID))
	_, err := c.do(ctx, http.MethodPost, path, map[string]any{
		"valueInputOption": "RAW",
		"data":             data,
	})
	return err
}

// BatchUpdate performs structural spreadsheet changes.
func (c *Client) BatchUpdate(ctx context.Context, payload any) ([]byte, error) {
	path := fmt.Sprintf("/v4/spreadsheets/%s:batchUpdate", url.PathEscape(c.cfg.SheetsSpreadsheetID))
	return c.do(ctx, http.MethodPost, path, payload)
}

// DeleteRows removes the given 0-based row indexes from a worksheet.
func (c *Client) DeleteRows(ctx context.Context, sheet string, rowIndexes []int) error {
	if len(rowIndexes) == 0 {
		return nil
	}
	ids, err := c.SheetIDs(ctx)
	if err != nil {
		return err
	}
	sheetID, ok := ids[sheet]
	if !ok {
		return fmt.Errorf("worksheet %s tidak ditemukan", sheet)
	}

	indexes := append([]int(nil), rowIndexes...)
	sort.Ints(indexes)

	type rowRange struct {
		start int
		end   int
	}
	ranges := make([]rowRange, 0, len(indexes))
	for _, idx := range indexes {
		if len(ranges) == 0 {
			ranges = append(ranges, rowRange{start: idx, end: idx + 1})
			continue
		}
		last := &ranges[len(ranges)-1]
		if idx < last.end {
			continue
		}
		if idx == last.end {
			last.end = idx + 1
			continue
		}
		ranges = append(ranges, rowRange{start: idx, end: idx + 1})
	}

	// Google applies delete requests in order, so remove the highest ranges
	// first to keep the remaining indexes valid.
	requests := make([]map[string]any, 0, len(ranges))
	for i := len(ranges) - 1; i >= 0; i-- {
		requests = append(requests, map[string]any{
			"deleteDimension": map[string]any{
				"range": map[string]any{
					"sheetId":    sheetID,
					"dimension":  "ROWS",
					"startIndex": ranges[i].start,
					"endIndex":   ranges[i].end,
				},
			},
		})
	}
	_, err = c.BatchUpdate(ctx, map[string]any{"requests": requests})
	return err
}

// ValueRange is a single range/value pair for values:batchUpdate.
type ValueRange struct {
	Range  string  `json:"range"`
	Values [][]any `json:"values"`
}

func (c *Client) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}

	attempts := c.cfg.SheetsMaxRetries
	if attempts < 1 {
		attempts = 1
	}
	base := c.cfg.SheetsRetryBaseDelay
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

		token, err := c.tokens.Token(ctx)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, c.cfg.SheetsAPIBaseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("buat request sheets: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("request sheets: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("baca response sheets: %w", readErr)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		apiErr := fmt.Errorf("sheets HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 500))
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = apiErr
			continue
		}
		return nil, apiErr
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("sheets gagal tanpa detail")
	}
	return nil, lastErr
}

func cellString(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		if value {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		if value == math.Trunc(value) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func columnLetter(n int) string {
	if n <= 0 {
		return "A"
	}
	var out []byte
	for n > 0 {
		n--
		out = append([]byte{byte('A' + n%26)}, out...)
		n /= 26
	}
	return string(out)
}
