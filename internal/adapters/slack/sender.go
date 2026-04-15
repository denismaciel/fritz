package slack

import (
	"context"
	"strings"

	"fritz/internal/heartbeat"
)

type Sender struct {
	client Client
}

func NewSender(client Client) *Sender {
	return &Sender{client: client}
}

func (s *Sender) Send(ctx context.Context, wake heartbeat.Wake, text string) error {
	return s.client.PostMessage(ctx, PostMessageRequest{
		Channel: wake.ChatID,
		Text:    strings.TrimSpace(text),
	})
}
