package service

import (
	"fmt"
	"strings"
	"sync"

	"one_more_percent/internal/ai"
)

type historyStore struct {
	mu    sync.Mutex
	max   int
	items map[int64][]ai.Message
}

func newHistoryStore(max int) *historyStore {
	if max <= 0 {
		max = 10
	}
	return &historyStore{max: max, items: map[int64][]ai.Message{}}
}

func (h *historyStore) Add(chatID int64, role, content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	msgs := append(h.items[chatID], ai.Message{Role: role, Content: content})
	if len(msgs) > h.max {
		msgs = msgs[len(msgs)-h.max:]
	}
	h.items[chatID] = msgs
}

func (h *historyStore) Get(chatID int64) []ai.Message {
	h.mu.Lock()
	defer h.mu.Unlock()

	src := h.items[chatID]
	out := make([]ai.Message, len(src))
	copy(out, src)
	return out
}

func formatHistory(messages []ai.Message) string {
	if len(messages) == 0 {
		return "belum ada"
	}
	var b strings.Builder
	for _, message := range messages {
		content := truncateRunes(message.Content, 500)
		switch message.Role {
		case ai.RoleUser:
			fmt.Fprintf(&b, "User: %s\n", content)
		case ai.RoleAssistant:
			fmt.Fprintf(&b, "Oneemp: %s\n", content)
		default:
			fmt.Fprintf(&b, "%s: %s\n", message.Role, content)
		}
	}
	return strings.TrimSpace(b.String())
}
