package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"fritz/internal/heartbeat"
	"fritz/internal/logx"
)

type Sender struct {
	client Client
}

func NewSender(client Client) *Sender {
	return &Sender{client: client}
}

func (s *Sender) Send(ctx context.Context, wake heartbeat.Wake, text string) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(wake.ChatID), 10, 64)
	if err != nil {
		logger := logx.Component("heartbeat")
		logger.Error().Err(err).Str("event", "heartbeat.send.invalid_chat").Str("chat_id", wake.ChatID).Msg("")
		return fmt.Errorf("invalid telegram chat id %q", wake.ChatID)
	}
	logger := logx.Component("heartbeat")
	logger.Info().
		Str("event", "heartbeat.send.start").
		Str("target_key", wake.TargetKey).
		Str("chat_id", wake.ChatID).
		Int("text_len", len(strings.TrimSpace(text))).
		Msg("")
	err = s.client.SendMessage(ctx, SendMessageRequest{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		logger.Error().Err(err).Str("event", "heartbeat.send.error").Str("target_key", wake.TargetKey).Msg("")
		return err
	}
	logger.Info().Str("event", "heartbeat.send.ok").Str("target_key", wake.TargetKey).Msg("")
	return nil
}
