package service

import (
	"strings"
	"sync"
	"time"
)

type pendingProposal struct {
	action    string
	schedule  extractedSchedule
	schedules []extractedSchedule
	expiresAt time.Time
}

type pendingStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[int64]pendingProposal
}

func newPendingStore(ttl time.Duration) *pendingStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &pendingStore{ttl: ttl, items: map[int64]pendingProposal{}}
}

func (p *pendingStore) Set(chatID int64, proposal pendingProposal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	proposal.expiresAt = time.Now().Add(p.ttl)
	p.items[chatID] = proposal
}

func (p *pendingStore) Get(chatID int64) (pendingProposal, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	proposal, ok := p.items[chatID]
	if !ok {
		return pendingProposal{}, false
	}
	if time.Now().After(proposal.expiresAt) {
		delete(p.items, chatID)
		return pendingProposal{}, false
	}
	return proposal, true
}

func (p *pendingStore) Clear(chatID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, chatID)
}

func normalizeShortReply(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Trim(text, " .,!?")
	return strings.Join(strings.Fields(text), " ")
}

func isSimpleConfirm(text string) bool {
	switch normalizeShortReply(text) {
	case "setuju", "ya", "iya", "y", "yes", "oke", "ok", "boleh", "gas", "lanjut",
		"bener", "benar", "betul", "sip", "silakan", "monggo":
		return true
	default:
		return false
	}
}

func isSimpleCancel(text string) bool {
	switch normalizeShortReply(text) {
	case "batal", "batalkan", "gajadi", "ga jadi", "gak jadi", "nggak", "ngga",
		"engga", "tidak", "jangan", "no", "cancel", "stop":
		return true
	default:
		return false
	}
}
