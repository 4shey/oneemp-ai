package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// UnmarshalJSON accepts IDs from models that return numbers as strings.
func (s *extractedSchedule) UnmarshalJSON(data []byte) error {
	type rawSchedule struct {
		ID        json.RawMessage `json:"id"`
		IDs       json.RawMessage `json:"ids"`
		Type      *string         `json:"type"`
		Date      *string         `json:"date"`
		DayOfWeek *string         `json:"day_of_week"`
		StartTime *string         `json:"start_time"`
		EndTime   *string         `json:"end_time"`
		Activity  *string         `json:"activity"`
		Active    json.RawMessage `json:"active"`
	}

	var raw rawSchedule
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var id *int
	var ids []int
	var err error

	trimmedID := bytes.TrimSpace(raw.ID)
	if len(trimmedID) > 0 && trimmedID[0] == '[' {
		ids, err = parseOptionalIntList(raw.ID)
	} else {
		id, err = parseOptionalInt(raw.ID)
	}
	if err != nil {
		return fmt.Errorf("schedule.id: %w", err)
	}

	extraIDs, err := parseOptionalIntList(raw.IDs)
	if err != nil {
		return fmt.Errorf("schedule.ids: %w", err)
	}
	ids = uniqueInts(append(ids, extraIDs...))

	active, err := parseOptionalBool(raw.Active)
	if err != nil {
		return fmt.Errorf("schedule.active: %w", err)
	}

	*s = extractedSchedule{
		ID:        id,
		IDs:       ids,
		Type:      raw.Type,
		Date:      raw.Date,
		DayOfWeek: raw.DayOfWeek,
		StartTime: raw.StartTime,
		EndTime:   raw.EndTime,
		Activity:  raw.Activity,
		Active:    active,
	}
	return nil
}

// UnmarshalJSON accepts schedule_id from models that return numbers as strings.
func (p *extractedProgress) UnmarshalJSON(data []byte) error {
	type rawProgress struct {
		ScheduleID json.RawMessage `json:"schedule_id"`
		Activity   *string         `json:"activity"`
		Status     *string         `json:"status"`
		Note       *string         `json:"note"`
		Date       *string         `json:"date"`
		All        json.RawMessage `json:"all"`
	}

	var raw rawProgress
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	scheduleID, err := parseOptionalInt(raw.ScheduleID)
	if err != nil {
		return fmt.Errorf("progress.schedule_id: %w", err)
	}
	all, err := parseOptionalBool(raw.All)
	if err != nil {
		return fmt.Errorf("progress.all: %w", err)
	}

	*p = extractedProgress{
		ScheduleID: scheduleID,
		Activity:   raw.Activity,
		Status:     raw.Status,
		Note:       raw.Note,
		Date:       raw.Date,
		All:        all,
	}
	return nil
}

func (s extractedSchedule) primaryID() *int {
	if s.ID != nil {
		return s.ID
	}
	if len(s.IDs) == 1 {
		id := s.IDs[0]
		return &id
	}
	return nil
}

func parseOptionalInt(raw json.RawMessage) (*int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, "null") {
			return nil, nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("harus angka integer, dapat %q", value)
		}
		return &parsed, nil
	}

	var parsed int
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("harus angka integer: %w", err)
	}
	return &parsed, nil
}

func parseOptionalIntList(raw json.RawMessage) ([]int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	if raw[0] != '[' {
		value, err := parseOptionalInt(raw)
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, nil
		}
		return []int{*value}, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]int, 0, len(items))
	for _, item := range items {
		value, err := parseOptionalInt(item)
		if err != nil {
			return nil, err
		}
		if value != nil {
			out = append(out, *value)
		}
	}
	return out, nil
}

func parseOptionalBool(raw json.RawMessage) (*bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes":
			parsed := true
			return &parsed, nil
		case "false", "0", "no":
			parsed := false
			return &parsed, nil
		case "":
			return nil, nil
		default:
			return nil, fmt.Errorf("harus boolean, dapat %q", value)
		}
	}

	var parsed bool
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("harus boolean: %w", err)
	}
	return &parsed, nil
}

func uniqueInts(in []int) []int {
	seen := make(map[int]bool, len(in))
	out := make([]int, 0, len(in))
	for _, value := range in {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
