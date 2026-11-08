package domain

import "context"

// Messenger delivers plain-text notifications and chat actions to the user.
type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendTyping(ctx context.Context, chatID int64) error
}
